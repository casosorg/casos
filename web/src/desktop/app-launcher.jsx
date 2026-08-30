import React, {useEffect, useMemo, useState} from "react";
import i18next from "i18next";
import {Pin, PinOff, Search, X} from "lucide-react";
import {cn} from "@/lib/utils";
import {AppIcon} from "@/desktop/desktop-icons";

const PAGE_SIZE = 35;

/**
 * Launchpad: every app in one place, searchable, with the pin that decides
 * whether an app also sits on the desktop.
 */
export function AppLauncher({open, apps, layout, onClose, onOpenApp, onLayoutChange}) {
  const [term, setTerm] = useState("");
  const [page, setPage] = useState(0);

  useEffect(() => {
    if (!open) {
      return undefined;
    }
    setTerm("");
    setPage(0);
    function onKeyDown(event) {
      if (event.key === "Escape") {
        onClose();
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open, onClose]);

  const matches = useMemo(() => {
    const needle = term.trim().toLowerCase();
    if (!needle) {
      return apps;
    }
    return apps.filter((app) => {
      const label = app.labelKey ? i18next.t(app.labelKey) : (app.name ?? "");
      return label.toLowerCase().includes(needle) || app.key.toLowerCase().includes(needle);
    });
  }, [apps, term]);

  const pages = Math.max(1, Math.ceil(matches.length / PAGE_SIZE));
  const current = Math.min(page, pages - 1);
  const visible = matches.slice(current * PAGE_SIZE, (current + 1) * PAGE_SIZE);

  if (!open) {
    return null;
  }

  function togglePinned(app) {
    const onDesktop = layout.desktop.includes(app.key);
    onLayoutChange(
      onDesktop
        ? {desktop: layout.desktop.filter((key) => key !== app.key), folder: [...layout.folder, app.key]}
        : {desktop: [...layout.desktop, app.key], folder: layout.folder.filter((key) => key !== app.key)}
    );
  }

  return (
    <div
      data-testid="desktop-launcher"
      className="fixed inset-0 z-[1000] flex flex-col bg-black/45 backdrop-blur-2xl"
      onClick={onClose}
    >
      <div className="flex items-center justify-center gap-3 px-6 pt-10" onClick={(event) => event.stopPropagation()}>
        <div className="relative w-full max-w-md">
          <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-white/70" />
          <input
            autoFocus
            value={term}
            onChange={(event) => {
              setTerm(event.target.value);
              setPage(0);
            }}
            placeholder={i18next.t("general:Search apps")}
            className="h-10 w-full rounded-xl border border-white/20 bg-white/15 pr-3 pl-9 text-sm text-white placeholder:text-white/60 focus:outline-none"
          />
        </div>
        <button
          type="button"
          aria-label={i18next.t("general:Close")}
          onClick={onClose}
          className="flex size-10 items-center justify-center rounded-xl border border-white/20 bg-white/15 text-white"
        >
          <X className="size-4" />
        </button>
      </div>

      <div className="flex min-h-0 flex-1 items-start justify-center overflow-auto p-10" onClick={(event) => event.stopPropagation()}>
        <div className="grid w-full max-w-5xl grid-cols-3 gap-x-4 gap-y-8 sm:grid-cols-5 lg:grid-cols-7">
          {visible.map((app) => (
            <div key={app.key} className="group/pin relative">
              <AppIcon
                app={app}
                onOpen={(target) => {
                  onOpenApp(target);
                  onClose();
                }}
              />
              <button
                type="button"
                aria-label={i18next.t("desktop:Pin to desktop")}
                title={i18next.t("desktop:Pin to desktop")}
                onClick={(event) => {
                  event.stopPropagation();
                  togglePinned(app);
                }}
                className={cn(
                  "absolute -top-1 right-2 flex size-6 items-center justify-center rounded-full bg-white/90 text-neutral-800 opacity-0 shadow transition-opacity group-hover/pin:opacity-100"
                )}
              >
                {layout.desktop.includes(app.key) ? <PinOff className="size-3" /> : <Pin className="size-3" />}
              </button>
            </div>
          ))}
          {visible.length === 0 && (
            <p className="col-span-full py-16 text-center text-sm text-white/80">{i18next.t("desktop:No apps found")}</p>
          )}
        </div>
      </div>

      {pages > 1 && (
        <div className="flex items-center justify-center gap-2 pb-8" onClick={(event) => event.stopPropagation()}>
          {Array.from({length: pages}).map((_, index) => (
            <button
              key={index}
              type="button"
              aria-label={`${i18next.t("desktop:Page")} ${index + 1}`}
              onClick={() => setPage(index)}
              className={cn("size-2.5 rounded-full transition-colors", index === current ? "bg-white" : "bg-white/40")}
            />
          ))}
        </div>
      )}
    </div>
  );
}

export default AppLauncher;
