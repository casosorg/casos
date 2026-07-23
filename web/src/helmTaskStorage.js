export const helmTaskStorageSchemaVersion = 1;

const helmTaskStorageMaxAgeMs = 24 * 60 * 60 * 1000;
const helmTaskStoragePrefix = chartName => `casos.helmTask.${encodeURIComponent(chartName)}.`;

export const helmTaskStorageKey = (chartName, namespace, releaseName) =>
  `${helmTaskStoragePrefix(chartName)}${encodeURIComponent(namespace)}.${encodeURIComponent(releaseName)}`;

export const removeStoredHelmTask = key => {
  if (!key) {return;}
  try {
    window.localStorage.removeItem(key);
  } catch (_) {
    // Storage may be unavailable; task polling still works for this session.
  }
};

export const helmTaskMatchesIdentity = (task, taskId, expectedIdentity) => Boolean(
  task && expectedIdentity &&
  String(task.id) === String(taskId) &&
  task.chartName === expectedIdentity.chartName &&
  task.namespace === expectedIdentity.namespace &&
  task.releaseName === expectedIdentity.releaseName
);

export const helmTaskPollRetryDelay = consecutiveFailures =>
  Math.min(2000 * (2 ** Math.max(consecutiveFailures - 1, 0)), 30000);

export const findStoredHelmTask = chartName => {
  const prefix = helmTaskStoragePrefix(chartName);
  const matches = [];
  const invalidKeys = [];
  try {
    for (let i = 0; i < window.localStorage.length; i += 1) {
      const key = window.localStorage.key(i);
      if (!key?.startsWith(prefix)) {continue;}
      const raw = window.localStorage.getItem(key);
      try {
        const stored = JSON.parse(raw);
        const createdAt = Number(stored?.createdAt);
        const isFresh = createdAt > Date.now() - helmTaskStorageMaxAgeMs;
        const hasTaskIdentity = /^\d+$/.test(String(stored?.taskId ?? "")) &&
          typeof stored?.namespace === "string" && stored.namespace &&
          typeof stored?.releaseName === "string" && stored.releaseName;
        const isCurrentSchema = stored?.schemaVersion === helmTaskStorageSchemaVersion &&
          stored.chartName === chartName;
        const isLegacySchema = stored?.schemaVersion === undefined;
        if (hasTaskIdentity && (isCurrentSchema || isLegacySchema) && isFresh) {
          matches.push({
            key,
            taskId: String(stored.taskId),
            createdAt,
            chartName,
            namespace: stored.namespace,
            releaseName: stored.releaseName,
          });
        } else {
          invalidKeys.push(key);
        }
      } catch (_) {
        invalidKeys.push(key);
      }
    }
  } catch (_) {
    return null;
  }
  invalidKeys.forEach(removeStoredHelmTask);
  matches.sort((a, b) => b.createdAt - a.createdAt);
  return matches[0] ?? null;
};
