import * as Setting from "../Setting";

function getHeaders() {
  return {"Accept-Language": Setting.getAcceptLanguage()};
}

export function getMonitorSummary() {
  return fetch(`${Setting.ServerUrl}/api/get-monitor-summary`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
  }).then(res => res.json());
}

export function getMonitorChecks() {
  return fetch(`${Setting.ServerUrl}/api/get-monitor-checks`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
  }).then(res => res.json());
}

export function getMonitorOverview() {
  return fetch(`${Setting.ServerUrl}/api/get-monitor-overview`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
  }).then(res => res.json());
}

export function getMonitorEvents(namespace = "", limit = 100) {
  const params = new URLSearchParams({limit});
  if (namespace) {params.set("namespace", namespace);}
  return fetch(`${Setting.ServerUrl}/api/get-monitor-events?${params}`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
  }).then(res => res.json());
}

export function getMonitorIssues() {
  return fetch(`${Setting.ServerUrl}/api/get-monitor-issues`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
  }).then(res => res.json());
}

export function getMonitorDiagnosis(issue, tailLines = 100, previous = true) {
  const params = new URLSearchParams({
    kind: issue.kind || "",
    name: issue.name || "",
    tailLines,
    previous,
  });
  if (issue.namespace) {params.set("namespace", issue.namespace);}
  return fetch(`${Setting.ServerUrl}/api/get-monitor-diagnosis?${params}`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
  }).then(res => res.json());
}
