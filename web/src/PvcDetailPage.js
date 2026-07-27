import React, {useCallback, useEffect, useState} from "react";
import {Descriptions, Tag} from "antd";
import {useParams} from "react-router-dom";
import * as MonitorBackend from "./backend/MonitorBackend";
import * as PvcBackend from "./backend/PvcBackend";
import MonitoringTab from "./MonitoringTab";
import ResourceDetailLayout from "./ResourceDetailLayout";
import ResourceEventsTab from "./ResourceEventsTab";

function PvcDetailPage() {
  const {namespace, name} = useParams();
  const [pvc, setPvc] = useState(null);
  const [overview, setOverview] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [activeTab, setActiveTab] = useState("overview");

  const fetchPvc = useCallback(() => {
    setLoading(true);
    setError(null);
    Promise.all([
      PvcBackend.getPvc(namespace, name),
      MonitorBackend.getPvcMonitorOverview(namespace, name, {mode: "total"}),
    ]).then(([pvcRes, overviewRes]) => {
      if (pvcRes.status === "ok") {
        setPvc(pvcRes.data || null);
      } else {
        setError(pvcRes.msg || "Failed to load PVC");
      }
      if (overviewRes.status === "ok") {
        setOverview(overviewRes.data || null);
      }
    }).catch(err => setError(err.message))
      .finally(() => setLoading(false));
  }, [namespace, name]);

  useEffect(() => {
    fetchPvc();
  }, [fetchPvc]);

  const fetchMonitoring = useCallback((query, signal) => MonitorBackend.getPvcMonitorOverview(namespace, name, query, signal), [namespace, name]);
  const tabs = [
    {
      key: "overview",
      label: "Overview",
      children: (
        <Descriptions bordered size="small" column={2}>
          <Descriptions.Item label="Namespace">{namespace}</Descriptions.Item>
          <Descriptions.Item label="Name">{name}</Descriptions.Item>
          <Descriptions.Item label="Status"><Tag>{pvc?.status || overview?.metadata?.status || "-"}</Tag></Descriptions.Item>
          <Descriptions.Item label="Storage Class">{pvc?.storageClassName || overview?.metadata?.storageClassName || "-"}</Descriptions.Item>
          <Descriptions.Item label="Access Mode">{pvc?.accessMode || (overview?.metadata?.accessModes || []).join(", ") || "-"}</Descriptions.Item>
          <Descriptions.Item label="Requested Storage">{pvc?.storage || overview?.metadata?.requestedStorage || "-"}</Descriptions.Item>
          <Descriptions.Item label="Volume">{pvc?.volumeName || overview?.metadata?.volumeName || "-"}</Descriptions.Item>
          <Descriptions.Item label="Created">{pvc?.createdAt || "-"}</Descriptions.Item>
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
        />
      ),
    },
    {
      key: "events",
      label: "Events",
      children: <ResourceEventsTab kind="PersistentVolumeClaim" namespace={namespace} name={name} active={activeTab === "events"} />,
    },
  ];

  return (
    <ResourceDetailLayout
      title={name}
      subtitle={`PVC / ${namespace}`}
      loading={loading}
      error={error}
      onRefresh={fetchPvc}
      activeTab={activeTab}
      onTabChange={setActiveTab}
      tabs={tabs}
    />
  );
}

export default PvcDetailPage;
