/**
 * The App Launchpad's form model.
 *
 * One shape is read from the cluster, edited by the form, previewed as YAML and
 * sent back — so what the preview shows and what the cluster receives cannot
 * disagree. The backend takes the same payload for an install and for an
 * upgrade (`/api/deploy-app` and `/api/upgrade-image-app`), which is why there
 * is one model rather than two.
 */

export const CPU_PRESETS = [
  {label: "0.1", value: "100m"},
  {label: "0.2", value: "200m"},
  {label: "0.5", value: "500m"},
  {label: "1", value: "1"},
  {label: "2", value: "2"},
  {label: "4", value: "4"},
  {label: "8", value: "8"},
];

export const MEMORY_PRESETS = [
  {label: "64Mi", value: "64Mi"},
  {label: "128Mi", value: "128Mi"},
  {label: "256Mi", value: "256Mi"},
  {label: "512Mi", value: "512Mi"},
  {label: "1Gi", value: "1Gi"},
  {label: "2Gi", value: "2Gi"},
  {label: "4Gi", value: "4Gi"},
  {label: "8Gi", value: "8Gi"},
  {label: "16Gi", value: "16Gi"},
];

export const SERVICE_TYPES = ["ClusterIP", "NodePort", "LoadBalancer"];

export function emptyAppForm(namespace = "default") {
  return {
    namespace,
    name: "",
    image: "",
    registry: {enabled: false, server: "docker.io", username: "", password: ""},
    replicas: 1,
    hpa: {enabled: false, minReplicas: 1, maxReplicas: 5, cpuTarget: 60},
    cpuLimit: "500m",
    memoryLimit: "512Mi",
    ports: [{containerPort: 80, protocol: "TCP", name: "http"}],
    serviceType: "NodePort",
    domains: [],
    command: "",
    args: "",
    envVars: [],
    configFiles: [],
    volumes: [],
  };
}

