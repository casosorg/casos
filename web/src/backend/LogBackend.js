import * as Setting from "../Setting";

export function getAggregatedLogs(namespace, deployment, keyword = "", tailLines = 200) {
  const params = new URLSearchParams({namespace, deployment, keyword, tailLines});
  return fetch(`${Setting.ServerUrl}/api/get-aggregated-logs?${params}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}
