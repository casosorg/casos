import * as Setting from "../Setting";

export function getAppTemplates() {
  return fetch(`${Setting.ServerUrl}/api/get-app-templates`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function deployApp(req) {
  return fetch(`${Setting.ServerUrl}/api/deploy-app`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(req),
  }).then(res => res.json());
}

export function searchHelmCharts(q, page, pageSize) {
  const params = new URLSearchParams({q: q ?? "", page: String(page ?? 1), pageSize: String(pageSize ?? 24)});
  return fetch(`${Setting.ServerUrl}/api/search-helm-charts?${params.toString()}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function getHelmRepoCharts(repoUrl) {
  const params = new URLSearchParams({repoUrl});
  return fetch(`${Setting.ServerUrl}/api/get-helm-repo-charts?${params.toString()}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function getHelmChartInfo(req) {
  const params = new URLSearchParams({
    repoUrl: req.repoUrl ?? "",
    chart: req.chart ?? "",
    version: req.version ?? "",
    source: req.source ?? "",
    repoName: req.repoName ?? "",
  });
  return fetch(`${Setting.ServerUrl}/api/get-helm-chart-info?${params.toString()}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function installHelmChart(req) {
  return fetch(`${Setting.ServerUrl}/api/install-helm-chart`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(req),
  }).then(res => res.json());
}

export function upgradeHelmRelease(req) {
  return fetch(`${Setting.ServerUrl}/api/upgrade-helm-release`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(req),
  }).then(res => res.json());
}

export function uninstallHelmRelease(req) {
  return fetch(`${Setting.ServerUrl}/api/uninstall-helm-release`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(req),
  }).then(res => res.json());
}

export function getHelmTask(id) {
  const params = new URLSearchParams({id});
  return fetch(`${Setting.ServerUrl}/api/get-helm-task?${params.toString()}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function getHelmReleases(namespace) {
  const params = new URLSearchParams({namespace: namespace ?? ""});
  return fetch(`${Setting.ServerUrl}/api/get-helm-releases?${params.toString()}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}
