import React, {useCallback, useEffect, useRef, useState} from "react";
import i18next from "i18next";
import {ExternalLink, Maximize2, Minimize2, Minus, X} from "lucide-react";
import {cn} from "@/lib/utils";
import {tintClass} from "@/desktop/registry";
import {WINDOW_SIZE, useDesktop} from "@/desktop/desktop-store";

/** Height of the desktop's top bar; a maximized window starts under it. */
export const TOP_BAR_HEIGHT = 52;

const MIN_WIDTH = 420;
const MIN_HEIGHT = 280;

const RESIZE_HANDLES = [
  {dir: "n", className: "top-0 left-3 right-3 h-1.5 cursor-ns-resize"},
  {dir: "s", className: "bottom-0 left-3 right-3 h-1.5 cursor-ns-resize"},
  {dir: "w", className: "left-0 top-3 bottom-3 w-1.5 cursor-ew-resize"},
  {dir: "e", className: "right-0 top-3 bottom-3 w-1.5 cursor-ew-resize"},
  {dir: "nw", className: "left-0 top-0 size-3 cursor-nwse-resize"},
  {dir: "ne", className: "right-0 top-0 size-3 cursor-nesw-resize"},
  {dir: "sw", className: "left-0 bottom-0 size-3 cursor-nesw-resize"},
  {dir: "se", className: "right-0 bottom-0 size-3 cursor-nwse-resize"},
];

function clampToViewport({x, y, width, height}) {
  const maxX = window.innerWidth - 80;
  const maxY = window.innerHeight - 60;
  return {
    width: Math.max(MIN_WIDTH, Math.min(width, window.innerWidth)),
    height: Math.max(MIN_HEIGHT, Math.min(height, window.innerHeight)),
    x: Math.min(Math.max(x, -width + 120), maxX),
    y: Math.min(Math.max(y, TOP_BAR_HEIGHT - 40), maxY),
  };
}

/**
 * One window: a title bar that drags, three size buttons, resize edges, and a
 * mask over the content while another window holds focus so a stray click
 * raises this one instead of landing inside the app.
 */
