import * as Setting from "../Setting";

const requestTimeout = 15000;

function request(url, options) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), requestTimeout);
  return fetch(url, {...options, signal: controller.signal})
    .catch((error) => {
      if (error.name === "AbortError") {throw new Error("Request timed out");}
      throw error;
    })
    .finally(() => clearTimeout(timer));
}

export function getAccount() {
  return request(`${Setting.ServerUrl}/api/get-account`, {
    method: "GET",
    credentials: "include",
  }).then(res => Setting.handleFetchResponse(res));
}

export function getSigninOptions() {
  return request(`${Setting.ServerUrl}/api/get-signin-options`, {
    method: "GET",
    credentials: "include",
  }).then(res => Setting.handleFetchResponse(res));
}

export function signin(code, state) {
  return request(`${Setting.ServerUrl}/api/signin?code=${code}&state=${state}`, {
    method: "POST",
    credentials: "include",
  }).then(res => Setting.handleFetchResponse(res));
}

export function signinWithPassword(username, password) {
  return request(`${Setting.ServerUrl}/api/signin`, {
    method: "POST",
    credentials: "include",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({username, password}),
  }).then(res => Setting.handleFetchResponse(res));
}

export function setup(setupToken, password) {
  return request(`${Setting.ServerUrl}/api/setup`, {
    method: "POST",
    credentials: "include",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({setupToken, password}),
  }).then(res => Setting.handleFetchResponse(res));
}

export function updateAccount(currentPassword, newPassword) {
  return request(`${Setting.ServerUrl}/api/update-account`, {
    method: "POST",
    credentials: "include",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({currentPassword, newPassword}),
  }).then(res => Setting.handleFetchResponse(res));
}

export function signout() {
  return request(`${Setting.ServerUrl}/api/signout`, {
    method: "POST",
    credentials: "include",
  }).then(res => Setting.handleFetchResponse(res));
}
