import * as Setting from "../Setting";

export function getStorageClasses() {
  return fetch(`${Setting.ServerUrl}/api/get-storageclasses`, {
    method: "GET",
    credentials: "include",
    headers: {"Accept-Language": Setting.getAcceptLanguage()},
  }).then(res => res.json());
}

export function addStorageClass(storageClass) {
  return fetch(`${Setting.ServerUrl}/api/add-storageclass`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(storageClass),
  }).then(res => res.json());
}

export function updateStorageClass(storageClass) {
  return fetch(`${Setting.ServerUrl}/api/update-storageclass`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify(storageClass),
  }).then(res => res.json());
}

export function deleteStorageClass(name) {
  return fetch(`${Setting.ServerUrl}/api/delete-storageclass`, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "Accept-Language": Setting.getAcceptLanguage(),
    },
    body: JSON.stringify({name}),
  }).then(res => res.json());
}
