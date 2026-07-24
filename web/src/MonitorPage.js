import React, {useCallback, useEffect, useMemo, useRef, useState} from "react";
import {Alert, Button, Card, Col, DatePicker, Descriptions, Drawer, Input, Modal, Row, Segmented, Space, Spin, Statistic, Switch, Table, Tag, Typography} from "antd";
import {
  AppstoreOutlined,
  BellOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ClusterOutlined,
  ExclamationCircleOutlined,
  FieldTimeOutlined,
  QuestionCircleOutlined,
  ReloadOutlined,
  WarningOutlined
} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import i18next from "i18next";
import * as MonitorBackend from "./backend/MonitorBackend";
import * as Setting from "./Setting";
import MonitorMetricChart from "./MonitorMetricChart";
import {
  MONITOR_AUTO_REFRESH_INTERVAL_MS,
  MONITOR_METRIC_REQUESTS,
  buildMonitorTimeRange
} from "./monitorMetrics";

const {Paragraph, Text} = Typography;
const {RangePicker} = DatePicker;

const statusMeta = {
  healthy: {color: "green", icon: <CheckCircleOutlined />},
  warning: {color: "gold", icon: <ExclamationCircleOutlined />},
  critical: {color: "red", icon: <CloseCircleOutlined />},
  unknown: {color: "default", icon: <QuestionCircleOutlined />},
};

const severityColor = {
  info: "blue",
  warning: "gold",
  critical: "red",
};

const eventTypeColor = {
  Normal: "blue",
  Warning: "gold",
};

const metricChartDefinitions = [
  {key: "cpu", title: "CPU Usage", unit: "percent"},
  {key: "memory", title: "Memory Usage", unit: "percent"},
  {key: "networkReceive", title: "Network Receive", unit: "bytes_per_second"},
  {key: "networkTransmit", title: "Network Transmit", unit: "bytes_per_second"},
  {key: "disk", title: "Node Disk Usage", unit: "percent"},
  {key: "storage", title: "PVC Storage Usage", unit: "percent"},
];

function registerMonitorI18nKeys() {
  // The existing i18n generator only scans literal i18next.t(...) calls.
  i18next.t("monitor:Abnormal Pods");
  i18next.t("monitor:Auto Refresh");
  i18next.t("monitor:Category");
  i18next.t("monitor:Check");
  i18next.t("monitor:Count");
  i18next.t("monitor:CPU Usage");
  i18next.t("monitor:Critical Checks");
  i18next.t("monitor:Current");
  i18next.t("monitor:Custom Range");
  i18next.t("monitor:Details");
  i18next.t("monitor:Diagnosis");
  i18next.t("monitor:Diagnosis Context");
  i18next.t("monitor:Event Center");
  i18next.t("monitor:Event Details");
  i18next.t("monitor:Failed to load diagnosis");
  i18next.t("monitor:Failed to load events");
  i18next.t("monitor:Failed to load health checks");
  i18next.t("monitor:Failed to load monitor issues");
  i18next.t("monitor:Failed to load monitor data");
  i18next.t("monitor:Failed to load resource trends");
  i18next.t("monitor:Failed to load monitor summary");
  i18next.t("monitor:Health Checks");
  i18next.t("monitor:Last 1 Hour");
  i18next.t("monitor:Last 6 Hours");
  i18next.t("monitor:Last 24 Hours");
  i18next.t("monitor:Last 7 Days");
  i18next.t("monitor:Last Seen");
  i18next.t("monitor:Log Preview");
  i18next.t("monitor:Last Checked");
  i18next.t("monitor:Message");
  i18next.t("monitor:Memory Usage");
  i18next.t("monitor:Monitor Issues");
  i18next.t("monitor:Network Receive");
  i18next.t("monitor:Network Transmit");
  i18next.t("monitor:No metric data");
  i18next.t("monitor:Node Disk Usage");
  i18next.t("monitor:Object");
  i18next.t("monitor:Overall Status");
  i18next.t("monitor:PVC Storage Usage");
  i18next.t("monitor:Previous");
  i18next.t("monitor:Ready Nodes");
  i18next.t("monitor:Reason");
  i18next.t("monitor:Related Events");
  i18next.t("monitor:Resource Trends");
  i18next.t("monitor:Running Pods");
  i18next.t("monitor:Select a custom time range");
  i18next.t("monitor:Source");
  i18next.t("monitor:Suggestion");
  i18next.t("monitor:Time");
  i18next.t("monitor:Warning Checks");
  i18next.t("monitor:Warning Events");
  i18next.t("monitor:severity critical");
  i18next.t("monitor:severity info");
  i18next.t("monitor:severity warning");
  i18next.t("monitor:status critical");
  i18next.t("monitor:status healthy");
  i18next.t("monitor:status unknown");
  i18next.t("monitor:status warning");
}

