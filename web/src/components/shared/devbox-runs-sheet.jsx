import React, {useState} from "react";
import i18next from "i18next";
import {FileText, Play, RefreshCw, Square, Trash2} from "lucide-react";
import * as DevboxBackend from "@/backend/DevboxBackend";
import * as JobBackend from "@/backend/JobBackend";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Checkbox} from "@/components/ui/checkbox";
import {Input} from "@/components/ui/input";
import {Textarea} from "@/components/ui/textarea";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {DataTable} from "@/components/shared/data-table";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {CodeText} from "@/components/shared/misc";
import {PodLogsSheet} from "@/components/shared/pod-logs-sheet";
import {ResourceSheet} from "@/components/shared/resource-sheet";
import {runAction, useResource} from "@/hooks/use-resource";

const POLL_INTERVAL = 5000;

const RUN_STATUS = {
  queued: {variant: "warning", label: "general:Queued"},
  running: {variant: "info", label: "simple:Running"},
  succeeded: {variant: "success", label: "devbox:Succeeded"},
  failed: {variant: "danger", label: "general:Failed"},
};

const isActive = (run) => run.status === "queued" || run.status === "running";

const emptyForm = () => ({command: "", gpu: false, cpuLimit: "", memoryLimit: ""});

// Blank leaves the DevBox's own limit in place.
const limitOrKeep = (value) => (value.trim() ? value.trim() : undefined);

