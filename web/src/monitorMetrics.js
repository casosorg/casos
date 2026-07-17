export const MONITOR_AUTO_REFRESH_INTERVAL_MS = 60 * 1000;

export const MONITOR_METRIC_REQUESTS = [
  {key: "cpu", scope: "cluster", metric: "cpu"},
  {key: "memory", scope: "cluster", metric: "memory"},
  {key: "networkReceive", scope: "cluster", metric: "network_receive"},
  {key: "networkTransmit", scope: "cluster", metric: "network_transmit"},
  {key: "disk", scope: "node", metric: "disk"},
  {key: "storage", scope: "pvc", metric: "storage"},
];

const MONITOR_TIME_PRESETS = {
  "1h": {duration: 60 * 60 * 1000, step: "15s"},
  "6h": {duration: 6 * 60 * 60 * 1000, step: "1m"},
  "24h": {duration: 24 * 60 * 60 * 1000, step: "5m"},
  "7d": {duration: 7 * 24 * 60 * 60 * 1000, step: "30m"},
};

export function buildMonitorTimeRange(preset, customRange, now = Date.now()) {
  let start;
  let end;
  let step;
  if (preset === "custom") {
    if (!Array.isArray(customRange) || customRange.length !== 2) {return null;}
    start = Number(customRange[0]);
    end = Number(customRange[1]);
    if (!Number.isFinite(start) || !Number.isFinite(end) || start >= end) {return null;}
    step = monitorStepForDuration(end - start);
  } else {
    const selected = MONITOR_TIME_PRESETS[preset];
    if (!selected) {return null;}
    end = now;
    start = end - selected.duration;
    step = selected.step;
  }
  return {
    start: new Date(start).toISOString(),
    end: new Date(end).toISOString(),
    step,
  };
}

export function buildMonitorTimeAxis(timeRange) {
  const min = Date.parse(timeRange?.start || "");
  const max = Date.parse(timeRange?.end || "");
  if (!Number.isFinite(min) || !Number.isFinite(max) || min >= max) {return null;}
  return {min, max, duration: max - min};
}

export function formatMonitorTimeAxisLabel(value, duration) {
  const date = monitorDate(value);
  if (!date) {return "";}
  if (duration <= 6 * 60 * 60 * 1000) {
    return `${padMonitorTime(date.getHours())}:${padMonitorTime(date.getMinutes())}`;
  }
  if (duration <= 3 * 24 * 60 * 60 * 1000) {
    return `${padMonitorTime(date.getMonth() + 1)}-${padMonitorTime(date.getDate())}\n${padMonitorTime(date.getHours())}:${padMonitorTime(date.getMinutes())}`;
  }
  if (duration <= 14 * 24 * 60 * 60 * 1000) {
    return `${padMonitorTime(date.getMonth() + 1)}-${padMonitorTime(date.getDate())}`;
  }
  return `${date.getFullYear()}-${padMonitorTime(date.getMonth() + 1)}-${padMonitorTime(date.getDate())}`;
}

export function formatMonitorTooltipTime(value) {
  const date = monitorDate(value);
  if (!date) {return "-";}
  return `${date.getFullYear()}-${padMonitorTime(date.getMonth() + 1)}-${padMonitorTime(date.getDate())} ${padMonitorTime(date.getHours())}:${padMonitorTime(date.getMinutes())}:${padMonitorTime(date.getSeconds())}`;
}

export function monitorStepForDuration(duration) {
  if (duration <= 60 * 60 * 1000) {return "15s";}
  if (duration <= 6 * 60 * 60 * 1000) {return "1m";}
  if (duration <= 24 * 60 * 60 * 1000) {return "5m";}
  if (duration <= 7 * 24 * 60 * 60 * 1000) {return "30m";}
  return "1h";
}

export function toMonitorChartSeries(dataSources) {
  const result = [];
  (dataSources || []).forEach(source => {
    const response = source?.data;
    (response?.series || []).forEach(series => {
      const timestamps = series.timestamps || [];
      const values = series.values || [];
      const points = [];
      const length = Math.min(timestamps.length, values.length);
      for (let i = 0; i < length; i++) {
        const timestamp = Number(timestamps[i]);
        const value = Number(values[i]);
        if (Number.isFinite(timestamp) && Number.isFinite(value)) {
          points.push([timestamp * 1000, value]);
        }
      }
      if (points.length === 0) {return;}
      const objectName = series.object || response.scope || "";
      result.push({
        name: objectName && objectName !== "cluster" ? objectName : source.label,
        type: "line",
        showSymbol: false,
        connectNulls: false,
        sampling: "lttb",
        data: points,
      });
    });
  });
  return result;
}

export function formatMonitorMetricValue(value, unit) {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) {return "-";}
  switch (unit) {
  case "percent":
    return `${numeric.toFixed(1)}%`;
  case "cores":
    return `${numeric.toFixed(numeric < 1 ? 3 : 2)} cores`;
  case "bytes":
    return formatBytes(numeric);
  case "bytes_per_second":
    return `${formatBytes(numeric)}/s`;
  default:
    return numeric.toLocaleString();
  }
}

function formatBytes(value) {
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let amount = Math.abs(value);
  let index = 0;
  while (amount >= 1024 && index < units.length - 1) {
    amount /= 1024;
    index++;
  }
  if (value < 0) {amount = -amount;}
  return `${amount.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

function monitorDate(value) {
  const timestamp = typeof value === "number" ? value : Date.parse(value);
  if (!Number.isFinite(timestamp)) {return null;}
  const date = new Date(timestamp);
  return Number.isFinite(date.getTime()) ? date : null;
}

function padMonitorTime(value) {
  return String(value).padStart(2, "0");
}
