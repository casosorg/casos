import * as Setting from "../Setting";

export function getDaemonSets(namespace = "") {
  return fetch(`${Setting.ServerUrl}/api/get-daemonsets?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function getDaemonSet(namespace, name) {
  const params = new URLSearchParams({namespace, name});
  return fetch(`${Setting.ServerUrl}/api/get-daemonset?${params}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addDaemonSet(daemonset) {
  return fetch(`${Setting.ServerUrl}/api/add-daemonset`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(daemonset),
  }).then(res => res.json());
}

export function updateDaemonSet(daemonset) {
  return fetch(`${Setting.ServerUrl}/api/update-daemonset`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(daemonset),
  }).then(res => res.json());
}

export function deleteDaemonSet(namespace, name) {
  return fetch(`${Setting.ServerUrl}/api/delete-daemonset`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}
