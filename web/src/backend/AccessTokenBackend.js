import * as Setting from "../Setting";

const lang = () => ({"Accept-Language": Setting.getAcceptLanguage()});
const jsonHeaders = () => ({"Content-Type": "application/json", ...lang()});

export function getAccessTokens() {
  return fetch(`${Setting.ServerUrl}/api/get-access-tokens`, {
    credentials: "include", headers: lang(),
  }).then(r => r.json());
}

export function addAccessToken(name, readOnly) {
  return fetch(`${Setting.ServerUrl}/api/add-access-token`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify({name, readOnly}),
  }).then(r => r.json());
}

export function deleteAccessToken(name) {
  return fetch(`${Setting.ServerUrl}/api/delete-access-token`, {
    method: "POST", credentials: "include", headers: jsonHeaders(), body: JSON.stringify({name}),
  }).then(r => r.json());
}
