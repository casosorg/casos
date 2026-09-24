import * as Setting from "../Setting";

export function getDevices() {
  return fetch(`${Setting.ServerUrl}/api/get-devices`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}
