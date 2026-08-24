/**
 * The desktop half of the app SDK.
 *
 * An app hosted in a window is a separate document: it cannot read who is
 * signed in, what language the desktop is in, or how to open a sibling app.
 * This is the channel that answers those questions — a request/reply protocol
 * over postMessage, wire-compatible with sealos's client SDK so an app written
 * for that desktop runs here unchanged.
 */

/** Requests an app may send. The values are the wire names; do not rename. */
export const API_NAME = {
  USER_GET_INFO: "user.getInfo",
  EVENT_BUS: "event-bus",
  GET_LANGUAGE: "getLanguage",
  GET_WORKSPACE_QUOTA: "account.getWorkspaceQuota",
  GET_HOST_CONFIG: "getHostConfig",
};

/** Events the desktop pushes down to apps. */
export const EVENT_NAME = {
  GET_APPS: "get-apps",
  CHANGE_I18N: "change_i18n",
};

/** Events an app may ask the desktop to run, via the event bus. */
export const DESKTOP_EVENT = {
  OPEN_APP: "openDesktopApp",
  CLOSE_APP: "closeDesktopApp",
  GET_APPS: "getApps",
  SHOW_MESSAGE: "showMessage",
};

function originOf(url) {
  try {
    return new URL(url, window.location.origin).origin;
  } catch {
    return null;
  }
}

class MasterSDK {
  constructor(options) {
    this.options = options;
    this.eventBus = new Map();
    this.handlers = {
      [API_NAME.USER_GET_INFO]: (message, source, origin) => this.replyWith(message, source, origin, () => {
        const session = this.options.getSession?.();
        if (!session) {
          throw new Error("not signed in");
        }
        return session;
      }),
      [API_NAME.GET_LANGUAGE]: (message, source, origin) => this.replyWith(message, source, origin, () => ({
        lng: this.options.getLanguage?.() ?? "en",
      })),
      [API_NAME.GET_WORKSPACE_QUOTA]: (message, source, origin) => this.replyWith(message, source, origin, async() => ({
        quota: (await this.options.getWorkspaceQuota?.()) ?? [],
      })),
      [API_NAME.GET_HOST_CONFIG]: (message, source, origin) => this.replyWith(message, source, origin, () => this.options.getHostConfig?.() ?? {}),
      [API_NAME.EVENT_BUS]: (message, source, origin) => this.replyWith(message, source, origin, () => {
        const {eventName, eventData} = message.data ?? {};
        const handler = this.eventBus.get(eventName);
        if (!handler) {
          throw new Error(`event ${eventName} is not registered`);
        }
        return handler(eventData);
      }),
    };
  }

  /**
   * Only origins the operator put on this desktop may ask anything. The list is
   * read on every message rather than captured once, because installing an app
   * adds an origin while the desktop is already running.
   */
  isOriginAllowed(origin) {
    if (origin === window.location.origin) {
      return true;
    }
    const allowed = this.options.getAllowedOrigins?.() ?? [];
    return allowed.includes("*") || allowed.includes(origin);
  }

  reply({source, origin, messageId, success, message = "", data = {}}) {
    if (!source || source === window || !this.isOriginAllowed(origin)) {
      return;
    }
    source.postMessage({masterOrigin: window.location.origin, messageId, success, message, data}, {targetOrigin: origin});
  }

  async replyWith(message, source, origin, produce) {
    try {
      const data = await produce();
      this.reply({source, origin, messageId: message.messageId, success: true, data: data ?? {}});
    } catch (error) {
      this.reply({source, origin, messageId: message.messageId, success: false, message: error?.message ?? String(error)});
    }
  }

  init() {
    const onMessage = ({data, origin, source}) => {
      if (!source || !data || typeof data !== "object") {
        return;
      }
      const {apiName, messageId} = data;
      if (!apiName || !messageId) {
        return;
      }
      if (!this.isOriginAllowed(origin)) {
        this.reply({source, origin, messageId, success: false, message: "unauthorized origin"});
        return;
      }
      const handler = this.handlers[apiName];
      if (!handler) {
        this.reply({source, origin, messageId, success: false, message: `${apiName} is not declared`});
        return;
      }
      handler(data, source, origin);
    };

    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }

  addEventListen(name, fn) {
    this.eventBus.set(name, fn);
    return () => this.eventBus.delete(name);
  }

  removeEventListen(name) {
    this.eventBus.delete(name);
  }

  /** Pushes an event down to every app window that is currently mounted. */
  sendMessageToAll(payload) {
    document.querySelectorAll("iframe").forEach((iframe) => {
      const target = originOf(iframe.src);
      if (!target || target === "null") {
        return;
      }
      try {
        iframe.contentWindow?.postMessage(payload, target);
      } catch {
        // A frame that has navigated away is not worth reporting.
      }
    });
  }
}

export let masterApp = null;

export function createMasterApp(options) {
  masterApp = new MasterSDK(options);
  return masterApp.init();
}
