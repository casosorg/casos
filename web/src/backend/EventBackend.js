import * as Setting from "../Setting";

export function getEvents(namespace = "all", type = "", limit = 200) {
  const params = new URLSearchParams({namespace, limit: String(limit)});
  if (type) {
    params.set("type", type);
  }
  return fetch(`${Setting.ServerUrl}/api/get-events?${params}`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}
