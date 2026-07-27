/* eslint-env jest */

import {
  buildMonitorTimeAxis,
  buildMonitorTimeRange,
  formatMonitorMetricValue,
  formatMonitorTimeAxisLabel,
  formatMonitorTooltipTime,
  monitorStepForDuration,
  shouldRunMonitorAutoRefresh,
  stableMonitorSeriesColor,
  toMonitorChartSeries
} from "./monitorMetrics";

describe("monitor metric helpers", () => {
  test("builds preset and custom time ranges", () => {
    const now = Date.parse("2026-07-15T08:00:00Z");
    expect(buildMonitorTimeRange("1h", null, now)).toEqual({
      start: "2026-07-15T07:00:00.000Z",
      end: "2026-07-15T08:00:00.000Z",
      step: "15s",
    });
    expect(buildMonitorTimeRange("7d", null, now)).toEqual({
      start: "2026-07-08T08:00:00.000Z",
      end: "2026-07-15T08:00:00.000Z",
      step: "30m",
    });
    expect(buildMonitorTimeRange("custom", [now - 24 * 60 * 60 * 1000, now], now)).toEqual({
      start: "2026-07-14T08:00:00.000Z",
      end: "2026-07-15T08:00:00.000Z",
      step: "5m",
    });
    expect(buildMonitorTimeRange("custom", null, now)).toBeNull();
  });

  test("chooses a bounded step for every preset duration", () => {
    expect(monitorStepForDuration(60 * 60 * 1000)).toBe("15s");
    expect(monitorStepForDuration(6 * 60 * 60 * 1000)).toBe("1m");
    expect(monitorStepForDuration(24 * 60 * 60 * 1000)).toBe("5m");
    expect(monitorStepForDuration(7 * 24 * 60 * 60 * 1000)).toBe("30m");
  });

  test("keeps the chart axis on the complete requested range", () => {
    expect(buildMonitorTimeAxis({
      start: "2026-07-15T07:00:00.000Z",
      end: "2026-07-15T08:00:00.000Z",
    })).toEqual({
      min: Date.parse("2026-07-15T07:00:00.000Z"),
      max: Date.parse("2026-07-15T08:00:00.000Z"),
      duration: 60 * 60 * 1000,
    });
    expect(buildMonitorTimeAxis({start: "invalid", end: "2026-07-15T08:00:00.000Z"})).toBeNull();
  });

  test("formats axis labels according to the visible duration", () => {
    const point = new Date(2026, 6, 15, 8, 5, 9).getTime();
    expect(formatMonitorTimeAxisLabel(point, 60 * 60 * 1000)).toBe("08:05");
    expect(formatMonitorTimeAxisLabel(point, 24 * 60 * 60 * 1000)).toBe("07-15\n08:05");
    expect(formatMonitorTimeAxisLabel(point, 7 * 24 * 60 * 60 * 1000)).toBe("07-15");
    expect(formatMonitorTimeAxisLabel(point, 30 * 24 * 60 * 60 * 1000)).toBe("2026-07-15");
    expect(formatMonitorTooltipTime(point)).toBe("2026-07-15 08:05:09");
  });

  test("converts aligned API arrays into ECharts points", () => {
    const chartSeries = toMonitorChartSeries([{
      label: "CPU",
      data: {
        scope: "cluster",
        series: [{object: "cluster", timestamps: [100, 160], values: [12.5, 25]}],
      },
    }]);
    expect(chartSeries).toHaveLength(1);
    expect(chartSeries[0].name).toBe("CPU");
    expect(chartSeries[0].data).toEqual([[100000, 12.5], [160000, 25]]);
    expect(chartSeries[0].lineStyle.color).toBe(stableMonitorSeriesColor("CPU"));
  });

  test("sorts chart series by stable display name", () => {
    const chartSeries = toMonitorChartSeries([{
      label: "Pods",
      data: {
        scope: "pod",
        series: [
          {object: "default/web", timestamps: [100], values: [1]},
          {object: "default/api", timestamps: [100], values: [2]},
        ],
      },
    }]);
    expect(chartSeries.map(series => series.name)).toEqual(["default/api", "default/web"]);
    expect(stableMonitorSeriesColor("default/api")).toBe(stableMonitorSeriesColor("default/api"));
  });

  test("applies monitoring auto-refresh rules", () => {
    expect(shouldRunMonitorAutoRefresh({autoRefresh: true, timePreset: "1h", active: true, documentHidden: false})).toBe(true);
    expect(shouldRunMonitorAutoRefresh({autoRefresh: true, timePreset: "1h", active: false, documentHidden: false})).toBe(false);
    expect(shouldRunMonitorAutoRefresh({autoRefresh: true, timePreset: "1h", active: true, documentHidden: true})).toBe(false);
    expect(shouldRunMonitorAutoRefresh({autoRefresh: false, timePreset: "1h", active: true, documentHidden: false})).toBe(false);
    expect(shouldRunMonitorAutoRefresh({autoRefresh: true, timePreset: "custom", customTimeRange: [1, 2], active: true, documentHidden: false})).toBe(false);
  });

  test("formats metric units", () => {
    expect(formatMonitorMetricValue(12.345, "percent")).toBe("12.3%");
    expect(formatMonitorMetricValue(1536, "bytes_per_second")).toBe("1.5 KiB/s");
  });
});
