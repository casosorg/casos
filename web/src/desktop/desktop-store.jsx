import React, {createContext, useContext, useMemo, useReducer} from "react";

/**
 * The window manager.
 *
 * A process is one open window: an app key, the address it opened at, and the
 * geometry the reader has since given it. Apps are single-instance, the way
 * they are on the sealos desktop — launching one that is already open raises
 * and restores it instead of stacking a second copy.
 */

const BASE_Z_INDEX = 10;

/** Beyond this many windows the least recently raised one is retired. */
const MAX_PROCESSES = 8;

const WINDOW_SIZE = {MAXIMIZE: "maximize", WINDOWED: "windowed", MINIMIZE: "minimize"};

export {WINDOW_SIZE};

function floatingGeometry(index) {
  const width = Math.min(1180, Math.round(window.innerWidth * 0.72));
  const height = Math.min(780, Math.round(window.innerHeight * 0.78));
  const offset = (index % 5) * 28;
  return {
    width,
    height,
    x: Math.max(16, Math.round((window.innerWidth - width) / 2) + offset),
    y: Math.max(72, Math.round((window.innerHeight - height) / 2) - 20 + offset),
  };
}

function reducer(state, action) {
  switch (action.type) {
  case "open": {
    const {app, path, size} = action;
    const existing = state.processes.find((process) => process.key === app.key);
    const zIndex = state.maxZIndex + 1;

    if (existing) {
      return {
        ...state,
        maxZIndex: zIndex,
        currentPid: existing.pid,
        processes: state.processes.map((process) =>
          process.pid === existing.pid
            ? {
              ...process,
              zIndex,
              path: path ?? process.path,
              // A launch always shows the window again, even when it was
              // sitting minimized in the dock.
              size: process.size === WINDOW_SIZE.MINIMIZE ? process.cacheSize : (size ?? process.size),
            }
            : process
        ),
      };
    }

    // The oldest window by stacking order is the one nobody has touched.
    const processes = state.processes.length >= MAX_PROCESSES
      ? state.processes.filter((process) => process.zIndex !== Math.min(...state.processes.map((item) => item.zIndex)))
      : state.processes;

    const pid = state.nextPid;
    const requested = size ?? WINDOW_SIZE.MAXIMIZE;
    return {
      ...state,
      nextPid: pid + 1,
      maxZIndex: zIndex,
      currentPid: pid,
      processes: [
        ...processes,
        {
          pid,
          key: app.key,
          app,
          path: path ?? app.path ?? "/",
          size: requested,
          cacheSize: requested === WINDOW_SIZE.MINIMIZE ? WINDOW_SIZE.MAXIMIZE : requested,
          zIndex,
          ...floatingGeometry(processes.length),
        },
      ],
    };
  }

  case "close": {
    const processes = state.processes.filter((process) => process.pid !== action.pid);
    return {
      ...state,
      processes,
      currentPid: state.currentPid === action.pid ? (processes[processes.length - 1]?.pid ?? -1) : state.currentPid,
    };
  }

  case "closeAll":
    return {...state, processes: [], currentPid: -1};

  case "focus": {
    const zIndex = state.maxZIndex + 1;
    return {
      ...state,
      maxZIndex: zIndex,
      currentPid: action.pid,
      processes: state.processes.map((process) =>
        process.pid === action.pid
          ? {...process, zIndex, size: process.size === WINDOW_SIZE.MINIMIZE ? process.cacheSize : process.size}
          : process
      ),
    };
  }

  case "setSize": {
    const zIndex = action.size === WINDOW_SIZE.MINIMIZE ? state.maxZIndex : state.maxZIndex + 1;
    return {
      ...state,
      maxZIndex: zIndex,
      currentPid: action.size === WINDOW_SIZE.MINIMIZE ? -1 : action.pid,
      processes: state.processes.map((process) => {
        if (process.pid !== action.pid) {
          return process;
        }
        return {
          ...process,
          size: action.size,
          // Minimizing has to remember what to restore to; the other two are
          // themselves the thing to restore to.
          cacheSize: action.size === WINDOW_SIZE.MINIMIZE ? process.size : action.size,
          zIndex: action.size === WINDOW_SIZE.MINIMIZE ? process.zIndex : zIndex,
        };
      }),
    };
  }

  case "setGeometry":
    return {
      ...state,
      processes: state.processes.map((process) =>
        process.pid === action.pid ? {...process, ...action.geometry} : process
      ),
    };

  case "minimizeAll": {
    const anyVisible = state.processes.some((process) => process.size !== WINDOW_SIZE.MINIMIZE);
    return {
      ...state,
      currentPid: anyVisible ? -1 : state.currentPid,
      processes: state.processes.map((process) => ({
        ...process,
        size: anyVisible ? WINDOW_SIZE.MINIMIZE : process.cacheSize,
        cacheSize: anyVisible ? process.size : process.cacheSize,
      })),
    };
  }

  default:
    return state;
  }
}

const initialState = {processes: [], maxZIndex: BASE_Z_INDEX, currentPid: -1, nextPid: 1};

const DesktopContext = createContext(null);

export function DesktopProvider({children, value}) {
  const [state, dispatch] = useReducer(reducer, initialState);

  const api = useMemo(() => ({
    openApp: (app, options = {}) => {
      if (app.kind === "link") {
        window.open(app.url, "_blank", "noreferrer");
        return;
      }
      dispatch({type: "open", app, path: options.path, size: options.size});
    },
    closeApp: (pid) => dispatch({type: "close", pid}),
    closeAll: () => dispatch({type: "closeAll"}),
    focusApp: (pid) => dispatch({type: "focus", pid}),
    setSize: (pid, size) => dispatch({type: "setSize", pid, size}),
    setGeometry: (pid, geometry) => dispatch({type: "setGeometry", pid, geometry}),
    minimizeAll: () => dispatch({type: "minimizeAll"}),
  }), []);

  const contextValue = useMemo(() => ({...state, ...api, ...value}), [state, api, value]);

  return <DesktopContext.Provider value={contextValue}>{children}</DesktopContext.Provider>;
}

export function useDesktop() {
  const context = useContext(DesktopContext);
  if (!context) {
    throw new Error("useDesktop must be used inside a DesktopProvider");
  }
  return context;
}
