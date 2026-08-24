import React, {useCallback, useEffect, useMemo, useState} from "react";
import {useTranslation} from "react-i18next";
import * as DashboardBackend from "@/backend/DashboardBackend";
import * as Setting from "@/Setting";
import {AccountDialog} from "@/components/shared/account-dialog";
import {APP_BAR_HEIGHT, AppDock} from "@/desktop/app-dock";
import {AppFrame} from "@/desktop/app-frame";
import {AppLauncher} from "@/desktop/app-launcher";
import {AppWindow} from "@/desktop/app-window";
import {DEFAULT_DESKTOP_KEYS, SYSTEM_APPS} from "@/desktop/registry";
import {DesktopClock} from "@/desktop/desktop-clock";
import {DesktopContextMenu} from "@/desktop/desktop-context-menu";
import {DesktopIcons} from "@/desktop/desktop-icons";
import {DesktopProvider, useDesktop} from "@/desktop/desktop-store";
import {DesktopTopBar} from "@/desktop/desktop-topbar";
import {DesktopTour} from "@/desktop/desktop-tour";
import {
  CUSTOM_WALLPAPER,
  DOCK_MODE,
  isLightWallpaper,
  readCustomWallpaper,
  readDockHidden,
  readDockMode,
  readLayout,
  readTourSeen,
  readWallpaper,
  reconcileLayout,
  wallpaperStyle,
  writeCustomWallpaper,
  writeDockHidden,
  writeDockMode,
  writeLayout,
  writeTourSeen,
  writeWallpaper,
} from "@/desktop/desktop-prefs";
import {useAppSdk} from "@/desktop/use-app-sdk";
import {useInstalledApps} from "@/desktop/use-installed-apps";
import {useIsNarrow} from "@/hooks/use-screen";
import {useUiMode} from "@/hooks/use-ui-mode";
import {useResource} from "@/hooks/use-resource";

/** The windows themselves, plus the icons and dock that launch them. */
function DesktopShell({
  apps,
  account,
  stats,
  wallpaper,
  customWallpaper,
  light,
  dockHidden,
  dockMode,
  onToggleDock,
  onToggleDockMode,
  onPickWallpaper,
  onPickCustomWallpaper,
  onExitDesktop,
  topBarProps,
}) {
  const {processes, openApp, closeAll} = useDesktop();
  const [launcherOpen, setLauncherOpen] = useState(false);
  const [menuPosition, setMenuPosition] = useState(null);
  const [tourOpen, setTourOpen] = useState(() => !readTourSeen());
  const [layout, setLayout] = useState(() => reconcileLayout(readLayout(), apps, DEFAULT_DESKTOP_KEYS));
  const narrow = useIsNarrow();
  const barMode = narrow || dockMode === DOCK_MODE.BAR;

  useAppSdk({apps, account, stats});

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
      style={wallpaperStyle(wallpaper, customWallpaper)}
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
        <AppWindow key={process.pid} process={process} narrow={narrow} bottomInset={barMode && !dockHidden ? APP_BAR_HEIGHT : 0}>
          <AppFrame process={process} />
        </AppWindow>
      ))}

      <AppDock
        apps={apps}
        hidden={dockHidden}
        mode={barMode ? DOCK_MODE.BAR : dockMode}
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
        customWallpaper={customWallpaper}
        dockHidden={dockHidden}
        dockMode={dockMode}
        onClose={() => setMenuPosition(null)}
        onPickWallpaper={onPickWallpaper}
        onPickCustomWallpaper={onPickCustomWallpaper}
        onToggleDock={onToggleDock}
        onToggleDockMode={onToggleDockMode}
        onResetLayout={resetLayout}
        onCloseAllWindows={closeAll}
        onStartTour={() => setTourOpen(true)}
        onExitDesktop={onExitDesktop}
      />

      <DesktopTour
        open={tourOpen}
        onClose={() => {
          setTourOpen(false);
          writeTourSeen(true);
        }}
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
  const [customWallpaper, setCustomWallpaper] = useState(readCustomWallpaper);
  const [dockHidden, setDockHidden] = useState(readDockHidden);
  const [dockMode, setDockMode] = useState(readDockMode);

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
        account={account}
        stats={stats}
        wallpaper={wallpaper}
        customWallpaper={customWallpaper}
        light={isLightWallpaper(wallpaper)}
        dockHidden={dockHidden}
        dockMode={dockMode}
        onToggleDock={() => {
          setDockHidden((hidden) => {
            writeDockHidden(!hidden);
            return !hidden;
          });
        }}
        onToggleDockMode={() => {
          setDockMode((mode) => {
            const next = mode === DOCK_MODE.BAR ? DOCK_MODE.DOCK : DOCK_MODE.BAR;
            writeDockMode(next);
            return next;
          });
        }}
        onPickWallpaper={(key) => {
          setWallpaper(key);
          writeWallpaper(key);
        }}
        onPickCustomWallpaper={(source) => {
          setCustomWallpaper(source);
          writeCustomWallpaper(source);
          setWallpaper(CUSTOM_WALLPAPER);
          writeWallpaper(CUSTOM_WALLPAPER);
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
