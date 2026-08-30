import React, {useEffect, useMemo, useState} from "react";
import i18next from "i18next";
import {useTranslation} from "react-i18next";
import {ArrowLeft, Code2, Plus, Rocket, X} from "lucide-react";
import * as ImageBackend from "@/backend/ImageBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import * as Setting from "@/Setting";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "@/components/ui/card";
import {Checkbox} from "@/components/ui/checkbox";
import {Input} from "@/components/ui/input";
import {Label} from "@/components/ui/label";
import {Textarea} from "@/components/ui/textarea";
import {MessageAlert} from "@/components/ui/alert";
import {DeploymentStorageEditor} from "@/components/shared/deployment-storage-editor";
import {Loading} from "@/components/shared/loading";
import {NumberInput} from "@/components/shared/number-input";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {PasswordInput} from "@/components/shared/password-input";
import {SimpleSelect} from "@/components/shared/simple-select";
import {
  CPU_PRESETS,
  MEMORY_PRESETS,
  SERVICE_TYPES,
  appYamlPreview,
  emptyAppForm,
  formFromDetail,
  payloadFromForm,
  validateAppForm,
} from "@/lib/launchpad";
import {runAction, useResource} from "@/hooks/use-resource";
import {useUiMode} from "@/hooks/use-ui-mode";
import {useWorkspace} from "@/hooks/use-workspace";
import {cn} from "@/lib/utils";

/** A row of preset buttons with a box for anything the presets do not cover. */
function QuantityPicker({value, presets, onChange, placeholder}) {
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {presets.map((preset) => (
        <Button
          key={preset.value}
          type="button"
          size="sm"
          variant={value === preset.value ? "default" : "outline"}
          onClick={() => onChange(preset.value)}
        >
          {preset.label}
        </Button>
      ))}
      <Input
        value={presets.some((preset) => preset.value === value) ? "" : (value ?? "")}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        className="h-8 w-28 font-mono text-xs"
      />
    </div>
  );
}

function SectionCard({title, description, children, action}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">{title}</CardTitle>
        {description ? <CardDescription>{description}</CardDescription> : null}
        {action ? <div className="mt-2">{action}</div> : null}
      </CardHeader>
      <CardContent className="grid gap-4">{children}</CardContent>
    </Card>
  );
}

