import React, {createContext, useCallback, useContext, useEffect, useMemo, useState} from "react";
import * as NamespaceBackend from "@/backend/NamespaceBackend";

const STORAGE_KEY = "workspaceNamespace";

/** The workspace that means "do not narrow anything". */
export const ALL_WORKSPACES = "";

const WorkspaceContext = createContext(null);

function readWorkspace() {
  try {
    return localStorage.getItem(STORAGE_KEY) ?? ALL_WORKSPACES;
  } catch {
    return ALL_WORKSPACES;
  }
}

function writeWorkspace(namespace) {
  try {
    if (namespace) {
      localStorage.setItem(STORAGE_KEY, namespace);
    } else {
      localStorage.removeItem(STORAGE_KEY);
    }
  } catch {
    // A workspace that does not survive a reload is still a workspace.
  }
}

/**
 * The namespace the reader is working in.
 *
 * casos can see the whole cluster, but almost nobody works on the whole cluster
 * at once. Choosing a workspace narrows the lists to it and decides where a new
 * app or database lands, without hiding that the rest of the cluster exists —
 * "all namespaces" stays one click away, and it is what a fresh install starts
 * on.
 */
export function WorkspaceProvider({signedIn = false, children}) {
  const [workspace, setWorkspaceState] = useState(readWorkspace);
  const [namespaces, setNamespaces] = useState([]);

  const refresh = useCallback(() => {
    // Namespaces are a signed-in question; asking on the sign-in page would only
    // produce a denial.
    if (!signedIn) {
      return;
    }
    NamespaceBackend.getNamespaces()
      .then((res) => {
        if (res.status !== "ok") {
          return;
        }
        setNamespaces((res.data ?? []).map((item) => item.name));
      })
      .catch(() => {
        // The selector simply stays empty; every page still works unnarrowed.
      });
  }, [signedIn]);

  useEffect(refresh, [refresh]);

  // A workspace whose namespace was deleted would silently hide everything.
  useEffect(() => {
    if (workspace && namespaces.length > 0 && !namespaces.includes(workspace)) {
      setWorkspaceState(ALL_WORKSPACES);
      writeWorkspace(ALL_WORKSPACES);
    }
  }, [workspace, namespaces]);

  const setWorkspace = useCallback((namespace) => {
    setWorkspaceState(namespace);
    writeWorkspace(namespace);
  }, []);

  const value = useMemo(() => ({workspace, setWorkspace, namespaces, refresh}), [workspace, setWorkspace, namespaces, refresh]);

  return <WorkspaceContext.Provider value={value}>{children}</WorkspaceContext.Provider>;
}

/**
 * Usable outside the provider — a window, a dialog rendered through a portal —
 * where it reports "all namespaces" rather than throwing.
 */
export function useWorkspace() {
  return useContext(WorkspaceContext) ?? {workspace: ALL_WORKSPACES, setWorkspace: () => {}, namespaces: [], refresh: () => {}};
}