/** Splits a command line the way a shell would for the simple cases. */
export function splitCommand(text) {
  const matches = String(text ?? "").match(/"[^"]*"|'[^']*'|\S+/g) ?? [];
  return matches.map((token) => token.replace(/^["']|["']$/g, ""));
}

function joinCommand(list) {
  return (list ?? [])
    .map((token) => (/\s/.test(token) ? `"${token}"` : token))
    .join(" ");
}

/** The form as it looks for an app that already exists. */
export function formFromDetail(detail) {
  const base = emptyAppForm(detail.namespace);
  return {
    ...base,
    namespace: detail.namespace,
    name: detail.name,
    image: detail.image ?? "",
    registry: {
      enabled: Boolean(detail.registryServer),
      server: detail.registryServer || "docker.io",
      username: "",
      password: "",
    },
    replicas: detail.replicas ?? 1,
    hpa: detail.hpa
      ? {
        enabled: true,
        minReplicas: detail.hpa.minReplicas || 1,
        maxReplicas: detail.hpa.maxReplicas || 5,
        cpuTarget: detail.hpa.cpuTarget || 60,
      }
      : base.hpa,
    // A blank limit is the app saying "no limit", which the form shows as an
    // empty box rather than as a preset nobody chose.
    cpuLimit: detail.cpuLimit ?? "",
    memoryLimit: detail.memoryLimit ?? "",
    ports: (detail.ports ?? []).map((port) => ({
      containerPort: port.containerPort,
      protocol: port.protocol || "TCP",
      name: port.name || "",
    })),
    serviceType: detail.serviceType || "ClusterIP",
    domains: (detail.domains ?? []).map((domain) => ({...domain})),
    command: joinCommand(detail.command),
    args: joinCommand(detail.args),
    envVars: (detail.envVars ?? []).map((env) => ({name: env.name, value: env.value ?? ""})),
    configFiles: (detail.configFiles ?? []).map((file) => ({...file})),
    volumes: (detail.volumes ?? []).map((volume) => ({...volume})),
  };
}

/** What the backend is sent. `mode` decides whether storage may still change. */
export function payloadFromForm(form, {mode = "create"} = {}) {
  const payload = {
    namespace: form.namespace || "default",
    name: form.name.trim(),
    image: form.image.trim(),
    replicas: form.hpa.enabled ? Number(form.hpa.minReplicas) || 1 : Number(form.replicas) || 1,
    ports: form.ports
      .filter((port) => Number(port.containerPort) > 0)
      .map((port, index) => ({
        name: port.name?.trim() || `port-${index + 1}`,
        containerPort: Number(port.containerPort),
        protocol: port.protocol || "TCP",
      })),
    serviceType: form.serviceType,
    envVars: form.envVars
      .filter((env) => env.name?.trim())
      .map((env) => ({name: env.name.trim(), value: env.value ?? ""})),
    // Quantities are sent even when blank: the backend reads an empty string as
    // "remove this limit", which is the only way to clear one from the form.
    cpuLimit: form.cpuLimit?.trim() ?? "",
    memoryLimit: form.memoryLimit?.trim() ?? "",
    command: splitCommand(form.command),
    args: splitCommand(form.args),
    configFiles: form.configFiles.filter((file) => file.mountPath?.trim()),
    domains: form.domains
      .filter((domain) => domain.host?.trim() && Number(domain.port) > 0)
      .map((domain) => ({
        host: domain.host.trim(),
        port: Number(domain.port),
        ingressClass: domain.ingressClass?.trim() ?? "",
      })),
    hpa: {
      enabled: form.hpa.enabled,
      minReplicas: Number(form.hpa.minReplicas) || 1,
      maxReplicas: Number(form.hpa.maxReplicas) || 1,
      cpuTarget: Number(form.hpa.cpuTarget) || 60,
    },
  };

  // Always stated, never omitted: the backend reads a missing field as "leave
  // this alone", which is what lets the App Store's upgrade keep whatever the
  // launchpad set up. Here the form is the whole truth, so switching the
  // private registry off has to say so.
  payload.registry = form.registry.enabled
    ? {
      server: form.registry.server.trim(),
      username: form.registry.username.trim(),
      password: form.registry.password,
    }
    : {server: "", username: "", password: ""};

  // Claims are bound for the life of the workload, so an edit never restates
  // them — sending them again would ask Kubernetes for a change it rejects.
  if (mode === "create") {
    payload.volumes = form.volumes
      .filter((volume) => volume.mountPath?.trim())
      .map((volume) => ({mountPath: volume.mountPath.trim(), size: volume.size?.trim() || "1Gi"}));
  }

  return payload;
}

export function validateAppForm(form) {
  const errors = {};
  const name = form.name.trim();
  if (!name) {
    errors.name = "required";
  } else if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(name)) {
    errors.name = "invalid";
  }
  if (!form.image.trim()) {
    errors.image = "required";
  }
  if (form.hpa.enabled && Number(form.hpa.maxReplicas) < Number(form.hpa.minReplicas)) {
    errors.hpa = "range";
  }
  if (form.domains.some((domain) => domain.host?.trim() && !Number(domain.port))) {
    errors.domains = "port";
  }
  return errors;
}

function yamlString(value) {
  const text = String(value ?? "");
  return /^[\w./:@-]+$/.test(text) ? text : JSON.stringify(text);
}

function yamlBlock(lines, indent) {
  return lines.map((line) => `${" ".repeat(indent)}${line}`).join("\n");
}

/**
 * The manifests the form stands for. This is a preview, not the source of
 * truth — the backend builds the real objects — so it shows the fields the form
 * owns and leaves the cluster's own defaults out.
 */
export function appYamlPreview(form) {
  const name = form.name.trim() || "my-app";
  const namespace = form.namespace || "default";
  const ports = form.ports.filter((port) => Number(port.containerPort) > 0);
  const documents = [];

  const container = [
    `- name: ${name}`,
    `  image: ${yamlString(form.image.trim() || "nginx:latest")}`,
  ];
  const command = splitCommand(form.command);
  if (command.length > 0) {
    container.push(`  command: [${command.map(yamlString).join(", ")}]`);
  }
  const args = splitCommand(form.args);
  if (args.length > 0) {
    container.push(`  args: [${args.map(yamlString).join(", ")}]`);
  }
  if (ports.length > 0) {
    container.push("  ports:");
    ports.forEach((port, index) => {
      container.push(`  - name: ${port.name?.trim() || `port-${index + 1}`}`);
      container.push(`    containerPort: ${Number(port.containerPort)}`);
      container.push(`    protocol: ${port.protocol || "TCP"}`);
    });
  }
  const envVars = form.envVars.filter((env) => env.name?.trim());
  if (envVars.length > 0) {
    container.push("  env:");
    envVars.forEach((env) => {
      container.push(`  - name: ${env.name.trim()}`);
      container.push(`    value: ${yamlString(env.value ?? "")}`);
    });
  }
  if (form.cpuLimit?.trim() || form.memoryLimit?.trim()) {
    container.push("  resources:");
    container.push("    limits:");
    if (form.cpuLimit?.trim()) {
      container.push(`      cpu: ${yamlString(form.cpuLimit.trim())}`);
    }
    if (form.memoryLimit?.trim()) {
      container.push(`      memory: ${yamlString(form.memoryLimit.trim())}`);
    }
  }

  const volumes = form.volumes.filter((volume) => volume.mountPath?.trim());
  const configFiles = form.configFiles.filter((file) => file.mountPath?.trim());
  if (volumes.length > 0 || configFiles.length > 0) {
    container.push("  volumeMounts:");
    volumes.forEach((volume, index) => {
      container.push(`  - name: vol-${index}`);
      container.push(`    mountPath: ${yamlString(volume.mountPath.trim())}`);
    });
    configFiles.forEach((file, index) => {
      container.push("  - name: app-config");
      container.push(`    mountPath: ${yamlString(file.mountPath.trim())}`);
      container.push(`    subPath: file-${index}`);
    });
  }

  const podVolumes = [];
  volumes.forEach((volume, index) => {
    podVolumes.push(`- name: vol-${index}`);
    podVolumes.push("  persistentVolumeClaim:");
    podVolumes.push(`    claimName: ${name}-vol-${index}`);
  });
  if (configFiles.length > 0) {
    podVolumes.push("- name: app-config");
    podVolumes.push("  configMap:");
    podVolumes.push(`    name: ${name}-config`);
  }

  const replicas = form.hpa.enabled ? Number(form.hpa.minReplicas) || 1 : Number(form.replicas) || 1;
  documents.push([
    "apiVersion: apps/v1",
    "kind: Deployment",
    "metadata:",
    `  name: ${name}`,
    `  namespace: ${namespace}`,
    "  labels:",
    "    app.kubernetes.io/managed-by: casos",
    `    app.kubernetes.io/instance: ${name}`,
    "spec:",
    `  replicas: ${replicas}`,
    "  selector:",
    "    matchLabels:",
    `      app: ${name}`,
    "  template:",
    "    metadata:",
    "      labels:",
    `        app: ${name}`,
    "    spec:",
    ...(form.registry.enabled ? ["      imagePullSecrets:", `      - name: ${name}-registry`] : []),
    "      containers:",
    yamlBlock(container, 6),
    ...(podVolumes.length > 0 ? ["      volumes:", yamlBlock(podVolumes, 6)] : []),
  ].join("\n"));

  if (ports.length > 0) {
    const servicePorts = ports.flatMap((port, index) => [
      `- name: ${port.name?.trim() || `port-${index + 1}`}`,
      `  port: ${Number(port.containerPort)}`,
      `  targetPort: ${Number(port.containerPort)}`,
      `  protocol: ${port.protocol || "TCP"}`,
    ]);
    documents.push([
      "apiVersion: v1",
      "kind: Service",
      "metadata:",
      `  name: ${name}`,
      `  namespace: ${namespace}`,
      "spec:",
      `  type: ${form.serviceType}`,
      "  selector:",
      `    app: ${name}`,
      "  ports:",
      yamlBlock(servicePorts, 2),
    ].join("\n"));
  }

  const domains = form.domains.filter((domain) => domain.host?.trim() && Number(domain.port) > 0);
  if (domains.length > 0) {
    const rules = domains.flatMap((domain) => [
      `- host: ${domain.host.trim()}`,
      "  http:",
      "    paths:",
      "    - path: /",
      "      pathType: Prefix",
      "      backend:",
      "        service:",
      `          name: ${name}`,
      "          port:",
      `            number: ${Number(domain.port)}`,
    ]);
    documents.push([
      "apiVersion: networking.k8s.io/v1",
      "kind: Ingress",
      "metadata:",
      `  name: ${name}`,
      `  namespace: ${namespace}`,
      "spec:",
      ...(domains[0].ingressClass?.trim() ? [`  ingressClassName: ${domains[0].ingressClass.trim()}`] : []),
      "  rules:",
      yamlBlock(rules, 2),
    ].join("\n"));
  }

  if (form.hpa.enabled) {
    documents.push([
      "apiVersion: autoscaling/v2",
      "kind: HorizontalPodAutoscaler",
      "metadata:",
      `  name: ${name}`,
      `  namespace: ${namespace}`,
      "spec:",
      "  scaleTargetRef:",
      "    apiVersion: apps/v1",
      "    kind: Deployment",
      `    name: ${name}`,
      `  minReplicas: ${Number(form.hpa.minReplicas) || 1}`,
      `  maxReplicas: ${Number(form.hpa.maxReplicas) || 1}`,
      "  metrics:",
      "  - type: Resource",
      "    resource:",
      "      name: cpu",
      "      target:",
      "        type: Utilization",
      `        averageUtilization: ${Number(form.hpa.cpuTarget) || 60}`,
    ].join("\n"));
  }

  if (configFiles.length > 0) {
    documents.push([
      "apiVersion: v1",
      "kind: ConfigMap",
      "metadata:",
      `  name: ${name}-config`,
      `  namespace: ${namespace}`,
      "data:",
      ...configFiles.flatMap((file, index) => [
        `  file-${index}: |`,
        ...String(file.content ?? "").split("\n").map((line) => `    ${line}`),
      ]),
    ].join("\n"));
  }

  return documents.join("\n---\n");
}
