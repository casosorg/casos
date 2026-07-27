import React from "react";
import {Alert, Col, Row, Space, Typography} from "antd";
import {useTranslation} from "react-i18next";
import MetricChartCard from "./MetricChartCard";
import MonitorTimeRangeSelector from "./MonitorTimeRangeSelector";
import SeriesModeControl from "./SeriesModeControl";
import SeriesObjectPicker from "./SeriesObjectPicker";
import useMonitoringOverview from "./useMonitoringOverview";

const {Paragraph} = Typography;

const monitorNoteTranslationKeys = {
  "Network metrics are shown at Pod total level only. Container network breakdown is not enabled because container_network_* series may duplicate the Pod network namespace.": "Pod network metrics total only note",
  "Metrics are calculated from the workload's current Pods. Deleted Pods from earlier rollouts are not included unless kube-state-metrics support is added later.": "Workload current Pods metrics note",
  "CPU and memory request/limit references use the current workload configuration.": "Workload request limit current config note",
  "PVC storage usage uses kubelet_volume_stats_* metrics. IOPS and throughput are not shown unless a reliable CSI/exporter source is available.": "PVC storage metrics source note",
};

function monitorNoteMessage(note, t) {
  const key = monitorNoteTranslationKeys[note];
  return key ? t(`monitor:${key}`) : note;
}

function MonitoringTab({
  active,
  fetcher,
  modes,
  initialMode = "total",
  podLimit = 10,
  renderSummary,
  renderBelow,
}) {
  const {t} = useTranslation();
  const state = useMonitoringOverview({fetcher, active, initialMode, initialPodLimit: podLimit});
  const pods = state.data?.pods || [];
  const hardLimit = state.data?.limits?.hardSeriesLimit || 20;
  const limit = state.data?.limits?.podLimit || podLimit;
  const showPodPicker = state.mode === "pod" && pods.length > limit;

  return (
    <Space direction="vertical" size={16} style={{width: "100%"}}>
      <Space direction="vertical" size={12} style={{width: "100%"}}>
        <Space wrap size={[12, 12]}>
          <MonitorTimeRangeSelector
            timePreset={state.timePreset}
            onTimePresetChange={state.setTimePreset}
            onCustomRangeChange={state.setCustomTimeRange}
            autoRefresh={state.autoRefresh}
            onAutoRefreshChange={state.setAutoRefresh}
            onRefresh={state.refresh}
            loading={state.loading}
            disabled={state.waitingForCustomRange}
          />
          <SeriesModeControl
            value={state.mode}
            modes={modes}
            onChange={value => {
              state.setMode(value);
              state.setSelectedObjects([]);
            }}
          />
        </Space>
        {showPodPicker && (
          <SeriesObjectPicker
            objects={pods}
            selectedObjects={state.selectedObjects}
            onChange={state.setSelectedObjects}
            limit={limit}
            hardLimit={hardLimit}
            disabled={state.loading}
          />
        )}
      </Space>

      {state.waitingForCustomRange && (
        <Alert type="info" showIcon message={t("monitor:Select a custom time range")} />
      )}
      {state.error && (
        <Alert type="error" showIcon message={t("monitor:Failed to load monitoring data")} description={state.error} />
      )}
      {(state.data?.notes || []).map(note => (
        <Alert key={note} type="info" showIcon message={monitorNoteMessage(note, t)} />
      ))}

      {renderSummary?.(state.data, state)}

      <Row gutter={[16, 16]}>
        {(state.data?.metrics || []).map(metric => (
          <Col xs={24} xl={12} key={metric.key}>
            <MetricChartCard metric={metric} loading={state.loading} />
          </Col>
        ))}
      </Row>

      {state.data && state.data.metrics?.length === 0 && !state.loading && (
        <Paragraph type="secondary">{t("monitor:No metric data")}</Paragraph>
      )}

      {renderBelow?.(state.data, state)}
    </Space>
  );
}

export default MonitoringTab;
