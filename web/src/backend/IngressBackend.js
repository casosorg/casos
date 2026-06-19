import * as Setting from "../Setting";

export function getIngresses(namespace = "") {
  return fetch(`${Setting.ServerUrl}/api/get-ingresses?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addIngress(ingress) {
  return fetch(`${Setting.ServerUrl}/api/add-ingress`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(ingress),
  }).then(res => res.json());
}

export function updateIngress(ingress) {
  return fetch(`${Setting.ServerUrl}/api/update-ingress`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(ingress),
  }).then(res => res.json());
}

export function deleteIngress(namespace, name) {
  return fetch(`${Setting.ServerUrl}/api/delete-ingress`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}

export function getIngressCertStatus(namespace, name) {
  return fetch(
    `${Setting.ServerUrl}/api/get-ingress-cert-status?namespace=${encodeURIComponent(namespace)}&name=${encodeURIComponent(name)}`,
    {
      method: "GET",
      credentials: "include",
      headers: {"Accept-Language": Setting.getAcceptLanguage()},
    }
  ).then(res => res.json());
}
