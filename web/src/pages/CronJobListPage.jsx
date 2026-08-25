import React, {useState} from "react";
import i18next from "i18next";
import {History, Pencil, Plus, RefreshCw, Trash2} from "lucide-react";
import * as CronJobBackend from "@/backend/CronJobBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import {runAction, useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {Label} from "@/components/ui/label";
import {Switch} from "@/components/ui/switch";
import {MessageAlert} from "@/components/ui/alert";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {SearchSelect, SimpleSelect} from "@/components/shared/simple-select";
import {NumberInput} from "@/components/shared/number-input";
import {CodeText, LabelWithTip} from "@/components/shared/misc";
import {CronJobHistorySheet} from "@/components/shared/cronjob-history-sheet";

const CONCURRENCY_POLICIES = ["Allow", "Forbid", "Replace"].map((policy) => ({label: policy, value: policy}));

const emptyForm = {
  namespace: "",
  name: "",
  schedule: "0 * * * *",
  image: "",
  command: "",
  concurrencyPolicy: "Allow",
  suspend: false,
  successfulJobsHistLimit: 3,
  failedJobsHistLimit: 1,
};

// Older records store the command as a bracketed string rather than an array;
// both shapes have to render as one editable line.
function commandToText(command) {
  if (Array.isArray(command)) {
    return command.join(" ");
  }
  return (command || "").replace(/^\[|\]$/g, "").replace(/,/g, " ");
}

