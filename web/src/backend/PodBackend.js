import * as Setting from "../Setting";

export function getPods(namespace = "") {
  return fetch(`${Setting.ServerUrl}/api/get-pods?namespace=${encodeURIComponent(namespace)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addPod(pod) {
  return fetch(`${Setting.ServerUrl}/api/add-pod`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(pod),
  }).then(res => res.json());
}

export function updatePod(pod) {
  return fetch(`${Setting.ServerUrl}/api/update-pod`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(pod),
  }).then(res => res.json());
}

export function getPodEvents(namespace, name) {
  return fetch(`${Setting.ServerUrl}/api/get-pod-events?namespace=${encodeURIComponent(namespace)}&name=${encodeURIComponent(name)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function getPodLogs(namespace, name, tailLines = 500) {
  return fetch(`${Setting.ServerUrl}/api/get-pod-logs?namespace=${encodeURIComponent(namespace)}&name=${encodeURIComponent(name)}&tailLines=${tailLines}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function searchDockerHubImages(q) {
  return fetch(`${Setting.ServerUrl}/api/search-docker-hub-images?q=${encodeURIComponent(q)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function deletePod(namespace, name) {
  return fetch(`${Setting.ServerUrl}/api/delete-pod`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name}),
  }).then(res => res.json());
}

export function openPodUI(namespace, name, containerPort) {
  return fetch(`${Setting.ServerUrl}/api/open-pod-ui`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({namespace, name, containerPort}),
  }).then(res => res.json());
}
