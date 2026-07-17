import React, {useEffect, useMemo, useRef} from "react";
import {Alert, Empty, Spin} from "antd";
import * as echarts from "echarts";
import {
  buildMonitorTimeAxis,
  formatMonitorMetricValue,
  formatMonitorTimeAxisLabel,
  formatMonitorTooltipTime,
  toMonitorChartSeries
} from "./monitorMetrics";

const METRIC_CHART_COLORS = [
  "#1677ff",
  "#0ea5e9",
  "#14b8a6",
  "#6366f1",
  "#8b5cf6",
  "#0891b2",
  "#0958d9",
  "#38bdf8",
];

function MonitorMetricChart({dataSources, unit, loading, error, emptyDescription}) {
  const containerRef = useRef(null);
  const chartRef = useRef(null);
  const chartSeries = useMemo(() => toMonitorChartSeries(dataSources), [dataSources]);
  const resolvedUnit = unit || dataSources?.find(source => source.data?.unit)?.data?.unit || "";
  const timeAxis = useMemo(() => {
    const rangeSource = dataSources?.find(source => source.data?.start && source.data?.end);
    return buildMonitorTimeAxis(rangeSource?.data);
  }, [dataSources]);
  const option = useMemo(() => {
    if (chartSeries.length === 0) {return null;}
    return {
      animation: false,
      color: METRIC_CHART_COLORS,
      tooltip: {
        trigger: "axis",
        confine: true,
        formatter: params => formatMonitorMetricTooltip(params, resolvedUnit),
      },
      legend: {type: "scroll", top: 0, left: 8, right: 8},
      grid: {left: 16, right: 24, top: 48, bottom: 16, containLabel: true},
      xAxis: {
        type: "time",
        boundaryGap: false,
        min: timeAxis?.min,
        max: timeAxis?.max,
        splitNumber: 6,
        axisLabel: {
          formatter: timeAxis ? value => formatMonitorTimeAxisLabel(value, timeAxis.duration) : undefined,
          hideOverlap: true,
          showMinLabel: true,
          showMaxLabel: true,
        },
      },
      yAxis: {
        type: "value",
        min: resolvedUnit === "percent" ? 0 : undefined,
        max: resolvedUnit === "percent" ? 100 : undefined,
        scale: resolvedUnit !== "percent",
        axisLabel: {formatter: value => formatMonitorMetricValue(value, resolvedUnit)},
      },
      series: chartSeries,
    };
  }, [chartSeries, resolvedUnit, timeAxis]);

  useEffect(() => {
    if (!containerRef.current) {return;}
    const chart = echarts.init(containerRef.current);
    chartRef.current = chart;
    const observer = new ResizeObserver(() => chart.resize());
    observer.observe(containerRef.current);
    return () => {
      observer.disconnect();
      chart.dispose();
      chartRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!chartRef.current) {return;}
    if (!option) {
      chartRef.current.clear();
      return;
    }
    chartRef.current.setOption(option, {notMerge: true});
    chartRef.current.resize();
  }, [option]);

  const hasData = chartSeries.length > 0;
  return (
    <Spin spinning={loading}>
      {error && (
        <Alert
          type="warning"
          showIcon
          message={error}
          style={{marginBottom: 12}}
        />
      )}
      <div style={{position: "relative", minHeight: 280}}>
        <div ref={containerRef} style={{height: 280, visibility: hasData ? "visible" : "hidden"}} />
        {!hasData && !loading && (
          <div style={{position: "absolute", inset: 0, display: "flex", alignItems: "center", justifyContent: "center"}}>
            <Empty
              image={Empty.PRESENTED_IMAGE_SIMPLE}
              description={error || emptyDescription}
            />
          </div>
        )}
      </div>
    </Spin>
  );
}

function formatMonitorMetricTooltip(params, unit) {
  const points = Array.isArray(params) ? params : [params];
  const firstPoint = points.find(Boolean);
  if (!firstPoint) {return "";}
  const timestamp = firstPoint.axisValue ?? (Array.isArray(firstPoint.value) ? firstPoint.value[0] : undefined);
  const rows = [`<strong>${formatMonitorTooltipTime(timestamp)}</strong>`];
  points.filter(Boolean).forEach(point => {
    const value = Array.isArray(point.value) ? point.value[1] : point.value;
    rows.push(`${point.marker || ""}${escapeMonitorTooltipText(point.seriesName || "")}: ${escapeMonitorTooltipText(formatMonitorMetricValue(value, unit))}`);
  });
  return rows.join("<br/>");
}

function escapeMonitorTooltipText(value) {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

export default MonitorMetricChart;