function CronJobListPage() {
  const {data: cronJobs, loading, error, refresh} = useResource(() => CronJobBackend.getCronJobs(), [], {initialData: []});
  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [mode, setMode] = useState("add");
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);
  const [historyCronJob, setHistoryCronJob] = useState(null);

  const namespaceOptions = (namespaces ?? []).map((item) => ({label: item.name, value: item.name}));

  function openAdd() {
    setMode("add");
    setEditing(null);
    setForm({...emptyForm, namespace: namespaces?.[0]?.name ?? "default"});
    setErrors({});
    setDialogOpen(true);
  }

  function openEdit(record) {
    setMode("edit");
    setEditing(record);
    setForm({
      namespace: record.namespace,
      name: record.name,
      schedule: record.schedule,
      image: record.image,
      command: commandToText(record.command),
      concurrencyPolicy: record.concurrencyPolicy || "Allow",
      suspend: record.suspend || false,
      successfulJobsHistLimit: record.successfulJobsHistLimit ?? 3,
      failedJobsHistLimit: record.failedJobsHistLimit ?? 1,
    });
    setErrors({});
    setDialogOpen(true);
  }

  async function handleSubmit() {
    const nextErrors = {};
    if (!form.namespace) {
      nextErrors.namespace = "Namespace is required";
    }
    if (!form.name) {
      nextErrors.name = "Name is required";
    }
    if (!form.schedule) {
      nextErrors.schedule = "Schedule is required";
    }
    if (!form.image) {
      nextErrors.image = "Image is required";
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const payload = {
      name: form.name,
      namespace: form.namespace,
      schedule: form.schedule,
      image: form.image,
      command: form.command ? form.command.trim().split(/\s+/).filter(Boolean) : [],
      concurrencyPolicy: form.concurrencyPolicy,
      suspend: form.suspend || false,
      successfulJobsHistLimit: form.successfulJobsHistLimit ?? 3,
      failedJobsHistLimit: form.failedJobsHistLimit ?? 1,
    };

    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(CronJobBackend.addCronJob(payload), {successMessage: "Cron Job created"})
        : await runAction(CronJobBackend.updateCronJob({...payload, resourceVersion: editing.resourceVersion}), {
          successMessage: "Cron Job updated",
        });
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(CronJobBackend.deleteCronJob(record.namespace, record.name), {
      successMessage: "Cron Job deleted",
    });
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 150, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {
      key: "schedule",
      title: "Schedule",
      dataIndex: "schedule",
      width: 170,
      render: (value) => <CodeText>{value}</CodeText>,
    },
    {key: "image", title: i18next.t("general:Image"), dataIndex: "image", ellipsis: true, className: "font-mono text-xs"},
    {
      key: "concurrencyPolicy",
      title: "Concurrency",
      dataIndex: "concurrencyPolicy",
      width: 140,
      render: (value) => <Badge variant="muted">{value || "Allow"}</Badge>,
    },
    {
      key: "suspend",
      title: "Suspended",
      dataIndex: "suspend",
      width: 120,
      render: (value) => <Badge variant={value ? "warning" : "success"}>{value ? "Yes" : "No"}</Badge>,
    },
    {key: "lastScheduleTime", title: "Last Schedule", dataIndex: "lastScheduleTime", width: 190, sortable: true},
    {key: "createdAt", title: i18next.t("general:Created"), dataIndex: "createdAt", width: 190, sortable: true},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 250,
      align: "right",
      render: (_, record) => (
        <div className="flex justify-end gap-2">
          <Button variant="outline" size="sm" onClick={() => openEdit(record)}>
            <Pencil />
            {i18next.t("general:Edit")}
          </Button>
          <Button variant="outline" size="sm" onClick={() => setHistoryCronJob(record)}>
            <History />
            History
          </Button>
          <ConfirmDialog
            title={`Delete Cron Job "${record.name}"?`}
            description={`In namespace ${record.namespace}.`}
            confirmText="Delete"
            onConfirm={() => handleDelete(record)}
          >
            <Button variant="outline" size="sm" className="text-destructive">
              <Trash2 />
            </Button>
          </ConfirmDialog>
        </div>
      ),
    },
  ];

  return (
    <PageContainer>
      {error ? <MessageAlert title="Failed to fetch Cron Jobs" description={error} /> : null}

      <DataTable
        scopeToWorkspace
        title={i18next.t("general:Cron Jobs")}
        description={`${cronJobs?.length ?? 0} cron jobs`}
        columns={columns}
        dataSource={cronJobs}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No Cron Jobs found"
        toolbar={
          <>
            <Button variant="outline" size="sm" onClick={() => refresh()} loading={loading}>
              <RefreshCw />
              {i18next.t("general:Refresh")}
            </Button>
            <Button size="sm" onClick={openAdd}>
              <Plus />
              {i18next.t("general:Add")}
            </Button>
          </>
        }
      />

      <CronJobHistorySheet
        cronJob={historyCronJob}
        open={historyCronJob !== null}
        onClose={() => setHistoryCronJob(null)}
      />

      <FormDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={mode === "add" ? "Add Cron Job" : "Edit Cron Job"}
        submitText={mode === "add" ? "Create" : "Update"}
        submitting={submitting}
        onSubmit={handleSubmit}
      >
        <Field label={i18next.t("general:Namespace")} required error={errors.namespace}>
          <SearchSelect
            value={form.namespace}
            onChange={(next) => setForm((prev) => ({...prev, namespace: next}))}
            options={namespaceOptions}
            placeholder="Select a namespace"
            disabled={mode === "edit"}
          />
        </Field>

        <Field label={i18next.t("general:Name")} htmlFor="cj-name" required error={errors.name}>
          <Input
            id="cj-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-cronjob"
            disabled={mode === "edit"}
          />
        </Field>

        <Field
          label={<LabelWithTip text="Schedule" tooltip="Cron expression — '0 * * * *' runs every hour" />}
          htmlFor="cj-schedule"
          required
          error={errors.schedule}
        >
          <Input
            id="cj-schedule"
            value={form.schedule}
            onChange={(event) => setForm((prev) => ({...prev, schedule: event.target.value}))}
            placeholder="0 * * * *"
            className="font-mono text-xs"
          />
        </Field>

        <Field label={i18next.t("general:Image")} htmlFor="cj-image" required error={errors.image}>
          <Input
            id="cj-image"
            value={form.image}
            onChange={(event) => setForm((prev) => ({...prev, image: event.target.value}))}
            placeholder="busybox:latest"
          />
        </Field>

        <Field label="Command" htmlFor="cj-command" hint="Space-separated.">
          <Input
            id="cj-command"
            value={form.command}
            onChange={(event) => setForm((prev) => ({...prev, command: event.target.value}))}
            placeholder={"echo \"hello world\""}
            className="font-mono text-xs"
          />
        </Field>

        <Field label="Concurrency Policy">
          <SimpleSelect
            value={form.concurrencyPolicy}
            onChange={(next) => setForm((prev) => ({...prev, concurrencyPolicy: next}))}
            options={CONCURRENCY_POLICIES}
          />
        </Field>

        <div className="flex flex-wrap items-end gap-6">
          <div className="flex items-center gap-2 pb-2">
            <Switch
              id="cj-suspend"
              checked={form.suspend}
              onCheckedChange={(next) => setForm((prev) => ({...prev, suspend: next}))}
            />
            <Label htmlFor="cj-suspend">Suspended</Label>
          </div>
          <Field label="Successful history">
            <NumberInput
              value={form.successfulJobsHistLimit}
              onChange={(next) => setForm((prev) => ({...prev, successfulJobsHistLimit: next}))}
              min={0}
            />
          </Field>
          <Field label="Failed history">
            <NumberInput
              value={form.failedJobsHistLimit}
              onChange={(next) => setForm((prev) => ({...prev, failedJobsHistLimit: next}))}
              min={0}
            />
          </Field>
        </div>
      </FormDialog>
    </PageContainer>
  );
}

export default CronJobListPage;
