import * as Setting from "../Setting";

function getHeaders() {
  return {"Accept-Language": Setting.getAcceptLanguage()};
}

export function getMonitorSummary() {
  return fetch(`${Setting.ServerUrl}/api/monitor/summary`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
  }).then(res => res.json());
}

export function getMonitorChecks() {
  return fetch(`${Setting.ServerUrl}/api/monitor/checks`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
  }).then(res => res.json());
}

export function getMonitorEvents(namespace = "", limit = 100) {
  const params = new URLSearchParams({limit});
  if (namespace) {params.set("namespace", namespace);}
  return fetch(`${Setting.ServerUrl}/api/monitor/events?${params}`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
  }).then(res => res.json());
}
