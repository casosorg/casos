import * as Setting from "../Setting";

export function getPvcs(namespace = "") {
  return fetch(`${Setting.ServerUrl}/api/get-pvcs?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addPvc(pvc) {
  return fetch(`${Setting.ServerUrl}/api/add-pvc`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(pvc),
  }).then(res => res.json());
}

export function deletePvc(namespace, name) {
  return fetch(`${Setting.ServerUrl}/api/delete-pvc`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
