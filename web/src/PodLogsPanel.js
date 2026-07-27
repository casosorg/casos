import React, {useEffect, useRef, useState} from "react";
import {Alert, Button, Select, Space, Tag, Tooltip} from "antd";
import {DownloadOutlined, VerticalAlignBottomOutlined} from "@ant-design/icons";
import * as PodBackend from "./backend/PodBackend";

const TAIL_OPTIONS = [
  {label: "100 lines", value: 100},
  {label: "500 lines", value: 500},
  {label: "1000 lines", value: 1000},
  {label: "5000 lines", value: 5000},
];

function PodLogsPanel({pod, active}) {
  const [container, setContainer] = useState("");
  const [tailLines, setTailLines] = useState(500);
  const [logs, setLogs] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const logsEndRef = useRef(null);
  const timerRef = useRef(null);
  const autoScrollRef = useRef(true);

  function stopPolling() {
    if (timerRef.current) {
      clearInterval(timerRef.current);
      timerRef.current = null;
    }
  }

  function fetchLogs(ctr = container, tail = tailLines) {
    if (!pod) {return;}
    setLoading(true);
    PodBackend.getPodLogs(pod.namespace, pod.name, ctr, tail).then(res => {
      if (res.status === "ok") {
        setLogs(res.data || "");
        setError(null);
        if (autoScrollRef.current) {
          setTimeout(() => logsEndRef.current?.scrollIntoView({behavior: "smooth"}), 50);
        }
      } else {
        setError(res.msg);
      }
    }).catch(err => setError(err.message))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    stopPolling();
    if (!active || !pod) {return undefined;}
    const defaultContainer = pod.containers?.[0] || "";
    setContainer(defaultContainer);
    setLogs("");
    fetchLogs(defaultContainer, tailLines);
    timerRef.current = setInterval(() => fetchLogs(defaultContainer, tailLines), 3000);
    return stopPolling;
  }, [active, pod]);

  useEffect(() => {
    stopPolling();
    if (!active || !pod) {return undefined;}
    fetchLogs(container, tailLines);
    timerRef.current = setInterval(() => fetchLogs(container, tailLines), 3000);
    return stopPolling;
  }, [container, tailLines]);

  function downloadLogs() {
    const blob = new Blob([logs], {type: "text/plain"});
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `${pod.namespace}_${pod.name}_${container || "default"}.log`;
    link.click();
    URL.revokeObjectURL(url);
  }

  const containerOptions = (pod?.containers || []).map(item => ({label: item, value: item}));
  return (
    <Space direction="vertical" style={{width: "100%"}} size={12}>
      <Space wrap>
        {containerOptions.length > 1 && (
          <Select value={container} onChange={setContainer} options={containerOptions} style={{width: 180}} />
        )}
        <Select value={tailLines} onChange={setTailLines} options={TAIL_OPTIONS} style={{width: 130}} />
        <Tooltip title="Download log">
          <Button icon={<DownloadOutlined />} onClick={downloadLogs} disabled={!logs} />
        </Tooltip>
        <Tooltip title="Scroll to bottom">
          <Button icon={<VerticalAlignBottomOutlined />} onClick={() => logsEndRef.current?.scrollIntoView({behavior: "smooth"})} />
        </Tooltip>
        <Tag color={loading ? "processing" : "success"}>{loading ? "refreshing" : "live · 3s"}</Tag>
      </Space>
      {error && <Alert type="error" showIcon message={error} />}
      <div
        onScroll={event => {
          const element = event.currentTarget;
          autoScrollRef.current = element.scrollHeight - element.scrollTop - element.clientHeight < 40;
        }}
        style={{
          background: "#0d1117",
          borderRadius: 6,
          padding: "12px 16px",
          fontFamily: "'Cascadia Code', 'Fira Mono', 'Consolas', monospace",
          fontSize: 13,
          lineHeight: 1.7,
          height: 520,
          overflowY: "auto",
          color: "#c9d1d9",
          whiteSpace: "pre-wrap",
          wordBreak: "break-all",
        }}
      >
        {!logs && !loading ? <span style={{color: "#6e7681"}}>No logs yet</span> : logs}
        <div ref={logsEndRef} />
      </div>
    </Space>
  );
}

export default PodLogsPanel;
