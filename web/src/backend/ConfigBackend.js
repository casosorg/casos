import * as Setting from "../Setting";

export function getApplicationConfig() {
  return fetch(`${Setting.ServerUrl}/api/get-application-config`, {
    method: "GET",
    credentials: "include",
  }).then(res => Setting.handleFetchResponse(res));
}
