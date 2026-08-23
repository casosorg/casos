import React, {useEffect, useMemo, useState} from "react";
import {useTranslation} from "react-i18next";
import {Database} from "lucide-react";
import * as ImageBackend from "@/backend/ImageBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import * as PodBackend from "@/backend/PodBackend";
import {runAction, useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Checkbox} from "@/components/ui/checkbox";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {Separator} from "@/components/ui/separator";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {KeyValueEditor} from "@/components/shared/key-value-editor";
import {DeploymentStorageEditor} from "@/components/shared/deployment-storage-editor";
import {PasswordInput} from "@/components/shared/password-input";
import {SimpleSelect} from "@/components/shared/simple-select";
import {Loading} from "@/components/shared/loading";
import {DATABASE_ENGINES} from "@/lib/imageCatalog";

const DEFAULT_VOLUME_SIZE = "5Gi";
const SERVICE_TYPES = ["ClusterIP", "NodePort", "LoadBalancer"];
const PASSWORD_ALPHABET = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789";

function sanitizeName(value) {
  const name = String(value ?? "")
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40)
    .replace(/-+$/, "");
  return name || "app";
}

function randomPassword(length = 20) {
  const bytes = new Uint32Array(length);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (byte) => PASSWORD_ALPHABET[byte % PASSWORD_ALPHABET.length]).join("");
}

function repositoryOf(image) {
  const ref = String(image ?? "").trim();
  const lastSlash = ref.lastIndexOf("/");
  const colon = ref.lastIndexOf(":");
  return colon > lastSlash ? ref.slice(0, colon) : ref;
}

function defaultAppName(repository) {
  const segments = repository.split("/");
  return sanitizeName(segments[segments.length - 1]);
}

/** The env vars a companion database would own, and the app must not also set. */
function databaseEnvKeys(hint) {
  return new Set([hint?.hostEnv, hint?.portEnv, hint?.nameEnv, hint?.userEnv, hint?.passwordEnv].filter(Boolean));
}

function envRowsFor(config, keys, wanted) {
  return (config.env ?? [])
    .filter((item) => item.configurable && keys.has(item.name) === wanted)
    .map((item) => ({key: item.name, value: item.value}));
}

/**
 * What the app is told about its database. The image ships the connection env
 * its compose file used — PKP_DB_HOST=db and a placeholder password — so every
 * one of those values is replaced by the database being created alongside.
 * Shared with the preview so the form cannot show one thing and deploy another.
 */
function wiredDatabaseEnv(hint, appName, database) {
  const engine = DATABASE_ENGINES[database.engine];
  const rows = [{key: hint.hostEnv, value: `${appName}-db`}];
  if (hint.portEnv) {
    rows.push({key: hint.portEnv, value: String(engine.port)});
  }
  if (hint.nameEnv) {
    rows.push({key: hint.nameEnv, value: database.name});
  }
  if (hint.userEnv) {
    rows.push({key: hint.userEnv, value: database.user});
  }
  if (hint.passwordEnv) {
    rows.push({key: hint.passwordEnv, value: database.password});
  }
  return rows;
}

function wiredEnvPreview(hint, appName, database) {
  return wiredDatabaseEnv(hint, appName, database).map((row) =>
    row.key === hint.passwordEnv ? {...row, value: "••••••••"} : row
  );
}

// The form the installed app hands back, so an upgrade starts from what is
// running rather than from the image's defaults: a variable someone typed at
// install time must survive the move to a newer tag.
function formOfInstalledApp(app) {
  return {
    namespace: app.namespace,
    name: app.name,
    serviceType: app.serviceType || "NodePort",
    ports: (app.ports ?? []).map((port) => ({
      port: port.containerPort,
      protocol: port.protocol || "TCP",
      expose: true,
    })),
    volumes: (app.volumes ?? []).map((volume) => ({claimName: volume.claimName, mountPath: volume.mountPath})),
    env: (app.envVars ?? []).map((item) => ({key: item.name, value: item.value ?? ""})),
  };
}

/**
 * Installs a container image as a Deployment, its PVCs and a Service, and
 * upgrades what it installed.
 *
 * On install every field starts from the image's own config — the ports it
 * exposes, the paths it declares as volumes, the env it ships with — so the
 * common case is to read the form rather than fill it. The one thing no image
 * records is that it needs a database at all; where its env names one, the
 * installer offers to create that database alongside and wire the two together.
 *
 * On upgrade the same form starts from the running app instead. Its volumes are
 * shown but not editable: a mount is bound to a claim named after its position
 * in the list, so re-ordering that list would hand an app someone else's data.
 */
