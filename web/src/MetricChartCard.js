import React from "react";
import {Alert, Card, Tag, Typography} from "antd";
import {useTranslation} from "react-i18next";
import MonitorMetricChart from "./MonitorMetricChart";

const {Text} = Typography;

function MetricChartCard({metric, loading}) {
  const {t} = useTranslation();
  const title = metric?.title ? t(`monitor:${metric.title}`, metric.title) : "-";
  const error = metric?.status === "error" ? metric?.error?.message : null;
  const empty = metric?.status === "empty";
  return (
    <Card
      size="small"
      title={title}
      extra={metric?.truncated ? <Tag color="gold">{t("monitor:Limited")}</Tag> : null}
      style={{height: "100%"}}
    >
      {metric?.error?.code === "prometheus_not_configured" && (
        <Alert
          type="info"
          showIcon
          message={t("monitor:Prometheus is not configured")}
          style={{marginBottom: 12}}
        />
      )}
      {empty && (
        <Text type="secondary">{t("monitor:No metric data")}</Text>
      )}
      <MonitorMetricChart
        dataSources={[{data: metric?.data, label: title}]}
        unit={metric?.unit}
        loading={loading}
        error={error}
        emptyDescription={t("monitor:No metric data")}
      />
    </Card>
  );
}

export default MetricChartCard;
