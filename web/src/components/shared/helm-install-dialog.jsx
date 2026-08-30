import React, {useCallback, useEffect, useRef, useState} from "react";
import {useTranslation} from "react-i18next";
import {ChevronDown} from "lucide-react";
import * as HelmBackend from "@/backend/HelmBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import {
  findStoredHelmTask,
  helmTaskMatchesIdentity,
  helmTaskPollRetryDelay,
  helmTaskStorageKey,
  helmTaskStorageSchemaVersion,
  removeStoredHelmTask,
} from "@/lib/helmTaskStorage";
import {resolveHelmCompatibilityError} from "@/lib/helmCompatibilityErrors";
import {formatBytes} from "@/lib/quantity";
import {Button} from "@/components/ui/button";
import {Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle} from "@/components/ui/dialog";
import {Input} from "@/components/ui/input";
import {Progress} from "@/components/ui/progress";
import {Textarea} from "@/components/ui/textarea";
import {MessageAlert} from "@/components/ui/alert";
import {Field} from "@/components/shared/form-dialog";
import {SearchSelect} from "@/components/shared/simple-select";
import {AiDots} from "@/components/shared/loading";
import {CodeText} from "@/components/shared/misc";
import {HelmCompatibilityErrorAlert} from "@/components/shared/helm-compatibility-alert";
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from "@/components/ui/collapsible";
import {useUiMode} from "@/hooks/use-ui-mode";
import {cn} from "@/lib/utils";

const TASK_NOT_FOUND_CODE = "helm_task_not_found";
const STREAM_IDLE_TIMEOUT = 30 * 1000;
const VALUES_RELOAD_DEBOUNCE = 500;
const POLL_INTERVAL = 2000;
const MAX_CONSECUTIVE_POLL_FAILURES = 6;
const RELEASE_NAME_PATTERN = /^[a-z0-9][a-z0-9-]*$/;

const VALUES_STAGE_LABELS = {
  index: "helm:Downloading repository index",
  chart: "helm:Downloading chart archive",
  oci: "helm:Pulling chart from registry",
  render: "helm:Rendering default values",
};

/**
 * What a chart load is doing while the values box is empty. A repository that
 * answers with chunked transfer encoding sends no Content-Length, so total is
 * 0 and only the byte count is shown rather than an invented percentage.
 */
function ValuesLoadProgress({progress}) {
  const {t} = useTranslation();
  const stageLabel = VALUES_STAGE_LABELS[progress?.stage];
  const loaded = progress?.loaded ?? 0;
  const total = progress?.total ?? 0;
  const percent = total > 0 ? Math.min(100, Math.round((loaded / total) * 100)) : null;

  return (
    <div className="flex flex-col items-center gap-2 py-8">
      <div className="text-muted-foreground flex items-center gap-3 text-sm">
        <AiDots size="small" />
        <span>{stageLabel ? t(stageLabel) : t("helm:Loading values")}</span>
        {percent !== null ? <span className="tabular-nums">{percent}%</span> : null}
      </div>
      {percent !== null ? <Progress value={percent} className="mt-1 w-full max-w-sm" /> : null}
      {loaded > 0 ? (
        <div className="text-muted-foreground/80 text-xs tabular-nums">
          {total > 0 ? `${formatBytes(loaded)} / ${formatBytes(total)}` : formatBytes(loaded)}
          {progress?.host ? ` · ${progress.host}` : ""}
        </div>
      ) : null}
    </div>
  );
}

/**
 * Installs or upgrades a Helm chart.
 *
 * A Helm operation outlives this dialog: the server runs it as a task and
 * streams its output. Two things follow from that. The task id is written to
 * localStorage so reopening the dialog re-attaches to an operation already in
 * flight, and if the stream goes quiet for longer than the idle timeout the
 * dialog falls back to polling the task rather than hanging on a dead socket.
 */
