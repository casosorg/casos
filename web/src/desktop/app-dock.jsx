import React, {useMemo, useRef, useState} from "react";
import i18next from "i18next";
import {ChevronDown, Grid3x3, House} from "lucide-react";
import {cn} from "@/lib/utils";
import {DOCK_PINNED_KEYS, tintClass} from "@/desktop/registry";
import {DOCK_MODE} from "@/desktop/desktop-prefs";
import {WINDOW_SIZE, useDesktop} from "@/desktop/desktop-store";

/** The task bar's height, which a maximized window has to stop above. */
export const APP_BAR_HEIGHT = 48;

const ICON_SIZE = 52;
/** How far from the cursor an icon still lifts, and how big it grows there. */
const MAGNIFY_RANGE = 140;
const MAGNIFY_SIZE = 72;

function scaleFor(distance) {
  if (distance === null || Math.abs(distance) > MAGNIFY_RANGE) {
    return ICON_SIZE;
  }
  const ratio = 1 - Math.abs(distance) / MAGNIFY_RANGE;
  return ICON_SIZE + (MAGNIFY_SIZE - ICON_SIZE) * ratio;
}

function DockIcon({app, running, size, label, onClick, onPointerEnter}) {
  const Icon = app.icon;
  const [hovered, setHovered] = useState(false);

  return (
    <div
      className="relative flex cursor-pointer flex-col items-center justify-end pb-1"
      onPointerEnter={(event) => {
        setHovered(true);
        onPointerEnter?.(event);
      }}
      onPointerLeave={() => setHovered(false)}
      onClick={onClick}
      data-testid={`dock-icon-${app.key}`}
    >
      {hovered && (
        <span className="bg-popover text-popover-foreground pointer-events-none absolute -top-9 rounded-md border px-2 py-1 text-xs font-medium whitespace-nowrap shadow-md">
          {label}
        </span>
      )}
      <span
        className={cn(
          "flex items-center justify-center rounded-2xl text-white shadow-lg transition-[width,height] duration-100 ease-out",
          app.iconUrl ? "bg-white" : tintClass(app.tint)
        )}
        style={{width: size, height: size}}
      >
        {app.iconUrl
          ? <img src={app.iconUrl} alt="" className="size-3/5 object-contain" />
          : Icon ? <Icon style={{width: size * 0.5, height: size * 0.5}} /> : null}
      </span>
      <span className={cn("mt-1 size-1.5 rounded-full bg-black/50 dark:bg-white/60", running ? "opacity-100" : "opacity-0")} />
    </div>
  );
}

/**
 * The other shape the launcher strip takes: a taskbar across the bottom edge,
 * where a running window is a labelled button rather than an icon that lifts.
 * It is what a reader who works in one window at a time wants, and it is the
 * mode sealos's app-bar toggle switches to.
 */
function AppBar({items, processes, currentPid, hidden, label, onOpenLauncher, onActivate, onShowDesktop}) {
  return (
    <div
      data-testid="desktop-appbar"
      className={cn(
        "absolute right-0 bottom-0 left-0 z-[900] flex h-12 items-center gap-1 border-t border-white/20 bg-white/30 px-2 backdrop-blur-2xl transition-transform duration-300 dark:bg-black/40",
        hidden && "translate-y-12"
      )}
    >
      <button
        type="button"
        aria-label={i18next.t("desktop:All apps")}
        onClick={onOpenLauncher}
        className="flex size-9 shrink-0 items-center justify-center rounded-lg text-neutral-800 hover:bg-black/10 dark:text-neutral-100 dark:hover:bg-white/15"
      >
        <Grid3x3 className="size-5" />
      </button>
      <button
        type="button"
        aria-label={i18next.t("desktop:Show desktop")}
        onClick={onShowDesktop}
        className="flex size-9 shrink-0 items-center justify-center rounded-lg text-neutral-800 hover:bg-black/10 dark:text-neutral-100 dark:hover:bg-white/15"
      >
        <House className="size-5" />
      </button>
      <div className="bg-black/15 mx-1 h-6 w-px shrink-0 dark:bg-white/20" />

      <div className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto">
        {items.map((app) => {
          const process = processes.find((item) => item.key === app.key);
          const Icon = app.icon;
          return (
            <button
              key={app.key}
              type="button"
              data-testid={`dock-icon-${app.key}`}
              onClick={() => onActivate(app)}
              className={cn(
                "flex h-9 max-w-48 min-w-0 shrink-0 items-center gap-2 rounded-lg px-2 text-sm text-neutral-800 hover:bg-black/10 dark:text-neutral-100 dark:hover:bg-white/15",
                process && "bg-black/10 dark:bg-white/15",
                process?.pid === currentPid && "ring-1 ring-black/20 dark:ring-white/30"
              )}
            >
              <span className={cn("flex size-6 shrink-0 items-center justify-center rounded-md text-white", app.iconUrl ? "bg-white" : tintClass(app.tint))}>
                {app.iconUrl ? <img src={app.iconUrl} alt="" className="size-4 object-contain" /> : Icon ? <Icon className="size-3.5" /> : null}
              </span>
              <span className="truncate">{label(app)}</span>
            </button>
          );
        })}
      </div>
    </div>
  );
}

