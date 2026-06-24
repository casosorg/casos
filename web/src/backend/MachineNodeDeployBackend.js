import * as Setting from "../Setting";

export function preflightMachineNode(request) {
  return fetch(`${Setting.ServerUrl}/api/preflight-machine-node`, {
    method: "POST",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
    body: JSON.stringify(request),
  }).then(res => Setting.handleFetchResponse(res));
}

export function deployMachineNode(request) {
  return fetch(`${Setting.ServerUrl}/api/deploy-machine-node`, {
    method: "POST",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
    body: JSON.stringify(request),
  }).then(res => Setting.handleFetchResponse(res));
}

export function repairMachineNode(request) {
  return fetch(`${Setting.ServerUrl}/api/repair-machine-node`, {
    method: "POST",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
    body: JSON.stringify(request),
  }).then(res => Setting.handleFetchResponse(res));
}

export function getMachineNodeTasks(owner, machineName) {
  return fetch(`${Setting.ServerUrl}/api/get-machine-node-tasks?owner=${encodeURIComponent(owner)}&machineName=${encodeURIComponent(machineName)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => Setting.handleFetchResponse(res));
}

export function getMachineNodeLogs(taskId) {
  return fetch(`${Setting.ServerUrl}/api/get-machine-node-logs?taskId=${encodeURIComponent(taskId)}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => Setting.handleFetchResponse(res));
}
