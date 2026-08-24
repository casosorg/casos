/**
 * What the desktop remembers between visits: where the icons sit, which
 * wallpaper is up, and whether the dock is tucked away. All of it is a display
 * preference, so it lives in localStorage rather than on the cluster.
 */

const LAYOUT_KEY = "desktopLayout";
const WALLPAPER_KEY = "desktopWallpaper";
const DOCK_KEY = "desktopDockHidden";

export const WALLPAPERS = [
  {
    key: "aurora",
    labelKey: "desktop:Aurora",
    style: {backgroundImage: "linear-gradient(135deg, #0f2027 0%, #203a43 45%, #2c5364 100%)"},
  },
  {
    key: "daylight",
    labelKey: "desktop:Daylight",
    style: {backgroundImage: "linear-gradient(160deg, #a1c4fd 0%, #c2e9fb 55%, #e2ebf0 100%)"},
  },
  {
    key: "dusk",
    labelKey: "desktop:Dusk",
    style: {backgroundImage: "linear-gradient(150deg, #1e3c72 0%, #2a5298 50%, #6a3093 100%)"},
  },
  {
    key: "sand",
    labelKey: "desktop:Sand",
    style: {backgroundImage: "linear-gradient(140deg, #fdfbfb 0%, #ebedee 40%, #dfe9f3 100%)"},
  },
  {
    key: "forest",
    labelKey: "desktop:Forest",
    style: {backgroundImage: "linear-gradient(140deg, #0b3d2e 0%, #11694f 55%, #2fa37c 100%)"},
  },
  {
    key: "graphite",
    labelKey: "desktop:Graphite",
    style: {backgroundImage: "linear-gradient(145deg, #16181d 0%, #23262d 55%, #33373f 100%)"},
  },
];

/** Wallpapers light enough that desktop labels need dark text. */
const LIGHT_WALLPAPERS = new Set(["daylight", "sand"]);

export function isLightWallpaper(key) {
  return LIGHT_WALLPAPERS.has(key);
}

export function readWallpaper() {
  try {
    const stored = localStorage.getItem(WALLPAPER_KEY);
    return WALLPAPERS.some((paper) => paper.key === stored) ? stored : WALLPAPERS[0].key;
  } catch {
    return WALLPAPERS[0].key;
  }
}

export function writeWallpaper(key) {
  try {
    localStorage.setItem(WALLPAPER_KEY, key);
  } catch {
    // A private-mode storage failure must not stop the wallpaper changing.
  }
}

export function readDockHidden() {
  try {
    return localStorage.getItem(DOCK_KEY) === "true";
  } catch {
    return false;
  }
}

export function writeDockHidden(hidden) {
  try {
    localStorage.setItem(DOCK_KEY, String(hidden));
  } catch {
    // Ignored for the same reason as above.
  }
}

export function readLayout() {
  try {
    const parsed = JSON.parse(localStorage.getItem(LAYOUT_KEY) ?? "null");
    if (!parsed || !Array.isArray(parsed.desktop) || !Array.isArray(parsed.folder)) {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

export function writeLayout(layout) {
  try {
    localStorage.setItem(LAYOUT_KEY, JSON.stringify(layout));
  } catch {
    // Ignored: a lost arrangement is better than a broken drag.
  }
}

/**
 * Reconciles a stored arrangement with the apps that actually exist now.
 * Apps nobody has placed yet land on the desktop — an app someone just
 * installed should be where they can see it, not filed away in the folder.
 */
export function reconcileLayout(stored, apps, defaultDesktopKeys) {
  const known = new Set(apps.map((app) => app.key));
  if (!stored) {
    const desktop = defaultDesktopKeys.filter((key) => known.has(key));
    const placed = new Set(desktop);
    return {desktop, folder: apps.filter((app) => !placed.has(app.key)).map((app) => app.key)};
  }

  const desktop = stored.desktop.filter((key) => known.has(key));
  const folder = stored.folder.filter((key) => known.has(key) && !desktop.includes(key));
  const placed = new Set([...desktop, ...folder]);
  const added = apps.filter((app) => !placed.has(app.key)).map((app) => app.key);
  return {desktop: [...desktop, ...added], folder};
}
