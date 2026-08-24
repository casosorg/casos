import React, {useCallback, useEffect, useMemo, useState} from "react";
import {useTranslation} from "react-i18next";
import * as DashboardBackend from "@/backend/DashboardBackend";
import * as Setting from "@/Setting";
import {AccountDialog} from "@/components/shared/account-dialog";
import {AppDock} from "@/desktop/app-dock";
import {AppFrame} from "@/desktop/app-frame";
import {AppLauncher} from "@/desktop/app-launcher";
import {AppWindow} from "@/desktop/app-window";
import {DEFAULT_DESKTOP_KEYS, SYSTEM_APPS} from "@/desktop/registry";
import {DesktopClock} from "@/desktop/desktop-clock";
import {DesktopContextMenu} from "@/desktop/desktop-context-menu";
import {DesktopIcons} from "@/desktop/desktop-icons";
import {DesktopProvider, useDesktop} from "@/desktop/desktop-store";
import {DesktopTopBar} from "@/desktop/desktop-topbar";
import {
  WALLPAPERS,
  isLightWallpaper,
  readDockHidden,
  readLayout,
  readWallpaper,
  reconcileLayout,
  writeDockHidden,
  writeLayout,
  writeWallpaper,
} from "@/desktop/desktop-prefs";
import {useInstalledApps} from "@/desktop/use-installed-apps";
import {useUiMode} from "@/hooks/use-ui-mode";
import {useResource} from "@/hooks/use-resource";

/** The windows themselves, plus the icons and dock that launch them. */
function DesktopShell({apps, stats, wallpaper, light, dockHidden, onToggleDock, onPickWallpaper, onExitDesktop, topBarProps}) {
  const {processes, openApp, closeAll} = useDesktop();
  const [launcherOpen, setLauncherOpen] = useState(false);
  const [menuPosition, setMenuPosition] = useState(null);
  const [layout, setLayout] = useState(() => reconcileLayout(readLayout(), apps, DEFAULT_DESKTOP_KEYS));

  // Installed apps arrive after the first paint, and uninstalled ones stop
  // arriving; either way the arrangement is re-read against what exists now.
  useEffect(() => {
    setLayout((current) => reconcileLayout(current, apps, DEFAULT_DESKTOP_KEYS));
  }, [apps]);

  const updateLayout = useCallback((next) => {
    setLayout(next);
    writeLayout(next);
  }, []);

  const resetLayout = useCallback(() => {
    const next = reconcileLayout(null, apps, DEFAULT_DESKTOP_KEYS);
    setLayout(next);
    writeLayout(next);
  }, [apps]);

  return (
    <div
      data-testid="desktop"
      className="fixed inset-0 flex flex-col overflow-hidden bg-cover bg-center"
      style={WALLPAPERS.find((paper) => paper.key === wallpaper)?.style}
      onContextMenu={(event) => {
        // Only the desktop itself takes over the browser menu; inside a window
        // the reader still gets the browser's own.
        if (event.target.closest("[data-testid^='desktop-window-']")) {
          return;
        }
        event.preventDefault();
        setMenuPosition({x: event.clientX, y: event.clientY});
      }}
    >
      <DesktopTopBar {...topBarProps} apps={apps} stats={stats} onOpenApp={openApp} onExitDesktop={onExitDesktop} />

      <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-10 overflow-auto py-10 pb-32">
        <DesktopClock light={light} />
        <DesktopIcons
          apps={apps}
          layout={layout}
          light={light}
          onLayoutChange={updateLayout}
          onOpenApp={openApp}
          onOpenLauncher={() => setLauncherOpen(true)}
        />
      </div>

      {processes.map((process) => (
        <AppWindow key={process.pid} process={process}>
          <AppFrame process={process} />
        </AppWindow>
      ))}

      <AppDock
        apps={apps}
        hidden={dockHidden}
        onToggleHidden={onToggleDock}
        onOpenLauncher={() => setLauncherOpen(true)}
      />

      <AppLauncher
        open={launcherOpen}
        apps={apps}
        layout={layout}
        onClose={() => setLauncherOpen(false)}
        onOpenApp={openApp}
        onLayoutChange={updateLayout}
      />

      <DesktopContextMenu
        position={menuPosition}
        wallpaper={wallpaper}
        dockHidden={dockHidden}
        onClose={() => setMenuPosition(null)}
        onPickWallpaper={onPickWallpaper}
        onToggleDock={onToggleDock}
        onResetLayout={resetLayout}
        onCloseAllWindows={closeAll}
        onExitDesktop={onExitDesktop}
      />
    </div>
  );
}

/**
 * The desktop: casos as an operating system rather than as a set of pages.
 *
 * Every screen the sidebar UI serves is available here as a window, so the two
 * are the same product seen two ways — nothing is reachable in one and missing
 * from the other.
 */
function DesktopPage(props) {
  useTranslation();
  const {account, site, themeAlgorithm, logo, onSignout, onUpdateSite, onUpdateAccount, setLogoAndThemeAlgorithm} = props;
  const {exitDesktop} = useUiMode();

  const [accountDialogOpen, setAccountDialogOpen] = useState(false);
  const [accountUpdatedAt, setAccountUpdatedAt] = useState(0);
  const [wallpaper, setWallpaper] = useState(readWallpaper);
  const [dockHidden, setDockHidden] = useState(readDockHidden);

  const {apps: installedApps} = useInstalledApps();
  const {data: stats} = useResource(() => DashboardBackend.getDashboard(), [], {
    initialData: null,
    toastOnError: false,
    pollInterval: 30000,
  });

  const apps = useMemo(() => [...SYSTEM_APPS, ...installedApps], [installedApps]);

  function handleOpenAccount() {
    if (Setting.isBasicLoginMode(account)) {
      setAccountDialogOpen(true);
      return;
    }
    const profileUrl = Setting.getMyProfileUrl(account);
    if (profileUrl !== "") {
      window.open(profileUrl, "_blank", "noreferrer");
    }
  }

  const desktopValue = useMemo(() => ({
    // What every window's routes need; passed through the desktop context so a
    // window does not have to be told which app it is hosting.
    appProps: {
      account,
      accountUpdatedAt,
      onOpenAccount: handleOpenAccount,
      onUpdateSite,
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }), [account, accountUpdatedAt, onUpdateSite]);

  const topBarProps = {
    account,
    logo: logo || Setting.getLogo(themeAlgorithm || [], site?.logoUrl),
    themeAlgorithm,
    onThemeChange: setLogoAndThemeAlgorithm,
    onOpenAccount: handleOpenAccount,
    onSignout,
  };

  return (
    <DesktopProvider value={desktopValue}>
      <DesktopShell
        apps={apps}
        stats={stats}
        wallpaper={wallpaper}
        light={isLightWallpaper(wallpaper)}
        dockHidden={dockHidden}
        onToggleDock={() => {
          setDockHidden((hidden) => {
            writeDockHidden(!hidden);
            return !hidden;
          });
        }}
        onPickWallpaper={(key) => {
          setWallpaper(key);
          writeWallpaper(key);
        }}
        onExitDesktop={exitDesktop}
        topBarProps={topBarProps}
      />

      <AccountDialog
        account={account}
        open={accountDialogOpen}
        onOpenChange={setAccountDialogOpen}
        onUpdateAccount={() => {
          onUpdateAccount?.();
          setAccountUpdatedAt(Date.now());
        }}
      />
    </DesktopProvider>
  );
}

export default DesktopPage;