export function HelmInstallDialog({open, chart, action = "install", onClose, onInstalled}) {
  const {t} = useTranslation();
  const {advanced} = useUiMode();
  const isUpgrade = action === "upgrade";
  const [optionsOpen, setOptionsOpen] = useState(false);

  const [namespaces, setNamespaces] = useState([]);
  const [form, setForm] = useState({releaseName: "", namespace: "", repoURL: "", version: ""});
  const [formErrors, setFormErrors] = useState({});

  const [valuesYAML, setValuesYAML] = useState("");
  const [valuesBaselineYAML, setValuesBaselineYAML] = useState("");
  const [valuesLoading, setValuesLoading] = useState(false);
  const [valuesLoadError, setValuesLoadError] = useState(null);
  const [valuesProgress, setValuesProgress] = useState(null);

  const [installing, setInstalling] = useState(false);
  const [pollingPaused, setPollingPaused] = useState(false);
  const [activeTaskId, setActiveTaskId] = useState(null);
  const [done, setDone] = useState(false);
  const [error, setError] = useState(null);
  const [storageWarning, setStorageWarning] = useState(null);
  const [logs, setLogs] = useState([]);

  const logEndRef = useRef(null);
  const taskIdRef = useRef(null);
  const taskStorageKeyRef = useRef(null);
  const taskIdentityRef = useRef(null);
  const pollTimerRef = useRef(null);
  const pollGenerationRef = useRef(0);
  const valuesGenerationRef = useRef(0);
  const streamAbortRef = useRef(null);
  const streamIdleTimerRef = useRef(null);
  const streamIdleControllerRef = useRef(null);
  const mountedRef = useRef(true);
  const submittingRef = useRef(false);

  const stopTaskPolling = useCallback(() => {
    pollGenerationRef.current += 1;
    if (pollTimerRef.current) {
      clearTimeout(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  }, []);

  const stopStreamIdleTimer = useCallback((controller = null) => {
    if (controller && streamIdleControllerRef.current !== controller) {
      return;
    }
    if (streamIdleTimerRef.current) {
      clearTimeout(streamIdleTimerRef.current);
      streamIdleTimerRef.current = null;
    }
    streamIdleControllerRef.current = null;
  }, []);

  const forgetTask = useCallback((storageKey = taskStorageKeyRef.current) => {
    removeStoredHelmTask(storageKey);
    if (!storageKey || taskStorageKeyRef.current === storageKey) {
      taskIdRef.current = null;
      setActiveTaskId(null);
      taskStorageKeyRef.current = null;
      taskIdentityRef.current = null;
    }
  }, []);

  const monitorTask = useCallback(
    (taskId, storageKey = taskStorageKeyRef.current, expectedIdentity = taskIdentityRef.current) => {
      if (!taskId) {
        setInstalling(false);
        setPollingPaused(false);
        submittingRef.current = false;
        return;
      }
      stopTaskPolling();
      setPollingPaused(false);
      const generation = pollGenerationRef.current;
      let consecutiveFailures = 0;

      function poll() {
        HelmBackend.getHelmOperationTask(taskId)
          .then((res) => {
            if (!mountedRef.current || generation !== pollGenerationRef.current) {
              return;
            }
            if (res.status !== "ok") {
              if (res.data === TASK_NOT_FOUND_CODE) {
                forgetTask(storageKey);
                submittingRef.current = false;
              } else {
                setPollingPaused(true);
                submittingRef.current = true;
              }
              setError(res.msg);
              setInstalling(false);
              return;
            }

            consecutiveFailures = 0;
            setError(null);
            const task = res.data;
            if (!task || !task.id || !task.status) {
              setError(t("helm:Unable to refresh Helm operation status: invalid response"));
              setInstalling(false);
              setPollingPaused(true);
              submittingRef.current = true;
              return;
            }

            // A recovered task id could belong to a different chart if storage
            // outlived the release; discard rather than report someone else's
            // progress as this dialog's.
            if (!helmTaskMatchesIdentity(task, taskId, expectedIdentity)) {
              forgetTask(storageKey);
              setError(t("helm:The saved Helm operation no longer matches this chart and was discarded"));
              setInstalling(false);
              setPollingPaused(false);
              submittingRef.current = false;
              return;
            }

            const taskLogs = Array.isArray(res.data2) ? res.data2 : [];
            setLogs(taskLogs.map((log) => (typeof log?.message === "string" ? log.message : "")).filter(Boolean));

            if (task.status === "succeeded") {
              setDone(true);
              setInstalling(false);
              setPollingPaused(false);
              submittingRef.current = false;
              forgetTask(storageKey);
              return;
            }
            if (task.status === "failed") {
              setError(task.errorMsg || t("helm:Helm operation failed"));
              setInstalling(false);
              setPollingPaused(false);
              submittingRef.current = false;
              forgetTask(storageKey);
              return;
            }

            setInstalling(true);
            pollTimerRef.current = setTimeout(poll, POLL_INTERVAL);
          })
          .catch((e) => {
            if (!mountedRef.current || generation !== pollGenerationRef.current) {
              return;
            }
            consecutiveFailures += 1;
            setError(t("helm:Unable to refresh Helm operation status", {error: e.message}));
            if (consecutiveFailures >= MAX_CONSECUTIVE_POLL_FAILURES) {
              setInstalling(false);
              setPollingPaused(true);
              submittingRef.current = true;
              return;
            }
            setInstalling(true);
            pollTimerRef.current = setTimeout(poll, helmTaskPollRetryDelay(consecutiveFailures));
          });
      }

      poll();
    },
    [forgetTask, stopTaskPolling, t]
  );

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      stopTaskPolling();
      stopStreamIdleTimer();
      streamAbortRef.current?.abort();
      streamAbortRef.current = null;
    };
  }, [stopStreamIdleTimer, stopTaskPolling]);

  useEffect(() => {
    if (!open || !chart) {
      return;
    }
    setError(null);
    setValuesLoadError(null);
    setStorageWarning(null);
    setLogs([]);
    setValuesYAML("");
    setValuesBaselineYAML("");
    setValuesLoading(false);
    setValuesProgress(null);
    setDone(false);
    setInstalling(false);
    setPollingPaused(false);
    setFormErrors({});
    taskIdRef.current = null;
    setActiveTaskId(null);
    taskStorageKeyRef.current = null;
    taskIdentityRef.current = null;
    submittingRef.current = false;
    stopTaskPolling();
    stopStreamIdleTimer();
    streamAbortRef.current?.abort();
    streamAbortRef.current = null;

    const savedTask = findStoredHelmTask(
      chart.chartName,
      isUpgrade ? {operation: action, namespace: chart.namespace, releaseName: chart.releaseName} : null
    );

    setForm({
      releaseName: isUpgrade ? chart.releaseName : savedTask?.releaseName || chart.chartName,
      namespace: isUpgrade ? chart.namespace : savedTask?.namespace ?? "",
      repoURL: chart.repoURL ?? "",
      version: chart.version ?? "",
    });

    if (savedTask) {
      taskIdRef.current = savedTask.taskId;
      setActiveTaskId(savedTask.taskId);
      taskStorageKeyRef.current = savedTask.key;
      taskIdentityRef.current = savedTask;
      submittingRef.current = true;
      setInstalling(true);
      monitorTask(savedTask.taskId, savedTask.key, savedTask);
    }

    NamespaceBackend.getNamespaces().then((res) => {
      if (!mountedRef.current || res.status !== "ok") {
        return;
      }
      const list = res.data ?? [];
      setNamespaces(list);
      setForm((previous) => {
        if (previous.namespace) {
          return previous;
        }
        const preferred = list.find((item) => item.name === "default") ? "default" : list[0]?.name ?? "default";
        return {...previous, namespace: preferred};
      });
    });
  }, [open, chart, isUpgrade, action, monitorTask, stopStreamIdleTimer, stopTaskPolling]);

  // Values are re-read whenever the chart coordinates change. Typing a version
  // or repo URL is debounced so each keystroke does not fetch a chart.
  useEffect(() => {
    const effectiveRepoURL = form.repoURL || chart?.repoURL;
    const effectiveVersion = form.version || chart?.version || "";
    const artifactHubRepository = effectiveRepoURL === chart?.repoURL ? chart?.artifactHubRepository : "";
    const generation = valuesGenerationRef.current + 1;
    valuesGenerationRef.current = generation;

    if (!open || !chart?.chartName || !effectiveRepoURL) {
      setValuesLoading(false);
      setValuesProgress(null);
      return undefined;
    }

    const controller = new AbortController();
    const changedByUser = effectiveRepoURL !== (chart?.repoURL ?? "") || effectiveVersion !== (chart?.version ?? "");
    setValuesLoading(true);
    setValuesProgress(null);

    const timer = setTimeout(
      () => {
        HelmBackend.getHelmChartValuesStream(
          chart.chartName,
          effectiveRepoURL,
          effectiveVersion,
          artifactHubRepository,
          (progress) => {
            if (mountedRef.current && generation === valuesGenerationRef.current) {
              setValuesProgress(progress);
            }
          },
          controller.signal
        )
          .then((values) => {
            if (!mountedRef.current || generation !== valuesGenerationRef.current) {
              return;
            }
            setValuesYAML(values);
            setValuesBaselineYAML(values);
            setValuesLoadError(null);
          })
          .catch((e) => {
            if (e.name !== "AbortError" && mountedRef.current && generation === valuesGenerationRef.current) {
              setValuesLoadError(e.message);
            }
          })
          .finally(() => {
            if (mountedRef.current && generation === valuesGenerationRef.current) {
              setValuesLoading(false);
              setValuesProgress(null);
            }
          });
      },
      changedByUser ? VALUES_RELOAD_DEBOUNCE : 0
    );

    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [open, chart, form.repoURL, form.version]);

  useEffect(() => {
    logEndRef.current?.scrollIntoView({behavior: "smooth"});
  }, [logs]);

  function handleClose() {
    stopTaskPolling();
    stopStreamIdleTimer();
    streamAbortRef.current?.abort();
    streamAbortRef.current = null;
    taskIdRef.current = null;
    setActiveTaskId(null);
    taskStorageKeyRef.current = null;
    taskIdentityRef.current = null;
    submittingRef.current = false;
    setValuesYAML("");
    setValuesBaselineYAML("");
    setError(null);
    setValuesLoadError(null);
    setValuesProgress(null);
    setStorageWarning(null);
    setLogs([]);
    setDone(false);
    setInstalling(false);
    setPollingPaused(false);
    onClose();
  }

  function validate() {
    const nextErrors = {};
    if (!form.releaseName) {
      nextErrors.releaseName = t("general:required");
    } else if (!RELEASE_NAME_PATTERN.test(form.releaseName)) {
      nextErrors.releaseName = t("helm:Release name pattern");
    }
    if (!form.namespace) {
      nextErrors.namespace = t("general:required");
    }
    if (isUpgrade && !form.repoURL) {
      nextErrors.repoURL = t("general:required");
    }
    setFormErrors(nextErrors);
    return Object.keys(nextErrors).length === 0;
  }

  function handleSubmit() {
    if (done) {
      onInstalled?.();
      handleClose();
      return;
    }
    if (submittingRef.current || valuesLoading || valuesLoadError) {
      return;
    }
    if (!validate()) {
      return;
    }

    stopTaskPolling();
    submittingRef.current = true;
    setInstalling(true);
    setPollingPaused(false);
    setError(null);
    setLogs([]);

    const streamController = new AbortController();
    stopStreamIdleTimer();
    streamAbortRef.current?.abort();
    streamAbortRef.current = streamController;

    function fallBackToPolling() {
      stopStreamIdleTimer(streamController);
      if (!mountedRef.current || streamAbortRef.current !== streamController || !taskIdRef.current) {
        return;
      }
      streamController.abort();
      monitorTask(taskIdRef.current, taskStorageKeyRef.current, taskIdentityRef.current);
    }

    function resetIdleTimer() {
      if (!taskIdRef.current) {
        return;
      }
      stopStreamIdleTimer();
      streamIdleControllerRef.current = streamController;
      streamIdleTimerRef.current = setTimeout(fallBackToPolling, STREAM_IDLE_TIMEOUT);
    }

    const releaseName = isUpgrade ? chart.releaseName : form.releaseName;
    const namespace = isUpgrade ? chart.namespace : form.namespace;
    const payload = {
      releaseName,
      namespace,
      chartName: chart.chartName,
      repoURL: form.repoURL || chart.repoURL,
      artifactHubRepository: (form.repoURL || chart.repoURL) === chart.repoURL ? chart.artifactHubRepository : "",
      version: form.version || chart.version,
      valuesYAML,
      valuesBaselineYAML,
    };

    const stream = isUpgrade ? HelmBackend.upgradeHelmChartStream : HelmBackend.installHelmChartStream;

    stream(
      payload,
      (line) => {
        if (!mountedRef.current || streamAbortRef.current !== streamController) {
          return;
        }
        if (line.startsWith("TASK_ID:")) {
          const taskId = line.slice("TASK_ID:".length).trim();
          const storageKey = helmTaskStorageKey(chart.chartName, namespace, releaseName);
          taskIdRef.current = taskId;
          setActiveTaskId(taskId);
          taskStorageKeyRef.current = storageKey;
          taskIdentityRef.current = {operation: action, chartName: chart.chartName, namespace, releaseName};
          try {
            window.localStorage.setItem(
              storageKey,
              JSON.stringify({
                schemaVersion: helmTaskStorageSchemaVersion,
                taskId,
                createdAt: Date.now(),
                operation: action,
                chartName: chart.chartName,
                namespace,
                releaseName,
              })
            );
          } catch {
            setStorageWarning(t("helm:This browser cannot save the Helm operation for later recovery"));
          }
        } else {
          setLogs((previous) => [...previous, line]);
        }
        resetIdleTimer();
      },
      streamController.signal
    )
      .then((status) => {
        if (!mountedRef.current || streamAbortRef.current !== streamController) {
          return;
        }
        if (status === "DONE") {
          stopStreamIdleTimer(streamController);
          setDone(true);
          setInstalling(false);
          setPollingPaused(false);
          setStorageWarning(null);
          submittingRef.current = false;
          forgetTask();
        }
      })
      .catch((e) => {
        if (!mountedRef.current || streamAbortRef.current !== streamController) {
          return;
        }
        stopStreamIdleTimer(streamController);
        if (streamController.signal.aborted) {
          return;
        }
        // A coded error is a definitive failure from the server; anything else
        // may just be a broken stream over a task that is still running.
        if (e.code) {
          setError(resolveHelmCompatibilityError(e, t));
          setInstalling(false);
          setPollingPaused(false);
          submittingRef.current = false;
          forgetTask();
          return;
        }
        if (taskIdRef.current) {
          monitorTask(taskIdRef.current, taskStorageKeyRef.current, taskIdentityRef.current);
          return;
        }
        setError(resolveHelmCompatibilityError(e, t));
        setInstalling(false);
        setPollingPaused(false);
        submittingRef.current = false;
      })
      .finally(() => {
        if (streamAbortRef.current === streamController) {
          stopStreamIdleTimer(streamController);
          streamAbortRef.current = null;
        }
      });
  }

  if (!chart) {
    return null;
  }

  const showLog = installing || pollingPaused || done || (error && logs.length > 0);
  const hasActiveTask = Boolean(activeTaskId) && !done;
  const closeLabel = hasActiveTask
    ? t("helm:Close and continue in background")
    : done
      ? t("general:Close")
      : t("general:Cancel");

  function lineClass(line, index) {
    if (line.startsWith("ERROR")) {
      return "text-red-400";
    }
    if (done && index === logs.length - 1) {
      return "text-green-400";
    }
    return "text-neutral-300";
  }

  // Both modes render the same controls; simple mode only shows one of them up
  // front and folds the rest away, so a reader who never opens the options
  // still installs with the defaults the backend adapters set.
  const releaseNameField = (
    <Field
      label={advanced ? t("helm:Release name") : t("general:App name")}
      htmlFor="helm-release"
      required
      error={formErrors.releaseName}
      hint={advanced ? undefined : t("simple:Lower-case letters, digits and dashes. This is the name the app is listed under.")}
    >
      <Input
        id="helm-release"
        value={form.releaseName}
        onChange={(event) => setForm((previous) => ({...previous, releaseName: event.target.value}))}
        disabled={isUpgrade}
      />
    </Field>
  );

  const namespaceField = (
    <Field label={advanced ? t("general:Namespaces") : t("simple:Group")} required error={formErrors.namespace}>
      <SearchSelect
        value={form.namespace}
        onChange={(next) => setForm((previous) => ({...previous, namespace: next}))}
        options={namespaces.map((item) => ({label: item.name, value: item.name}))}
        disabled={isUpgrade}
      />
    </Field>
  );

  const versionField = (
    <Field label={t("general:Version")} htmlFor="helm-version">
      <Input
        id="helm-version"
        value={form.version}
        onChange={(event) => setForm((previous) => ({...previous, version: event.target.value}))}
        placeholder={chart.version ?? "latest"}
      />
    </Field>
  );

  const repoField = (
    <Field label={t("helm:Repo URL")} htmlFor="helm-repo" required error={formErrors.repoURL}>
      <Input
        id="helm-repo"
        value={form.repoURL}
        onChange={(event) => setForm((previous) => ({...previous, repoURL: event.target.value}))}
        placeholder="https://example.com/charts"
      />
    </Field>
  );

  const valuesField = (
    <Field label={t("helm:Values (YAML)")}>
      {valuesLoading ? (
        <ValuesLoadProgress progress={valuesProgress} />
      ) : (
        <Textarea
          value={valuesYAML}
          onChange={(event) => setValuesYAML(event.target.value)}
          rows={14}
          spellCheck={false}
          className="scrollbar-thin resize-y font-mono text-xs"
        />
      )}
    </Field>
  );

  return (
    <Dialog open={open} onOpenChange={(next) => (next ? null : handleClose())}>
      <DialogContent className="sm:max-w-3xl" onInteractOutside={(event) => event.preventDefault()}>
        <DialogHeader>
          <DialogTitle className="flex flex-wrap items-center gap-2">
            {advanced ? (
              <>
                {t(isUpgrade ? "helm:Upgrade" : "helm:Install chart")} <CodeText>{chart.chartName}</CodeText>
                {chart.repoURL ? <span className="text-muted-foreground text-xs font-normal">{chart.repoURL}</span> : null}
              </>
            ) : (
              t(isUpgrade ? "simple:Update {{name}}" : "simple:Install {{name}}", {name: chart.displayName || chart.chartName})
            )}
          </DialogTitle>
        </DialogHeader>

        <div className="scrollbar-thin grid max-h-[65vh] gap-4 overflow-y-auto px-0.5">
          {error ? <HelmCompatibilityErrorAlert error={error} t={t} onClose={() => setError(null)} /> : null}
          {valuesLoadError ? <MessageAlert title={valuesLoadError} /> : null}
          {storageWarning ? (
            <MessageAlert variant="warning" title={storageWarning} action={
              <Button variant="outline" size="sm" onClick={() => setStorageWarning(null)}>
                {t("general:OK")}
              </Button>
            } />
          ) : null}
          {hasActiveTask ? (
            <MessageAlert variant="info" title={t("helm:Closing this window does not cancel the Helm operation")} />
          ) : null}

          {!showLog ? (
            advanced ? (
              <>
                <div className="grid gap-3 sm:grid-cols-[1fr_1fr_140px]">
                  {releaseNameField}
                  {namespaceField}
                  {versionField}
                </div>
                {isUpgrade ? repoField : null}
                {valuesField}
              </>
            ) : (
              <>
                {releaseNameField}
                {valuesLoading ? (
                  <p className="text-muted-foreground flex items-center gap-2 text-sm">
                    <AiDots size="small" />
                    {t("simple:Getting the app ready, one moment...")}
                  </p>
                ) : (
                  <p className="text-muted-foreground text-sm">
                    {t("simple:CasOS picks working settings for everything else. Open the advanced options only if you know you need to change something.")}
                  </p>
                )}
                <Collapsible open={optionsOpen} onOpenChange={setOptionsOpen}>
                  <CollapsibleTrigger asChild>
                    <Button type="button" variant="ghost" size="sm" className="-ml-2 gap-1.5">
                      <ChevronDown className={cn("size-4 transition-transform", optionsOpen && "rotate-180")} />
                      {t("simple:Advanced options")}
                    </Button>
                  </CollapsibleTrigger>
                  <CollapsibleContent className="grid gap-4 pt-3">
                    <div className="grid gap-3 sm:grid-cols-[1fr_140px]">
                      {namespaceField}
                      {versionField}
                    </div>
                    {isUpgrade ? repoField : null}
                    {valuesField}
                  </CollapsibleContent>
                </Collapsible>
              </>
            )
          ) : (
            <div className="scrollbar-thin h-[340px] overflow-y-auto rounded-lg bg-neutral-950 p-3 font-mono text-xs leading-relaxed">
              {logs.length === 0 && (installing || pollingPaused) ? (
                <span className="flex items-center gap-2 text-neutral-500">
                  {installing ? <AiDots size="small" /> : null}
                  {pollingPaused
                    ? t("helm:Status check paused")
                    : `${t(isUpgrade ? "helm:Upgrading" : "helm:Installing")}...`}
                </span>
              ) : null}
              {logs.map((line, index) => (
                <div key={index} className={lineClass(line, index)}>
                  {line}
                </div>
              ))}
              {installing && logs.length > 0 ? (
                <span className="mt-1 inline-flex items-center text-neutral-500">
                  <AiDots size="small" />
                </span>
              ) : null}
              <div ref={logEndRef} />
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose}>
            {closeLabel}
          </Button>
          {!done && !pollingPaused ? (
            <Button
              loading={installing || (!advanced && valuesLoading)}
              disabled={valuesLoading || Boolean(valuesLoadError)}
              onClick={handleSubmit}
            >
              {t(isUpgrade ? "helm:Upgrade" : "general:Install")}
            </Button>
          ) : null}
          {pollingPaused ? (
            <Button onClick={() => monitorTask(taskIdRef.current, taskStorageKeyRef.current, taskIdentityRef.current)}>
              {t("helm:Retry status check")}
            </Button>
          ) : null}
          {done ? <Button onClick={handleSubmit}>{t("general:Done")}</Button> : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default HelmInstallDialog;
