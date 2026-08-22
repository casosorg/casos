import React, {createContext, useCallback, useContext, useMemo, useState} from "react";

const STORAGE_KEY = "uiMode";

/**
 * Simple mode is the default: the navigation, the home page, the App Store and
 * the install dialog each show the smallest set of controls that still gets a
 * non-technical reader to a running app. Advanced mode restores the full
 * Kubernetes surface. Only the chrome reacts to this — every route stays
 * reachable by URL in either mode, so a bookmark never 404s.
 */
export function readUiMode() {
  try {
    return localStorage.getItem(STORAGE_KEY) === "advanced" ? "advanced" : "simple";
  } catch {
    return "simple";
  }
}

const UiModeContext = createContext({mode: "simple", advanced: false, setMode: () => {}, toggleMode: () => {}});

export function UiModeProvider({children}) {
  const [mode, setModeState] = useState(readUiMode);

  const setMode = useCallback((next) => {
    const value = next === "advanced" ? "advanced" : "simple";
    setModeState(value);
    try {
      localStorage.setItem(STORAGE_KEY, value);
    } catch {
      // Private-mode storage failures must not break the switch itself.
    }
  }, []);

  const value = useMemo(
    () => ({mode, advanced: mode === "advanced", setMode, toggleMode: () => setMode(mode === "advanced" ? "simple" : "advanced")}),
    [mode, setMode]
  );

  return <UiModeContext.Provider value={value}>{children}</UiModeContext.Provider>;
}

export function useUiMode() {
  return useContext(UiModeContext);
}
