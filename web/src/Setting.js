import {message} from "antd";

export let ServerUrl = "";

export function initServerUrl() {
  const fullServerUrl = window.location.origin;
  if (fullServerUrl === "http://localhost:8001") {
    ServerUrl = "http://localhost:9000";
  }
}

export function showMessage(type, msg) {
  if (type === "success") {
    message.success(msg);
  } else if (type === "error") {
    message.error(msg);
  } else if (type === "info") {
    message.info(msg);
  }
}

export function getItem(label, key, icon, children) {
  return {key, icon, children, label};
}

export function getAcceptLanguage() {
  return "en";
}

export function isMobile() {
  return window.innerWidth < 768;
}
