import React, {useState} from "react";
import i18next from "i18next";
import {Pencil, Plus, RefreshCw, Trash2} from "lucide-react";
import * as JobBackend from "@/backend/JobBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import {runAction, useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {SearchSelect} from "@/components/shared/simple-select";
import {NumberInput} from "@/components/shared/number-input";
import {LabelWithTip} from "@/components/shared/misc";

const emptyForm = {
  namespace: "",
  name: "",
  image: "",
  command: "",
  containerName: "",
  completions: 1,
  parallelism: 1,
  backoffLimit: 6,
};

function JobStatus({record}) {
  const badges = [];
  if (record.active > 0) {
    badges.push(
      <Badge key="active" variant="info">
        Active: {record.active}
      </Badge>
    );
  }
  if (record.succeeded > 0) {
    badges.push(
      <Badge key="succeeded" variant="success">
        Succeeded: {record.succeeded}
      </Badge>
    );
  }
  if (record.failed > 0) {
    badges.push(
      <Badge key="failed" variant="danger">
        Failed: {record.failed}
      </Badge>
    );
  }
  if (badges.length === 0) {
    badges.push(
      <Badge key="pending" variant="muted">
        Pending
      </Badge>
    );
  }
  return <div className="flex flex-wrap gap-1">{badges}</div>;
}

function JobListPage() {
  const {data: jobs, loading, error, refresh} = useResource(() => JobBackend.getJobs(), [], {initialData: []});
  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [mode, setMode] = useState("add");
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);

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
      image: record.image,
      command: Array.isArray(record.command) ? record.command.join(" ") : record.command ?? "",
      containerName: "",
      completions: record.completions,
      parallelism: record.parallelism,
      backoffLimit: record.backoffLimit,
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
    if (!form.image) {
      nextErrors.image = "Image is required";
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const command = (form.command ?? "").trim();
    const payload = {
      namespace: form.namespace,
      name: form.name,
      image: form.image,
      containerName: form.containerName ?? "",
      command: command ? command.split(/\s+/) : [],
      completions: form.completions ?? 1,
      parallelism: form.parallelism ?? 1,
      backoffLimit: form.backoffLimit ?? 6,
    };

    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(JobBackend.addJob(payload), {successMessage: "Job created"})
        : await runAction(JobBackend.updateJob({...payload, resourceVersion: editing.resourceVersion}), {
          successMessage: "Job updated",
        });
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(JobBackend.deleteJob(record.namespace, record.name), {successMessage: "Job deleted"});
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 160, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {key: "image", title: i18next.t("general:Image"), dataIndex: "image", ellipsis: true, className: "font-mono text-xs"},
    {
      key: "completions",
      title: "Completions",
      dataIndex: "completions",
      width: 130,
      align: "right",
      render: (value) => <span className="tabular-nums">{value ?? 1}</span>,
    },
    {key: "status", title: i18next.t("general:Status"), width: 210, render: (_, record) => <JobStatus record={record} />},
    {key: "createdAt", title: i18next.t("general:Created"), dataIndex: "createdAt", width: 190, sortable: true},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 170,
      align: "right",
      render: (_, record) => (
        <div className="flex justify-end gap-2">
          <Button variant="outline" size="sm" onClick={() => openEdit(record)}>
            <Pencil />
            {i18next.t("general:Edit")}
          </Button>
          <ConfirmDialog
            title={`Delete Job "${record.name}"?`}
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
      {error ? <MessageAlert title="Failed to fetch Jobs" description={error} /> : null}

      <DataTable
        scopeToWorkspace
        title={i18next.t("general:Jobs")}
        description={`${jobs?.length ?? 0} jobs`}
        columns={columns}
        dataSource={jobs}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No Jobs found"
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

      <FormDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={mode === "add" ? "Add Job" : "Edit Job"}
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

        <Field label={i18next.t("general:Name")} htmlFor="job-name" required error={errors.name}>
          <Input
            id="job-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-job"
            disabled={mode === "edit"}
          />
        </Field>

        <Field label={i18next.t("general:Image")} htmlFor="job-image" required error={errors.image}>
          <Input
            id="job-image"
            value={form.image}
            onChange={(event) => setForm((prev) => ({...prev, image: event.target.value}))}
            placeholder="busybox:latest"
          />
        </Field>

        <Field
          label={
            <LabelWithTip
              text="Command"
              tooltip="Space-separated command to run in the container. Leave empty to use the image default."
            />
          }
          htmlFor="job-command"
        >
          <Input
            id="job-command"
            value={form.command}
            onChange={(event) => setForm((prev) => ({...prev, command: event.target.value}))}
            placeholder={"e.g. sh -c \"echo hello\""}
            className="font-mono text-xs"
          />
        </Field>

        {mode === "add" ? (
          <Field label="Container Name" htmlFor="job-container" hint="Leave empty to use the Job name.">
            <Input
              id="job-container"
              value={form.containerName}
              onChange={(event) => setForm((prev) => ({...prev, containerName: event.target.value}))}
            />
          </Field>
        ) : null}

        <div className="flex flex-wrap gap-6">
          <Field label="Completions">
            <NumberInput value={form.completions} onChange={(next) => setForm((prev) => ({...prev, completions: next}))} min={1} />
          </Field>
          <Field label="Parallelism">
            <NumberInput value={form.parallelism} onChange={(next) => setForm((prev) => ({...prev, parallelism: next}))} min={1} />
          </Field>
          <Field label="Backoff Limit">
            <NumberInput value={form.backoffLimit} onChange={(next) => setForm((prev) => ({...prev, backoffLimit: next}))} min={0} />
          </Field>
        </div>
      </FormDialog>
    </PageContainer>
  );
}

export default JobListPage;
