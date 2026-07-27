import * as Setting from "../Setting";

export function getNodes() {
  return fetch(`${Setting.ServerUrl}/api/get-nodes`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function getNode(name) {
  return fetch(`${Setting.ServerUrl}/api/get-node?name=${encodeURIComponent(name)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function updateNode(node) {
  return fetch(`${Setting.ServerUrl}/api/update-node`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(node),
  }).then(res => res.json());
}

export function deleteNode(name) {
  return fetch(`${Setting.ServerUrl}/api/delete-node`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({name}),
  }).then(res => res.json());
}

export function getWorkerKubeconfig(nodeName) {
  return fetch(`${Setting.ServerUrl}/api/get-worker-kubeconfig?nodeName=${encodeURIComponent(nodeName)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}
