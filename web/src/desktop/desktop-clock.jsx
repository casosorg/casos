import React, {useEffect, useState} from "react";
import i18next from "i18next";
import {cn} from "@/lib/utils";

function pad(value) {
  return String(value).padStart(2, "0");
}

/** The clock the desktop wears above its icons. */
export function DesktopClock({light}) {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const timer = setInterval(() => setNow(new Date()), 10000);
    return () => clearInterval(timer);
  }, []);

  const locale = i18next.language === "zh" ? "zh-CN" : "en-US";
  const date = now.toLocaleDateString(locale, {year: "numeric", month: "long", day: "numeric", weekday: "long"});

  return (
    <div
      data-testid="desktop-clock"
      className={cn(
        "flex select-none flex-col items-center",
        light ? "text-neutral-900" : "text-white drop-shadow-[0_1px_3px_rgba(0,0,0,0.5)]"
      )}
    >
      <span className="text-5xl font-semibold tracking-tight tabular-nums sm:text-6xl">
        {pad(now.getHours())}:{pad(now.getMinutes())}
      </span>
      <span className="mt-1 text-sm font-medium">{date}</span>
    </div>
  );
}

export default DesktopClock;
