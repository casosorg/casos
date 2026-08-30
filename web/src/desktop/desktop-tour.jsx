import React, {useEffect, useLayoutEffect, useState} from "react";
import i18next from "i18next";
import {Button} from "@/components/ui/button";

const PADDING = 8;

/**
 * The first-run tour.
 *
 * Each step points at something already on screen rather than describing it, so
 * the reader learns where a control is at the same time as what it does. A step
 * whose target is missing — a dock that is hidden, a search box a narrow screen
 * dropped — is skipped rather than pointing at nothing.
 */
const STEPS = [
  {
    selector: "[data-testid='desktop-icons']",
    titleKey: "onboarding:Your apps",
    bodyKey: "onboarding:Every screen casos has is an app here. Click one to open it in a window.",
  },
  {
    selector: "[data-testid='desktop-dock'], [data-testid='desktop-appbar']",
    titleKey: "onboarding:The dock",
    bodyKey: "onboarding:Pinned apps and anything running. Click a running app to put it away and again to bring it back.",
  },
  {
    selector: "[data-testid='desktop-search']",
    titleKey: "onboarding:Find anything",
    bodyKey: "onboarding:Type an app's name to launch it without hunting for its icon.",
  },
  {
    selector: "[data-testid='desktop-notifications']",
    titleKey: "onboarding:What the cluster is saying",
    bodyKey: "onboarding:Warnings the cluster recorded collect here, newest first.",
  },
  {
    selector: "[data-testid='desktop']",
    titleKey: "onboarding:Make it yours",
    bodyKey: "onboarding:Right-click the desktop to change the wallpaper, switch the dock to a task bar, or rearrange the icons.",
    whole: false,
  },
];

function rectOf(selector) {
  const node = document.querySelector(selector);
  if (!node) {
    return null;
  }
  const rect = node.getBoundingClientRect();
  if (rect.width === 0 || rect.height === 0) {
    return null;
  }
  return rect;
}

export function DesktopTour({open, onClose}) {
  const [index, setIndex] = useState(0);
  const [rect, setRect] = useState(null);

  const visible = open ? STEPS.filter((step) => (step.whole === false ? true : rectOf(step.selector) !== null)) : [];
  const step = visible[index];

  useLayoutEffect(() => {
    if (!step) {
      return undefined;
    }
    function measure() {
      setRect(step.whole === false ? null : rectOf(step.selector));
    }
    measure();
    window.addEventListener("resize", measure);
    return () => window.removeEventListener("resize", measure);
  }, [step]);

  useEffect(() => {
    if (!open) {
      setIndex(0);
    }
  }, [open]);

  if (!open || !step) {
    return null;
  }

  const spotlight = rect
    ? {
      top: rect.top - PADDING,
      left: rect.left - PADDING,
      width: rect.width + PADDING * 2,
      height: rect.height + PADDING * 2,
    }
    : null;

  // The card sits under the highlight when there is room, and over it otherwise.
  const cardTop = spotlight
    ? spotlight.top + spotlight.height + 12 > window.innerHeight - 190
      ? Math.max(16, spotlight.top - 178)
      : spotlight.top + spotlight.height + 12
    : Math.round(window.innerHeight / 2 - 90);
  const cardLeft = spotlight
    ? Math.min(Math.max(16, spotlight.left), window.innerWidth - 360)
    : Math.round(window.innerWidth / 2 - 172);

  const last = index === visible.length - 1;

  return (
    <div data-testid="desktop-tour" className="fixed inset-0 z-[1200]">
      {spotlight ? (
        <div
          className="pointer-events-none absolute rounded-xl ring-2 ring-white/80 transition-all duration-200"
          style={{...spotlight, boxShadow: "0 0 0 9999px rgba(0,0,0,0.55)"}}
        />
      ) : (
        <div className="absolute inset-0 bg-black/55" />
      )}

      <div
        className="bg-popover text-popover-foreground absolute w-[344px] rounded-xl border p-4 shadow-2xl"
        style={{top: cardTop, left: cardLeft}}
      >
        <p className="text-sm font-semibold">{i18next.t(step.titleKey)}</p>
        <p className="text-muted-foreground mt-1 text-sm">{i18next.t(step.bodyKey)}</p>
        <div className="mt-4 flex items-center justify-between">
          <span className="text-muted-foreground text-xs">{index + 1} / {visible.length}</span>
          <div className="flex gap-2">
            <Button variant="ghost" size="sm" onClick={onClose}>
              {i18next.t("onboarding:Skip")}
            </Button>
            <Button size="sm" onClick={() => (last ? onClose() : setIndex((current) => current + 1))}>
              {last ? i18next.t("general:Done") : i18next.t("onboarding:Next")}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

export default DesktopTour;
