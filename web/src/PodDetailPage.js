import React, {useCallback, useEffect, useMemo, useState} from "react";
import {Alert, Descriptions, Table, Tag} from "antd";
import {Link, useParams} from "react-router-dom";
import {useTranslation} from "react-i18next";
import * as PodBackend from "./backend/PodBackend";
import * as MonitorBackend from "./backend/MonitorBackend";
import MonitoringTab from "./MonitoringTab";
import PodFilesPanel from "./PodFilesPanel";
import PodLogsPanel from "./PodLogsPanel";
import PodTerminalPanel from "./PodTerminalPanel";
import ResourceDetailLayout from "./ResourceDetailLayout";
import ResourceEventsTab from "./ResourceEventsTab";
import {resourceLabel, resourcePath} from "./resourceRoutes";

function PodDetailPage() {
  const {t} = useTranslation();
  const {namespace, name} = useParams();
  const [pod, setPod] = useState(null);
  const [overview, setOverview] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [activeTab, setActiveTab] = useState("overview");

  const fetchPod = useCallback(() => {
    setLoading(true);
    setError(null);
    Promise.all([
      PodBackend.getPod(namespace, name),
      MonitorBackend.getPodMonitorOverview(namespace, name, {mode: "total"}),
    ]).then(([podRes, overviewRes]) => {
      if (podRes.status === "ok") {
        setPod(podRes.data || null);
      } else {
        setError(podRes.msg || "Failed to load pod");
      }
      if (overviewRes.status === "ok") {
        setOverview(overviewRes.data || null);
      }
    }).catch(err => setError(err.message))
      .finally(() => setLoading(false));
  }, [namespace, name]);

  useEffect(() => {
    fetchPod();
  }, [fetchPod]);

  const fetchMonitoring = useCallback((query, signal) => MonitorBackend.getPodMonitorOverview(namespace, name, query, signal), [namespace, name]);
  const toolPod = useMemo(() => ({
    namespace,
    name,
    containers: (overview?.containers || []).map(container => container.name).filter(Boolean).length > 0
      ? overview.containers.map(container => container.name)
      : (pod?.containers || []),
  }), [namespace, name, overview, pod]);
  const owner = overview?.owner;
  const node = overview?.node;
  const renderMonitoringSummary = useCallback((data) => {
    if (!data) {return null;}
    const metadata = data.metadata || {};
    const phase = metadata.phase || pod?.phase || "";
    const nodeName = metadata.nodeName || pod?.nodeName || "";
    const metrics = data.metrics || [];
    const allMetricsEmpty = metrics.length > 0 && metrics.every(metric => metric.status === "empty");
    if (!nodeName) {
      return (
        <Alert
          type="warning"
          showIcon
          message={t("monitor:Pod is not scheduled to a Node")}
          description={t("monitor:Pod metrics require the Pod to be scheduled and reported by kubelet or cAdvisor.")}
        />
      );
    }
    if (phase && phase !== "Running") {
      return (
        <Alert
          type="warning"
          showIcon
          message={t("monitor:Pod is not Running")}
          description={t("monitor:Pod metrics may be empty until the Pod is running and kubelet reports container usage.")}
        />
      );
    }
    if (allMetricsEmpty) {
      return (
        <Alert
          type="info"
          showIcon
          message={t("monitor:Prometheus returned no Pod metrics")}
          description={t("monitor:Check that kubelet or cAdvisor metrics are scraped with namespace and pod labels for this Pod.")}
        />
      );
    }
    return null;
  }, [pod, t]);
  const containerColumns = [
    {title: "Name", dataIndex: "name", key: "name", ellipsis: true},
    {title: "Image", dataIndex: "image", key: "image", ellipsis: true},
    {title: "Ready", dataIndex: "ready", key: "ready", width: 90, render: value => value ? <Tag color="green">yes</Tag> : <Tag>no</Tag>},
    {title: "State", dataIndex: "state", key: "state", width: 120},
    {title: "Reason", dataIndex: "reason", key: "reason", width: 160, render: value => value || "-"},
    {title: "Restarts", dataIndex: "restartCount", key: "restartCount", width: 100},
    {title: "Last State", dataIndex: "lastState", key: "lastState", width: 130, render: value => value || "-"},
    {title: "Last Reason", dataIndex: "lastReason", key: "lastReason", width: 150, render: value => value || "-"},
    {title: "OOMKilled", dataIndex: "oomKilled", key: "oomKilled", width: 110, render: value => value ? <Tag color="red">yes</Tag> : <Tag>no</Tag>},
  ];
  const tabs = [
    {
      key: "overview",
      label: "Overview",
      children: (
        <>
          <Descriptions bordered size="small" column={2} style={{marginBottom: 16}}>
            <Descriptions.Item label="Namespace">{namespace}</Descriptions.Item>
            <Descriptions.Item label="Name">{name}</Descriptions.Item>
            <Descriptions.Item label="Phase"><Tag>{pod?.phase || overview?.metadata?.phase || "-"}</Tag></Descriptions.Item>
            <Descriptions.Item label="Status">{overview?.metadata?.status || pod?.phase || "-"}</Descriptions.Item>
            <Descriptions.Item label="Scheduled On">
              {node?.name ? <Link to={resourcePath("node", "", node.name)}>{resourceLabel(node)}</Link> : pod?.nodeName || "-"}
            </Descriptions.Item>
            <Descriptions.Item label="Controlled By">
              {owner?.name ? <Link to={resourcePath(owner.kind, owner.namespace, owner.name)}>{resourceLabel(owner)}</Link> : "-"}
            </Descriptions.Item>
            <Descriptions.Item label="Restart Count">{overview?.metadata?.restartCount ?? "-"}</Descriptions.Item>
            <Descriptions.Item label="Pod IP">{overview?.metadata?.podIP || "-"}</Descriptions.Item>
            <Descriptions.Item label="Host IP">{overview?.metadata?.hostIP || "-"}</Descriptions.Item>
            <Descriptions.Item label="Created">{pod?.createdAt || "-"}</Descriptions.Item>
          </Descriptions>
          <Table
            rowKey="name"
            columns={containerColumns}
            dataSource={overview?.containers || []}
            size="middle"
            pagination={false}
            scroll={{x: 1180}}
          />
        </>
      ),
    },
    {
      key: "monitoring",
      label: "Monitoring",
      children: (
        <MonitoringTab
          active={activeTab === "monitoring"}
          fetcher={fetchMonitoring}
          modes={[
            {value: "total", label: "Total", labelKey: "monitor:Total"},
            {value: "container", label: "By Container", labelKey: "monitor:By Container"},
          ]}
          initialMode="total"
          renderSummary={renderMonitoringSummary}
        />
      ),
    },
    {
      key: "events",
      label: "Events",
      children: <ResourceEventsTab kind="Pod" namespace={namespace} name={name} active={activeTab === "events"} />,
    },
    {
      key: "logs",
      label: "Logs",
      children: <PodLogsPanel pod={toolPod} active={activeTab === "logs"} />,
    },
    {
      key: "files",
      label: "Files",
      children: <PodFilesPanel pod={toolPod} active={activeTab === "files"} />,
    },
    {
      key: "terminal",
      label: "Terminal",
      children: <PodTerminalPanel pod={toolPod} active={activeTab === "terminal"} />,
    },
  ];

  return (
    <ResourceDetailLayout
      title={name}
      subtitle={`Pod / ${namespace}`}
      loading={loading}
      error={error}
      onRefresh={fetchPod}
      activeTab={activeTab}
      onTabChange={setActiveTab}
      tabs={tabs}
    />
  );
}

export default PodDetailPage;
