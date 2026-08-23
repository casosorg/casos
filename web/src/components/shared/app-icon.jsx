import * as React from "react";
import {appTileGradient, chartIconUrl} from "@/lib/appCatalog";
import {cn} from "@/lib/utils";

const SIZES = {
  sm: {box: "size-8 rounded-lg", text: "text-xs", pad: "p-1"},
  md: {box: "size-11 rounded-xl", text: "text-base", pad: "p-1.5"},
  lg: {box: "size-14 rounded-2xl", text: "text-xl", pad: "p-2"},
};

/**
 * An app's own logo, with a coloured monogram standing in wherever there is no
 * logo to show. Chart logos are remote URLs — the chart declares one, the
 * cluster hands it back with the release — so the monogram is not only for
 * charts that declare nothing: it is also what a card falls back to when the
 * image cannot be fetched, which is the normal case on a cluster with no way
 * out to the internet.
 *
 * Logos are drawn on white at every theme: they are authored for a light
 * background, and several of them are dark line art that would vanish on a
 * dark tile.
 */
export function AppIcon({src, chartName, name, size = "md", className}) {
  const url = src || chartIconUrl(chartName) || null;
  const [failed, setFailed] = React.useState(false);
  React.useEffect(() => setFailed(false), [url]);

  const label = (name || chartName || "?").trim().charAt(0).toUpperCase() || "?";
  const style = SIZES[size] ?? SIZES.md;

  if (!url || failed) {
    return (
      <span
        aria-hidden="true"
        className={cn(
          "flex shrink-0 items-center justify-center font-semibold text-white shadow-sm",
          style.box,
          style.text,
          className
        )}
        style={{backgroundImage: appTileGradient(chartName || name || "")}}
      >
        {label}
      </span>
    );
  }

  return (
    <span
      aria-hidden="true"
      className={cn(
        "flex shrink-0 items-center justify-center overflow-hidden border border-black/5 bg-white shadow-sm dark:border-white/10",
        style.box,
        className
      )}
    >
      <img
        src={url}
        alt=""
        onError={() => setFailed(true)}
        className={cn("size-full object-contain", style.pad)}
      />
    </span>
  );
}
