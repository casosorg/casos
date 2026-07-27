import React, {useEffect, useRef, useState} from "react";
import {Alert, Select, Space} from "antd";
import {Terminal} from "xterm";
import {FitAddon} from "xterm-addon-fit";
import * as Setting from "./Setting";
import "xterm/css/xterm.css";

function PodTerminalPanel({pod, active}) {
  const containerRef = useRef(null);
  const termRef = useRef(null);
  const fitAddonRef = useRef(null);
  const wsRef = useRef(null);
  const [container, setContainer] = useState("");
  const [error, setError] = useState(null);

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
    const encoded = new TextEncoder().encode(payload);
    const frame = new Uint8Array(1 + encoded.length);
    frame[0] = 1;
    frame.set(encoded, 1);
    wsRef.current.send(frame);
  }

  function openTerminal(ctr) {
    if (!pod || !ctr) {return;}
    cleanup();
    setError(null);
    const term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: "'Cascadia Code', 'Fira Mono', Consolas, monospace",
      theme: {background: "#0d1117", foreground: "#c9d1d9", cursor: "#58a6ff"},
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
      namespace: pod.namespace,
      name: pod.name,
      container: ctr,
    }));
    ws.binaryType = "arraybuffer";
    wsRef.current = ws;
    ws.onopen = () => sendResize(term.cols, term.rows);
    ws.onmessage = event => {
      if (termRef.current !== term) {return;}
      term.write(typeof event.data === "string" ? event.data : new Uint8Array(event.data));
    };
    ws.onclose = () => {
      if (termRef.current === term) {
        term.write("\r\n\x1b[31m[connection closed]\x1b[0m\r\n");
      }
    };
    ws.onerror = () => {
      setError("WebSocket error");
      if (termRef.current === term) {
        term.write("\r\n\x1b[31m[websocket error]\x1b[0m\r\n");
      }
    };
    term.onData(data => {
      if (!wsRef.current || wsRef.current.readyState !== WebSocket.OPEN) {return;}
      const encoded = new TextEncoder().encode(data);
      const frame = new Uint8Array(1 + encoded.length);
      frame[0] = 0;
      frame.set(encoded, 1);
      wsRef.current.send(frame);
    });
    term.onResize(({cols, rows}) => sendResize(cols, rows));
  }

  useEffect(() => {
    if (!active || !pod) {
      cleanup();
      return undefined;
    }
    const defaultContainer = pod.containers?.[0] || "";
    setContainer(defaultContainer);
    const timer = setTimeout(() => openTerminal(defaultContainer), 100);
    return () => {
      clearTimeout(timer);
      cleanup();
    };
  }, [active, pod]);

  useEffect(() => {
    if (!active) {return undefined;}
    const handleResize = () => fitAddonRef.current?.fit();
    window.addEventListener("resize", handleResize);
    return () => window.removeEventListener("resize", handleResize);
  }, [active]);

  const containerOptions = (pod?.containers || []).map(item => ({label: item, value: item}));
  return (
    <Space direction="vertical" style={{width: "100%"}} size={12}>
      {error && <Alert type="error" showIcon message={error} />}
      {containerOptions.length > 1 && (
        <Select
          value={container}
          options={containerOptions}
          style={{width: 180}}
          onChange={value => {
            setContainer(value);
            openTerminal(value);
          }}
        />
      )}
      <div style={{background: "#0d1117", borderRadius: 6, padding: 8}}>
        <div ref={containerRef} style={{width: "100%", height: 520}} />
      </div>
    </Space>
  );
}

export default PodTerminalPanel;
