import React, {useEffect, useRef} from "react";
import {Terminal} from "xterm";
import {FitAddon} from "xterm-addon-fit";
import * as Setting from "@/Setting";
import {cn} from "@/lib/utils";
import "xterm/css/xterm.css";

// The exec websocket multiplexes two channels on one binary connection: a
// leading 0 byte marks stdin, a leading 1 byte marks a JSON resize message.
const CHANNEL_STDIN = 0;
const CHANNEL_RESIZE = 1;

function frame(channel, payload) {
  const encoded = new TextEncoder().encode(payload);
  const buffer = new Uint8Array(1 + encoded.length);
  buffer[0] = channel;
  buffer.set(encoded, 1);
  return buffer;
}

/**
 * An interactive shell attached to one container, and nothing else: no chrome,
 * no picker. Whoever renders it decides where the shell appears — a sheet next
 * to a pod list, or a window on the desktop.
 *
 * `openDelay` exists because xterm measures its element to choose a grid size,
 * and measuring one that is still sliding into view gives a terminal sized for
 * a half-open pane.
 *
 * `endpoint` is what makes this reusable: the same pipe carries a shell in a
 * pod and a database's own client, because the backend decides which command to
 * run and both speak the same two-channel protocol.
 */
export function PodShell({namespace, name, container, className, openDelay = 0, endpoint = "/api/pod-terminal", params}) {
  const mountRef = useRef(null);
  const termRef = useRef(null);
  const fitAddonRef = useRef(null);
  const socketRef = useRef(null);

  useEffect(() => {
    if (!namespace || !name) {
      return undefined;
    }

    function cleanup() {
      socketRef.current?.close();
      socketRef.current = null;
      termRef.current?.dispose();
      termRef.current = null;
    }

    function sendResize(cols, rows) {
      if (socketRef.current?.readyState === WebSocket.OPEN) {
        socketRef.current.send(frame(CHANNEL_RESIZE, JSON.stringify({cols, rows})));
      }
    }

    function open() {
      const term = new Terminal({
        cursorBlink: true,
        fontSize: 14,
        fontFamily: "'Cascadia Code', 'Fira Mono', Consolas, monospace",
        theme: {background: "#0a0a0a", foreground: "#d4d4d4", cursor: "#60a5fa"},
      });
      const fitAddon = new FitAddon();
      term.loadAddon(fitAddon);
      termRef.current = term;
      fitAddonRef.current = fitAddon;

      if (mountRef.current) {
        term.open(mountRef.current);
        fitAddon.fit();
      }

      const socket = new WebSocket(Setting.getWebSocketUrl(endpoint, params ?? {namespace, name, container}));
      socket.binaryType = "arraybuffer";
      socketRef.current = socket;

      socket.onopen = () => sendResize(term.cols, term.rows);
      socket.onmessage = (event) => {
        if (termRef.current !== term) {
          return;
        }
        term.write(typeof event.data === "string" ? event.data : new Uint8Array(event.data));
      };
      socket.onclose = () => {
        if (termRef.current === term) {
          term.write("\r\n\x1b[31m[connection closed]\x1b[0m\r\n");
        }
      };
      socket.onerror = () => {
        if (termRef.current === term) {
          term.write("\r\n\x1b[31m[websocket error]\x1b[0m\r\n");
        }
      };

      term.onData((data) => {
        if (socketRef.current?.readyState === WebSocket.OPEN) {
          socketRef.current.send(frame(CHANNEL_STDIN, data));
        }
      });
      term.onResize(({cols, rows}) => sendResize(cols, rows));
    }

    const timer = setTimeout(open, openDelay);
    return () => {
      clearTimeout(timer);
      cleanup();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [namespace, name, container, openDelay, endpoint, JSON.stringify(params ?? null)]);

  // A window being dragged wider has to hand the new grid size to the shell on
  // the other end, or the remote program keeps wrapping at the old width.
  useEffect(() => {
    const node = mountRef.current;
    if (!node || typeof ResizeObserver === "undefined") {
      return undefined;
    }
    const observer = new ResizeObserver(() => fitAddonRef.current?.fit());
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  return <div ref={mountRef} className={cn("min-h-0 flex-1", className)} />;
}

export default PodShell;
