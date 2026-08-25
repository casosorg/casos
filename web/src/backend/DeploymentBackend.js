import * as Setting from "../Setting";

export function getDeployments(namespace = "") {
  return fetch(`${Setting.ServerUrl}/api/get-deployments?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addDeployment(deployment) {
  return fetch(`${Setting.ServerUrl}/api/add-deployment`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(deployment),
  }).then(res => res.json());
}

export function updateDeployment(deployment) {
  return fetch(`${Setting.ServerUrl}/api/update-deployment`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(deployment),
  }).then(res => res.json());
}

export function restartDeployment(namespace, name) {
  return fetch(`${Setting.ServerUrl}/api/restart-deployment`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}

export function deleteDeployment(namespace, name) {
  return fetch(`${Setting.ServerUrl}/api/delete-deployment`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}

export function getDeploymentRevisions(namespace, name) {
  const params = new URLSearchParams({namespace, name});
  return fetch(`${Setting.ServerUrl}/api/get-deployment-revisions?${params}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function rollbackDeployment(payload) {
  return fetch(`${Setting.ServerUrl}/api/rollback-deployment`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(payload),
  }).then(res => res.json());
}
