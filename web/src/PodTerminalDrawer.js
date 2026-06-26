import React, {useEffect, useRef} from "react";
import {Drawer, Select, Space} from "antd";
import {Terminal} from "xterm";
import {FitAddon} from "xterm-addon-fit";
import * as Setting from "./Setting";
import "xterm/css/xterm.css";

/**
 * Props:
 *   pod      {object|null}  pod summary with {namespace, name, containers:[]}
 *   open     {boolean}
 *   onClose  {Function}
 */
function PodTerminalDrawer({pod, open, onClose}) {
  const containerRef = useRef(null);
  const termRef = useRef(null);
  const fitAddonRef = useRef(null);
  const wsRef = useRef(null);
  const selectedContainer = useRef("");
  const [container, setContainer] = React.useState("");

  function cleanup() {
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    if (termRef.current) {
      termRef.current.dispose();
      termRef.current = null;
    }
  }

  function sendResize(cols, rows) {
    if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {return;}
    const payload = JSON.stringify({cols, rows});
    const enc = new TextEncoder().encode(payload);
    const frame = new Uint8Array(1 + enc.length);
    frame[0] = 1;
    frame.set(enc, 1);
    wsRef.current.send(frame);
  }

  function openTerminal(ns, name, ctr) {
    cleanup();

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: "'Cascadia Code', 'Fira Mono', Consolas, monospace",
      theme: {
        background: "#0d1117",
        foreground: "#c9d1d9",
        cursor: "#58a6ff",
      },
    });
    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    termRef.current = term;
    fitAddonRef.current = fitAddon;

    if (containerRef.current) {
      term.open(containerRef.current);
      fitAddon.fit();
    }

    const ws = new WebSocket(Setting.getWebSocketUrl("/api/pod-terminal", {
      namespace: ns,
      name,
      container: ctr,
    }));
    ws.binaryType = "arraybuffer";
    wsRef.current = ws;

    ws.onopen = () => {
      // send initial size
      sendResize(term.cols, term.rows);
    };

    ws.onmessage = (e) => {
      if (termRef.current !== term) {return;}
      if (typeof e.data === "string") {
        term.write(e.data);
        return;
      }

      term.write(new Uint8Array(e.data));
    };

    ws.onclose = () => {
      if (termRef.current === term) {
        term.write("\r\n\x1b[31m[connection closed]\x1b[0m\r\n");
      }
    };

    ws.onerror = () => {
      if (termRef.current === term) {
        term.write("\r\n\x1b[31m[websocket error]\x1b[0m\r\n");
      }
    };

    // stdin: send user keystrokes
    term.onData((data) => {
      if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {return;}
      const enc = new TextEncoder().encode(data);
      const frame = new Uint8Array(1 + enc.length);
      frame[0] = 0;
      frame.set(enc, 1);
      wsRef.current.send(frame);
    });

    // resize
    term.onResize(({cols, rows}) => {
      sendResize(cols, rows);
    });
  }

  // mount/unmount the terminal DOM element when the drawer opens
  useEffect(() => {
    if (!open || !pod) {
      cleanup();
      return;
    }
    const defaultCtr = pod.containers?.[0] ?? "";
    setContainer(defaultCtr);
    selectedContainer.current = defaultCtr;
    // slight delay to let the Drawer finish animating before measuring
    const t = setTimeout(() => {
      openTerminal(pod.namespace, pod.name, defaultCtr);
    }, 200);
    return () => {
      clearTimeout(t);
      cleanup();
    };
  }, [open, pod]);

  // handle window resize → refit the terminal
  useEffect(() => {
    if (!open) {return;}
    const handleResize = () => {
      if (fitAddonRef.current && termRef.current) {
        fitAddonRef.current.fit();
      }
    };
    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, [open]);

  function switchContainer(ctr) {
    setContainer(ctr);
    selectedContainer.current = ctr;
    if (pod) {
      openTerminal(pod.namespace, pod.name, ctr);
    }
  }

  const containerOptions = (pod?.containers ?? []).map(c => ({label: c, value: c}));
  const multiContainer = containerOptions.length > 1;
  const drawerTitle = pod ? `Terminal — ${pod.namespace} / ${pod.name}` : "Terminal";

  return (
    <Drawer
      title={drawerTitle}
      open={open}
      onClose={onClose}
      width={900}
      destroyOnHidden
      extra={
        multiContainer ? (
          <Space>
            <Select
              size="small"
              value={container}
              onChange={switchContainer}
              options={containerOptions}
              style={{width: 160}}
              placeholder="Container"
            />
          </Space>
        ) : null
      }
      styles={{body: {padding: "12px 16px", background: "#0d1117"}}}
    >
      <div
        ref={containerRef}
        style={{width: "100%", height: "calc(100vh - 120px)"}}
      />
    </Drawer>
  );
}

export default PodTerminalDrawer;
