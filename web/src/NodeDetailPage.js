import React, {useCallback, useEffect, useState} from "react";
import {Descriptions, Tag} from "antd";
import {useParams} from "react-router-dom";
import * as NodeBackend from "./backend/NodeBackend";
import * as MonitorBackend from "./backend/MonitorBackend";
import MonitoringTab from "./MonitoringTab";
import MonitorPodsTable from "./MonitorPodsTable";
import ResourceDetailLayout from "./ResourceDetailLayout";
import ResourceEventsTab from "./ResourceEventsTab";

function NodeDetailPage() {
  const {name} = useParams();
  const [node, setNode] = useState(null);
  const [overview, setOverview] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [activeTab, setActiveTab] = useState("overview");

  const fetchNode = useCallback(() => {
    setLoading(true);
    setError(null);
    Promise.all([
      NodeBackend.getNode(name),
      MonitorBackend.getNodeMonitorOverview(name, {mode: "total"}),
    ]).then(([nodeRes, overviewRes]) => {
      if (nodeRes.status === "ok") {
        setNode(nodeRes.data || null);
      } else {
        setError(nodeRes.msg || "Failed to load node");
      }
      if (overviewRes.status === "ok") {
        setOverview(overviewRes.data || null);
      }
    }).catch(err => setError(err.message))
      .finally(() => setLoading(false));
  }, [name]);

  useEffect(() => {
    fetchNode();
  }, [fetchNode]);

  const fetchMonitoring = useCallback((query, signal) => MonitorBackend.getNodeMonitorOverview(name, query, signal), [name]);
  const title = node?.name || name;
  const tabs = [
    {
      key: "overview",
      label: "Overview",
      children: (
        <Descriptions bordered size="small" column={2}>
          <Descriptions.Item label="Name">{node?.name || name}</Descriptions.Item>
          <Descriptions.Item label="Status"><Tag color={node?.status === "Ready" ? "green" : "red"}>{node?.status || "-"}</Tag></Descriptions.Item>
          <Descriptions.Item label="Roles">{(node?.roles || []).map(role => <Tag key={role}>{role}</Tag>)}</Descriptions.Item>
          <Descriptions.Item label="Pods">{overview?.metadata?.podCount ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="Internal IP">{node?.internalIP || "-"}</Descriptions.Item>
          <Descriptions.Item label="External IP">{node?.externalIP || "-"}</Descriptions.Item>
          <Descriptions.Item label="Kubelet">{node?.kubeletVersion || "-"}</Descriptions.Item>
          <Descriptions.Item label="OS / Arch">{node?.os ? `${node.os} / ${node.arch}` : "-"}</Descriptions.Item>
          <Descriptions.Item label="Unschedulable">{node?.unschedulable ? "yes" : "no"}</Descriptions.Item>
          <Descriptions.Item label="Created">{node?.createdAt || "-"}</Descriptions.Item>
        </Descriptions>
      ),
    },
    {
      key: "monitoring",
      label: "Monitoring",
      children: (
        <MonitoringTab
          active={activeTab === "monitoring"}
          fetcher={fetchMonitoring}
          modes={[{value: "total", label: "Total", labelKey: "monitor:Total"}]}
          initialMode="total"
          renderBelow={data => <MonitorPodsTable pods={data?.pods || overview?.pods || []} loading={false} />}
        />
      ),
    },
    {
      key: "pods",
      label: "Pods",
      children: <MonitorPodsTable pods={overview?.pods || []} loading={loading} />,
    },
    {
      key: "events",
      label: "Events",
      children: <ResourceEventsTab kind="Node" name={name} active={activeTab === "events"} />,
    },
  ];

  return (
    <ResourceDetailLayout
      title={title}
      subtitle="Node"
      loading={loading}
      error={error}
      onRefresh={fetchNode}
      activeTab={activeTab}
      onTabChange={setActiveTab}
      tabs={tabs}
    />
  );
}

export default NodeDetailPage;
