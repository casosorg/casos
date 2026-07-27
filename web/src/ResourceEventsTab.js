import React, {useEffect, useMemo, useState} from "react";
import {Alert, Button, Table, Tag} from "antd";
import {ReloadOutlined} from "@ant-design/icons";
import * as MonitorBackend from "./backend/MonitorBackend";

function formatTime(value) {
  if (!value) {return "-";}
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {return value;}
  return parsed.toLocaleString();
}

function eventDisplayTime(event) {
  return event.lastTimestamp || event.eventTime || event.firstTimestamp;
}

function ResourceEventsTab({kind, namespace, name, active}) {
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);

  function fetchEvents() {
    if (!kind || !name) {return;}
    setLoading(true);
    setError(null);
    MonitorBackend.getMonitorResourceEvents(kind, namespace, name, 100).then(res => {
      if (res.status === "ok") {
        setEvents(res.data || []);
      } else {
        setError(res.msg || "Failed to load events");
      }
    }).catch(err => setError(err.message))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    if (active) {fetchEvents();}
  }, [active, kind, namespace, name]);

  const columns = useMemo(() => [
    {title: "Time", key: "time", width: 190, render: (_, record) => formatTime(eventDisplayTime(record))},
    {title: "Type", dataIndex: "type", key: "type", width: 110, render: value => <Tag color={value === "Warning" ? "gold" : "blue"}>{value || "-"}</Tag>},
    {title: "Reason", dataIndex: "reason", key: "reason", width: 180},
    {title: "Message", dataIndex: "message", key: "message", ellipsis: true},
    {title: "Count", dataIndex: "count", key: "count", width: 90},
    {title: "Source", key: "source", width: 220, ellipsis: true, render: (_, record) => record.source || record.reportingController || "-"},
  ], []);

  return (
    <>
      {error && <Alert type="error" showIcon message={error} style={{marginBottom: 12}} />}
      <Table
        rowKey={(record, index) => `${record.reason}-${record.lastTimestamp}-${index}`}
        columns={columns}
        dataSource={events}
        loading={loading}
        size="middle"
        pagination={{pageSize: 20}}
        scroll={{x: 980}}
        title={() => (
          <Button size="small" icon={<ReloadOutlined />} loading={loading} onClick={fetchEvents}>
            Refresh
          </Button>
        )}
      />
    </>
  );
}

export default ResourceEventsTab;
