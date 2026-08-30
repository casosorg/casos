import Sdk from "casdoor-js-sdk";
import i18next from "i18next";
import {toast} from "sonner";

// The Vite dev server proxies /api, /k8s and /.well-known to the Go backend, and
// in production the backend serves this bundle itself. So a relative URL is
// always correct and ServerUrl stays empty unless a build explicitly points the
// frontend somewhere else.
export let ServerUrl = import.meta.env.VITE_SERVER_URL ?? "";
export let CasdoorSdk;

export function initServerUrl() {
  ServerUrl = import.meta.env.VITE_SERVER_URL ?? "";
}

export function initCasdoorSdk(config) {
  CasdoorSdk = new Sdk({
    serverUrl: config.serverUrl,
    clientId: config.clientId,
    appName: config.appName || "",
    organizationName: config.organizationName || "",
    redirectPath: config.redirectPath || "/callback",
  });
}

export function isCasdoorAvailable() {
  return CasdoorSdk !== undefined;
}

export function isBasicLoginMode(account) {
  if (account === undefined || account === null) {
    return false;
  }
  return account.owner === "basic";
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
  if (!isCasdoorAvailable()) {
    return "";
  }
  return CasdoorSdk.getSigninUrl();
}

export function getSignupUrl() {
  if (!isCasdoorAvailable()) {
    return "";
  }
  return CasdoorSdk.getSignupUrl();
}

export function getUserProfileUrl(userName, account) {
  if (!isCasdoorAvailable() || isBasicLoginMode(account)) {
    return "";
  }
  return CasdoorSdk.getUserProfileUrl(userName, account);
}

export function getMyProfileUrl(account) {
  if (!isCasdoorAvailable() || isBasicLoginMode(account)) {
    return "";
  }
  return CasdoorSdk.getMyProfileUrl(account);
}

export function signin() {
  return CasdoorSdk.signin(ServerUrl);
}

export function goToLink(link) {
  window.location.href = link;
}

// showMessage keeps the call sites of the old antd `message` API working while
// the toast itself is sonner, so pages do not each grow their own notification
// style.
export function showMessage(type, msg) {
  if (!msg) {
    return;
  }
  if (type === "success") {
    toast.success(msg);
  } else if (type === "error") {
    toast.error(msg);
  } else if (type === "warning") {
    toast.warning(msg);
  } else {
    toast.info(msg);
  }
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

// `brand` alone tints nothing, so the accent is pushed onto the shadcn tokens
// that buttons, active sidebar rows and focus rings actually paint with. Inline
// on <html> it also outranks the `.dark` block, so one value covers both themes.
const THEME_COLOR_TOKENS = ["--brand", "--primary", "--sidebar-primary", "--ring", "--sidebar-ring"];
const THEME_COLOR_FOREGROUND_TOKENS = ["--primary-foreground", "--sidebar-primary-foreground"];

function normalizeThemeColor(color) {
  const value = (color || "").trim();
  return (/^#([0-9a-f]{3}|[0-9a-f]{6})$/i).test(value) ? value : "";
}

// Text sitting on the accent has to flip with it, or a pale brand colour leaves
// white-on-white buttons.
function getThemeForegroundColor(hex) {
  const digits = hex.slice(1);
  const full = digits.length === 3 ? digits.split("").map((c) => c + c).join("") : digits;
  const channel = (offset) => {
    const c = parseInt(full.slice(offset, offset + 2), 16) / 255;
    return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  };
  const luminance = 0.2126 * channel(0) + 0.7152 * channel(2) + 0.0722 * channel(4);
  return luminance > 0.45 ? "oklch(0.145 0 0)" : "oklch(0.985 0 0)";
}

export function setThemeColor(color) {
  const root = document.documentElement;
  const value = normalizeThemeColor(color);
  if (!value) {
    localStorage.removeItem("themeColor");
    THEME_COLOR_TOKENS.concat(THEME_COLOR_FOREGROUND_TOKENS).forEach((token) => root.style.removeProperty(token));
    return;
  }

  localStorage.setItem("themeColor", value);
  const foreground = getThemeForegroundColor(value);
  THEME_COLOR_TOKENS.forEach((token) => root.style.setProperty(token, value));
  THEME_COLOR_FOREGROUND_TOKENS.forEach((token) => root.style.setProperty(token, foreground));
}

export function getThemeColor() {
  return localStorage.getItem("themeColor") || "";
}

export function isDarkTheme(themeAlgorithm) {
  return Array.isArray(themeAlgorithm) && themeAlgorithm.includes("dark");
}

// Dark mode is a class on <html> because that is what the Tailwind `dark:`
// variant keys off; the data-theme attribute is kept for the handful of plain
// CSS rules and third-party widgets (xterm) that read it.
export function applyThemeAlgorithm(themeAlgorithm) {
  const dark = isDarkTheme(themeAlgorithm);
  document.documentElement.classList.toggle("dark", dark);
  document.documentElement.setAttribute("data-theme", dark ? "dark" : "light");
}

export function readThemeAlgorithm() {
  try {
    const raw = localStorage.getItem("themeAlgorithm");
    if (raw) {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) {return parsed;}
    }
  } catch {
    // A corrupt value is not worth failing startup over.
  }
  return ["default"];
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
  const defaultFooterHtml = `<a target="_blank" href="https://github.com/casosorg/casos" rel="noreferrer"><img style="height: 30px; width: auto; padding-bottom: 3px;" alt="CasOS" src="${logoUrl}" /></a>`;
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
