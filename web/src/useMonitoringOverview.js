import {useCallback, useEffect, useRef, useState} from "react";
import {
  MONITOR_AUTO_REFRESH_INTERVAL_MS,
  buildMonitorTimeRange,
  shouldRunMonitorAutoRefresh
} from "./monitorMetrics";

function useMonitoringOverview({
  fetcher,
  active = true,
  initialMode = "total",
  initialPodLimit = 10,
}) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [timePreset, setTimePreset] = useState("1h");
  const [customTimeRange, setCustomTimeRange] = useState(null);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [mode, setMode] = useState(initialMode);
  const [selectedObjects, setSelectedObjects] = useState([]);
  const [podLimit, setPodLimit] = useState(initialPodLimit);
  const requestRef = useRef(0);
  const abortRef = useRef(null);

  const waitingForCustomRange = timePreset === "custom" && !customTimeRange;

  const refresh = useCallback(() => {
    if (!active || !fetcher) {return;}
    const timeRange = buildMonitorTimeRange(timePreset, customTimeRange);
    if (!timeRange) {
      abortRef.current?.abort();
      setLoading(false);
      return;
    }
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    const requestId = ++requestRef.current;
    setLoading(true);
    setError(null);
    fetcher({
      ...timeRange,
      mode,
      podLimit,
      selectedObjects,
    }, controller.signal).then(res => {
      if (controller.signal.aborted || requestId !== requestRef.current) {return;}
      if (res.status === "ok") {
        setData(res.data || null);
      } else {
        setError(res.msg || "Failed to load monitoring data");
      }
    }).catch(err => {
      if (controller.signal.aborted || requestId !== requestRef.current) {return;}
      setError(err.message);
    }).finally(() => {
      if (!controller.signal.aborted && requestId === requestRef.current) {
        setLoading(false);
      }
    });
  }, [active, customTimeRange, fetcher, mode, podLimit, selectedObjects, timePreset]);

  useEffect(() => {
    refresh();
    return () => {
      requestRef.current++;
      abortRef.current?.abort();
    };
  }, [refresh]);

  useEffect(() => {
    if (!shouldRunMonitorAutoRefresh({
      autoRefresh,
      timePreset,
      customTimeRange,
      active,
      documentHidden: document.hidden,
    })) {
      return undefined;
    }
    const timer = window.setInterval(() => {
      if (!document.hidden) {
        refresh();
      }
    }, MONITOR_AUTO_REFRESH_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [active, autoRefresh, customTimeRange, refresh, timePreset]);

  useEffect(() => {
    const onVisibilityChange = () => {
      if (!document.hidden && shouldRunMonitorAutoRefresh({autoRefresh, timePreset, customTimeRange, active})) {
        refresh();
      }
    };
    document.addEventListener("visibilitychange", onVisibilityChange);
    return () => document.removeEventListener("visibilitychange", onVisibilityChange);
  }, [active, autoRefresh, customTimeRange, refresh, timePreset]);

  return {
    data,
    loading,
    error,
    timePreset,
    setTimePreset,
    customTimeRange,
    setCustomTimeRange,
    autoRefresh,
    setAutoRefresh,
    mode,
    setMode,
    selectedObjects,
    setSelectedObjects,
    podLimit,
    setPodLimit,
    waitingForCustomRange,
    refresh,
  };
}

export default useMonitoringOverview;
