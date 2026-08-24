import React, {useMemo, useRef, useState} from "react";
import i18next from "i18next";
import {ChevronDown, Grid3x3, House} from "lucide-react";
import {cn} from "@/lib/utils";
import {DOCK_PINNED_KEYS, tintClass} from "@/desktop/registry";
import {WINDOW_SIZE, useDesktop} from "@/desktop/desktop-store";

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
 * The dock: pinned apps, whatever else is running, and the two controls sealos
 * puts there — show-the-desktop on the left and the app folder on the right.
 */
export function AppDock({apps, onOpenLauncher, hidden, onToggleHidden}) {
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
              label={app.labelKey ? i18next.t(app.labelKey) : app.name}
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
