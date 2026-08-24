import React from "react";
import {MemoryRouter} from "react-router-dom";
import {AppRoutes} from "@/routes";
import {UiModeProvider} from "@/hooks/use-ui-mode";
import {APP_KIND} from "@/desktop/registry";
import {useDesktop} from "@/desktop/desktop-store";

/**
 * What lives inside a window.
 *
 * Ours render the same route table the sidebar UI serves, each under a router
 * of its own, so two windows can sit on different pages at once and neither
 * one's navigation touches the browser address bar. Everything else — an
 * installed chart's own web UI — is an iframe, which is how every sealos app is
 * hosted.
 */
export function AppFrame({process}) {
  const {appProps} = useDesktop();

  if (process.app.kind === APP_KIND.IFRAME) {
    return (
      <iframe
        title={process.app.name ?? process.key}
        src={process.app.url}
        id={`app-window-${process.key}`}
        className="size-full border-0 bg-white"
        allow="camera;microphone;clipboard-write;clipboard-read;fullscreen"
      />
    );
  }

  return (
    <MemoryRouter initialEntries={[process.path]}>
      <UiModeProvider>
        <div className="bg-muted/30 size-full overflow-auto">
          <AppRoutes {...appProps} />
        </div>
      </UiModeProvider>
    </MemoryRouter>
  );
}

export default AppFrame;
