import React, {useEffect, useRef, useState} from "react";
import {Alert, Button, Form, Input, Modal, Select, Spin, Typography} from "antd";
import {useTranslation} from "react-i18next";
import * as HelmBackend from "./backend/HelmBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import {
  findStoredHelmTask,
  helmTaskMatchesIdentity,
  helmTaskPollRetryDelay,
  helmTaskStorageKey,
  helmTaskStorageSchemaVersion,
  removeStoredHelmTask
} from "./helmTaskStorage";

const {Text} = Typography;
const helmOperationTaskNotFoundCode = "helm_task_not_found";

export default function HelmInstallModal({open, chart, onClose, onInstalled}) {
  const {t} = useTranslation();
  const [form] = Form.useForm();
  const [namespaces, setNamespaces] = useState([]);
  const [valuesYAML, setValuesYAML] = useState("");
  const [valuesLoading, setValuesLoading] = useState(false);
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
  const streamAbortRef = useRef(null);
  const mountedRef = useRef(true);
  const submittingRef = useRef(false);

  const stopTaskPolling = () => {
    pollGenerationRef.current += 1;
    if (pollTimerRef.current) {
      clearTimeout(pollTimerRef.current);
      pollTimerRef.current = null;
    }
  };

  const forgetTask = (storageKey = taskStorageKeyRef.current) => {
    removeStoredHelmTask(storageKey);
    if (!storageKey || taskStorageKeyRef.current === storageKey) {
      taskIdRef.current = null;
      setActiveTaskId(null);
      taskStorageKeyRef.current = null;
      taskIdentityRef.current = null;
    }
  };

  const monitorTask = (
    taskId,
    storageKey = taskStorageKeyRef.current,
    expectedIdentity = taskIdentityRef.current
  ) => {
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
    const poll = () => {
      HelmBackend.getHelmOperationTask(taskId)
        .then(res => {
          if (!mountedRef.current || generation !== pollGenerationRef.current) {return;}
          if (res.status !== "ok") {
            if (res.data === helmOperationTaskNotFoundCode) {
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
            setError(t("helm:Unable to refresh installation status: invalid response"));
            setInstalling(false);
            setPollingPaused(true);
            submittingRef.current = true;
            return;
          }
          const matchesExpectedTask = helmTaskMatchesIdentity(task, taskId, expectedIdentity);
          if (!matchesExpectedTask) {
            forgetTask(storageKey);
            setError(t("helm:The saved installation task no longer matches this chart and was discarded"));
            setInstalling(false);
            setPollingPaused(false);
            submittingRef.current = false;
            return;
          }
          const taskLogs = Array.isArray(res.data2) ? res.data2 : [];
          setLogs(taskLogs
            .map(log => typeof log?.message === "string" ? log.message : "")
            .filter(Boolean));
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
          pollTimerRef.current = setTimeout(poll, 2000);
        })
        .catch(e => {
          if (!mountedRef.current || generation !== pollGenerationRef.current) {return;}
          consecutiveFailures += 1;
          setError(t("helm:Unable to refresh installation status", {error: e.message}));
          if (consecutiveFailures >= 6) {
            setInstalling(false);
            setPollingPaused(true);
            submittingRef.current = true;
            return;
          }
          setInstalling(true);
          const retryDelay = helmTaskPollRetryDelay(consecutiveFailures);
          pollTimerRef.current = setTimeout(poll, retryDelay);
        });
    };
    poll();
  };

  useEffect(() => {
    if (!open || !chart) {return;}
    setError(null);
    setStorageWarning(null);
    setLogs([]);
    setDone(false);
    setInstalling(false);
    setPollingPaused(false);
    taskIdRef.current = null;
    setActiveTaskId(null);
    taskStorageKeyRef.current = null;
    taskIdentityRef.current = null;
    submittingRef.current = false;
    stopTaskPolling();
    streamAbortRef.current?.abort();
    streamAbortRef.current = null;

    const savedTask = findStoredHelmTask(chart.chartName);
    if (savedTask) {
      taskIdRef.current = savedTask.taskId;
      setActiveTaskId(savedTask.taskId);
      taskStorageKeyRef.current = savedTask.key;
      taskIdentityRef.current = savedTask;
      submittingRef.current = true;
      setInstalling(true);
      monitorTask(savedTask.taskId, savedTask.key, savedTask);
    }

    NamespaceBackend.getNamespaces().then(res => {
      if (!mountedRef.current) {return;}
      if (res.status === "ok") {
        const ns = res.data ?? [];
        setNamespaces(ns);
        const def = ns.find(n => n.name === "default") ? "default" : (ns[0]?.name ?? "default");
        form.setFieldsValue({
          releaseName: savedTask?.releaseName || chart.chartName,
          namespace: savedTask?.namespace || def,
          version: chart.version ?? "",
        });
      }
    });

    if (chart.chartName && chart.repoURL) {
      setValuesLoading(true);
      setValuesYAML("");
      HelmBackend.getHelmChartValues(chart.chartName, chart.repoURL, chart.version ?? "")
        .then(res => {
          if (!mountedRef.current) {return;}
          if (res.status === "ok") {
            setValuesYAML(res.data ?? "");
          } else {
            setError(res.msg);
          }
        })
        .finally(() => {
          if (mountedRef.current) {setValuesLoading(false);}
        });
    }
  }, [open, chart, form]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      stopTaskPolling();
      streamAbortRef.current?.abort();
      streamAbortRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (logEndRef.current) {
      logEndRef.current.scrollIntoView({behavior: "smooth"});
    }
  }, [logs]);

  const handleClose = () => {
    stopTaskPolling();
    streamAbortRef.current?.abort();
    streamAbortRef.current = null;
    taskIdRef.current = null;
    setActiveTaskId(null);
    taskStorageKeyRef.current = null;
    taskIdentityRef.current = null;
    submittingRef.current = false;
    form.resetFields();
    setValuesYAML("");
    setError(null);
    setStorageWarning(null);
    setLogs([]);
    setDone(false);
    setInstalling(false);
    setPollingPaused(false);
    onClose();
  };

  const handleOk = () => {
    if (done) {
      onInstalled?.();
      handleClose();
      return;
    }
    if (submittingRef.current) {return;}
    stopTaskPolling();
    submittingRef.current = true;
    form.validateFields().then(values => {
      setInstalling(true);
      setPollingPaused(false);
      setError(null);
      setLogs([]);
      const streamController = new AbortController();
      streamAbortRef.current?.abort();
      streamAbortRef.current = streamController;

      HelmBackend.installHelmChartStream(
        {
          releaseName: values.releaseName,
          namespace: values.namespace,
          chartName: chart.chartName,
          repoURL: chart.repoURL,
          version: values.version || chart.version,
          valuesYAML,
        },
        line => {
          if (!mountedRef.current) {return;}
          if (line.startsWith("TASK_ID:")) {
            const taskId = line.slice("TASK_ID:".length).trim();
            const storageKey = helmTaskStorageKey(chart.chartName, values.namespace, values.releaseName);
            taskIdRef.current = taskId;
            setActiveTaskId(taskId);
            taskStorageKeyRef.current = storageKey;
            taskIdentityRef.current = {
              chartName: chart.chartName,
              namespace: values.namespace,
              releaseName: values.releaseName,
            };
            try {
              window.localStorage.setItem(storageKey, JSON.stringify({
                schemaVersion: helmTaskStorageSchemaVersion,
                taskId,
                createdAt: Date.now(),
                chartName: chart.chartName,
                namespace: values.namespace,
                releaseName: values.releaseName,
              }));
            } catch (_) {
              setStorageWarning(t("helm:This browser cannot save the installation task for later recovery"));
            }
          } else {
            setLogs(prev => [...prev, line]);
          }
        },
        streamController.signal
      )
        .then(status => {
          if (!mountedRef.current) {return;}
          if (status === "DONE") {
            setDone(true);
            setInstalling(false);
            setPollingPaused(false);
            setStorageWarning(null);
            submittingRef.current = false;
            forgetTask();
          }
        })
        .catch(e => {
          if (!mountedRef.current) {return;}
          if (streamController.signal.aborted) {return;}
          if (taskIdRef.current) {
            monitorTask(taskIdRef.current, taskStorageKeyRef.current, taskIdentityRef.current);
            return;
          }
          setError(e.message);
          setInstalling(false);
          setPollingPaused(false);
          submittingRef.current = false;
        })
        .finally(() => {
          if (streamAbortRef.current === streamController) {
            streamAbortRef.current = null;
          }
        });
    }).catch(() => {
      submittingRef.current = false;
    });
  };

  if (!chart) {return null;}

  const nsOptions = namespaces.map(ns => ({label: ns.name, value: ns.name}));
  const showLog = installing || pollingPaused || done || (error && logs.length > 0);
  const hasActiveTask = Boolean(activeTaskId) && !done;
  let closeLabel = t("general:Cancel");
  if (hasActiveTask) {
    closeLabel = t("helm:Close and continue in background");
  } else if (done) {
    closeLabel = t("general:Close");
  }

  const lineColor = (line, i, total) => {
    if (line.startsWith("ERROR")) {return "#f87171";}
    if (done && i === total - 1) {return "#4ade80";}
    return "#d4d4d4";
  };

  return (
    <Modal
      title={
        <span>
          {t("helm:Install chart")} <Text code>{chart.chartName}</Text>
          {chart.repoURL && (
            <Text style={{marginLeft: 8, fontSize: 12, color: "rgba(0,0,0,0.45)"}}>
              {chart.repoURL}
            </Text>
          )}
        </span>
      }
      open={open}
      onCancel={handleClose}
      closable
      maskClosable={false}
      footer={
        <div style={{display: "flex", justifyContent: "flex-end", gap: 8}}>
          <Button onClick={handleClose}>
            {closeLabel}
          </Button>
          {!done && !pollingPaused && (
            <Button type="primary" loading={installing} onClick={handleOk}>
              {t("helm:Install")}
            </Button>
          )}
          {pollingPaused && (
            <Button
              type="primary"
              onClick={() => monitorTask(taskIdRef.current, taskStorageKeyRef.current, taskIdentityRef.current)}
            >
              {t("helm:Retry status check")}
            </Button>
          )}
          {done && (
            <Button type="primary" onClick={handleOk}>
              {t("general:Done")}
            </Button>
          )}
        </div>
      }
      width={700}
      destroyOnHidden
    >
      {error && (
        <Alert type="error" message={error} showIcon style={{marginBottom: 16}} closable onClose={() => setError(null)} />
      )}

      {storageWarning && (
        <Alert
          type="warning"
          message={storageWarning}
          showIcon
          style={{marginBottom: 16}}
          closable
          onClose={() => setStorageWarning(null)}
        />
      )}

      {hasActiveTask && (
        <Alert
          type="info"
          message={t("helm:Closing this window does not cancel the installation")}
          showIcon
          style={{marginBottom: 16}}
        />
      )}

      {!showLog && (
        <Form form={form} layout="vertical">
          <div style={{display: "flex", gap: 12}}>
            <Form.Item
              style={{flex: 1}}
              label={t("helm:Release name")}
              name="releaseName"
              rules={[
                {required: true},
                {pattern: /^[a-z0-9][a-z0-9-]*$/, message: t("helm:Release name pattern")},
              ]}
            >
              <Input />
            </Form.Item>
            <Form.Item style={{flex: 1}} label={t("general:Namespaces")} name="namespace" rules={[{required: true}]}>
              <Select options={nsOptions} showSearch />
            </Form.Item>
            <Form.Item style={{width: 130}} label={t("helm:Version")} name="version">
              <Input placeholder={chart.version ?? "latest"} />
            </Form.Item>
          </div>

          <Form.Item label={t("helm:Values (YAML)")}>
            {valuesLoading ? (
              <div style={{textAlign: "center", padding: 24}}>
                <Spin size="small" />
                <Text style={{marginLeft: 8, color: "rgba(0,0,0,0.45)"}}>{t("helm:Loading values")}</Text>
              </div>
            ) : (
              <textarea
                value={valuesYAML}
                onChange={e => setValuesYAML(e.target.value)}
                rows={14}
                style={{
                  width: "100%", fontFamily: "monospace", fontSize: 12,
                  padding: "8px 10px", borderRadius: 6,
                  border: "1px solid #d9d9d9", resize: "vertical", outline: "none",
                  boxSizing: "border-box",
                }}
                spellCheck={false}
              />
            )}
          </Form.Item>
        </Form>
      )}

      {showLog && (
        <div
          style={{
            background: "#1a1a1a", borderRadius: 6, padding: "10px 14px",
            fontFamily: "monospace", fontSize: 12, color: "#d4d4d4",
            height: 340, overflowY: "auto", lineHeight: 1.6,
          }}
        >
          {logs.length === 0 && (installing || pollingPaused) && (
            <span style={{color: "#888"}}>
              {installing && <Spin size="small" style={{marginRight: 8}} />}
              {pollingPaused ? t("helm:Status check paused") : `${t("helm:Installing")}...`}
            </span>
          )}
          {logs.map((line, i) => (
            <div key={i} style={{color: lineColor(line, i, logs.length)}}>
              {line}
            </div>
          ))}
          {installing && logs.length > 0 && (
            <span style={{color: "#888", display: "inline-flex", alignItems: "center", gap: 6, marginTop: 4}}>
              <Spin size="small" />
            </span>
          )}
          <div ref={logEndRef} />
        </div>
      )}
    </Modal>
  );
}