export function DevboxRunsSheet({devbox, open, onClose, onChanged}) {
  const [formOpen, setFormOpen] = useState(false);
  const [form, setForm] = useState(emptyForm);
  const [submitting, setSubmitting] = useState(false);
  const [logsRun, setLogsRun] = useState(null);
  const [removeTarget, setRemoveTarget] = useState(null);

  const {data, loading, refresh} = useResource(
    () => DevboxBackend.getDevboxRuns(devbox.namespace, devbox.name)
      .then((res) => (res.status === "ok" ? {...res, data: {runs: res.data ?? [], gpu: res.data2 ?? ""}} : res)),
    [devbox?.namespace, devbox?.name, open],
    {initialData: {runs: [], gpu: ""}, enabled: open && Boolean(devbox), pollInterval: open ? POLL_INTERVAL : 0}
  );
  const {runs, gpu} = data;

  function setField(key, value) {
    setForm((prev) => ({...prev, [key]: value}));
  }

  async function submitRun() {
    setSubmitting(true);
    try {
      await runAction(
        DevboxBackend.runDevbox({
          namespace: devbox.namespace,
          devbox: devbox.name,
          command: form.command.trim(),
          gpu: form.gpu,
          cpuLimit: limitOrKeep(form.cpuLimit),
          memoryLimit: limitOrKeep(form.memoryLimit),
        }),
        {
          successMessage: i18next.t("devbox:Run submitted"),
          onSuccess: () => {
            setFormOpen(false);
            refresh({silent: true});
            onChanged?.();
          },
        }
      );
    } finally {
      setSubmitting(false);
    }
  }

  function removeRun() {
    if (!removeTarget) {
      return;
    }
    runAction(JobBackend.deleteJob(removeTarget.namespace, removeTarget.name), {
      successMessage: isActive(removeTarget) ? i18next.t("devbox:Run stopped") : i18next.t("devbox:Run deleted"),
      onSuccess: () => {
        setRemoveTarget(null);
        refresh({silent: true});
        onChanged?.();
      },
    });
  }

  const columns = [
    {
      key: "status",
      title: i18next.t("general:Status"),
      dataIndex: "status",
      width: 200,
      render: (value, record) => {
        const status = RUN_STATUS[value] ?? RUN_STATUS.queued;
        return (
          <div className="min-w-0 space-y-1">
            <div className="flex items-center gap-1.5">
              <Badge variant={status.variant}>{i18next.t(status.label)}</Badge>
              {record.gpu ? <Badge variant="outline">GPU</Badge> : null}
            </div>
            {record.message ? (
              <div className="text-muted-foreground line-clamp-2 text-xs" title={record.message}>{record.message}</div>
            ) : null}
          </div>
        );
      },
    },
    {
      key: "command",
      title: i18next.t("launchpad:Command"),
      dataIndex: "command",
      minWidth: 200,
      ellipsis: true,
      render: (value) => <CodeText>{value}</CodeText>,
    },
    {key: "createdAt", title: i18next.t("general:Created"), dataIndex: "createdAt", width: 160},
    {key: "startedAt", title: i18next.t("devbox:Started"), dataIndex: "startedAt", width: 160, render: (value) => value || "—"},
    {key: "finishedAt", title: i18next.t("devbox:Finished"), dataIndex: "finishedAt", width: 160, render: (value) => value || "—"},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 130,
      align: "right",
      render: (_value, record) => (
        <div className="flex items-center justify-end gap-0.5">
          <Button size="sm" variant="outline" disabled={!record.podName} onClick={() => setLogsRun(record)}>
            <FileText />
            {i18next.t("general:Logs")}
          </Button>
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={isActive(record) ? i18next.t("general:Stop") : i18next.t("general:Delete")}
            title={isActive(record) ? i18next.t("general:Stop") : i18next.t("general:Delete")}
            onClick={() => setRemoveTarget(record)}
          >
            {isActive(record) ? <Square /> : <Trash2 />}
          </Button>
        </div>
      ),
    },
  ];

  return (
    <>
      <ResourceSheet
        open={open}
        onOpenChange={(next) => (next ? null : onClose())}
        title={i18next.t("devbox:Runs of {{name}}", {name: devbox?.name ?? ""})}
        description={i18next.t("devbox:A run executes a command in this DevBox's image, on its home disk, while the editor stays open.")}
        size="xl"
        toolbar={
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              data-testid="devbox-new-run"
              onClick={() => {
                setForm(emptyForm());
                setFormOpen(true);
              }}
            >
              <Play />
              {i18next.t("devbox:New run")}
            </Button>
            <Button variant="outline" size="sm" onClick={() => refresh()}>
              <RefreshCw />
              {i18next.t("general:Refresh")}
            </Button>
          </div>
        }
      >
        <DataTable
          testId="devbox-runs-table"
          columns={columns}
          dataSource={runs}
          rowKey="name"
          loading={loading}
          dense
          emptyIcon={Play}
          emptyText={i18next.t("devbox:Nothing has run from this DevBox yet.")}
        />
      </ResourceSheet>

      <FormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        title={i18next.t("devbox:New run")}
        description={i18next.t("devbox:Runs in the workspace folder, with the same environment as the editor. A failed run is not retried; finished runs are kept for 7 days.")}
        onSubmit={submitRun}
        submitText={i18next.t("devbox:Run")}
        submitting={submitting}
        submitDisabled={!form.command.trim()}
      >
        <Field label={i18next.t("launchpad:Command")} htmlFor="devbox-run-command" required>
          <Textarea
            id="devbox-run-command"
            rows={3}
            className="font-mono text-xs"
            value={form.command}
            onChange={(e) => setField("command", e.target.value)}
            placeholder="python train.py --epochs 10"
            data-testid="devbox-run-command"
          />
        </Field>
        <Field
          label="GPU"
          hint={gpu ? i18next.t("devbox:Shared with other GPU apps on the node.") : i18next.t("devbox:No node in this cluster has an NVIDIA GPU.")}
        >
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={form.gpu} disabled={!gpu} onCheckedChange={(checked) => setField("gpu", Boolean(checked))} />
            {gpu ? i18next.t("devbox:Use the GPU ({{name}})", {name: gpu}) : i18next.t("devbox:Use the GPU")}
          </label>
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label={i18next.t("launchpad:CPU limit")} htmlFor="devbox-run-cpu">
            <Input
              id="devbox-run-cpu"
              value={form.cpuLimit}
              onChange={(e) => setField("cpuLimit", e.target.value)}
              placeholder={i18next.t("devbox:Same as the DevBox")}
            />
          </Field>
          <Field label={i18next.t("launchpad:Memory limit")} htmlFor="devbox-run-memory">
            <Input
              id="devbox-run-memory"
              value={form.memoryLimit}
              onChange={(e) => setField("memoryLimit", e.target.value)}
              placeholder={i18next.t("devbox:Same as the DevBox")}
            />
          </Field>
        </div>
      </FormDialog>

      <PodLogsSheet
        pod={logsRun ? {namespace: logsRun.namespace, name: logsRun.podName, containers: ["run"]} : null}
        open={Boolean(logsRun)}
        onClose={() => setLogsRun(null)}
      />

      <ConfirmDialog
        open={Boolean(removeTarget)}
        onOpenChange={(next) => (next ? null : setRemoveTarget(null))}
        title={removeTarget && isActive(removeTarget) ? i18next.t("devbox:Stop this run?") : i18next.t("devbox:Delete this run?")}
        description={i18next.t("devbox:The run and its logs are removed. Whatever it wrote to the home disk stays.")}
        confirmText={removeTarget && isActive(removeTarget) ? i18next.t("general:Stop") : i18next.t("general:Delete")}
        onConfirm={removeRun}
      />
    </>
  );
}

export default DevboxRunsSheet;
