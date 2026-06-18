import * as Setting from "../Setting";

export function getCasbinRules(scope) {
  return fetch(`${Setting.ServerUrl}/api/get-casbin-rules?scope=${encodeURIComponent(scope)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addCasbinRule(rule) {
  return fetch(`${Setting.ServerUrl}/api/add-casbin-rule`, {
    method: "POST",
    credentials: "include",
    headers: {"Content-Type": "application/json", "Accept-Language": Setting.getAcceptLanguage()},
    body: JSON.stringify(rule),
  }).then(res => res.json());
}

export function deleteCasbinRule(id, scope) {
  return fetch(`${Setting.ServerUrl}/api/delete-casbin-rule`, {
    method: "POST",
    credentials: "include",
    headers: {"Content-Type": "application/json", "Accept-Language": Setting.getAcceptLanguage()},
    body: JSON.stringify({id: String(id), scope}),
  }).then(res => res.json());
}

export function reloadCasbinEnforcer(scope) {
  return fetch(`${Setting.ServerUrl}/api/reload-casbin-enforcer`, {
    method: "POST",
    credentials: "include",
    headers: {"Content-Type": "application/json", "Accept-Language": Setting.getAcceptLanguage()},
    body: JSON.stringify({scope: scope || ""}),
  }).then(res => res.json());
}
