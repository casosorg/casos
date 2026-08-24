import * as Setting from "../Setting";

const lang = () => ({"Accept-Language": Setting.getAcceptLanguage()});
const jsonHeaders = () => ({"Content-Type": "application/json", ...lang()});

function post(path, payload) {
  return fetch(`${Setting.ServerUrl}${path}`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(payload),
  }).then(r => r.json());
}

export function getDatabaseEngines() {
  return fetch(`${Setting.ServerUrl}/api/get-database-engines`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function getDatabases(namespace = "") {
  const params = new URLSearchParams();
  if (namespace) {params.set("namespace", namespace);}
  return fetch(`${Setting.ServerUrl}/api/get-databases?${params}`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function getDatabase(namespace, name) {
  const params = new URLSearchParams({namespace, name});
  return fetch(`${Setting.ServerUrl}/api/get-database?${params}`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function createDatabase(payload) {
  return post("/api/create-database", payload);
}

export function updateDatabase(payload) {
  return post("/api/update-database", payload);
}

export function scaleDatabase(payload) {
  return post("/api/scale-database", payload);
}

export function deleteDatabase(payload) {
  return post("/api/delete-database", payload);
}

export function backupDatabase(payload) {
  return post("/api/backup-database", payload);
}

export function restoreDatabase(payload) {
  return post("/api/restore-database", payload);
}

export function deleteDatabaseBackup(payload) {
  return post("/api/delete-database-backup", payload);
}

/** Backups live in the database pod, so downloading one is a pod file read. */
export function backupDownloadUrl(namespace, pod, file) {
  const params = new URLSearchParams({
    namespace,
    name: pod,
    container: "database",
    path: `/backups/${file}`,
  });
  return `${Setting.ServerUrl}/api/pod-file-download?${params}`;
}
