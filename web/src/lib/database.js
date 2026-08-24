/**
 * What the database pages agree on: how an engine is coloured, and the shape of
 * the form both creating and editing one uses.
 *
 * The engine catalogue itself comes from the backend (`/api/get-database-engines`)
 * — it is the side that has to honour the choice, so it is the side that says
 * which engines and versions exist.
 */

const ENGINE_TINTS = {
  postgresql: "bg-gradient-to-br from-sky-400 to-blue-600",
  mysql: "bg-gradient-to-br from-orange-400 to-amber-600",
  mongodb: "bg-gradient-to-br from-emerald-400 to-green-600",
  redis: "bg-gradient-to-br from-rose-400 to-red-600",
};

export function engineTint(engine) {
  return ENGINE_TINTS[engine] ?? "bg-gradient-to-br from-zinc-400 to-zinc-600";
}

export const DATABASE_STATUS_VARIANTS = {
  running: "success",
  pending: "warning",
  stopped: "muted",
  failed: "danger",
};

export const STORAGE_PRESETS = ["1Gi", "5Gi", "10Gi", "20Gi", "50Gi", "100Gi"];

export function emptyDatabaseForm(namespace = "default") {
  return {
    namespace,
    name: "",
    engine: "postgresql",
    version: "",
    user: "",
    database: "",
    password: "",
    cpuLimit: "500m",
    memoryLimit: "1Gi",
    storage: "5Gi",
    publicAccess: false,
  };
}

export function formFromDatabase(detail) {
  return {
    namespace: detail.namespace,
    name: detail.name,
    engine: detail.engine,
    version: detail.version ?? "",
    user: detail.user ?? "",
    database: detail.database ?? "",
    password: "",
    cpuLimit: detail.cpuLimit ?? "",
    memoryLimit: detail.memoryLimit ?? "",
    storage: detail.storage ?? "",
    publicAccess: Boolean(detail.publicAccess),
  };
}

export function databasePayload(form, {mode = "create"} = {}) {
  const payload = {
    namespace: form.namespace || "default",
    name: form.name.trim(),
    engine: form.engine,
    version: form.version,
    cpuLimit: form.cpuLimit?.trim() ?? "",
    memoryLimit: form.memoryLimit?.trim() ?? "",
    storage: form.storage?.trim() ?? "",
    publicAccess: form.publicAccess,
  };
  // Credentials are set once. Changing them later would leave the engine's own
  // stored user out of step with the Secret, which is worse than not offering.
  if (mode === "create") {
    payload.user = form.user.trim();
    payload.database = form.database.trim();
    payload.password = form.password;
  }
  return payload;
}

export function validateDatabaseForm(form) {
  const errors = {};
  const name = form.name.trim();
  if (!name) {
    errors.name = "required";
  } else if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(name)) {
    errors.name = "invalid";
  }
  if (form.storage && !/^\d+(\.\d+)?(Ki|Mi|Gi|Ti)?$/.test(form.storage.trim())) {
    errors.storage = "invalid";
  }
  return errors;
}
