import * as Setting from "../Setting";

const lang = () => ({"Accept-Language": Setting.getAcceptLanguage()});
const jsonHeaders = () => ({"Content-Type": "application/json", ...lang()});

export function deployDevbox(payload) {
  return fetch(`${Setting.ServerUrl}/api/deploy-devbox`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(payload),
  }).then(r => r.json());
}

export function getDevboxes(namespace = "") {
  const params = new URLSearchParams();
  if (namespace) {params.set("namespace", namespace);}
  return fetch(`${Setting.ServerUrl}/api/get-devboxes?${params}`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function freezeDevbox(payload) {
  return fetch(`${Setting.ServerUrl}/api/freeze-devbox`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(payload),
  }).then(r => r.json());
}

export function getDevboxRuns(namespace = "", devbox = "") {
  const params = new URLSearchParams();
  if (namespace) {params.set("namespace", namespace);}
  if (devbox) {params.set("devbox", devbox);}
  return fetch(`${Setting.ServerUrl}/api/get-devbox-runs?${params}`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function cancelDevboxRun(payload) {
  return fetch(`${Setting.ServerUrl}/api/cancel-devbox-run`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(payload),
  }).then(r => r.json());
}
