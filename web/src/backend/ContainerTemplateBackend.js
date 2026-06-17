import * as Setting from "../Setting";

export function getContainerTemplates() {
  return fetch(`${Setting.ServerUrl}/api/get-container-templates`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function deployContainerTemplate(templateName, namespace) {
  return fetch(`${Setting.ServerUrl}/api/deploy-container-template`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({templateName, namespace}),
  }).then(res => res.json());
}
