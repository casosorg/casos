import * as Setting from "../Setting";

const lang = () => ({"Accept-Language": Setting.getAcceptLanguage()});
const jsonHeaders = () => ({"Content-Type": "application/json", ...lang()});

function post(path, payload) {
  return fetch(`${Setting.ServerUrl}${path}`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(payload),
  }).then(r => r.json());
}

function language() {
  return Setting.getLanguage?.() ?? "en";
}

export function getTemplates({search = "", category = ""} = {}) {
  const params = new URLSearchParams({language: language()});
  if (search) {params.set("search", search);}
  if (category && category !== "all") {params.set("category", category);}
  return fetch(`${Setting.ServerUrl}/api/get-templates?${params}`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function getTemplate(name, namespace = "default") {
  const params = new URLSearchParams({name, namespace, language: language()});
  return fetch(`${Setting.ServerUrl}/api/get-template?${params}`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function syncTemplates() {
  return post("/api/sync-templates", {});
}

export function previewTemplate(payload) {
  return post("/api/preview-template", payload);
}

export function deployTemplate(payload) {
  return post("/api/deploy-template", payload);
}

export function getTemplateInstances(namespace = "") {
  const params = new URLSearchParams();
  if (namespace) {params.set("namespace", namespace);}
  return fetch(`${Setting.ServerUrl}/api/get-template-instances?${params}`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function getTemplateInstance(namespace, name) {
  const params = new URLSearchParams({namespace, name});
  return fetch(`${Setting.ServerUrl}/api/get-template-instance?${params}`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function deleteTemplateInstance(payload) {
  return post("/api/delete-template-instance", payload);
}
