import React, {useCallback, useEffect, useState} from "react";
import {Descriptions, Tag, Typography} from "antd";
import {useParams} from "react-router-dom";
import * as DaemonSetBackend from "./backend/DaemonSetBackend";
import * as DeploymentBackend from "./backend/DeploymentBackend";
import * as MonitorBackend from "./backend/MonitorBackend";
import * as StatefulSetBackend from "./backend/StatefulSetBackend";
import MonitoringTab from "./MonitoringTab";
import MonitorPodsTable from "./MonitorPodsTable";
import ResourceDetailLayout from "./ResourceDetailLayout";
import ResourceEventsTab from "./ResourceEventsTab";

const {Paragraph} = Typography;

const workloadMeta = {
  deployment: {
    kind: "Deployment",
    subtitle: "Deployment",
    get: DeploymentBackend.getDeployment,
  },
  statefulset: {
    kind: "StatefulSet",
    subtitle: "StatefulSet",
    get: StatefulSetBackend.getStatefulSet,
  },
  daemonset: {
    kind: "DaemonSet",
    subtitle: "DaemonSet",
    get: DaemonSetBackend.getDaemonSet,
  },
};

function WorkloadDetailPage({type}) {
  const {namespace, name} = useParams();
  const meta = workloadMeta[type];
  const [resource, setResource] = useState(null);
  const [overview, setOverview] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [activeTab, setActiveTab] = useState("overview");

  const fetchResource = useCallback(() => {
    setLoading(true);
    setError(null);
    Promise.all([
      meta.get(namespace, name),
      MonitorBackend.getWorkloadMonitorOverview(meta.kind, namespace, name, {mode: "total", podLimit: 10}),
    ]).then(([resourceRes, overviewRes]) => {
      if (resourceRes.status === "ok") {
        setResource(resourceRes.data || null);
      } else {
        setError(resourceRes.msg || `Failed to load ${meta.kind}`);
      }
      if (overviewRes.status === "ok") {
        setOverview(overviewRes.data || null);
      }
    }).catch(err => setError(err.message))
      .finally(() => setLoading(false));
  }, [meta, namespace, name]);

  useEffect(() => {
    fetchResource();
  }, [fetchResource]);

  const fetchMonitoring = useCallback((query, signal) => MonitorBackend.getWorkloadMonitorOverview(meta.kind, namespace, name, query, signal), [meta, namespace, name]);
  const resourceConfig = overview?.metadata?.resourceConfiguration || {};
  const tabs = [
    {
      key: "overview",
      label: "Overview",
      children: (
        <Descriptions bordered size="small" column={2}>
          <Descriptions.Item label="Namespace">{namespace}</Descriptions.Item>
          <Descriptions.Item label="Name">{name}</Descriptions.Item>
          <Descriptions.Item label="Replicas">{overview?.metadata?.replicas ?? resource?.replicas ?? resource?.desiredNumberScheduled ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="Ready">{overview?.metadata?.readyReplicas ?? resource?.readyReplicas ?? resource?.numberReady ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="Image">{resource?.image || "-"}</Descriptions.Item>
          <Descriptions.Item label="Current Pods">{overview?.metadata?.currentPodCount ?? "-"}</Descriptions.Item>
          <Descriptions.Item label="CPU Request">{resourceConfig.cpuRequestCores ?? 0} cores</Descriptions.Item>
          <Descriptions.Item label="CPU Limit">{resourceConfig.cpuLimitCores ?? 0} cores</Descriptions.Item>
          <Descriptions.Item label="Memory Request">{resourceConfig.memoryRequestBytes ?? 0} bytes</Descriptions.Item>
          <Descriptions.Item label="Memory Limit">{resourceConfig.memoryLimitBytes ?? 0} bytes</Descriptions.Item>
          <Descriptions.Item label="Created">{resource?.createdAt || "-"}</Descriptions.Item>
          <Descriptions.Item label="Selector">
            {Object.entries(resource?.selector || {}).map(([key, value]) => <Tag key={key}>{key}={value}</Tag>)}
          </Descriptions.Item>
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
          modes={[
            {value: "total", label: "Total", labelKey: "monitor:Total"},
            {value: "pod", label: "By Pod", labelKey: "monitor:By Pod"},
          ]}
          initialMode="total"
          podLimit={10}
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
      children: <ResourceEventsTab kind={meta.kind} namespace={namespace} name={name} active={activeTab === "events"} />,
    },
    {
      key: "configuration",
      label: "Configuration",
      children: (
        <>
          <Paragraph type="secondary">
            Request and limit references in Monitoring use the current Kubernetes configuration.
          </Paragraph>
          <pre style={{whiteSpace: "pre-wrap", margin: 0}}>
            {JSON.stringify({
              selector: resource?.selector || {},
              envVars: resource?.envVars || [],
              volumes: resource?.volumes || [],
              resourceConfiguration: resourceConfig,
            }, null, 2)}
          </pre>
        </>
      ),
    },
  ];

  return (
    <ResourceDetailLayout
      title={name}
      subtitle={`${meta.subtitle} / ${namespace}`}
      loading={loading}
      error={error}
      onRefresh={fetchResource}
      activeTab={activeTab}
      onTabChange={setActiveTab}
      tabs={tabs}
    />
  );
}

export default WorkloadDetailPage;
