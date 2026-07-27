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

export function getMonitorMetrics(query, signal) {
  const params = new URLSearchParams();
  ["scope", "metric", "namespace", "name", "start", "end", "step"].forEach(key => {
    if (query[key] !== undefined && query[key] !== null && query[key] !== "") {
      params.set(key, query[key]);
    }
  });
  return fetch(`${Setting.ServerUrl}/api/get-monitor-metrics?${params}`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
    ...(signal ? {signal} : {}),
  }).then(res => res.json());
}

function buildMonitorOverviewParams(query) {
  const params = new URLSearchParams();
  ["kind", "namespace", "name", "mode", "start", "end", "step", "podLimit"].forEach(key => {
    if (query[key] !== undefined && query[key] !== null && query[key] !== "") {
      params.set(key, query[key]);
    }
  });
  if (query.selectedObjects?.length) {
    params.set("selectedPods", query.selectedObjects.join(","));
  }
  return params;
}

function getMonitorOverviewApi(path, query, signal) {
  const params = buildMonitorOverviewParams(query || {});
  return fetch(`${Setting.ServerUrl}${path}?${params}`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
    ...(signal ? {signal} : {}),
  }).then(res => res.json());
}

export function getNodeMonitorOverview(name, query, signal) {
  return getMonitorOverviewApi("/api/get-node-monitor-overview", {...query, name}, signal);
}

export function getPodMonitorOverview(namespace, name, query, signal) {
  return getMonitorOverviewApi("/api/get-pod-monitor-overview", {...query, namespace, name}, signal);
}

export function getWorkloadMonitorOverview(kind, namespace, name, query, signal) {
  return getMonitorOverviewApi("/api/get-workload-monitor-overview", {...query, kind, namespace, name}, signal);
}

export function getPvcMonitorOverview(namespace, name, query, signal) {
  return getMonitorOverviewApi("/api/get-pvc-monitor-overview", {...query, namespace, name}, signal);
}

export function getMonitorTop(resource, metric, limit = 5, namespace = "", signal) {
  const params = new URLSearchParams({resource, metric, limit});
  if (namespace) {params.set("namespace", namespace);}
  return fetch(`${Setting.ServerUrl}/api/get-monitor-top?${params}`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
    ...(signal ? {signal} : {}),
  }).then(res => res.json());
}

export function getMonitorResourceInventory(signal) {
  return fetch(`${Setting.ServerUrl}/api/get-monitor-resource-inventory`, {
    method: "GET",
    credentials: "include",
    headers: getHeaders(),
    ...(signal ? {signal} : {}),
  }).then(res => res.json());
}

export function getMonitorResourceEvents(kind, namespace, name, limit = 100) {
  const params = new URLSearchParams({kind, name, limit});
  if (namespace) {params.set("namespace", namespace);}
  return fetch(`${Setting.ServerUrl}/api/get-monitor-resource-events?${params}`, {
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
