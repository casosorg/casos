import React, {createContext, useCallback, useContext, useMemo} from "react";
import {useHistory, useLocation} from "react-router-dom";
import {counterpartPath, isSimplePath, pathForMode} from "@/nav";

const STORAGE_KEY = "uiMode";

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
export function readUiMode() {
  try {
    return localStorage.getItem(STORAGE_KEY) === "advanced" ? "advanced" : "simple";
  } catch {
    return "simple";
  }
}

function writeUiMode(mode) {
  try {
    localStorage.setItem(STORAGE_KEY, mode);
  } catch {
    // Private-mode storage failures must not break the switch itself.
  }
}

const UiModeContext = createContext({
  mode: "simple",
  advanced: false,
  resolvePath: (path) => path,
  switchMode: () => {},
  toggleMode: () => {},
});

export function UiModeProvider({children}) {
  const location = useLocation();
  const history = useHistory();
  const mode = isSimplePath(location.pathname) ? "simple" : "advanced";

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
      toggleMode: () => switchMode(mode === "advanced" ? "simple" : "advanced"),
    }),
    [mode, switchMode]
  );

  return <UiModeContext.Provider value={value}>{children}</UiModeContext.Provider>;
}

export function useUiMode() {
  return useContext(UiModeContext);
}
