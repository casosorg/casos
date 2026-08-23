import * as Setting from "../Setting";

const lang = () => ({"Accept-Language": Setting.getAcceptLanguage()});
const jsonHeaders = () => ({"Content-Type": "application/json", ...lang()});

export function getImageConfig(image, platform = "") {
  const params = new URLSearchParams({image});
  if (platform) {params.set("platform", platform);}
  return fetch(`${Setting.ServerUrl}/api/get-image-config?${params}`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function deployApp(payload) {
  return fetch(`${Setting.ServerUrl}/api/deploy-app`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(payload),
  }).then(r => r.json());
}

export function getImageApps(namespace = "") {
  const params = new URLSearchParams();
  if (namespace) {params.set("namespace", namespace);}
  return fetch(`${Setting.ServerUrl}/api/get-image-apps?${params}`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function upgradeApp(payload) {
  return fetch(`${Setting.ServerUrl}/api/upgrade-image-app`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(payload),
  }).then(r => r.json());
}

export function scaleApp(payload) {
  return fetch(`${Setting.ServerUrl}/api/scale-image-app`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(payload),
  }).then(r => r.json());
}

export function uninstallApp(payload) {
  return fetch(`${Setting.ServerUrl}/api/uninstall-image-app`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify(payload),
  }).then(r => r.json());
}
