import * as Setting from "../Setting";

export function getStatefulSets(namespace = "") {
  return fetch(`${Setting.ServerUrl}/api/get-statefulsets?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function getStatefulSet(namespace, name) {
  const params = new URLSearchParams({namespace, name});
  return fetch(`${Setting.ServerUrl}/api/get-statefulset?${params}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addStatefulSet(statefulset) {
  return fetch(`${Setting.ServerUrl}/api/add-statefulset`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(statefulset),
  }).then(res => res.json());
}

export function updateStatefulSet(statefulset) {
  return fetch(`${Setting.ServerUrl}/api/update-statefulset`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(statefulset),
  }).then(res => res.json());
}

export function deleteStatefulSet(namespace, name) {
  return fetch(`${Setting.ServerUrl}/api/delete-statefulset`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
