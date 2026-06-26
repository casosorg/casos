import {Tooltip, message, theme} from "antd";
import * as Conf from "./Conf";
import {QuestionCircleOutlined} from "@ant-design/icons";
import React from "react";
import Sdk from "casdoor-js-sdk";
import i18next from "i18next";

export let ServerUrl = "";
export let CasdoorSdk;

export function initServerUrl() {
  const fullServerUrl = window.location.origin;
  if (fullServerUrl === "http://localhost:8001") {
    ServerUrl = "http://localhost:9000";
  }
}

export function initWebConfig() {
  const cookies = Object.fromEntries(
    document.cookie.split("; ").filter(Boolean).map(c => {
      const idx = c.indexOf("=");
      return [c.slice(0, idx), c.slice(idx + 1)];
    })
  );
  if (cookies["jsonWebConfig"] && cookies["jsonWebConfig"] !== "null") {
    try {
      const decoded = decodeURIComponent(cookies["jsonWebConfig"].replace(/\+/g, " "));
      const config = JSON.parse(decoded);
      Conf.setConfig(config);
    } catch (_) {
      // malformed cookie — proceed with defaults
    }
  }
}

export function initCasdoorSdk(config) {
  CasdoorSdk = new Sdk(config);
}

export function getWebSocketUrl(path, params = {}) {
  const baseUrl = ServerUrl || window.location.origin;
  const url = new URL(path, baseUrl);
  Object.entries(params).forEach(([key, value]) => {
    if (value !== null && value !== undefined) {
      url.searchParams.set(key, value);
    }
  });
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url.toString();
}

export function getSigninUrl() {
  return CasdoorSdk.getSigninUrl();
}

export function getSignupUrl() {
  return CasdoorSdk.getSignupUrl();
}

export function getUserProfileUrl(userName, account) {
  return CasdoorSdk.getUserProfileUrl(userName, account);
}

export function getMyProfileUrl(account) {
  return CasdoorSdk.getMyProfileUrl(account);
}

export function signin() {
  return CasdoorSdk.signin(ServerUrl);
}

export function goToLink(link) {
  window.location.href = link;
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

export function getLanguage() {
  return i18next.language;
}

export function setLanguage(language) {
  localStorage.setItem("language", language);
  i18next.changeLanguage(language);
}

export function getAcceptLanguage() {
  return getLanguage() || "en";
}

export const Countries = [
  {key: "en", label: "English", country: "US", alt: "English"},
  {key: "zh", label: "中文", country: "CN", alt: "中文"},
];

export function isMobile() {
  return window.innerWidth < 768;
}

export function deepCopy(obj) {
  return JSON.parse(JSON.stringify(obj));
}

export function handleFetchResponse(response) {
  const contentType = response.headers.get("content-type");
  if (contentType && contentType.indexOf("application/json") !== -1) {
    return response.json();
  }
  return response.text().then(text => ({status: "error", msg: text}));
}

export function isAdminUser(account) {
  if (!account) {return false;}
  return true;
}

export function getAvatarColor(s) {
  const colorList = ["#f56a00", "#7265e6", "#ffbf00", "#00a2ae"];
  let hash = 0;
  for (let i = 0; i < (s || "").length; i++) {
    const c = s.charCodeAt(i);
    hash = ((hash << 5) - hash) + c;
    hash = hash & hash;
  }
  return colorList[Math.abs(hash) % 4];
}

export function getShortName(s) {
  return (s || "").charAt(0).toUpperCase();
}

export function setThemeColor(color) {
  if (!color) {return;}
  localStorage.setItem("themeColor", color);
}

export function getThemeColor() {
  return localStorage.getItem("themeColor") || "#404040";
}

export function getAlgorithm(themeAlgorithmNames) {
  return (themeAlgorithmNames || ["default"]).sort().reverse().map((algorithmName) => {
    if (algorithmName === "dark") {return theme.darkAlgorithm;}
    if (algorithmName === "compact") {return theme.compactAlgorithm;}
    return theme.defaultAlgorithm;
  });
}

export function getLogo(themes, storeLogoUrl) {
  const defaultLogoUrl = "https://cdn.casvisor.com/casdoor/resource/built-in/admin/casos-logo_2000x500.png";
  const logoUrl = storeLogoUrl || defaultLogoUrl;
  if (Array.isArray(themes) && themes.includes("dark")) {
    return logoUrl.replace(/\.png$/, "_white.png");
  }
  return logoUrl;
}

export function getFooterHtml(themes, storeFooterHtml, site) {
  const logoUrl = getLogo([], site?.logoUrl);
  const defaultFooterHtml = `<a target="_blank" href="https://github.com/casosorg/casos" rel="noreferrer"><img style="padding-bottom: 3px;" height="30" alt="CasOS" src="${logoUrl}" /></a>`;
  const footerHtml = storeFooterHtml || defaultFooterHtml;
  if (Array.isArray(themes) && themes.includes("dark")) {
    return footerHtml.replace(/(\.png)/g, "_white$1");
  }
  return footerHtml;
}

export function getFaviconUrl(themes, storeFaviconUrl) {
  const defaultFaviconUrl = "https://cdn.casvisor.com/casdoor/resource/built-in/admin/casos-logo.png";
  const faviconUrl = storeFaviconUrl || defaultFaviconUrl;
  if (Array.isArray(themes) && themes.includes("dark")) {
    return faviconUrl.replace(/\.png$/, "_white.png");
  }
  return faviconUrl;
}

export function getHtmlTitle(siteHtmlTitle) {
  return siteHtmlTitle || "CasOS";
}

export function getNavbarHtml(themes, storeNavbarHtml) {
  const navbarHtml = storeNavbarHtml || "";
  if (Array.isArray(themes) && themes.includes("dark")) {
    return navbarHtml.replace(/(\.png)/g, "_white$1");
  }
  return navbarHtml;
}

export function getLabel(text, tooltip) {
  return (
    <React.Fragment>
      <span style={{marginRight: 4}}>{text}</span>
      <Tooltip placement="top" title={tooltip}>
        <QuestionCircleOutlined style={{color: "var(--ant-color-text-secondary)"}} />
      </Tooltip>
    </React.Fragment>
  );
}

export function getFormattedDate(dateStr) {
  if (!dateStr) {return "";}
  return new Date(dateStr).toLocaleDateString();
}

export function getRandomName() {
  return Math.random().toString(36).substring(2, 8);
}

export function isResponseDenied(data) {
  return data.msg === "Unauthorized operation" || data.msg === "this operation requires admin privilege";
}
