import React from "react";
import {Link} from "react-router-dom";
import {Table, Tag} from "antd";
import {formatMonitorMetricValue} from "./monitorMetrics";
import {resourcePath} from "./resourceRoutes";

const phaseColor = {
  Running: "green",
  Pending: "gold",
  Succeeded: "blue",
  Failed: "red",
  Unknown: "default",
};

function MonitorPodsTable({pods, loading}) {
  const columns = [
    {
      title: "Namespace",
      dataIndex: "namespace",
      key: "namespace",
      width: 150,
      ellipsis: true,
    },
    {
      title: "Pod",
      dataIndex: "name",
      key: "name",
      ellipsis: true,
      render: (name, record) => <Link to={resourcePath("pod", record.namespace, name)}>{name}</Link>,
    },
    {
      title: "Node",
      dataIndex: "nodeName",
      key: "nodeName",
      width: 180,
      ellipsis: true,
      render: value => value ? <Link to={resourcePath("node", "", value)}>{value}</Link> : "-",
    },
    {
      title: "Owner",
      key: "owner",
      width: 240,
      ellipsis: true,
      render: (_, record) => {
        const owner = record.owner;
        if (!owner?.name) {return "-";}
        const path = resourcePath(owner.kind, owner.namespace, owner.name);
        const label = `${owner.kind} / ${owner.name}`;
        return path ? <Link to={path}>{label}</Link> : label;
      },
    },
    {
      title: "CPU",
      dataIndex: "currentCpuCores",
      key: "currentCpuCores",
      width: 110,
      render: (value, record) => record.metricsAvailable ? formatMonitorMetricValue(value, "cores") : "-",
    },
    {
      title: "Memory",
      dataIndex: "currentMemoryBytes",
      key: "currentMemoryBytes",
      width: 120,
      render: (value, record) => record.metricsAvailable ? formatMonitorMetricValue(value, "bytes") : "-",
    },
    {title: "Restarts", dataIndex: "restartCount", key: "restartCount", width: 95},
    {
      title: "Status",
      dataIndex: "status",
      key: "status",
      width: 130,
      render: (value, record) => <Tag color={phaseColor[record.phase] || "default"}>{value || record.phase || "Unknown"}</Tag>,
    },
  ];
  return (
    <Table
      rowKey={record => `${record.namespace}/${record.name}`}
      columns={columns}
      dataSource={pods || []}
      loading={loading}
      size="middle"
      pagination={{pageSize: 20}}
      scroll={{x: 1180}}
    />
  );
}

export default MonitorPodsTable;