/**
 * The dock: pinned apps, whatever else is running, and the two controls sealos
 * puts there — show-the-desktop on the left and the app folder on the right.
 */
export function AppDock({apps, onOpenLauncher, hidden, onToggleHidden, mode = DOCK_MODE.DOCK}) {
  const {processes, currentPid, openApp, focusApp, setSize, minimizeAll} = useDesktop();
  const [pointerX, setPointerX] = useState(null);
  const iconRefs = useRef({});

  const items = useMemo(() => {
    const byKey = new Map(apps.map((app) => [app.key, app]));
    const pinned = DOCK_PINNED_KEYS.map((key) => byKey.get(key)).filter(Boolean);
    const pinnedKeys = new Set(pinned.map((app) => app.key));
    const running = processes
      .map((process) => byKey.get(process.key) ?? process.app)
      .filter((app) => app && !pinnedKeys.has(app.key));
    return [...pinned, ...running];
  }, [apps, processes]);

  function handleClick(app) {
    const process = processes.find((item) => item.key === app.key);
    if (!process) {
      openApp(app);
      return;
    }
    // Clicking the window you are already looking at puts it away again, which
    // is what every dock does.
    if (process.pid === currentPid && process.size !== WINDOW_SIZE.MINIMIZE) {
      setSize(process.pid, WINDOW_SIZE.MINIMIZE);
      return;
    }
    focusApp(process.pid);
  }

  function measure(event) {
    setPointerX(event.clientX);
  }

  function sizeOf(key) {
    if (pointerX === null) {
      return ICON_SIZE;
    }
    const node = iconRefs.current[key];
    if (!node) {
      return ICON_SIZE;
    }
    const rect = node.getBoundingClientRect();
    return scaleFor(pointerX - (rect.left + rect.width / 2));
  }

  const labelOf = (app) => (app.labelKey ? i18next.t(app.labelKey) : app.name);

  if (mode === DOCK_MODE.BAR) {
    return (
      <AppBar
        items={items}
        processes={processes}
        currentPid={currentPid}
        hidden={hidden}
        label={labelOf}
        onOpenLauncher={onOpenLauncher}
        onActivate={handleClick}
        onShowDesktop={minimizeAll}
      />
    );
  }

  return (
    <div
      className={cn(
        "absolute bottom-1 left-1/2 z-[900] flex -translate-x-1/2 flex-col items-center px-4 transition-transform duration-300",
        hidden && "translate-y-[86px]"
      )}
      onPointerMove={measure}
      onPointerLeave={() => setPointerX(null)}
    >
      <button
        type="button"
        aria-label={i18next.t("desktop:Toggle dock")}
        onClick={onToggleHidden}
        className="mb-1 flex h-5 w-16 items-center justify-center rounded-full border border-white/20 bg-white/30 text-neutral-800 shadow backdrop-blur-xl dark:bg-black/30 dark:text-neutral-100"
      >
        <ChevronDown className={cn("size-4 transition-transform", hidden && "rotate-180")} />
      </button>

      <div
        data-testid="desktop-dock"
        className="flex h-[78px] items-end gap-3 rounded-3xl border border-white/20 bg-white/30 px-3 shadow-xl backdrop-blur-2xl dark:bg-black/30"
      >
        <div ref={(node) => (iconRefs.current["home"] = node)}>
          <DockIcon
            app={{key: "home", icon: House, tint: "slate"}}
            label={i18next.t("desktop:Show desktop")}
            size={sizeOf("home")}
            running={false}
            onClick={minimizeAll}
          />
        </div>

        {items.map((app) => (
          <div key={app.key} ref={(node) => (iconRefs.current[app.key] = node)}>
            <DockIcon
              app={app}
              label={labelOf(app)}
              size={sizeOf(app.key)}
              running={processes.some((process) => process.key === app.key)}
              onClick={() => handleClick(app)}
            />
          </div>
        ))}

        <div ref={(node) => (iconRefs.current["launcher"] = node)}>
          <DockIcon
            app={{key: "launcher", icon: Grid3x3, tint: "zinc"}}
            label={i18next.t("desktop:All apps")}
            size={sizeOf("launcher")}
            running={false}
            onClick={onOpenLauncher}
          />
        </div>
      </div>
    </div>
  );
}

export default AppDock;
