import React, {createContext, useCallback, useContext, useMemo} from "react";
import {useHistory, useLocation} from "react-router-dom";
import {counterpartPath, isDesktopPath, isSimplePath, pathForMode} from "@/nav";

const STORAGE_KEY = "uiMode";
// Which of the two page modes the desktop was entered from, so leaving it
// lands a reader back where they were rather than always on the advanced
// dashboard.
const PAGE_MODE_KEY = "uiPageMode";

/**
 * Simple mode is the default: the navigation, the home page, the App Store and
 * the install dialog each show the smallest set of controls that still gets a
 * non-technical reader to a running app. Advanced mode restores the full
 * Kubernetes surface.
 *
 * Which of the two a page renders in is decided by its URL — simple mode's
 * pages live under /simple — so no address ever means two different screens and
 * a shared link opens what its sender was looking at. The stored value below is
 * only a preference: it decides where "/" lands, and the mode switch keeps it up
 * to date.
 */
const MODES = ["simple", "advanced", "desktop"];

export function readUiMode() {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    return MODES.includes(stored) ? stored : "simple";
  } catch {
    return "simple";
  }
}

function writeUiMode(mode) {
  try {
    localStorage.setItem(STORAGE_KEY, mode);
    if (mode !== "desktop") {
      localStorage.setItem(PAGE_MODE_KEY, mode);
    }
  } catch {
    // Private-mode storage failures must not break the switch itself.
  }
}

/** The page mode to return to when the desktop is closed. */
export function readPageMode() {
  try {
    const stored = localStorage.getItem(PAGE_MODE_KEY);
    return stored === "advanced" ? "advanced" : "simple";
  } catch {
    return "simple";
  }
}

const UiModeContext = createContext({
  mode: "simple",
  advanced: false,
  resolvePath: (path) => path,
  switchMode: () => {},
  toggleMode: () => {},
  openDesktop: () => {},
  exitDesktop: () => {},
});

export function UiModeProvider({children}) {
  const location = useLocation();
  const history = useHistory();
  const mode = isDesktopPath(location.pathname)
    ? "desktop"
    : isSimplePath(location.pathname) ? "simple" : "advanced";

  const switchMode = useCallback((next, path) => {
    writeUiMode(next);
    history.push(path ?? counterpartPath(location.pathname, next));
  }, [history, location.pathname]);

  const value = useMemo(
    () => ({
      mode,
      advanced: mode === "advanced",
      // Pages both modes render link through this, so a simple-mode reader is
      // never bounced into the advanced surface by a button.
      resolvePath: (advancedPath) => pathForMode(advancedPath, mode),
      switchMode,
      openDesktop: () => switchMode("desktop"),
      exitDesktop: () => switchMode(readPageMode()),
      // The toggle only ever moves between the two page-based modes; the
      // desktop is reached by its own control, and leaving it lands wherever
      // the reader was before.
      toggleMode: () => switchMode(mode === "advanced" ? "simple" : "advanced"),
    }),
    [mode, switchMode]
  );

  return <UiModeContext.Provider value={value}>{children}</UiModeContext.Provider>;
}

export function useUiMode() {
  return useContext(UiModeContext);
}
