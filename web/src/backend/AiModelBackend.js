import * as Setting from "../Setting";

export function getAiModels() {
  return fetch(`${Setting.ServerUrl}/api/get-ai-models`, {
    credentials: "include", headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(r => r.json());
}

export function getAiProviders() {
  return fetch(`${Setting.ServerUrl}/api/get-ai-providers`, {
    credentials: "include", headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(r => r.json());
}

export function saveAiProvider(provider) {
  return fetch(`${Setting.ServerUrl}/api/save-ai-provider`, {
    method: "POST", credentials: "include", headers: {"Content-Type": "application/json"}, body: JSON.stringify(provider),
  }).then(r => r.json());
}

export function deleteAiProvider(name) {
  return fetch(`${Setting.ServerUrl}/api/delete-ai-provider`, {
    method: "POST", credentials: "include", headers: {"Content-Type": "application/json"}, body: JSON.stringify({name}),
  }).then(r => r.json());
}
