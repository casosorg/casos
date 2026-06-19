import React, {useCallback, useEffect, useRef, useState} from "react";
import {Alert, Button, Input, Select, Space, Spin, Tag, Tooltip} from "antd";
import {ClearOutlined, DownloadOutlined, SearchOutlined, VerticalAlignBottomOutlined} from "@ant-design/icons";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import * as DeploymentBackend from "./backend/DeploymentBackend";
import * as LogBackend from "./backend/LogBackend";

// 10 distinct colors for pod name badges
const POD_COLORS = [
  "#1677ff", "#52c41a", "#fa8c16", "#eb2f96", "#722ed1",
  "#13c2c2", "#faad14", "#f5222d", "#2f54eb", "#389e0d",
];

const TAIL_OPTIONS = [
  {label: "100 lines / pod", value: 100},
  {label: "200 lines / pod", value: 200},
  {label: "500 lines / pod", value: 500},
  {label: "1000 lines / pod", value: 1000},
];

function assignPodColors(pods) {
  const map = {};
  pods.forEach((p, i) => {
    map[p] = POD_COLORS[i % POD_COLORS.length];
  });
  return map;
}

// Highlight keyword occurrences in text (returns array of React nodes)
function highlightText(text, keyword) {
  if (!keyword) {return text;}
  const parts = text.split(new RegExp(`(${keyword.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")})`, "gi"));
  return parts.map((part, i) =>
    part.toLowerCase() === keyword.toLowerCase()
      ? <mark key={i} style={{background: "#ffe58f", padding: 0, borderRadius: 2}}>{part}</mark>
      : part
  );
}

