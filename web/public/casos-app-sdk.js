/**
 * The app half of the desktop SDK, served as a plain script so an app hosted in
 * a casos window can talk to the desktop without a build step:
 *
 *   <script src="https://<casos-host>/casos-app-sdk.js"></script>
 *   const session = await casosApp.getSession();
 *
 * The protocol is the one sealos's client SDK speaks, and `sealosApp` is
 * aliased to the same object, so an app written for that desktop needs no
 * change to run here.
 */
(function () {
  "use strict";

  var API_NAME = {
    USER_GET_INFO: "user.getInfo",
    EVENT_BUS: "event-bus",
    GET_LANGUAGE: "getLanguage",
    GET_WORKSPACE_QUOTA: "account.getWorkspaceQuota",
    GET_HOST_CONFIG: "getHostConfig"
  };

  var WINDOW_SIZE = {maximized: "maximize", windowed: "windowed", minimized: "minimize"};

  function uuid() {
    if (window.crypto && window.crypto.randomUUID) {
      return window.crypto.randomUUID();
    }
    return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, function (c) {
      var r = (Math.random() * 16) | 0;
      return (c === "x" ? r : (r & 0x3) | 0x8).toString(16);
    });
  }

  function ClientSDK() {
    this.initialized = false;
    this.desktopOrigin = "*";
    this.callbacks = new Map();
    this.eventBus = new Map();
    this.session = null;
  }

  ClientSDK.prototype.send = function (apiName, data) {
    var self = this;
    if (!this.initialized) {
      return Promise.reject(new Error("app sdk is not initialized"));
    }
    var messageId = uuid();
    var promise = new Promise(function (resolve, reject) {
      var timer = setTimeout(function () {
        self.callbacks.delete(messageId);
        reject(new Error("desktop did not answer in time"));
      }, 10000);

      self.callbacks.set(messageId, function (reply) {
        clearTimeout(timer);
        if (reply.success) {
          resolve(reply.data);
        } else {
          reject(new Error(reply.message || "request failed"));
        }
      });
    });

    (window.top || window.parent).postMessage(
      {messageId: messageId, apiName: apiName, clientLocation: window.location.origin, data: data || {}},
      this.desktopOrigin
    );

    return promise;
  };

  ClientSDK.prototype.init = function () {
    var self = this;

    function onMessage(event) {
      var data = event.data;
      if (!data || typeof data !== "object" || !event.source) {
        return;
      }
      // A desktop-initiated event, rather than a reply to something we asked.
      if (data.apiName === API_NAME.EVENT_BUS && data.eventName) {
        var handler = self.eventBus.get(data.eventName);
        if (handler) {
          handler(data.data);
        }
        return;
      }
      if (data.messageId && self.callbacks.has(data.messageId)) {
        self.desktopOrigin = event.origin;
        self.callbacks.get(data.messageId)(data);
        self.callbacks.delete(data.messageId);
      }
    }

    window.addEventListener("message", onMessage);
    this.initialized = true;

    return function () {
      window.removeEventListener("message", onMessage);
      self.initialized = false;
    };
  };

  ClientSDK.prototype.getSession = function () {
    var self = this;
    if (this.session) {
      return Promise.resolve(this.session);
    }
    return this.send(API_NAME.USER_GET_INFO).then(function (session) {
      self.session = session;
      return session;
    });
  };

  ClientSDK.prototype.getLanguage = function () {
    return this.send(API_NAME.GET_LANGUAGE);
  };

  ClientSDK.prototype.getWorkspaceQuota = function () {
    return this.send(API_NAME.GET_WORKSPACE_QUOTA);
  };

  ClientSDK.prototype.getHostConfig = function () {
    return this.send(API_NAME.GET_HOST_CONFIG);
  };

  ClientSDK.prototype.runEvents = function (eventName, eventData) {
    return this.send(API_NAME.EVENT_BUS, {eventName: eventName, eventData: eventData});
  };

  ClientSDK.prototype.openApp = function (options) {
    var opts = options || {};
    var payload = {};
    for (var key in opts) {
      if (Object.prototype.hasOwnProperty.call(opts, key) && key !== "appSize") {
        payload[key] = opts[key];
      }
    }
    payload.pathname = opts.pathname || "/";
    if (opts.appSize) {
      payload.appSize = WINDOW_SIZE[opts.appSize] || opts.appSize;
    }
    return this.runEvents("openDesktopApp", payload);
  };

  ClientSDK.prototype.closeApp = function () {
    return this.runEvents("closeDesktopApp", {});
  };

  ClientSDK.prototype.getApps = function () {
    return this.runEvents("getApps", {});
  };

  ClientSDK.prototype.showMessage = function (message) {
    return this.runEvents("showMessage", message);
  };

  ClientSDK.prototype.addAppEventListen = function (name, fn) {
    var self = this;
    this.eventBus.set(name, fn);
    return function () {
      self.eventBus.delete(name);
    };
  };

  ClientSDK.prototype.removeAppEventListen = function (name) {
    this.eventBus.delete(name);
  };

  var app = new ClientSDK();
  app.init();

  window.casosApp = app;
  // Drop-in compatibility for apps built against the sealos desktop.
  if (!window.sealosApp) {
    window.sealosApp = app;
  }
})();