export function ImageInstallDialog({open, image, action = "install", app, onClose, onInstalled}) {
  const {t} = useTranslation();
  const upgrading = action === "upgrade";
  const repository = upgrading ? (app?.repository ?? "") : repositoryOf(image);

  const [tag, setTag] = useState("");
  const [tags, setTags] = useState([]);
  const [config, setConfig] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [submitting, setSubmitting] = useState(false);
  const [errors, setErrors] = useState({});
  const [form, setForm] = useState(null);
  const [database, setDatabase] = useState(null);

  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {
    initialData: [],
    toastOnError: false,
    enabled: open && !upgrading,
  });

  useEffect(() => {
    if (!open) {
      return;
    }
    setTag(upgrading ? (app?.tag ?? "") : "");
    setTags([]);
    setErrors({});
    setConfig(null);
    setDatabase(null);
    // A different image starts from its own config, never from the last one's;
    // an upgrade starts from the app that is running.
    setForm(upgrading && app ? formOfInstalledApp(app) : null);
    PodBackend.getDockerHubImageTags(repository).then((res) => {
      if (res.status === "ok") {
        setTags(res.data ?? []);
      }
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, repository, upgrading, app?.name, app?.namespace]);

  // Re-read the config whenever the tag changes: ports, volumes and env all
  // belong to one specific tag, not to the repository.
  useEffect(() => {
    if (!open || !repository) {
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    ImageBackend.getImageConfig(tag ? `${repository}:${tag}` : repository)
      .then((res) => {
        if (cancelled) {
          return;
        }
        if (res.status !== "ok") {
          setError(res.msg);
          setConfig(null);
          return;
        }
        const loaded = res.data ?? {};
        setConfig(loaded);
        if (upgrading) {
          return;
        }
        const name = defaultAppName(repository);
        const hint = loaded.database;
        const wired = hint && DATABASE_ENGINES[hint.engine];
        // With a database offered, its four connection variables belong to that
        // section rather than the env editor: leaving them here would show the
        // image's placeholder values next to the ones actually deployed.
        const ownedKeys = wired ? databaseEnvKeys(hint) : new Set();
        setForm((previous) => ({
          namespace: previous?.namespace ?? "default",
          name: previous?.name ?? name,
          serviceType: previous?.serviceType ?? "NodePort",
          // A web image often exposes several ports; the lowest is the one it
          // actually serves on, and the rest are alternates nobody asked for.
          ports: (loaded.ports ?? []).map((port, index) => ({...port, expose: index === 0})),
          volumes: (loaded.volumes ?? []).map((path) => ({mountPath: path, size: DEFAULT_VOLUME_SIZE})),
          env: envRowsFor(loaded, ownedKeys, false),
        }));
        setDatabase(
          wired
            ? {
              enabled: true,
              engine: hint.engine,
              name: sanitizeName(hint.name || name),
              user: sanitizeName(hint.user || name),
              password: randomPassword(),
              size: DEFAULT_VOLUME_SIZE,
            }
            : null
        );
      })
      .catch((e) => {
        if (!cancelled) {
          setError(e.message);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [open, repository, tag, upgrading]);

  // A newer tag can expose a port the installed one did not. Those are offered
  // unchecked, next to the ports the app already answers on.
  useEffect(() => {
    if (!upgrading || !config) {
      return;
    }
    setForm((previous) => {
      if (!previous) {
        return previous;
      }
      const known = new Set(previous.ports.map((port) => port.port));
      const added = (config.ports ?? [])
        .filter((port) => !known.has(port.port))
        .map((port) => ({...port, expose: false}));
      return added.length > 0 ? {...previous, ports: [...previous.ports, ...added]} : previous;
    });
  }, [config, upgrading]);

  const hiddenEnv = useMemo(() => {
    if (!config || !form) {
      return [];
    }
    const present = new Set(form.env.map((row) => row.key));
    const owned = database?.enabled ? databaseEnvKeys(config.database) : new Set();
    return (config.env ?? []).filter((item) => !present.has(item.name) && !owned.has(item.name));
  }, [config, form, database]);

  function updateForm(patch) {
    setForm((previous) => ({...previous, ...patch}));
  }

  // Turning the companion database off hands its four connection variables back
  // to the env editor, so the app can be pointed at a database that already
  // exists instead.
  function toggleDatabase(enabled) {
    const owned = databaseEnvKeys(config.database);
    setDatabase({...database, enabled});
    setForm((previous) => ({
      ...previous,
      env: enabled
        ? previous.env.filter((row) => !owned.has(row.key))
        : [...previous.env, ...envRowsFor(config, owned, true)],
    }));
  }

  function togglePort(index, checked) {
    updateForm({ports: form.ports.map((port, portIndex) => (portIndex === index ? {...port, expose: checked} : port))});
  }

  function exposedPorts() {
    return form.ports
      .filter((port) => port.expose)
      .map((port) => ({name: `port-${port.port}`, containerPort: port.port, protocol: port.protocol}));
  }

  async function handleUpgrade(envRows) {
    const updated = await runAction(
      ImageBackend.upgradeApp({
        namespace: app.namespace,
        name: app.name,
        image: config.image,
        serviceType: form.serviceType,
        ports: exposedPorts(),
        envVars: envRows.map((row) => ({name: row.key, value: row.value})),
        volumes: form.volumes.map((volume) => ({mountPath: volume.mountPath})),
      }),
      {successMessage: t("image:App updated")}
    );
    return Boolean(updated);
  }

  async function handleInstall(envRows) {
    const appName = sanitizeName(form.name);
    let rows = envRows;

    if (database?.enabled) {
      const engine = DATABASE_ENGINES[database.engine];
      const databaseName = `${appName}-db`;
      const created = await runAction(
        ImageBackend.deployApp({
          namespace: form.namespace,
          name: databaseName,
          owner: appName,
          component: "database",
          image: engine.image,
          replicas: 1,
          serviceType: "ClusterIP",
          ports: [{name: "db", containerPort: engine.port, protocol: "TCP"}],
          envVars: engine
            .env({...database, rootPassword: randomPassword()})
            .map((entry) => ({name: entry.key, value: entry.value})),
          volumes: [{mountPath: engine.mountPath, size: database.size}],
        }),
        {successMessage: t("image:Database created")}
      );
      if (!created) {
        return false;
      }

      const wired = new Map(rows.map((row) => [row.key, row.value]));
      for (const row of wiredDatabaseEnv(config.database, appName, database)) {
        wired.set(row.key, row.value);
      }
      rows = Array.from(wired, ([key, value]) => ({key, value}));
    }

    const installed = await runAction(
      ImageBackend.deployApp({
        namespace: form.namespace,
        name: appName,
        image: config.image,
        replicas: 1,
        serviceType: form.serviceType,
        ports: exposedPorts(),
        envVars: rows.map((row) => ({name: row.key, value: row.value})),
        volumes: form.volumes.filter((volume) => volume.mountPath.trim()),
      }),
      {successMessage: t("image:App installed")}
    );
    return Boolean(installed);
  }

  async function handleSubmit() {
    if (!form || !config) {
      return;
    }
    const nextErrors = {};
    if (!form.name.trim()) {
      nextErrors.name = t("policy:required");
    }
    if (database?.enabled) {
      if (!database.name.trim()) {
        nextErrors.databaseName = t("policy:required");
      }
      if (!database.password) {
        nextErrors.databasePassword = t("policy:required");
      }
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const envRows = form.env.filter((row) => row.key.trim());
    setSubmitting(true);
    const done = upgrading ? await handleUpgrade(envRows) : await handleInstall(envRows);
    setSubmitting(false);
    if (done) {
      onInstalled?.();
      onClose?.();
    }
  }

  const engine = database ? DATABASE_ENGINES[database.engine] : null;

  return (
    <FormDialog
      open={open}
      onOpenChange={(next) => (next ? null : onClose())}
      title={upgrading ? app?.name : config?.title || repository}
      description={repository}
      size="lg"
      submitText={upgrading ? t("helm:Upgrade") : t("helm:Install")}
      cancelText={t("general:Cancel")}
      submitting={submitting}
      submitDisabled={loading || !form || Boolean(error)}
      onSubmit={handleSubmit}
    >
      {error ? <MessageAlert title={error} /> : null}
      {loading || !form ? (
        <Loading />
      ) : (
        <>
          <div className="text-muted-foreground flex flex-wrap items-center gap-1.5 text-xs">
            {config.version ? <Badge variant="muted">{config.version}</Badge> : null}
            {config.platform ? <Badge variant="muted">{config.platform}</Badge> : null}
            {config.vendor ? <span className="truncate">{config.vendor}</span> : null}
          </div>
          {config.description ? <p className="text-muted-foreground text-xs leading-relaxed">{config.description}</p> : null}

          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t("image:App name")} htmlFor="image-install-name" required error={errors.name}>
              <Input
                id="image-install-name"
                value={form.name}
                onChange={(event) => updateForm({name: event.target.value})}
                placeholder="my-app"
                disabled={upgrading}
              />
            </Field>
            <Field label={t("general:Namespace")} htmlFor="image-install-namespace">
              {upgrading ? (
                <Input id="image-install-namespace" value={form.namespace} disabled />
              ) : (
                <SimpleSelect
                  id="image-install-namespace"
                  value={form.namespace}
                  onChange={(next) => updateForm({namespace: next})}
                  options={(namespaces ?? []).map((item) => ({label: item.name, value: item.name}))}
                />
              )}
            </Field>
            <Field
              label={t("image:Tag")}
              htmlFor="image-install-tag"
              hint={upgrading ? t("image:Installed tag", {tag: app?.tag ?? ""}) : config.digest ? config.digest.slice(0, 19) : ""}
            >
              <SimpleSelect
                id="image-install-tag"
                value={tag || config.tag}
                onChange={(next) => setTag(next)}
                options={Array.from(new Set([config.tag, ...tags])).map((item) => ({label: item, value: item}))}
              />
            </Field>
            <Field label={t("image:Service type")} htmlFor="image-install-service">
              <SimpleSelect
                id="image-install-service"
                value={form.serviceType}
                onChange={(next) => updateForm({serviceType: next})}
                options={SERVICE_TYPES.map((item) => ({label: item, value: item}))}
              />
            </Field>
          </div>

          <Field label={t("image:Ports")} hint={form.ports.length === 0 ? t("image:This image declares no ports") : ""}>
            <div className="flex flex-wrap gap-3">
              {form.ports.map((port, index) => (
                <label key={port.port} className="flex cursor-pointer items-center gap-2 text-sm">
                  <Checkbox checked={port.expose} onCheckedChange={(checked) => togglePort(index, checked === true)} />
                  <span className="font-mono text-xs">
                    {port.port}/{port.protocol}
                  </span>
                </label>
              ))}
            </div>
          </Field>

          <Field
            label={t("image:Storage")}
            hint={upgrading ? "" : t("image:Prefilled from the paths the image declares as volumes")}
          >
            <DeploymentStorageEditor
              mode={upgrading ? "edit" : "add"}
              value={form.volumes}
              onChange={(next) => updateForm({volumes: next})}
            />
          </Field>

          <Field label={t("image:Environment")}>
            <KeyValueEditor
              value={form.env}
              onChange={(next) => updateForm({env: next})}
              keyPlaceholder="NAME"
              valuePlaceholder="value"
              addLabel={t("image:Add variable")}
            />
            {hiddenEnv.length > 0 ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="justify-self-start"
                onClick={() =>
                  updateForm({env: [...form.env, ...hiddenEnv.map((item) => ({key: item.name, value: item.value}))]})
                }
              >
                {t("image:Show the image's other variables", {count: hiddenEnv.length})}
              </Button>
            ) : null}
          </Field>

          {engine ? (
            <>
              <Separator />
              <Field>
                <label className="flex cursor-pointer items-start gap-2">
                  <Checkbox
                    checked={database.enabled}
                    onCheckedChange={(checked) => toggleDatabase(checked === true)}
                    className="mt-0.5"
                  />
                  <span className="grid gap-1">
                    <span className="flex items-center gap-1.5 text-sm font-medium">
                      <Database className="size-3.5" />
                      {t("image:Create a database for this app", {engine: engine.label})}
                    </span>
                    <span className="text-muted-foreground text-xs">
                      {t("image:Database hint", {host: config.database.hostEnv})}
                    </span>
                  </span>
                </label>
              </Field>

              {database.enabled ? (
                <div className="grid gap-4 sm:grid-cols-3">
                  <Field label={t("image:Database name")} htmlFor="image-db-name" required error={errors.databaseName}>
                    <Input
                      id="image-db-name"
                      value={database.name}
                      onChange={(event) => setDatabase({...database, name: event.target.value})}
                      className="font-mono text-xs"
                    />
                  </Field>
                  <Field label={t("image:Database user")} htmlFor="image-db-user">
                    <Input
                      id="image-db-user"
                      value={database.user}
                      onChange={(event) => setDatabase({...database, user: event.target.value})}
                      className="font-mono text-xs"
                    />
                  </Field>
                  <Field label={t("image:Database password")} htmlFor="image-db-password" error={errors.databasePassword}>
                    <PasswordInput
                      id="image-db-password"
                      value={database.password}
                      onChange={(event) => setDatabase({...database, password: event.target.value})}
                      className="font-mono text-xs"
                    />
                  </Field>
                </div>
              ) : null}

              {database.enabled ? (
                <div className="text-muted-foreground grid gap-0.5 font-mono text-[11px]">
                  {wiredEnvPreview(config.database, sanitizeName(form.name), database).map((row) => (
                    <span key={row.key}>
                      {row.key} = {row.value}
                    </span>
                  ))}
                </div>
              ) : null}
            </>
          ) : null}

          <p className="text-muted-foreground text-xs">
            {t(upgrading ? "image:Upgrade summary" : "image:Install summary", {
              image: config.image,
              namespace: form.namespace,
            })}
          </p>
        </>
      )}
    </FormDialog>
  );
}

export default ImageInstallDialog;
