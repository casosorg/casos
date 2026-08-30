import React, {useRef, useState} from "react";
import i18next from "i18next";
import {cn} from "@/lib/utils";
import {tintClass} from "@/desktop/registry";

/** One launchable tile: the icon, its name, and the drag handle that is both. */
export function AppIcon({app, light, onOpen, onDragStart, onDragEnd, onDropBefore, compact}) {
  const Icon = app.icon;
  const [over, setOver] = useState(false);
  const label = app.labelKey ? i18next.t(app.labelKey) : app.name;

  return (
    <button
      type="button"
      data-testid={`desktop-icon-${app.key}`}
      className="group flex w-full flex-col items-center gap-2 outline-none"
      draggable={Boolean(onDragStart)}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onDragOver={(event) => {
        if (onDropBefore) {
          event.preventDefault();
          setOver(true);
        }
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(event) => {
        setOver(false);
        if (onDropBefore) {
          event.preventDefault();
          event.stopPropagation();
          onDropBefore(event);
        }
      }}
      onClick={() => onOpen(app)}
      onDoubleClick={() => onOpen(app)}
    >
      <span
        className={cn(
          "flex items-center justify-center rounded-3xl text-white shadow-lg ring-1 ring-black/5 transition-transform duration-150 group-hover:scale-105 group-focus-visible:scale-105",
          compact ? "size-14 rounded-2xl" : "size-[68px]",
          app.iconUrl ? "bg-white" : tintClass(app.tint),
          over && "ring-2 ring-white"
        )}
      >
        {app.iconUrl
          ? <img src={app.iconUrl} alt="" className={cn("object-contain", compact ? "size-8" : "size-10")} />
          : Icon ? <Icon className={compact ? "size-6" : "size-8"} /> : null}
      </span>
      <span
        className={cn(
          "line-clamp-2 text-center text-xs leading-4 font-medium break-words",
          light ? "text-neutral-900" : "text-white drop-shadow-[0_1px_2px_rgba(0,0,0,0.6)]"
        )}
      >
        {label}
      </span>
    </button>
  );
}

/** The folder every app that is not on the desktop lives in. */
function MoreAppsFolder({apps, light, onOpen, onDrop}) {
  const [over, setOver] = useState(false);

  return (
    <button
      type="button"
      data-testid="desktop-folder"
      className="group flex w-full flex-col items-center gap-2 outline-none"
      onDragOver={(event) => {
        event.preventDefault();
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(event) => {
        event.preventDefault();
        event.stopPropagation();
        setOver(false);
        onDrop(event);
      }}
      onClick={onOpen}
    >
      <span
        className={cn(
          "grid size-[68px] grid-cols-2 grid-rows-2 gap-1 rounded-3xl border border-white/20 bg-white/25 p-2 shadow-lg backdrop-blur-xl transition-transform duration-150 group-hover:scale-105 dark:bg-black/25",
          over && "ring-2 ring-white"
        )}
      >
        {apps.slice(0, 4).map((app) => {
          const Icon = app.icon;
          return (
            <span key={app.key} className={cn("flex items-center justify-center rounded-md text-white", app.iconUrl ? "bg-white" : tintClass(app.tint))}>
              {app.iconUrl ? <img src={app.iconUrl} alt="" className="size-4 object-contain" /> : Icon ? <Icon className="size-3.5" /> : null}
            </span>
          );
        })}
      </span>
      <span
        className={cn(
          "line-clamp-2 text-center text-xs leading-4 font-medium",
          light ? "text-neutral-900" : "text-white drop-shadow-[0_1px_2px_rgba(0,0,0,0.6)]"
        )}
      >
        {i18next.t("general:All apps")}
      </span>
    </button>
  );
}

/**
 * The icon grid. Icons drag to reorder, onto the folder to file away, and out
 * of the launcher back onto the desktop — the same three gestures the sealos
 * desktop supports.
 */
export function DesktopIcons({apps, layout, onLayoutChange, onOpenApp, onOpenLauncher, light}) {
  const dragKeyRef = useRef(null);
  const byKey = new Map(apps.map((app) => [app.key, app]));
  const desktopApps = layout.desktop.map((key) => byKey.get(key)).filter(Boolean);
  const folderApps = layout.folder.map((key) => byKey.get(key)).filter(Boolean);

  function moveTo(targetKey) {
    const dragged = dragKeyRef.current;
    if (!dragged || dragged === targetKey) {
      return;
    }
    const desktop = layout.desktop.filter((key) => key !== dragged);
    const folder = layout.folder.filter((key) => key !== dragged);
    const index = targetKey === null ? desktop.length : desktop.indexOf(targetKey);
    desktop.splice(index < 0 ? desktop.length : index, 0, dragged);
    onLayoutChange({desktop, folder});
  }

  function moveToFolder() {
    const dragged = dragKeyRef.current;
    if (!dragged || layout.folder.includes(dragged)) {
      return;
    }
    onLayoutChange({
      desktop: layout.desktop.filter((key) => key !== dragged),
      folder: [...layout.folder, dragged],
    });
  }

  return (
    <div
      data-testid="desktop-icons"
      className="mx-auto w-full max-w-5xl px-6"
      onDragOver={(event) => event.preventDefault()}
      onDrop={() => moveTo(null)}
    >
      <div className="grid grid-cols-3 gap-x-4 gap-y-7 sm:grid-cols-5 lg:grid-cols-7">
        {desktopApps.map((app) => (
          <AppIcon
            key={app.key}
            app={app}
            light={light}
            onOpen={onOpenApp}
            onDragStart={() => (dragKeyRef.current = app.key)}
            onDragEnd={() => (dragKeyRef.current = null)}
            onDropBefore={() => moveTo(app.key)}
          />
        ))}
        <MoreAppsFolder apps={folderApps} light={light} onOpen={onOpenLauncher} onDrop={moveToFolder} />
      </div>
    </div>
  );
}

export default DesktopIcons;
