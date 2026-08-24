import React, {useEffect, useRef, useState} from "react";
import i18next from "i18next";
import {Check, Image as ImageIcon, LayoutPanelLeft, PanelBottom, RotateCcw, XSquare} from "lucide-react";
import {cn} from "@/lib/utils";
import {WALLPAPERS} from "@/desktop/desktop-prefs";

/**
 * The desktop's right-click menu. Everything here is a property of the desktop
 * itself — the wallpaper, the dock, the arrangement — which is why none of it
 * is in the account menu.
 */
export function DesktopContextMenu({position, wallpaper, dockHidden, onClose, onPickWallpaper, onToggleDock, onResetLayout, onCloseAllWindows, onExitDesktop}) {
  const ref = useRef(null);
  const [showWallpapers, setShowWallpapers] = useState(false);

  useEffect(() => {
    function onPointerDown(event) {
      if (!ref.current?.contains(event.target)) {
        onClose();
      }
    }
    function onKeyDown(event) {
      if (event.key === "Escape") {
        onClose();
      }
    }
    window.addEventListener("pointerdown", onPointerDown);
    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("pointerdown", onPointerDown);
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [onClose]);

  if (!position) {
    return null;
  }

  // Menus opened near the right or bottom edge would otherwise run off screen.
  const left = Math.min(position.x, window.innerWidth - 240);
  const top = Math.min(position.y, window.innerHeight - 260);

  return (
    <div
      ref={ref}
      data-testid="desktop-context-menu"
      className="bg-popover text-popover-foreground fixed z-[1100] w-56 rounded-xl border p-1 shadow-xl"
      style={{left, top}}
    >
      <MenuItem icon={ImageIcon} label={i18next.t("desktop:Change wallpaper")} onClick={() => setShowWallpapers((open) => !open)} />
      {showWallpapers && (
        <div className="grid grid-cols-3 gap-1.5 p-1.5">
          {WALLPAPERS.map((paper) => (
            <button
              key={paper.key}
              type="button"
              title={i18next.t(paper.labelKey)}
              onClick={() => {
                onPickWallpaper(paper.key);
                onClose();
              }}
              className={cn(
                "relative h-10 rounded-md border",
                paper.key === wallpaper ? "ring-primary ring-2" : "hover:opacity-80"
              )}
              style={paper.style}
            >
              {paper.key === wallpaper && <Check className="absolute inset-0 m-auto size-4 text-white drop-shadow" />}
            </button>
          ))}
        </div>
      )}
      <MenuItem
        icon={PanelBottom}
        label={dockHidden ? i18next.t("desktop:Show dock") : i18next.t("desktop:Hide dock")}
        onClick={() => {
          onToggleDock();
          onClose();
        }}
      />
      <MenuItem
        icon={RotateCcw}
        label={i18next.t("desktop:Reset icon layout")}
        onClick={() => {
          onResetLayout();
          onClose();
        }}
      />
      <MenuItem
        icon={XSquare}
        label={i18next.t("desktop:Close all windows")}
        onClick={() => {
          onCloseAllWindows();
          onClose();
        }}
      />
      <div className="bg-border my-1 h-px" />
      <MenuItem
        icon={LayoutPanelLeft}
        label={i18next.t("desktop:Management view")}
        onClick={() => {
          onExitDesktop();
          onClose();
        }}
      />
    </div>
  );
}

function MenuItem({icon: Icon, label, onClick}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="hover:bg-accent flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm"
    >
      <Icon className="size-4" />
      {label}
    </button>
  );
}

export default DesktopContextMenu;
