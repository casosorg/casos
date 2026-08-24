import {useEffect, useMemo, useRef} from "react";
import i18next from "i18next";
import * as Setting from "@/Setting";
import {APP_KIND} from "@/desktop/registry";
import {DESKTOP_EVENT, EVENT_NAME, createMasterApp, masterApp} from "@/desktop/app-sdk";
import {useDesktop} from "@/desktop/desktop-store";

function appOrigin(url) {
  try {
    return new URL(url, window.location.origin).origin;
  } catch {
    return null;
  }
}

/** What an app is allowed to know about the desktop it is running on. */
function sessionOf(account) {
  if (!account) {
    return null;
  }
  return {
    user: {
      id: account.id || account.name,
      name: account.name,
      displayName: account.displayName ?? account.name,
      avatar: account.avatar ?? "",
      nsid: account.namespace ?? "default",
    },
    workspace: {namespace: account.namespace ?? "default"},
  };
}

/** Cluster capacity, in the shape sealos apps expect a workspace quota in. */
function quotaOf(stats) {
  if (!stats) {
    return [];
  }
  return [
    {type: "cpu", limit: (stats.clusterCPUTotalM ?? 0) / 1000, used: (stats.clusterCPUUsedM ?? 0) / 1000},
    {type: "memory", limit: (stats.clusterMemTotalMi ?? 0) / 1024, used: (stats.clusterMemUsedMi ?? 0) / 1024},
  ];
}

/** A ref that always holds the latest render's value. */
function useLatest(value) {
  const ref = useRef(value);
  ref.current = value;
  return ref;
}

/**
 * Opens the SDK channel for the apps on this desktop.
 *
 * Only the origins of apps this cluster actually installed may talk to it: an
 * app's own origin is proof the operator put it here, and nothing else is. The
 * channel is opened once and reads everything that moves through refs, so a
 * poll that refreshes the cluster stats does not tear it down mid-request.
 */
export function useAppSdk({apps, account, stats}) {
  const {openApp, closeApp, currentPid, processes} = useDesktop();

  const allowedOrigins = useMemo(
    () => Array.from(new Set(apps.filter((app) => app.url).map((app) => appOrigin(app.url)).filter(Boolean))),
    [apps]
  );

  const allowedOriginsRef = useLatest(allowedOrigins);
  const accountRef = useLatest(account);
  const statsRef = useLatest(stats);
  const appsRef = useLatest(apps);
  const focusedRef = useLatest(processes.find((process) => process.pid === currentPid));

  useEffect(() => {
    return createMasterApp({
      getAllowedOrigins: () => allowedOriginsRef.current,
      getSession: () => sessionOf(accountRef.current),
      getLanguage: () => Setting.getLanguage(),
      getWorkspaceQuota: () => quotaOf(statsRef.current),
      getHostConfig: () => ({
        cloud: {domain: window.location.hostname, port: window.location.port, regionUid: "default"},
        features: {subscription: false},
      }),
    });
  }, [allowedOriginsRef, accountRef, statsRef]);

  useEffect(() => {
    if (!masterApp) {
      return undefined;
    }

    const removers = [
      masterApp.addEventListen(DESKTOP_EVENT.OPEN_APP, (data = {}) => {
        const target = appsRef.current.find((app) => app.key === data.appKey || app.name === data.appKey);
        if (!target) {
          throw new Error(`no app named ${data.appKey}`);
        }
        const query = data.query ? `?${new URLSearchParams(data.query)}` : "";
        openApp(target, {path: data.pathname ? `${data.pathname}${query}` : undefined, size: data.appSize});
        return {opened: target.key};
      }),

      masterApp.addEventListen(DESKTOP_EVENT.CLOSE_APP, () => {
        const focused = focusedRef.current;
        if (focused) {
          closeApp(focused.pid);
        }
        return {closed: focused?.key ?? null};
      }),

      masterApp.addEventListen(DESKTOP_EVENT.GET_APPS, () => appsRef.current.map((app) => ({
        key: app.key,
        name: app.labelKey ? i18next.t(app.labelKey) : app.name,
        icon: app.iconUrl ?? null,
        kind: app.kind ?? APP_KIND.INTERNAL,
        url: app.url ?? null,
      }))),

      masterApp.addEventListen(DESKTOP_EVENT.SHOW_MESSAGE, (data = {}) => {
        const type = data.type === "error" || data.type === "warning" ? data.type : "success";
        Setting.showMessage(type, String(data.message ?? ""));
        return {};
      }),
    ];

    return () => removers.forEach((remove) => remove?.());
  }, [openApp, closeApp, appsRef, focusedRef]);

  // Apps render their own chrome, so a language change on the desktop has to be
  // handed down rather than inherited.
  useEffect(() => {
    function broadcast(lng) {
      masterApp?.sendMessageToAll({apiName: "event-bus", eventName: EVENT_NAME.CHANGE_I18N, data: {currentLanguage: lng}});
    }
    i18next.on("languageChanged", broadcast);
    return () => i18next.off("languageChanged", broadcast);
  }, []);
}
