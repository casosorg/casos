/**
 * Where an installed app can be reached, and what disks it holds.
 *
 * Both are properties of one app rather than of the cluster, which is why they
 * are assembled per Helm release here instead of being browsed as three
 * separate object lists. The backend stamps every Service, Ingress and volume
 * with the release that owns it (see controllers/helm_ownership.go); this reads
 * that field back.
 */

// A NodePort service is reached through any node's address; a LoadBalancer
// through the addresses the controller assigned it. The scheme is inferred the
// same way the old UI did: port 443, or a port named for HTTPS.
export function serviceAccessUrls(record, nodeIP) {
  if (record.type !== "NodePort" && record.type !== "LoadBalancer") {
    return [];
  }
  const isLoadBalancer = record.type === "LoadBalancer";
  const ports = (record.ports ?? []).filter((port) => (isLoadBalancer ? port.port : port.nodePort));
  const addresses = isLoadBalancer ? (record.loadBalancerAddresses ?? []) : nodeIP ? [nodeIP] : [];

  const urls = [];
  addresses.forEach((address) => {
    ports.forEach((port) => {
      const exposed = isLoadBalancer ? port.port : port.nodePort;
      const host = address.includes(":") ? `[${address}]` : address;
      const portName = String(port.name ?? "").toLowerCase();
      const scheme = port.port === 443 || portName.includes("https") || portName.includes("websecure") ? "https" : "http";
      urls.push(`${scheme}://${host}:${exposed}`);
    });
  });
  return urls;
}

// An Ingress rule already names the host the app answers on; TLS on the
// Ingress is what decides the scheme.
export function ingressAccessUrls(record) {
  const scheme = record.tlsEnabled ? "https" : "http";
  const urls = [];
  (record.rules ?? []).forEach((rule) => {
    if (!rule.host) {
      return;
    }
    const path = rule.path && rule.path !== "/" ? rule.path : "";
    urls.push(`${scheme}://${rule.host}${path}`);
  });
  return urls;
}

// Any node's address stands in for "the cluster" when resolving a NodePort:
// an external address if one exists, otherwise the internal one.
export function clusterNodeAddress(nodes) {
  const list = nodes ?? [];
  return list.find((node) => node.externalIP)?.externalIP ?? list.find((node) => node.internalIP)?.internalIP ?? null;
}

function ownerKey(namespace, releaseName) {
  return `${namespace}/${releaseName}`;
}

/**
 * Indexes Services, Ingresses and volumes by the release that owns them.
 * Returns a Map keyed by "namespace/releaseName"; a release with nothing of its
 * own is simply absent, which callers read as "no address yet".
 *
 * Ingress addresses come first: a domain name is what someone would rather be
 * given than an IP and a port.
 */
export function groupAppResources({services, ingresses, pvcs, nodes}) {
  const nodeIP = clusterNodeAddress(nodes);
  const byRelease = new Map();

  function entryFor(record) {
    if (!record.helmRelease) {
      return null;
    }
    const key = ownerKey(record.namespace, record.helmRelease);
    if (!byRelease.has(key)) {
      byRelease.set(key, {urls: [], disks: []});
    }
    return byRelease.get(key);
  }

  (ingresses ?? []).forEach((ingress) => {
    const entry = entryFor(ingress);
    if (entry) {
      entry.urls.push(...ingressAccessUrls(ingress));
    }
  });

  (services ?? []).forEach((service) => {
    const entry = entryFor(service);
    if (entry) {
      entry.urls.push(...serviceAccessUrls(service, nodeIP));
    }
  });

  (pvcs ?? []).forEach((pvc) => {
    const entry = entryFor(pvc);
    if (entry) {
      entry.disks.push({name: pvc.name, storage: pvc.storage, status: pvc.status});
    }
  });

  byRelease.forEach((entry) => {
    entry.urls = [...new Set(entry.urls)];
  });
  return byRelease;
}

export function appResourcesOf(grouped, release) {
  return grouped.get(ownerKey(release.namespace, release.name)) ?? {urls: [], disks: []};
}
