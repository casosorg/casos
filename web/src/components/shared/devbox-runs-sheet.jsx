import React, {useCallback, useEffect, useState} from "react";
import i18next from "i18next";
import {FileText, RefreshCw, Snowflake, XCircle} from "lucide-react";
import * as DevboxBackend from "@/backend/DevboxBackend";
import * as PodBackend from "@/backend/PodBackend";
import * as Setting from "@/Setting";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle} from "@/components/ui/dialog";
import {DataTable} from "@/components/shared/data-table";
import {ResourceSheet} from "@/components/shared/resource-sheet";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {CodeText} from "@/components/shared/misc";
import {runAction} from "@/hooks/use-resource";

const RUN_POLL_INTERVAL = 10000;

const RUN_STATUS_VARIANTS = {
  queued: "muted",
  running: "info",
  succeeded: "success",
  failed: "danger",
};

const RUN_STATUS_LABELS = {
  queued: "devbox:Queued",
  running: "general:Running",
  succeeded: "devbox:Succeeded",
  failed: "devbox:Failed",
};

function RunLogDialog({namespace, run, onClose}) {
  const [logs, setLogs] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!run?.podName) {
      return;
    }
    let cancelled = false;
    setLoading(true);
    PodBackend.getPodLogs(namespace, run.podName, "", 500)
      .then((res) => {
        if (!cancelled) {
          setLogs(res.status === "ok" ? res.data ?? "" : res.msg);
        }
      })
      .catch((error) => !cancelled && setLogs(error.message))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, [namespace, run]);

  return (
    <Dialog open={Boolean(run)} onOpenChange={(next) => (next ? null : onClose())}>
      <DialogContent className="sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FileText className="size-4" />
            {run?.name}
          </DialogTitle>
        </DialogHeader>
        {loading ? (
          <p className="text-muted-foreground text-sm">{i18next.t("general:Loading...")}</p>
        ) : (
          <pre className="scrollbar-thin max-h-[60vh] overflow-auto rounded-lg bg-neutral-950 p-4 font-mono text-xs break-all whitespace-pre-wrap text-neutral-200">
            {logs || i18next.t("devbox:No output yet.")}
          </pre>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {i18next.t("general:Close")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * The runs a workspace has been frozen for, newest first. It polls while it is
 * open, because the thing a person opens this for is a run that has not
 * finished yet.
 */
export function DevboxRunsSheet({devbox, open, onClose, onChanged}) {
  const [runs, setRuns] = useState([]);
  const [loading, setLoading] = useState(false);
  const [logTarget, setLogTarget] = useState(null);
  const [cancelTarget, setCancelTarget] = useState(null);

  const fetchRuns = useCallback((silent = false) => {
    if (!devbox) {
      return;
    }
    if (!silent) {
      setLoading(true);
    }
    DevboxBackend.getDevboxRuns(devbox.namespace, devbox.name)
      .then((res) => {
        if (res.status === "ok") {
          setRuns(res.data ?? []);
        } else {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch((error) => Setting.showMessage("error", error.message))
      .finally(() => setLoading(false));
  }, [devbox]);

  useEffect(() => {
    if (!open) {
      setRuns([]);
      return;
    }
    fetchRuns();
    const timer = setInterval(() => fetchRuns(true), RUN_POLL_INTERVAL);
    return () => clearInterval(timer);
  }, [open, fetchRuns]);

  function cancelRun() {
    if (!cancelTarget) {
      return;
    }
    runAction(DevboxBackend.cancelDevboxRun({namespace: cancelTarget.namespace, name: cancelTarget.name}), {
      successMessage: i18next.t("devbox:Run cancelled"),
      onSuccess: () => {
        setCancelTarget(null);
        fetchRuns(true);
        onChanged?.();
      },
    });
  }

  const columns = [
    {
      key: "status",
      title: i18next.t("general:Status"),
      dataIndex: "status",
      width: 130,
      render: (value, record) => (
        <div className="flex items-center gap-1.5">
          <Badge variant={RUN_STATUS_VARIANTS[value] ?? "muted"}>
            {i18next.t(RUN_STATUS_LABELS[value] ?? "devbox:Queued")}
          </Badge>
          {record.thawOnFinish ? (
            <Snowflake className="text-muted-foreground size-3.5" title={i18next.t("devbox:The workspace comes back when this run ends.")} />
          ) : null}
        </div>
      ),
    },
    {
      key: "command",
      title: i18next.t("launchpad:Command"),
      dataIndex: "command",
      minWidth: 220,
      ellipsis: true,
      render: (value) => <CodeText>{value}</CodeText>,
    },
    {
      key: "gpu",
      title: i18next.t("devbox:GPUs"),
      dataIndex: "gpu",
      width: 80,
      render: (value) => <span className="tabular-nums">{value || "—"}</span>,
    },
    {key: "startedAt", title: i18next.t("devbox:Started"), dataIndex: "startedAt", width: 170, render: (value) => value || "—"},
    {key: "finishedAt", title: i18next.t("devbox:Finished"), dataIndex: "finishedAt", width: 170, render: (value) => value || "—"},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 150,
      align: "right",
      render: (_value, record) => (
        <div className="flex items-center justify-end gap-1">
          <Button size="sm" variant="outline" disabled={!record.podName} onClick={() => setLogTarget(record)}>
            <FileText />
            {i18next.t("general:Logs")}
          </Button>
          {record.status === "queued" || record.status === "running" ? (
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={i18next.t("devbox:Cancel run")}
              title={i18next.t("devbox:Cancel run")}
              onClick={() => setCancelTarget(record)}
            >
              <XCircle />
            </Button>
          ) : null}
        </div>
      ),
    },
  ];

  return (
    <>
      <ResourceSheet
        open={open}
        onOpenChange={(next) => (next ? null : onClose())}
        title={i18next.t("devbox:Runs — {{name}}", {name: devbox?.name ?? ""})}
        description={i18next.t("devbox:Work queued from this workspace. A run reads and writes the same home disk.")}
        size="lg"
        toolbar={
          <Button variant="outline" size="sm" onClick={() => fetchRuns()} loading={loading}>
            <RefreshCw />
            {i18next.t("general:Refresh")}
          </Button>
        }
      >
        <DataTable
          columns={columns}
          dataSource={runs}
          rowKey="name"
          loading={loading}
          dense
          emptyText={i18next.t("devbox:Nothing has been queued from this workspace yet.")}
        />
      </ResourceSheet>

      <RunLogDialog namespace={devbox?.namespace ?? ""} run={logTarget} onClose={() => setLogTarget(null)} />

      <ConfirmDialog
        open={Boolean(cancelTarget)}
        onOpenChange={(next) => (next ? null : setCancelTarget(null))}
        title={i18next.t("devbox:Cancel run")}
        description={i18next.t("devbox:The run stops and its pod is removed. Whatever it wrote to the home disk stays.")}
        confirmText={i18next.t("devbox:Cancel run")}
        onConfirm={cancelRun}
      />
    </>
  );
}

export default DevboxRunsSheet;
