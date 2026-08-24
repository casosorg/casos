/**
 * What the desktop remembers between visits: where the icons sit, which
 * wallpaper is up, and whether the dock is tucked away. All of it is a display
 * preference, so it lives in localStorage rather than on the cluster.
 */

const LAYOUT_KEY = "desktopLayout";
const WALLPAPER_KEY = "desktopWallpaper";
const CUSTOM_WALLPAPER_KEY = "desktopCustomWallpaper";
const DOCK_KEY = "desktopDockHidden";
const DOCK_MODE_KEY = "desktopDockMode";
const TOUR_KEY = "desktopTourSeen";

/** The two shapes the launcher strip can take, as sealos's app bar toggle does. */
export const DOCK_MODE = {DOCK: "dock", BAR: "bar"};

/** The key a wallpaper of the reader's own is filed under. */
export const CUSTOM_WALLPAPER = "custom";

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
    if (stored === CUSTOM_WALLPAPER) {
      return readCustomWallpaper() ? CUSTOM_WALLPAPER : WALLPAPERS[0].key;
    }
    return WALLPAPERS.some((paper) => paper.key === stored) ? stored : WALLPAPERS[0].key;
  } catch {
    return WALLPAPERS[0].key;
  }
}

/** The reader's own wallpaper: an address, or an image they dropped in. */
export function readCustomWallpaper() {
  try {
    return localStorage.getItem(CUSTOM_WALLPAPER_KEY) || "";
  } catch {
    return "";
  }
}

export function writeCustomWallpaper(source) {
  try {
    if (source) {
      localStorage.setItem(CUSTOM_WALLPAPER_KEY, source);
    } else {
      localStorage.removeItem(CUSTOM_WALLPAPER_KEY);
    }
  } catch {
    // An image too large for storage simply does not persist.
  }
}

/** What to paint the desktop with, whichever kind of wallpaper is chosen. */
export function wallpaperStyle(key, custom) {
  if (key === CUSTOM_WALLPAPER && custom) {
    return {backgroundImage: `url(${JSON.stringify(custom)})`, backgroundSize: "cover", backgroundPosition: "center"};
  }
  return WALLPAPERS.find((paper) => paper.key === key)?.style ?? WALLPAPERS[0].style;
}

export function readDockMode() {
  try {
    return localStorage.getItem(DOCK_MODE_KEY) === DOCK_MODE.BAR ? DOCK_MODE.BAR : DOCK_MODE.DOCK;
  } catch {
    return DOCK_MODE.DOCK;
  }
}

export function writeDockMode(mode) {
  try {
    localStorage.setItem(DOCK_MODE_KEY, mode);
  } catch {
    // Ignored for the same reason as above.
  }
}

export function readTourSeen() {
  try {
    return localStorage.getItem(TOUR_KEY) === "true";
  } catch {
    return true;
  }
}

export function writeTourSeen(seen) {
  try {
    localStorage.setItem(TOUR_KEY, String(seen));
  } catch {
    // A tour that shows twice is better than one that crashes the desktop.
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