function LogSearchPage() {
  const [namespaces, setNamespaces] = useState([]);
  const [namespace, setNamespace] = useState("");
  const [deployments, setDeployments] = useState([]);
  const [deployment, setDeployment] = useState("");
  const [keyword, setKeyword] = useState("");
  const [tailLines, setTailLines] = useState(200);

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [lines, setLines] = useState([]);
  const [podColorMap, setPodColorMap] = useState({});
  const [searched, setSearched] = useState(false);

  const logEndRef = useRef(null);
  const autoScrollRef = useRef(true);

  useEffect(() => {
    NamespaceBackend.getNamespaces().then(res => {
      if (res.status === "ok") {
        const nsList = res.data ?? [];
        setNamespaces(nsList);
        const def = nsList.find(n => n.name === "default") ?? nsList[0];
        if (def) {setNamespace(def.name);}
      }
    }).catch(() => {});
  }, []);

  useEffect(() => {
    if (!namespace) {return;}
    setDeployment("");
    setDeployments([]);
    DeploymentBackend.getDeployments(namespace).then(res => {
      if (res.status === "ok") {
        setDeployments(res.data ?? []);
      }
    }).catch(() => {});
  }, [namespace]);

  const handleSearch = useCallback(() => {
    if (!namespace || !deployment) {return;}
    setLoading(true);
    setError(null);
    setLines([]);
    setSearched(true);
    autoScrollRef.current = true;

    LogBackend.getAggregatedLogs(namespace, deployment, keyword, tailLines)
      .then(res => {
        if (res.status === "ok") {
          const data = res.data ?? {lines: [], pods: []};
          setLines(data.lines ?? []);
          setPodColorMap(assignPodColors(data.pods ?? []));
          setTimeout(() => {
            if (autoScrollRef.current) {
              logEndRef.current?.scrollIntoView({behavior: "smooth"});
            }
          }, 50);
        } else {
          setError(res.msg);
        }
      })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false));
  }, [namespace, deployment, keyword, tailLines]);

  function handleDownload() {
    const text = lines.map(l => `[${l.pod}][${l.container}] ${l.text}`).join("\n");
    const blob = new Blob([text], {type: "text/plain"});
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${namespace}_${deployment}_aggregated.log`;
    a.click();
    URL.revokeObjectURL(url);
  }

  const nsOptions = namespaces.map(n => ({label: n.name, value: n.name}));
  const depOptions = deployments.map(d => ({label: d.name, value: d.name}));

  const podNames = Object.keys(podColorMap);

  return (
    <div style={{padding: "24px", height: "100%", display: "flex", flexDirection: "column", gap: 16}}>
      {/* Controls */}
      <div style={{
        display: "flex",
        flexWrap: "wrap",
        gap: 8,
        alignItems: "center",
        background: "#fff",
        padding: "14px 16px",
        borderRadius: 8,
        boxShadow: "0 1px 4px rgba(0,0,0,.06)",
      }}>
        <Select
          placeholder="Namespace"
          value={namespace || undefined}
          onChange={v => setNamespace(v)}
          options={nsOptions}
          style={{width: 160}}
          showSearch
        />
        <Select
          placeholder="Deployment"
          value={deployment || undefined}
          onChange={v => setDeployment(v)}
          options={depOptions}
          style={{width: 200}}
          showSearch
          disabled={!namespace}
          notFoundContent={namespace ? "No deployments" : "Select a namespace first"}
        />
        <Input
          placeholder="Keyword filter (optional)"
          value={keyword}
          onChange={e => setKeyword(e.target.value)}
          onPressEnter={handleSearch}
          style={{width: 220}}
          prefix={<SearchOutlined style={{color: "#bbb"}} />}
          allowClear
        />
        <Select
          value={tailLines}
          onChange={setTailLines}
          options={TAIL_OPTIONS}
          style={{width: 160}}
        />
        <Button
          type="primary"
          icon={<SearchOutlined />}
          onClick={handleSearch}
          loading={loading}
          disabled={!namespace || !deployment}
        >
          Search
        </Button>
        <Button
          icon={<ClearOutlined />}
          onClick={() => {setLines([]); setError(null); setSearched(false);}}
          disabled={lines.length === 0 && !error}
        >
          Clear
        </Button>
        <div style={{marginLeft: "auto"}}>
          <Space>
            {lines.length > 0 && (
              <span style={{color: "#888", fontSize: 12}}>
                {lines.length} lines from {podNames.length} pod{podNames.length !== 1 ? "s" : ""}
              </span>
            )}
            <Tooltip title="Scroll to bottom">
              <Button
                size="small"
                icon={<VerticalAlignBottomOutlined />}
                onClick={() => {
                  autoScrollRef.current = true;
                  logEndRef.current?.scrollIntoView({behavior: "smooth"});
                }}
                disabled={lines.length === 0}
              />
            </Tooltip>
            <Tooltip title="Download log">
              <Button
                size="small"
                icon={<DownloadOutlined />}
                onClick={handleDownload}
                disabled={lines.length === 0}
              />
            </Tooltip>
          </Space>
        </div>
      </div>

      {/* Pod color legend */}
      {podNames.length > 0 && (
        <div style={{display: "flex", flexWrap: "wrap", gap: 6}}>
          {podNames.map(p => (
            <Tag key={p} color={podColorMap[p]} style={{fontFamily: "monospace", fontSize: 12}}>
              {p}
            </Tag>
          ))}
        </div>
      )}

      {/* Log viewer */}
      <div style={{flex: 1, minHeight: 0}}>
        {error && <Alert type="error" message={error} showIcon style={{marginBottom: 12}} />}
        <Spin spinning={loading}>
          <div
            onScroll={e => {
              const el = e.currentTarget;
              autoScrollRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 60;
            }}
            style={{
              background: "#0d1117",
              borderRadius: 8,
              padding: "12px 16px",
              fontFamily: "'Cascadia Code', 'Fira Mono', Consolas, monospace",
              fontSize: 12.5,
              lineHeight: 1.75,
              height: "calc(100vh - 260px)",
              overflowY: "auto",
              color: "#c9d1d9",
            }}
          >
            {!searched && !loading && (
              <span style={{color: "#4d5566"}}>
                Select a namespace and deployment, then click Search to view aggregated logs.
              </span>
            )}
            {searched && !loading && lines.length === 0 && !error && (
              <span style={{color: "#4d5566"}}>
                No log lines found{keyword ? ` matching "${keyword}"` : ""}.
              </span>
            )}
            {lines.map((line, i) => {
              const color = podColorMap[line.pod] ?? "#888";
              return (
                <div key={i} style={{display: "flex", gap: 8, marginBottom: 1}}>
                  <span style={{
                    color,
                    flexShrink: 0,
                    fontWeight: 600,
                    minWidth: 0,
                    maxWidth: 200,
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  }}
                  title={`${line.pod} / ${line.container}`}
                  >
                    [{line.pod}]
                  </span>
                  <span style={{wordBreak: "break-all", whiteSpace: "pre-wrap"}}>
                    {highlightText(line.text, keyword)}
                  </span>
                </div>
              );
            })}
            <div ref={logEndRef} />
          </div>
        </Spin>
      </div>
    </div>
  );
}

export default LogSearchPage;