export function AppWindow({process, children}) {
  const {closeApp, focusApp, setSize, setGeometry, currentPid} = useDesktop();
  const [drag, setDrag] = useState(null);
  const frameRef = useRef(null);

  const isFocused = currentPid === process.pid;
  const maximized = process.size === WINDOW_SIZE.MAXIMIZE;
  const minimized = process.size === WINDOW_SIZE.MINIMIZE;

  const geometry = drag ?? {x: process.x, y: process.y, width: process.width, height: process.height};

  const startPointer = useCallback((event, mode) => {
    if (event.button !== 0) {
      return;
    }
    event.preventDefault();
    focusApp(process.pid);
    if (maximized && mode !== "move") {
      return;
    }

    const start = {
      pointerX: event.clientX,
      pointerY: event.clientY,
      x: process.x,
      y: process.y,
      width: process.width,
      height: process.height,
    };
    // Dragging a maximized window pulls it out of full screen under the
    // cursor, the way a tab tears off a browser window.
    const restoring = maximized && mode === "move";
    if (restoring) {
      start.x = Math.max(0, event.clientX - process.width / 2);
      start.y = Math.max(TOP_BAR_HEIGHT, event.clientY - 14);
      setSize(process.pid, WINDOW_SIZE.WINDOWED);
    }

    let next = {x: start.x, y: start.y, width: start.width, height: start.height};

    function onMove(moveEvent) {
      const dx = moveEvent.clientX - start.pointerX;
      const dy = moveEvent.clientY - start.pointerY;
      if (mode === "move") {
        next = clampToViewport({...start, x: start.x + dx, y: start.y + dy});
      } else {
        const bounds = {...start};
        if (mode.includes("e")) {
          bounds.width = start.width + dx;
        }
        if (mode.includes("s")) {
          bounds.height = start.height + dy;
        }
        if (mode.includes("w")) {
          bounds.width = start.width - dx;
          bounds.x = start.x + dx;
        }
        if (mode.includes("n")) {
          bounds.height = start.height - dy;
          bounds.y = start.y + dy;
        }
        next = clampToViewport(bounds);
      }
      setDrag(next);
    }

    function onUp() {
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      setDrag(null);
      setGeometry(process.pid, next);
    }

    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
  }, [focusApp, maximized, process, setGeometry, setSize]);

  // A window left half off screen after the browser is resized is a window the
  // reader cannot reach the title bar of.
  useEffect(() => {
    function onResize() {
      if (process.size !== WINDOW_SIZE.WINDOWED) {
        return;
      }
      setGeometry(process.pid, clampToViewport({
        x: process.x,
        y: process.y,
        width: process.width,
        height: process.height,
      }));
    }
    window.addEventListener("resize", onResize);
    return () => window.removeEventListener("resize", onResize);
  }, [process, setGeometry]);

  const Icon = process.app.icon;
  const title = process.app.labelKey ? i18next.t(process.app.labelKey) : process.app.name;

  const style = maximized
    ? {top: TOP_BAR_HEIGHT, left: 0, width: "100%", height: `calc(100% - ${TOP_BAR_HEIGHT}px)`, zIndex: process.zIndex}
    : {
      top: geometry.y,
      left: geometry.x,
      width: geometry.width,
      height: geometry.height,
      zIndex: process.zIndex,
    };

  return (
    <div
      ref={frameRef}
      data-testid={`desktop-window-${process.key}`}
      data-size={process.size}
      className={cn(
        "bg-background fixed flex flex-col overflow-hidden border shadow-2xl",
        maximized ? "rounded-none" : "rounded-xl",
        minimized && "pointer-events-none scale-[0.08] opacity-0",
        drag ? "transition-none" : "transition-[opacity,transform] duration-200 ease-out",
        minimized && "origin-bottom"
      )}
      style={style}
      onPointerDown={() => {
        if (!isFocused) {
          focusApp(process.pid);
        }
      }}
    >
      <div
        className={cn(
          "flex h-9 shrink-0 select-none items-center gap-2 border-b px-3",
          isFocused ? "bg-muted" : "bg-muted/60"
        )}
        onPointerDown={(event) => startPointer(event, "move")}
        onDoubleClick={() => setSize(process.pid, maximized ? WINDOW_SIZE.WINDOWED : WINDOW_SIZE.MAXIMIZE)}
      >
        <span className={cn("flex size-5 shrink-0 items-center justify-center rounded-md text-white", tintClass(process.app.tint))}>
          {process.app.iconUrl
            ? <img src={process.app.iconUrl} alt="" className="size-4 rounded-sm object-contain" />
            : Icon ? <Icon className="size-3" /> : null}
        </span>
        <span className="truncate text-xs font-medium">{title}</span>

        <div className="ml-auto flex items-center gap-0.5">
          {/* An app that refuses to be framed still has to be reachable. */}
          {process.app.url ? (
            <WindowButton label={i18next.t("desktop:Open in browser")} onClick={() => window.open(process.app.url, "_blank", "noreferrer")}>
              <ExternalLink className="size-3.5" />
            </WindowButton>
          ) : null}
          <WindowButton label={i18next.t("desktop:Minimize")} onClick={() => setSize(process.pid, WINDOW_SIZE.MINIMIZE)}>
            <Minus className="size-3.5" />
          </WindowButton>
          <WindowButton
            label={maximized ? i18next.t("desktop:Restore") : i18next.t("desktop:Maximize")}
            onClick={() => setSize(process.pid, maximized ? WINDOW_SIZE.WINDOWED : WINDOW_SIZE.MAXIMIZE)}
          >
            {maximized ? <Minimize2 className="size-3.5" /> : <Maximize2 className="size-3.5" />}
          </WindowButton>
          <WindowButton label={i18next.t("desktop:Close")} destructive onClick={() => closeApp(process.pid)}>
            <X className="size-3.5" />
          </WindowButton>
        </div>
      </div>

      <div className="relative min-h-0 flex-1">
        {children}
        {(!isFocused || drag) && (
          <div
            className="absolute inset-0 z-10"
            onPointerDown={() => focusApp(process.pid)}
          />
        )}
      </div>

      {!maximized && RESIZE_HANDLES.map((handle) => (
        <div
          key={handle.dir}
          className={cn("absolute z-20", handle.className)}
          onPointerDown={(event) => startPointer(event, handle.dir)}
        />
      ))}
    </div>
  );
}

function WindowButton({children, label, onClick, destructive}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className={cn(
        "text-muted-foreground flex size-6 items-center justify-center rounded-md transition-colors",
        destructive ? "hover:bg-destructive hover:text-white" : "hover:bg-foreground/10 hover:text-foreground"
      )}
      onPointerDown={(event) => event.stopPropagation()}
      onClick={(event) => {
        event.stopPropagation();
        onClick();
      }}
    >
      {children}
    </button>
  );
}

export default AppWindow;