registerMonitorI18nKeys();

function formatTime(value) {
  if (!value) {return "-";}
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {return value;}
  return parsed.toLocaleString();
}

function renderStatusTag(status, t) {
  const meta = statusMeta[status] || statusMeta.unknown;
  return (
    <Tag color={meta.color} icon={meta.icon}>
      {t(`monitor:status ${status || "unknown"}`)}
    </Tag>
  );
}

function eventDisplayTime(event) {
  return event.lastTimestamp || event.eventTime || event.firstTimestamp;
}

function objectLabel(record) {
  if (!record) {return "-";}
  const name = record.namespace ? `${record.namespace}/${record.name}` : record.name;
  return `${record.kind || "-"} / ${name || "-"}`;
}

function MonitorPage() {
  const {t} = useTranslation();
  const [summary, setSummary] = useState(null);
  const [checks, setChecks] = useState([]);
  const [issues, setIssues] = useState([]);
  const [events, setEvents] = useState([]);
  const [loading, setLoading] = useState(true);
  const [issuesLoading, setIssuesLoading] = useState(true);
  const [eventsLoading, setEventsLoading] = useState(true);
  const [error, setError] = useState(null);
  const [issuesError, setIssuesError] = useState(null);
  const [eventsError, setEventsError] = useState(null);
  const [namespaceFilter, setNamespaceFilter] = useState("");
  const [selectedEvent, setSelectedEvent] = useState(null);
  const [selectedIssue, setSelectedIssue] = useState(null);
  const [diagnosis, setDiagnosis] = useState(null);
  const [diagnosisLoading, setDiagnosisLoading] = useState(false);
  const [diagnosisError, setDiagnosisError] = useState(null);
  const [metricData, setMetricData] = useState({});
  const [metricErrors, setMetricErrors] = useState({});
  const [metricsLoading, setMetricsLoading] = useState(false);
  const [timePreset, setTimePreset] = useState("1h");
  const [customTimeRange, setCustomTimeRange] = useState(null);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const metricRequestRef = useRef(0);
  const metricAbortRef = useRef(null);

  function fetchOverview() {
    setLoading(true);
    setError(null);
    MonitorBackend.getMonitorOverview().then(res => {
      if (res.status === "ok") {
        setSummary(res.data?.summary || null);
        setChecks(res.data?.checks || []);
      } else {
        setError(res.msg || t("monitor:Failed to load monitor data"));
      }
    }).catch(err => {
      setError(err.message);
      Setting.showMessage("error", err.message);
    }).finally(() => setLoading(false));
  }

  function fetchEvents(namespace = namespaceFilter) {
    setEventsLoading(true);
    setEventsError(null);
    MonitorBackend.getMonitorEvents(namespace, 100).then(res => {
      if (res.status === "ok") {
        setEvents(res.data || []);
      } else {
        setEventsError(res.msg);
      }
    }).catch(err => {
      setEventsError(err.message);
    }).finally(() => setEventsLoading(false));
  }

  function fetchIssues() {
    setIssuesLoading(true);
    setIssuesError(null);
    MonitorBackend.getMonitorIssues().then(res => {
      if (res.status === "ok") {
        setIssues(res.data || []);
      } else {
        setIssuesError(res.msg || t("monitor:Failed to load monitor issues"));
      }
    }).catch(err => {
      setIssuesError(err.message);
    }).finally(() => setIssuesLoading(false));
  }

  const fetchMetricTrends = useCallback(() => {
    const timeRange = buildMonitorTimeRange(timePreset, customTimeRange);
    if (!timeRange) {
      metricAbortRef.current?.abort();
      setMetricData({});
      setMetricErrors({});
      setMetricsLoading(false);
      return;
    }

    metricAbortRef.current?.abort();
    const controller = new AbortController();
    metricAbortRef.current = controller;
    const requestID = ++metricRequestRef.current;
    setMetricsLoading(true);
    setMetricErrors({});

    Promise.all(MONITOR_METRIC_REQUESTS.map(async request => {
      try {
        const response = await MonitorBackend.getMonitorMetrics({...request, ...timeRange}, controller.signal);
        return {request, response};
      } catch (requestError) {
        return {request, error: requestError};
      }
    })).then(results => {
      if (controller.signal.aborted || requestID !== metricRequestRef.current) {return;}
      const nextData = {};
      const nextErrors = {};
      results.forEach(result => {
        if (result.response?.status === "ok") {
          nextData[result.request.key] = result.response.data;
        } else {
          nextErrors[result.request.key] = result.response?.msg || result.error?.message || t("monitor:Failed to load resource trends");
        }
      });
      setMetricData(nextData);
      setMetricErrors(nextErrors);
    }).finally(() => {
      if (!controller.signal.aborted && requestID === metricRequestRef.current) {
        setMetricsLoading(false);
      }
    });
  }, [customTimeRange, t, timePreset]);

  const openDiagnosis = useCallback((issue) => {
    setSelectedIssue(issue);
    setDiagnosis(null);
    setDiagnosisError(null);
    setDiagnosisLoading(true);
    MonitorBackend.getMonitorDiagnosis(issue, 100, true).then(res => {
      if (res.status === "ok") {
        setDiagnosis(res.data || null);
      } else {
        setDiagnosisError(res.msg || t("monitor:Failed to load diagnosis"));
      }
    }).catch(err => {
      setDiagnosisError(err.message);
    }).finally(() => setDiagnosisLoading(false));
  }, [t]);

  useEffect(() => {
    fetchOverview();
    fetchIssues();
    fetchEvents("");
  }, []);

  useEffect(() => {
    fetchMetricTrends();
    return () => {
      metricRequestRef.current++;
      metricAbortRef.current?.abort();
    };
  }, [fetchMetricTrends]);

  useEffect(() => {
    if (!autoRefresh) {return undefined;}
    const timer = window.setInterval(fetchMetricTrends, MONITOR_AUTO_REFRESH_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [autoRefresh, fetchMetricTrends]);

  const checkColumns = useMemo(() => [
    {title: t("monitor:Check"), dataIndex: "name", key: "name", width: 280, ellipsis: true},
    {title: t("monitor:Category"), dataIndex: "category", key: "category", width: 130, render: value => <Tag>{value}</Tag>},
    {title: t("general:Status"), dataIndex: "status", key: "status", width: 130, render: value => renderStatusTag(value, t)},
    {
      title: t("trivy:Severity"),
      dataIndex: "severity",
      key: "severity",
      width: 130,
      render: value => <Tag color={severityColor[value] || "default"}>{t(`monitor:severity ${value || "info"}`)}</Tag>,
    },
    {title: t("monitor:Message"), dataIndex: "message", key: "message", width: 340, ellipsis: true},
    {title: t("monitor:Suggestion"), dataIndex: "suggestion", key: "suggestion", width: 360, ellipsis: true},
    {title: t("monitor:Last Checked"), dataIndex: "lastCheckedAt", key: "lastCheckedAt", width: 190, render: formatTime},
  ], [t]);

  const eventColumns = useMemo(() => [
    {
      title: t("monitor:Time"),
      key: "time",
      width: 190,
      render: (_, record) => formatTime(eventDisplayTime(record)),
    },
    {
      title: t("policy:Type"),
      dataIndex: "type",
      key: "type",
      width: 110,
      render: value => <Tag color={eventTypeColor[value] || "default"}>{value || "-"}</Tag>,
    },
    {title: t("policy:Namespace"), dataIndex: "namespace", key: "namespace", width: 150},
    {
      title: t("monitor:Object"),
      key: "object",
      width: 260,
      ellipsis: true,
      render: (_, record) => `${record.involvedObjectKind || "-"} / ${record.involvedObjectName || "-"}`,
    },
    {title: t("monitor:Reason"), dataIndex: "reason", key: "reason", width: 180},
    {
      title: t("monitor:Message"),
      dataIndex: "message",
      key: "message",
      width: 420,
      ellipsis: true,
    },
    {title: t("monitor:Count"), dataIndex: "count", key: "count", width: 90},
    {
      title: t("general:Action"),
      key: "action",
      width: 110,
      render: (_, record) => (
        <Button size="small" onClick={() => setSelectedEvent(record)}>
          {t("monitor:Details")}
        </Button>
      ),
    },
  ], [t]);

  const issueColumns = useMemo(() => [
    {
      title: t("trivy:Severity"),
      dataIndex: "severity",
      key: "severity",
      width: 120,
      render: value => <Tag color={severityColor[value] || "default"}>{t(`monitor:severity ${value || "info"}`)}</Tag>,
    },
    {
      title: t("monitor:Object"),
      key: "object",
      width: 280,
      ellipsis: true,
      render: (_, record) => objectLabel(record),
    },
    {title: t("monitor:Reason"), dataIndex: "reason", key: "reason", width: 180},
    {
      title: t("monitor:Message"),
      dataIndex: "message",
      key: "message",
      width: 360,
      ellipsis: true,
    },
    {
      title: t("monitor:Suggestion"),
      dataIndex: "suggestion",
      key: "suggestion",
      width: 360,
      ellipsis: true,
    },
    {
      title: t("monitor:Last Seen"),
      dataIndex: "lastSeenAt",
      key: "lastSeenAt",
      width: 190,
      render: formatTime,
    },
    {
      title: t("general:Action"),
      key: "action",
      width: 110,
      render: (_, record) => (
        <Button size="small" onClick={() => openDiagnosis(record)}>
          {t("monitor:Diagnosis")}
        </Button>
      ),
    },
  ], [openDiagnosis, t]);

  const diagnosisEventColumns = useMemo(() => [
    {
      title: t("monitor:Time"),
      key: "time",
      width: 180,
      render: (_, record) => formatTime(eventDisplayTime(record)),
    },
    {
      title: t("policy:Type"),
      dataIndex: "type",
      key: "type",
      width: 100,
      render: value => <Tag color={eventTypeColor[value] || "default"}>{value || "-"}</Tag>,
    },
    {title: t("monitor:Reason"), dataIndex: "reason", key: "reason", width: 160},
    {
      title: t("monitor:Message"),
      dataIndex: "message",
      key: "message",
      width: 420,
      ellipsis: true,
    },
    {title: t("monitor:Count"), dataIndex: "count", key: "count", width: 80},
  ], [t]);

  const overallStatus = summary?.overallStatus || "unknown";
  const statusColor = statusMeta[overallStatus]?.color || "default";
  const statusIcon = statusMeta[overallStatus]?.icon || statusMeta.unknown.icon;
  const statusValueColor = statusColor === "green" ? "#16a34a" : statusColor === "gold" ? "#d48806" : statusColor === "red" ? "#cf1322" : undefined;
  const timeRangeOptions = [
    {label: t("monitor:Last 1 Hour"), value: "1h"},
    {label: t("monitor:Last 6 Hours"), value: "6h"},
    {label: t("monitor:Last 24 Hours"), value: "24h"},
    {label: t("monitor:Last 7 Days"), value: "7d"},
    {label: t("monitor:Custom Range"), value: "custom"},
  ];
  const waitingForCustomRange = timePreset === "custom" && !customTimeRange;
  const metricsUnavailableError = Object.keys(metricErrors).length === MONITOR_METRIC_REQUESTS.length ? Object.values(metricErrors)[0] : null;

  if (loading && !summary) {
    return (
      <div style={{display: "flex", justifyContent: "center", alignItems: "center", height: 400}}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <div style={{padding: 24}}>
      {error && (
        <Alert
          type="error"
          showIcon
          message={t("monitor:Failed to load monitor data")}
          description={error}
          style={{marginBottom: 16, borderRadius: 8}}
        />
      )}

      <Row gutter={[16, 16]}>
        <Col xs={24} sm={12} lg={6}>
          <Card variant="borderless" style={{borderRadius: 8, border: "1px solid #e8e8e8", minHeight: 110}}>
            <Statistic
              title={t("monitor:Overall Status")}
              value={t(`monitor:status ${overallStatus}`)}
              prefix={React.cloneElement(statusIcon, {style: {color: statusValueColor}})}
              valueStyle={{color: statusValueColor, fontSize: 24}}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card variant="borderless" style={{borderRadius: 8, border: "1px solid #e8e8e8", minHeight: 110}}>
            <Statistic
              title={t("monitor:Ready Nodes")}
              value={summary?.nodeReady ?? 0}
              suffix={`/ ${summary?.nodeTotal ?? 0}`}
              prefix={<ClusterOutlined style={{color: "#1677ff"}} />}
              valueStyle={{color: "#1677ff"}}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card variant="borderless" style={{borderRadius: 8, border: "1px solid #e8e8e8", minHeight: 110}}>
            <Statistic
              title={t("monitor:Running Pods")}
              value={summary?.podRunning ?? 0}
              suffix={`/ ${summary?.podTotal ?? 0}`}
              prefix={<AppstoreOutlined style={{color: "#0ea5e9"}} />}
              valueStyle={{color: "#0ea5e9"}}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card variant="borderless" style={{borderRadius: 8, border: "1px solid #e8e8e8", minHeight: 110}}>
            <Statistic
              title={t("monitor:Abnormal Pods")}
              value={summary?.podAbnormal ?? 0}
              prefix={<WarningOutlined style={{color: (summary?.podAbnormal ?? 0) > 0 ? "#cf1322" : "#14b8a6"}} />}
              valueStyle={{color: (summary?.podAbnormal ?? 0) > 0 ? "#cf1322" : "#14b8a6"}}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card variant="borderless" style={{borderRadius: 8, border: "1px solid #e8e8e8", minHeight: 110}}>
            <Statistic
              title={t("monitor:Warning Events")}
              value={summary?.warningEventCount ?? 0}
              prefix={<BellOutlined style={{color: (summary?.warningEventCount ?? 0) > 0 ? "#d48806" : "#14b8a6"}} />}
              valueStyle={{color: (summary?.warningEventCount ?? 0) > 0 ? "#d48806" : "#14b8a6"}}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card variant="borderless" style={{borderRadius: 8, border: "1px solid #e8e8e8", minHeight: 110}}>
            <Statistic
              title={t("monitor:Critical Checks")}
              value={summary?.criticalCheckCount ?? 0}
              prefix={<CloseCircleOutlined style={{color: (summary?.criticalCheckCount ?? 0) > 0 ? "#cf1322" : "#14b8a6"}} />}
              valueStyle={{color: (summary?.criticalCheckCount ?? 0) > 0 ? "#cf1322" : "#14b8a6"}}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card variant="borderless" style={{borderRadius: 8, border: "1px solid #e8e8e8", minHeight: 110}}>
            <Statistic
              title={t("monitor:Warning Checks")}
              value={summary?.warningCheckCount ?? 0}
              prefix={<ExclamationCircleOutlined style={{color: (summary?.warningCheckCount ?? 0) > 0 ? "#d48806" : "#14b8a6"}} />}
              valueStyle={{color: (summary?.warningCheckCount ?? 0) > 0 ? "#d48806" : "#14b8a6"}}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card variant="borderless" style={{borderRadius: 8, border: "1px solid #e8e8e8", minHeight: 110}}>
            <Statistic
              title={t("monitor:Last Checked")}
              value={formatTime(summary?.lastCheckedAt)}
              prefix={<FieldTimeOutlined style={{color: "#6366f1"}} />}
              valueStyle={{fontSize: 16, color: "#6366f1"}}
            />
          </Card>
        </Col>
      </Row>

      <Card
        title={t("monitor:Resource Trends")}
        variant="borderless"
        style={{borderRadius: 8, border: "1px solid #e8e8e8", marginTop: 16}}
      >
        <Space wrap size={[12, 12]} style={{marginBottom: 16}}>
          <Segmented
            options={timeRangeOptions}
            value={timePreset}
            onChange={setTimePreset}
          />
          {timePreset === "custom" && (
            <RangePicker
              showTime={{format: "HH:mm"}}
              format="YYYY-MM-DD HH:mm"
              onChange={dates => {
                const validDates = dates?.length === 2 && dates.every(Boolean);
                setCustomTimeRange(validDates ? dates.map(date => date.valueOf()) : null);
              }}
            />
          )}
          <Space size={8}>
            <Text>{t("monitor:Auto Refresh")}</Text>
            <Switch checked={autoRefresh} onChange={setAutoRefresh} />
          </Space>
          <Button
            icon={<ReloadOutlined />}
            loading={metricsLoading}
            disabled={waitingForCustomRange}
            onClick={fetchMetricTrends}
          >
            {t("general:Refresh")}
          </Button>
        </Space>

        {waitingForCustomRange ? (
          <Alert
            type="info"
            showIcon
            message={t("monitor:Select a custom time range")}
          />
        ) : (
          <>
            {metricsUnavailableError && (
              <Alert
                type="warning"
                showIcon
                message={t("monitor:Failed to load resource trends")}
                description={metricsUnavailableError}
                style={{marginBottom: 16}}
              />
            )}
            <Row gutter={[16, 16]}>
              {metricChartDefinitions.map(chart => (
                <Col xs={24} xl={12} key={chart.key}>
                  <Card
                    size="small"
                    title={t(`monitor:${chart.title}`)}
                    style={{height: "100%"}}
                  >
                    <MonitorMetricChart
                      dataSources={[{data: metricData[chart.key], label: t(`monitor:${chart.title}`)}]}
                      unit={chart.unit}
                      loading={metricsLoading}
                      error={metricsUnavailableError ? null : metricErrors[chart.key]}
                      emptyDescription={t("monitor:No metric data")}
                    />
                  </Card>
                </Col>
              ))}
            </Row>
          </>
        )}
      </Card>

      <Card
        title={t("monitor:Health Checks")}
        variant="borderless"
        style={{borderRadius: 8, border: "1px solid #e8e8e8", marginTop: 16}}
        extra={
          <Button icon={<ReloadOutlined />} loading={loading} onClick={fetchOverview}>
            {t("general:Refresh")}
          </Button>
        }
      >
        <Table
          rowKey="id"
          columns={checkColumns}
          dataSource={checks}
          loading={loading}
          size="middle"
          pagination={false}
          scroll={{x: 1560}}
        />
      </Card>

      <Card
        title={t("monitor:Monitor Issues")}
        variant="borderless"
        style={{borderRadius: 8, border: "1px solid #e8e8e8", marginTop: 16}}
        extra={
          <Button icon={<ReloadOutlined />} loading={issuesLoading} onClick={fetchIssues}>
            {t("general:Refresh")}
          </Button>
        }
      >
        {issuesError && (
          <Alert
            type="error"
            showIcon
            message={t("monitor:Failed to load monitor issues")}
            description={issuesError}
            style={{marginBottom: 16, borderRadius: 8}}
          />
        )}
        <Table
          rowKey="id"
          columns={issueColumns}
          dataSource={issues}
          loading={issuesLoading}
          size="middle"
          pagination={{pageSize: 20}}
          scroll={{x: 1600}}
          onRow={(record) => ({
            onDoubleClick: () => openDiagnosis(record),
          })}
        />
      </Card>

      <Card
        title={t("monitor:Event Center")}
        variant="borderless"
        style={{borderRadius: 8, border: "1px solid #e8e8e8", marginTop: 16}}
        extra={
          <Space>
            <Input
              allowClear
              value={namespaceFilter}
              onChange={e => setNamespaceFilter(e.target.value)}
              onPressEnter={() => fetchEvents(namespaceFilter)}
              placeholder={t("policy:Namespace")}
              style={{width: 220}}
            />
            <Button icon={<ReloadOutlined />} loading={eventsLoading} onClick={() => fetchEvents(namespaceFilter)}>
              {t("general:Refresh")}
            </Button>
          </Space>
        }
      >
        {eventsError && (
          <Alert
            type="error"
            showIcon
            message={t("monitor:Failed to load events")}
            description={eventsError}
            style={{marginBottom: 16, borderRadius: 8}}
          />
        )}
        <Table
          rowKey={(record, index) => `${record.namespace}-${record.involvedObjectKind}-${record.involvedObjectName}-${record.reason}-${eventDisplayTime(record)}-${index}`}
          columns={eventColumns}
          dataSource={events}
          loading={eventsLoading}
          size="middle"
          pagination={{pageSize: 20}}
          scroll={{x: 1510}}
          onRow={(record) => ({
            onDoubleClick: () => setSelectedEvent(record),
          })}
        />
      </Card>

      <Modal
        title={t("monitor:Event Details")}
        open={!!selectedEvent}
        onCancel={() => setSelectedEvent(null)}
        footer={<Button onClick={() => setSelectedEvent(null)}>{t("general:Close")}</Button>}
        width={760}
        destroyOnHidden
      >
        {selectedEvent && (
          <Space direction="vertical" size={16} style={{width: "100%"}}>
            <Descriptions bordered size="small" column={1}>
              <Descriptions.Item label={t("monitor:Time")}>{formatTime(eventDisplayTime(selectedEvent))}</Descriptions.Item>
              <Descriptions.Item label={t("policy:Type")}><Tag color={eventTypeColor[selectedEvent.type] || "default"}>{selectedEvent.type || "-"}</Tag></Descriptions.Item>
              <Descriptions.Item label={t("policy:Namespace")}>{selectedEvent.namespace || "-"}</Descriptions.Item>
              <Descriptions.Item label={t("monitor:Object")}>{selectedEvent.involvedObjectKind || "-"} / {selectedEvent.involvedObjectName || "-"}</Descriptions.Item>
              <Descriptions.Item label={t("monitor:Reason")}>{selectedEvent.reason || "-"}</Descriptions.Item>
              <Descriptions.Item label={t("monitor:Count")}>{selectedEvent.count ?? 0}</Descriptions.Item>
              <Descriptions.Item label={t("monitor:Source")}>{selectedEvent.source || selectedEvent.reportingController || "-"}</Descriptions.Item>
            </Descriptions>
            <Paragraph style={{whiteSpace: "pre-wrap", marginBottom: 0}}>
              {selectedEvent.message || "-"}
            </Paragraph>
          </Space>
        )}
      </Modal>

      <Drawer
        title={t("monitor:Diagnosis Context")}
        open={!!selectedIssue}
        onClose={() => {
          setSelectedIssue(null);
          setDiagnosis(null);
          setDiagnosisError(null);
        }}
        width={820}
        destroyOnHidden
      >
        {diagnosisError && (
          <Alert
            type="error"
            showIcon
            message={t("monitor:Failed to load diagnosis")}
            description={diagnosisError}
            style={{marginBottom: 16, borderRadius: 8}}
          />
        )}
        <Spin spinning={diagnosisLoading}>
          {diagnosis && (
            <Space direction="vertical" size={16} style={{width: "100%"}}>
              <Descriptions bordered size="small" column={1}>
                <Descriptions.Item label={t("monitor:Object")}>{objectLabel(diagnosis.issue)}</Descriptions.Item>
                <Descriptions.Item label={t("trivy:Severity")}><Tag color={severityColor[diagnosis.issue?.severity] || "default"}>{t(`monitor:severity ${diagnosis.issue?.severity || "info"}`)}</Tag></Descriptions.Item>
                <Descriptions.Item label={t("monitor:Reason")}>{diagnosis.issue?.reason || "-"}</Descriptions.Item>
                <Descriptions.Item label={t("monitor:Message")}>{diagnosis.issue?.message || "-"}</Descriptions.Item>
                <Descriptions.Item label={t("monitor:Suggestion")}>{diagnosis.issue?.suggestion || "-"}</Descriptions.Item>
                <Descriptions.Item label={t("monitor:Last Seen")}>{formatTime(diagnosis.issue?.lastSeenAt)}</Descriptions.Item>
              </Descriptions>

              <div>
                <Text strong>{t("monitor:Related Events")}</Text>
                <Table
                  size="small"
                  rowKey={(record, index) => `${record.namespace}-${record.reason}-${eventDisplayTime(record)}-${index}`}
                  columns={diagnosisEventColumns}
                  dataSource={diagnosis.relatedEvents || []}
                  pagination={false}
                  scroll={{x: 920}}
                  style={{marginTop: 8}}
                />
              </div>

              <div>
                <Text strong>{t("monitor:Log Preview")}</Text>
                <Space direction="vertical" size={12} style={{width: "100%", marginTop: 8}}>
                  {(diagnosis.logPreview || []).map((log, index) => (
                    <div key={`${log.container}-${log.previous}-${index}`}>
                      <Space style={{marginBottom: 6}}>
                        <Tag>{log.container || "-"}</Tag>
                        <Tag color={log.previous ? "gold" : "blue"}>{log.previous ? t("monitor:Previous") : t("monitor:Current")}</Tag>
                        <Tag>{`tail ${log.tailLines || 0}`}</Tag>
                      </Space>
                      <Paragraph style={{whiteSpace: "pre-wrap", maxHeight: 220, overflow: "auto", padding: 12, border: "1px solid #f0f0f0", borderRadius: 6, marginBottom: 0}}>
                        {log.error || log.content || "-"}
                      </Paragraph>
                    </div>
                  ))}
                </Space>
              </div>

              <div>
                <Text strong>{t("monitor:Diagnosis")}</Text>
                <Paragraph style={{whiteSpace: "pre-wrap", maxHeight: 260, overflow: "auto", padding: 12, border: "1px solid #f0f0f0", borderRadius: 6, marginTop: 8, marginBottom: 0}}>
                  {JSON.stringify(diagnosis.aiContext || {}, null, 2)}
                </Paragraph>
              </div>
            </Space>
          )}
        </Spin>
      </Drawer>
    </div>
  );
}

export default MonitorPage;