function FormRow({label, hint, error, children, className}) {
  return (
    <div className={cn("grid gap-1.5", className)}>
      <Label className="text-sm">{label}</Label>
      {children}
      {error ? <p className="text-destructive text-xs">{error}</p> : null}
      {hint && !error ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
    </div>
  );
}

/**
 * The App Launchpad's form: one container turned into a running application.
 *
 * The same screen creates and edits, because the fields are the same and the
 * backend takes the same payload either way. What differs is what an existing
 * app will not let anyone change — its name, and the volumes already bound to
 * it — and those are shown read-only rather than hidden.
 */
function LaunchpadEditPage(props) {
  useTranslation();
  const {history, match} = props;
  const {resolvePath} = useUiMode();
  const editing = Boolean(match.params.name);
  const namespaceParam = match.params.namespace;
  const nameParam = match.params.name;

  const {workspace} = useWorkspace();
  // A new one belongs where the reader is working, not wherever the cluster
  // happens to call home.
  const [form, setForm] = useState(() => emptyAppForm(namespaceParam || workspace || "default"));
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);
  const [showYaml, setShowYaml] = useState(false);
  const [loading, setLoading] = useState(editing);
  const [loadError, setLoadError] = useState(null);

  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});

  useEffect(() => {
    if (!editing) {
      return;
    }
    setLoading(true);
    ImageBackend.getImageApp(namespaceParam, nameParam)
      .then((res) => {
        if (res.status === "ok") {
          setForm(formFromDetail(res.data));
        } else {
          setLoadError(res.msg);
        }
      })
      .catch((error) => setLoadError(error.message))
      .finally(() => setLoading(false));
  }, [editing, namespaceParam, nameParam]);

  const yaml = useMemo(() => appYamlPreview(form), [form]);

  function set(patch) {
    setForm((current) => ({...current, ...patch}));
  }

  function setPorts(next) {
    set({ports: next});
  }

  function submit() {
    const found = validateAppForm(form);
    setErrors(found);
    if (Object.keys(found).length > 0) {
      return;
    }
    setSubmitting(true);
    const payload = payloadFromForm(form, {mode: editing ? "edit" : "create"});
    const request = editing ? ImageBackend.upgradeApp(payload) : ImageBackend.deployApp(payload);
    runAction(request, {
      successMessage: editing
        ? i18next.t("launchpad:App updated")
        : i18next.t("launchpad:App deployed"),
      onSuccess: () => history.push(resolvePath(`/launchpad/${payload.namespace}/${payload.name}`)),
    }).finally(() => setSubmitting(false));
  }

  if (loading) {
    return <Loading type="page" />;
  }

  return (
    <PageContainer>
      <PageHeader
        title={editing ? `${i18next.t("launchpad:Edit app")} — ${nameParam}` : i18next.t("launchpad:Deploy app")}
        description={i18next.t("launchpad:One container, everything it needs to be reachable and to stay up.")}
        actions={
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={() => history.push(resolvePath("/launchpad"))}>
              <ArrowLeft />
              {i18next.t("launchpad:Back")}
            </Button>
            <Button variant={showYaml ? "default" : "outline"} onClick={() => setShowYaml((open) => !open)}>
              <Code2 />
              {i18next.t("launchpad:YAML")}
            </Button>
            <Button onClick={submit} disabled={submitting} data-testid="launchpad-submit">
              <Rocket />
              {editing ? i18next.t("launchpad:Save changes") : i18next.t("general:Deploy")}
            </Button>
          </div>
        }
      />

      {loadError ? <MessageAlert title={loadError} /> : null}

      <div className={cn("grid gap-4", showYaml && "xl:grid-cols-[minmax(0,1fr)_minmax(0,26rem)]")}>
        <div className="grid gap-4">
          <SectionCard title={i18next.t("launchpad:Basics")}>
            <div className="grid gap-4 sm:grid-cols-2">
              <FormRow
                label={i18next.t("general:App name")}
                error={errors.name === "required" ? i18next.t("launchpad:A name is required") : errors.name ? i18next.t("launchpad:Lowercase letters, digits and dashes only") : null}
              >
                <Input
                  value={form.name}
                  disabled={editing}
                  onChange={(event) => set({name: event.target.value})}
                  placeholder="my-app"
                  data-testid="launchpad-name"
                />
              </FormRow>
              <FormRow label={i18next.t("general:Namespace")}>
                <SimpleSelect
                  value={form.namespace}
                  onChange={(next) => set({namespace: next})}
                  disabled={editing}
                  options={namespaces.map((item) => ({label: item.name, value: item.name}))}
                  placeholder="default"
                />
              </FormRow>
            </div>
            <FormRow
              label={i18next.t("general:Image")}
              hint={i18next.t("launchpad:The container image and its tag, for example nginx 1.27")}
              error={errors.image ? i18next.t("launchpad:An image is required") : null}
            >
              <Input
                value={form.image}
                onChange={(event) => set({image: event.target.value})}
                placeholder="nginx:latest"
                className="font-mono text-sm"
                data-testid="launchpad-image"
              />
            </FormRow>
            <div className="grid gap-3">
              <label className="flex items-center gap-2 text-sm">
                <Checkbox
                  checked={form.registry.enabled}
                  onCheckedChange={(checked) => set({registry: {...form.registry, enabled: Boolean(checked)}})}
                />
                {i18next.t("launchpad:This image is private")}
              </label>
              {form.registry.enabled ? (
                <div className="grid gap-3 sm:grid-cols-3">
                  <FormRow label={i18next.t("launchpad:Registry")}>
                    <Input
                      value={form.registry.server}
                      onChange={(event) => set({registry: {...form.registry, server: event.target.value}})}
                      placeholder="docker.io"
                    />
                  </FormRow>
                  <FormRow label={i18next.t("general:Username")}>
                    <Input
                      value={form.registry.username}
                      onChange={(event) => set({registry: {...form.registry, username: event.target.value}})}
                    />
                  </FormRow>
                  <FormRow
                    label={i18next.t("general:Password")}
                    hint={editing ? i18next.t("launchpad:Leave blank to keep the stored password") : null}
                  >
                    <PasswordInput
                      value={form.registry.password}
                      onChange={(event) => set({registry: {...form.registry, password: event.target.value}})}
                    />
                  </FormRow>
                </div>
              ) : null}
            </div>
          </SectionCard>

          <SectionCard
            title={i18next.t("launchpad:Size and scaling")}
            description={i18next.t("launchpad:What one copy may use, and how many copies run.")}
          >
            <div className="grid gap-4 sm:grid-cols-2">
              <FormRow label={i18next.t("launchpad:CPU limit")}>
                <QuantityPicker
                  value={form.cpuLimit}
                  presets={CPU_PRESETS}
                  onChange={(next) => set({cpuLimit: next})}
                  placeholder="e.g. 250m"
                />
              </FormRow>
              <FormRow label={i18next.t("launchpad:Memory limit")}>
                <QuantityPicker
                  value={form.memoryLimit}
                  presets={MEMORY_PRESETS}
                  onChange={(next) => set({memoryLimit: next})}
                  placeholder="e.g. 384Mi"
                />
              </FormRow>
            </div>

            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={form.hpa.enabled}
                onCheckedChange={(checked) => set({hpa: {...form.hpa, enabled: Boolean(checked)}})}
                data-testid="launchpad-hpa"
              />
              {i18next.t("launchpad:Scale automatically with CPU load")}
            </label>

            {form.hpa.enabled ? (
              <div className="grid gap-3 sm:grid-cols-3">
                <FormRow label={i18next.t("launchpad:Minimum copies")}>
                  <NumberInput
                    value={form.hpa.minReplicas}
                    min={1}
                    onChange={(next) => set({hpa: {...form.hpa, minReplicas: next}})}
                  />
                </FormRow>
                <FormRow
                  label={i18next.t("launchpad:Maximum copies")}
                  error={errors.hpa ? i18next.t("launchpad:The maximum must not be below the minimum") : null}
                >
                  <NumberInput
                    value={form.hpa.maxReplicas}
                    min={1}
                    onChange={(next) => set({hpa: {...form.hpa, maxReplicas: next}})}
                  />
                </FormRow>
                <FormRow label={i18next.t("launchpad:Target CPU %")}>
                  <NumberInput
                    value={form.hpa.cpuTarget}
                    min={1}
                    max={100}
                    onChange={(next) => set({hpa: {...form.hpa, cpuTarget: next}})}
                  />
                </FormRow>
              </div>
            ) : (
              <FormRow label={i18next.t("launchpad:Copies")} className="sm:max-w-40">
                <NumberInput value={form.replicas} min={0} onChange={(next) => set({replicas: next})} />
              </FormRow>
            )}
          </SectionCard>

          <SectionCard
            title={i18next.t("launchpad:Network")}
            description={i18next.t("launchpad:The ports the container listens on, and how the outside reaches them.")}
            action={
              <Button
                size="sm"
                variant="outline"
                onClick={() => setPorts([...form.ports, {containerPort: 8080, protocol: "TCP", name: ""}])}
              >
                <Plus />
                {i18next.t("launchpad:Add port")}
              </Button>
            }
          >
            <div className="grid gap-2">
              {form.ports.map((port, index) => (
                <div key={index} className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)_auto] items-center gap-2">
                  <NumberInput
                    value={port.containerPort}
                    min={1}
                    max={65535}
                    onChange={(next) => setPorts(form.ports.map((item, itemIndex) => (itemIndex === index ? {...item, containerPort: next} : item)))}
                  />
                  <SimpleSelect
                    value={port.protocol}
                    onChange={(next) => setPorts(form.ports.map((item, itemIndex) => (itemIndex === index ? {...item, protocol: next} : item)))}
                    options={[{label: "TCP", value: "TCP"}, {label: "UDP", value: "UDP"}]}
                    size="sm"
                  />
                  <Input
                    value={port.name ?? ""}
                    onChange={(event) => setPorts(form.ports.map((item, itemIndex) => (itemIndex === index ? {...item, name: event.target.value} : item)))}
                    placeholder={i18next.t("launchpad:Port name")}
                    className="h-8 text-xs"
                  />
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={i18next.t("general:Remove")}
                    onClick={() => setPorts(form.ports.filter((_, itemIndex) => itemIndex !== index))}
                  >
                    <X />
                  </Button>
                </div>
              ))}
              {form.ports.length === 0 ? (
                <p className="text-muted-foreground text-xs">{i18next.t("launchpad:Without a port the app will not be reachable from outside its pod.")}</p>
              ) : null}
            </div>

            <FormRow
              label={i18next.t("launchpad:How it is exposed")}
              hint={i18next.t("launchpad:NodePort publishes the app on every node's address; ClusterIP keeps it inside the cluster.")}
              className="sm:max-w-60"
            >
              <SimpleSelect
                value={form.serviceType}
                onChange={(next) => set({serviceType: next})}
                options={SERVICE_TYPES.map((type) => ({label: type, value: type}))}
              />
            </FormRow>

            <div className="grid gap-2">
              <div className="flex items-center justify-between">
                <Label className="text-sm">{i18next.t("launchpad:Domains")}</Label>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => set({domains: [...form.domains, {host: "", port: form.ports[0]?.containerPort ?? 80, ingressClass: "", https: true}]})}
                >
                  <Plus />
                  {i18next.t("launchpad:Add domain")}
                </Button>
              </div>
              {form.domains.map((domain, index) => (
                <div key={index} className="grid grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_auto_auto] items-center gap-2">
                  <Input
                    value={domain.host}
                    onChange={(event) => set({domains: form.domains.map((item, itemIndex) => (itemIndex === index ? {...item, host: event.target.value} : item))})}
                    placeholder="app.example.com"
                    className="h-8 font-mono text-xs"
                  />
                  <NumberInput
                    value={domain.port}
                    min={1}
                    max={65535}
                    onChange={(next) => set({domains: form.domains.map((item, itemIndex) => (itemIndex === index ? {...item, port: next} : item))})}
                  />
                  <Input
                    value={domain.ingressClass ?? ""}
                    onChange={(event) => set({domains: form.domains.map((item, itemIndex) => (itemIndex === index ? {...item, ingressClass: event.target.value} : item))})}
                    placeholder={i18next.t("launchpad:Ingress class")}
                    className="h-8 text-xs"
                  />
                  <label className="flex items-center gap-1.5 text-xs whitespace-nowrap">
                    <Checkbox
                      checked={Boolean(domain.https)}
                      onCheckedChange={(next) => set({domains: form.domains.map((item, itemIndex) => (itemIndex === index ? {...item, https: Boolean(next)} : item))})}
                    />
                    {i18next.t("launchpad:HTTPS")}
                  </label>
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={i18next.t("general:Remove")}
                    onClick={() => set({domains: form.domains.filter((_, itemIndex) => itemIndex !== index)})}
                  >
                    <X />
                  </Button>
                </div>
              ))}
              {errors.domains ? <p className="text-destructive text-xs">{i18next.t("launchpad:A domain needs the port it forwards to")}</p> : null}
              {form.domains.length === 0 ? (
                <p className="text-muted-foreground text-xs">{i18next.t("launchpad:A domain needs an ingress controller in the cluster and DNS pointing at it.")}</p>
              ) : (
                <p className="text-muted-foreground text-xs">
                  {i18next.t("launchpad:HTTPS asks Let's Encrypt for a certificate once the app is saved. The domain has to already resolve to this cluster.")}
                </p>
              )}
            </div>
          </SectionCard>

          <SectionCard title={i18next.t("general:Environment")}>
            <div className="grid gap-2">
              <div className="flex items-center justify-between">
                <Label className="text-sm">{i18next.t("launchpad:Environment variables")}</Label>
                <Button size="sm" variant="outline" onClick={() => set({envVars: [...form.envVars, {name: "", value: ""}]})}>
                  <Plus />
                  {i18next.t("launchpad:Add variable")}
                </Button>
              </div>
              {form.envVars.map((env, index) => (
                <div key={index} className="grid grid-cols-[minmax(0,1fr)_minmax(0,2fr)_auto] items-center gap-2">
                  <Input
                    value={env.name}
                    onChange={(event) => set({envVars: form.envVars.map((item, itemIndex) => (itemIndex === index ? {...item, name: event.target.value} : item))})}
                    placeholder="KEY"
                    className="h-8 font-mono text-xs"
                  />
                  <Input
                    value={env.value ?? ""}
                    onChange={(event) => set({envVars: form.envVars.map((item, itemIndex) => (itemIndex === index ? {...item, value: event.target.value} : item))})}
                    placeholder="value"
                    className="h-8 font-mono text-xs"
                  />
                  <Button
                    size="icon-sm"
                    variant="ghost"
                    aria-label={i18next.t("general:Remove")}
                    onClick={() => set({envVars: form.envVars.filter((_, itemIndex) => itemIndex !== index)})}
                  >
                    <X />
                  </Button>
                </div>
              ))}
            </div>

            <div className="grid gap-3 sm:grid-cols-2">
              <FormRow label={i18next.t("launchpad:Command")} hint={i18next.t("launchpad:Overrides the image's entrypoint")}>
                <Input
                  value={form.command}
                  onChange={(event) => set({command: event.target.value})}
                  placeholder="/bin/sh -c"
                  className="font-mono text-xs"
                />
              </FormRow>
              <FormRow label={i18next.t("launchpad:Arguments")}>
                <Input
                  value={form.args}
                  onChange={(event) => set({args: event.target.value})}
                  placeholder="server --port 8080"
                  className="font-mono text-xs"
                />
              </FormRow>
            </div>
          </SectionCard>

          <SectionCard
            title={i18next.t("launchpad:Files and storage")}
            description={i18next.t("launchpad:Config files written into the container, and disks that survive a restart.")}
            action={
              <Button size="sm" variant="outline" onClick={() => set({configFiles: [...form.configFiles, {mountPath: "", content: ""}]})}>
                <Plus />
                {i18next.t("launchpad:Add config file")}
              </Button>
            }
          >
            <div className="grid gap-3">
              {form.configFiles.map((file, index) => (
                <div key={index} className="grid gap-2 rounded-lg border p-3">
                  <div className="flex items-center gap-2">
                    <Input
                      value={file.mountPath}
                      onChange={(event) => set({configFiles: form.configFiles.map((item, itemIndex) => (itemIndex === index ? {...item, mountPath: event.target.value} : item))})}
                      placeholder="/etc/nginx/nginx.conf"
                      className="h-8 font-mono text-xs"
                    />
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label={i18next.t("general:Remove")}
                      onClick={() => set({configFiles: form.configFiles.filter((_, itemIndex) => itemIndex !== index)})}
                    >
                      <X />
                    </Button>
                  </div>
                  <Textarea
                    value={file.content ?? ""}
                    onChange={(event) => set({configFiles: form.configFiles.map((item, itemIndex) => (itemIndex === index ? {...item, content: event.target.value} : item))})}
                    rows={6}
                    className="font-mono text-xs"
                    placeholder={i18next.t("launchpad:File contents")}
                  />
                </div>
              ))}
            </div>

            <div className="grid gap-2">
              <div className="flex items-center justify-between">
                <Label className="text-sm">{i18next.t("general:Storage")}</Label>
                {!editing ? (
                  <Button size="sm" variant="outline" onClick={() => set({volumes: [...form.volumes, {mountPath: "", size: "1Gi"}]})}>
                    <Plus />
                    {i18next.t("launchpad:Add disk")}
                  </Button>
                ) : null}
              </div>
              <DeploymentStorageEditor
                mode={editing ? "edit" : "add"}
                value={form.volumes}
                onChange={(next) => set({volumes: next})}
              />
            </div>
          </SectionCard>
        </div>

        {showYaml ? (
          <Card className="xl:sticky xl:top-4 xl:max-h-[calc(100vh-6rem)]">
            <CardHeader>
              <CardTitle className="text-base">{i18next.t("launchpad:Manifests")}</CardTitle>
              <CardDescription>{i18next.t("launchpad:What this form asks the cluster for.")}</CardDescription>
              <div className="mt-2 flex items-center gap-2">
                <Badge variant="secondary">{form.namespace || "default"}</Badge>
                <Button size="sm" variant="outline" onClick={() => {
                  navigator.clipboard?.writeText(yaml);
                  Setting.showMessage("success", i18next.t("launchpad:Copied"));
                }}>
                  {i18next.t("launchpad:Copy")}
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              <pre className="bg-muted/50 max-h-[60vh] overflow-auto rounded-lg p-3 font-mono text-xs" data-testid="launchpad-yaml">
                {yaml}
              </pre>
            </CardContent>
          </Card>
        ) : null}
      </div>
    </PageContainer>
  );
}

export default LaunchpadEditPage;
