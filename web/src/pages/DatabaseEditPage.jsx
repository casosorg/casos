import React, {useEffect, useMemo, useState} from "react";
import i18next from "i18next";
import {useTranslation} from "react-i18next";
import {ArrowLeft, Database, Save} from "lucide-react";
import * as DatabaseBackend from "@/backend/DatabaseBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import {Button} from "@/components/ui/button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "@/components/ui/card";
import {Checkbox} from "@/components/ui/checkbox";
import {Input} from "@/components/ui/input";
import {Label} from "@/components/ui/label";
import {MessageAlert} from "@/components/ui/alert";
import {Loading} from "@/components/shared/loading";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {PasswordInput} from "@/components/shared/password-input";
import {SimpleSelect} from "@/components/shared/simple-select";
import {CPU_PRESETS, MEMORY_PRESETS} from "@/lib/launchpad";
import {
  STORAGE_PRESETS,
  databasePayload,
  emptyDatabaseForm,
  engineTint,
  formFromDatabase,
  validateDatabaseForm,
} from "@/lib/database";
import {cn} from "@/lib/utils";
import {runAction, useResource} from "@/hooks/use-resource";

function PresetRow({value, presets, onChange, placeholder}) {
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {presets.map((preset) => {
        const presetValue = typeof preset === "string" ? preset : preset.value;
        const presetLabel = typeof preset === "string" ? preset : preset.label;
        return (
          <Button
            key={presetValue}
            type="button"
            size="sm"
            variant={value === presetValue ? "default" : "outline"}
            onClick={() => onChange(presetValue)}
          >
            {presetLabel}
          </Button>
        );
      })}
      <Input
        value={presets.some((preset) => (typeof preset === "string" ? preset : preset.value) === value) ? "" : (value ?? "")}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        className="h-8 w-28 font-mono text-xs"
      />
    </div>
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
 * Creating and editing a database.
 *
 * What an existing database will not change is what separates the two: its
 * engine, its name and its credentials are set once, because the engine stores
 * them itself and rewriting the Secret alone would only lie about them.
 */
function DatabaseEditPage(props) {
  useTranslation();
  const {history, match} = props;
  const editing = Boolean(match.params.name);
  const namespaceParam = match.params.namespace;
  const nameParam = match.params.name;

  const [form, setForm] = useState(() => emptyDatabaseForm(namespaceParam || "default"));
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);
  const [loading, setLoading] = useState(editing);
  const [loadError, setLoadError] = useState(null);

  const {data: engines} = useResource(() => DatabaseBackend.getDatabaseEngines(), [], {initialData: [], toastOnError: false});
  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});

  useEffect(() => {
    if (!editing) {
      return;
    }
    setLoading(true);
    DatabaseBackend.getDatabase(namespaceParam, nameParam)
      .then((res) => {
        if (res.status === "ok") {
          setForm(formFromDatabase(res.data));
        } else {
          setLoadError(res.msg);
        }
      })
      .catch((error) => setLoadError(error.message))
      .finally(() => setLoading(false));
  }, [editing, namespaceParam, nameParam]);

  const engine = useMemo(
    () => engines.find((item) => item.key === form.engine) ?? null,
    [engines, form.engine]
  );

  function set(patch) {
    setForm((current) => ({...current, ...patch}));
  }

  function pickEngine(next) {
    const chosen = engines.find((item) => item.key === next);
    // An engine with a fixed superuser has no user field to keep, and one with
    // no database names has no name to keep; carrying either across a switch
    // would send the new engine a value it never showed anyone.
    set({
      engine: next,
      version: chosen?.versions?.[0] ?? "",
      user: chosen?.fixedUser ? "" : form.user,
      database: chosen?.supportsDatabaseName ? form.database : "",
    });
  }

  function submit() {
    const found = validateDatabaseForm(form);
    setErrors(found);
    if (Object.keys(found).length > 0) {
      return;
    }
    setSubmitting(true);
    const payload = databasePayload(form, {mode: editing ? "edit" : "create"});
    const request = editing ? DatabaseBackend.updateDatabase(payload) : DatabaseBackend.createDatabase(payload);
    runAction(request, {
      successMessage: editing ? i18next.t("database:Database updated") : i18next.t("database:Database created"),
      onSuccess: () => history.push(`/databases/${payload.namespace}/${payload.name}`),
    }).finally(() => setSubmitting(false));
  }

  if (loading) {
    return <Loading type="page" />;
  }

  return (
    <PageContainer>
      <PageHeader
        title={editing ? `${i18next.t("database:Edit database")} — ${nameParam}` : i18next.t("database:New database")}
        description={i18next.t("database:Pick an engine and a size; the credentials, storage and address are set up for you.")}
        actions={
          <div className="flex items-center gap-2">
            <Button variant="outline" onClick={() => history.push("/databases")}>
              <ArrowLeft />
              {i18next.t("launchpad:Back")}
            </Button>
            <Button onClick={submit} disabled={submitting} data-testid="database-submit">
              <Save />
              {editing ? i18next.t("launchpad:Save changes") : i18next.t("database:Create")}
            </Button>
          </div>
        }
      />

      {loadError ? <MessageAlert title={loadError} /> : null}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{i18next.t("database:Engine")}</CardTitle>
          <CardDescription>{i18next.t("database:An engine cannot be changed once the data is in it.")}</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {engines.map((item) => (
              <button
                key={item.key}
                type="button"
                disabled={editing}
                data-testid={`database-engine-${item.key}`}
                onClick={() => pickEngine(item.key)}
                className={cn(
                  "flex items-center gap-3 rounded-xl border p-3 text-left transition-colors",
                  form.engine === item.key ? "border-primary bg-accent" : "hover:bg-accent/50",
                  editing && "opacity-60"
                )}
              >
                <span className={cn("flex size-9 shrink-0 items-center justify-center rounded-lg text-white", engineTint(item.key))}>
                  <Database className="size-4.5" />
                </span>
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium">{item.label}</span>
                  <span className="text-muted-foreground block truncate text-xs">{i18next.t("database:Port")} {item.port}</span>
                </span>
              </button>
            ))}
          </div>

          <div className="grid gap-4 sm:grid-cols-3">
            <FormRow label={i18next.t("database:Version")}>
              <SimpleSelect
                value={form.version || engine?.versions?.[0] || ""}
                onChange={(next) => set({version: next})}
                options={(engine?.versions ?? []).map((version) => ({label: version, value: version}))}
                placeholder={i18next.t("database:Version")}
              />
            </FormRow>
            <FormRow
              label={i18next.t("database:Name")}
              error={errors.name === "required"
                ? i18next.t("launchpad:A name is required")
                : errors.name ? i18next.t("launchpad:Lowercase letters, digits and dashes only") : null}
            >
              <Input
                value={form.name}
                disabled={editing}
                onChange={(event) => set({name: event.target.value})}
                placeholder="my-database"
                data-testid="database-name"
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
        </CardContent>
      </Card>

      {!editing ? (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{i18next.t("database:Credentials")}</CardTitle>
            <CardDescription>{i18next.t("database:Leave the password blank and one is generated. You can read it back on the database's page.")}</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4 sm:grid-cols-3">
            <FormRow
              label={i18next.t("launchpad:Username")}
              hint={engine?.fixedUser ? i18next.t("database:This engine only has one superuser.") : null}
            >
              <Input
                value={engine?.fixedUser ? engine.defaultUser : form.user}
                disabled={engine?.fixedUser}
                onChange={(event) => set({user: event.target.value})}
                placeholder={engine?.defaultUser ?? ""}
              />
            </FormRow>
            <FormRow
              label={i18next.t("database:Database name")}
              hint={engine?.supportsDatabaseName ? null : i18next.t("database:This engine has no database names.")}
            >
              <Input
                value={form.database}
                disabled={engine ? !engine.supportsDatabaseName : false}
                onChange={(event) => set({database: event.target.value})}
                placeholder={form.name || "app"}
              />
            </FormRow>
            <FormRow label={i18next.t("launchpad:Password")}>
              <PasswordInput
                value={form.password}
                onChange={(event) => set({password: event.target.value})}
                placeholder={i18next.t("database:Generated")}
              />
            </FormRow>
          </CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{i18next.t("database:Size")}</CardTitle>
          <CardDescription>{i18next.t("database:Storage can grow later but never shrink.")}</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <FormRow label={i18next.t("launchpad:CPU limit")}>
              <PresetRow value={form.cpuLimit} presets={CPU_PRESETS} onChange={(next) => set({cpuLimit: next})} placeholder="e.g. 250m" />
            </FormRow>
            <FormRow label={i18next.t("launchpad:Memory limit")}>
              <PresetRow value={form.memoryLimit} presets={MEMORY_PRESETS} onChange={(next) => set({memoryLimit: next})} placeholder="e.g. 512Mi" />
            </FormRow>
          </div>
          <FormRow
            label={i18next.t("database:Storage")}
            error={errors.storage ? i18next.t("database:Use a size like 10Gi") : null}
          >
            <PresetRow value={form.storage} presets={STORAGE_PRESETS} onChange={(next) => set({storage: next})} placeholder="e.g. 20Gi" />
          </FormRow>
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={form.publicAccess}
              onCheckedChange={(checked) => set({publicAccess: Boolean(checked)})}
              data-testid="database-public"
            />
            {i18next.t("database:Reachable from outside the cluster")}
          </label>
          <p className="text-muted-foreground text-xs">
            {i18next.t("database:Applications inside the cluster always reach it by its service name. Outside access publishes it on a node port.")}
          </p>
        </CardContent>
      </Card>
    </PageContainer>
  );
}

export default DatabaseEditPage;
