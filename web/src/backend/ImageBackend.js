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
