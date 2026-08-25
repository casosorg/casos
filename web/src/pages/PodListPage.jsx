import React, {useState} from "react";
import {useHistory} from "react-router-dom";
import i18next from "i18next";
import {Boxes, FolderOpen, ListOrdered, Pencil, Plus, RefreshCw, ScrollText, Terminal, Trash2} from "lucide-react";
import * as PodBackend from "@/backend/PodBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import {runAction, useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {SearchSelect} from "@/components/shared/simple-select";
import {StatusBadge} from "@/components/shared/status-badge";
import {KeyValueEditor, fromEntries, toEntries} from "@/components/shared/key-value-editor";
import {DockerHubDialog} from "@/components/shared/docker-hub-dialog";
import {PodLogsSheet} from "@/components/shared/pod-logs-sheet";
import {PodTerminalSheet} from "@/components/shared/pod-terminal-sheet";
import {PodFilesSheet} from "@/components/shared/pod-files-sheet";
import {PodEventsSheet} from "@/components/shared/pod-events-sheet";

// List page for each workload kind that can control a pod. Deleting the pod
// only makes the controller recreate it, so the owner cell links here instead.
const ownerPages = {
  Deployment: "/deployments",
  StatefulSet: "/statefulsets",
  DaemonSet: "/daemonsets",
  Job: "/jobs",
  CronJob: "/cronjobs",
};

// The Deployment page opens its edit modal from these params; the others just
// land on the list, where each row already has a Delete button.
function ownerLink(pod) {
  const page = ownerPages[pod.ownerKind];
  if (!page) {
    return null;
  }
  if (pod.ownerKind !== "Deployment") {
    return page;
  }
  return `${page}?namespace=${encodeURIComponent(pod.namespace)}&name=${encodeURIComponent(pod.ownerName)}`;
}

const PHASE_VARIANTS = {
  Running: "success",
  Pending: "warning",
  Succeeded: "info",
  Failed: "danger",
  Unknown: "muted",
};

const emptyForm = {namespace: "", name: "", image: "", containerName: "", labelEntries: []};

// A pod named after its image is what people almost always want; deriving it
// saves two fields of typing and keeps the name DNS-safe.
function deriveName(image) {
  const base = image.split(":")[0].split("/").pop();
  return base
    .toLowerCase()
    .replace(/[^a-z0-9-]/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

function PodListPage() {
  const history = useHistory();

  const {data: pods, loading, error, refresh} = useResource(() => PodBackend.getPods(), [], {initialData: []});
  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [mode, setMode] = useState("add");
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);
  const [dockerHubOpen, setDockerHubOpen] = useState(false);

  const [logsPod, setLogsPod] = useState(null);
  const [terminalPod, setTerminalPod] = useState(null);
  const [filesPod, setFilesPod] = useState(null);
  const [eventsPod, setEventsPod] = useState(null);

  const namespaceOptions = (namespaces ?? []).map((item) => ({label: item.name, value: item.name}));
  const isEdit = mode === "edit";

  function openAdd() {
    const names = (namespaces ?? []).map((item) => item.name);
    setMode("add");
    setEditing(null);
    setForm({...emptyForm, namespace: names.includes("default") ? "default" : names[0] ?? "default"});
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
      containerName: "",
      labelEntries: toEntries(record.labels),
    });
    setErrors({});
    setDialogOpen(true);
  }

  function handleImageChange(next) {
    setForm((prev) => {
      if (isEdit || !next.trim()) {
        return {...prev, image: next};
      }
      const derived = deriveName(next.trim());
      return {...prev, image: next, name: derived, containerName: derived};
    });
  }

  async function handleSubmit() {
    const nextErrors = {};
    if (!form.namespace) {
      nextErrors.namespace = "Namespace is required";
    }
    if (!form.name) {
      nextErrors.name = "Name is required";
    }
    if (!isEdit && !form.image) {
      nextErrors.image = "Image is required";
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const labels = fromEntries(form.labelEntries);
    setSubmitting(true);
    const ok = isEdit
      ? await runAction(
        PodBackend.updatePod({
          namespace: editing.namespace,
          name: editing.name,
          labels,
          resourceVersion: editing.resourceVersion,
        }),
        {successMessage: "Pod labels updated"}
      )
      : await runAction(
        PodBackend.addPod({
          namespace: form.namespace,
          name: form.name,
          image: form.image,
          containerName: form.containerName || "app",
          labels,
        }),
        {successMessage: "Pod created"}
      );
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(PodBackend.deletePod(record.namespace, record.name), {successMessage: "Pod deleted"});
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 140, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {
      key: "owner",
      title: "Controlled By",
      width: 230,
      render: (_, record) => {
        if (!record.ownerName) {
          return <span className="text-muted-foreground">—</span>;
        }
        const link = ownerLink(record);
        const label = record.ownerKind === "Deployment" ? record.ownerName : `${record.ownerKind}: ${record.ownerName}`;
        if (!link) {
          return <Badge variant="muted">{label}</Badge>;
        }
        return (
          <button type="button" onClick={() => history.push(link)} className="text-info text-left hover:underline">
            {label}
          </button>
        );
      },
    },
    {
      key: "image",
      title: i18next.t("general:Image"),
      dataIndex: "image",
      // Every other column here is fixed-width, so without a floor the image is
      // the one that gives, and a registry-qualified tag clips to nothing.
      minWidth: 260,
      ellipsis: true,
      className: "font-mono text-xs",
    },
    {
      key: "nodeName",
      title: "Node",
      dataIndex: "nodeName",
      width: 170,
      sortable: true,
      render: (value) => value || <span className="text-muted-foreground">—</span>,
    },
    {
      key: "phase",
      title: "Phase",
      dataIndex: "phase",
      width: 120,
      sortable: true,
      render: (value) => <StatusBadge status={value} variants={PHASE_VARIANTS} />,
    },
    {key: "createdAt", title: i18next.t("general:Created"), dataIndex: "createdAt", width: 190, sortable: true},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 340,
      align: "right",
      render: (_, record) => {
        const running = record.phase === "Running";
        return (
          <div className="flex justify-end gap-1">
            <SimpleTooltip title="Logs">
              <Button variant="outline" size="icon-sm" onClick={() => setLogsPod(record)} aria-label="Logs">
                <ScrollText className="size-4" />
              </Button>
            </SimpleTooltip>
            <SimpleTooltip title={running ? "Terminal" : "Pod is not running"}>
              <span>
                <Button
                  variant="outline"
                  size="icon-sm"
                  disabled={!running}
                  onClick={() => setTerminalPod(record)}
                  aria-label="Terminal"
                >
                  <Terminal className="size-4" />
                </Button>
              </span>
            </SimpleTooltip>
            <SimpleTooltip title={running ? "Files" : "Pod is not running"}>
              <span>
                <Button
                  variant="outline"
                  size="icon-sm"
                  disabled={!running}
                  onClick={() => setFilesPod(record)}
                  aria-label="Files"
                >
                  <FolderOpen className="size-4" />
                </Button>
              </span>
            </SimpleTooltip>
            <SimpleTooltip title="Events">
              <Button variant="outline" size="icon-sm" onClick={() => setEventsPod(record)} aria-label="Events">
                <ListOrdered className="size-4" />
              </Button>
            </SimpleTooltip>
            <SimpleTooltip title="Edit labels">
              <Button variant="outline" size="icon-sm" onClick={() => openEdit(record)} aria-label="Edit">
                <Pencil className="size-4" />
              </Button>
            </SimpleTooltip>

            {record.ownerName ? (
              <SimpleTooltip
                title={`Managed by ${record.ownerKind} "${record.ownerName}" — deleting this pod only makes it come back under a new name. Delete the ${record.ownerKind} instead.`}
              >
                <span>
                  <Button variant="outline" size="icon-sm" disabled aria-label="Delete">
                    <Trash2 className="size-4" />
                  </Button>
                </span>
              </SimpleTooltip>
            ) : (
              <ConfirmDialog title={`Delete Pod "${record.name}"?`} confirmText="Delete" onConfirm={() => handleDelete(record)}>
                <Button variant="outline" size="icon-sm" className="text-destructive" aria-label="Delete">
                  <Trash2 className="size-4" />
                </Button>
              </ConfirmDialog>
            )}
          </div>
        );
      },
    },
  ];

  return (
    <PageContainer>
      {error ? <MessageAlert title="Failed to fetch pods" description={error} /> : null}

      <DataTable
        scopeToWorkspace
        title={i18next.t("general:Pods")}
        description={`${pods?.length ?? 0} pods`}
        columns={columns}
        dataSource={pods}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No pods found"
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
        title={isEdit ? "Edit Pod Labels" : "Add Pod"}
        description={isEdit ? "The pod spec is immutable after creation — only labels can be updated." : undefined}
        submitText={isEdit ? "Update" : "Create"}
        submitting={submitting}
        onSubmit={handleSubmit}
      >
        <Field label={i18next.t("general:Namespace")} required error={errors.namespace}>
          <SearchSelect
            value={form.namespace}
            onChange={(next) => setForm((prev) => ({...prev, namespace: next}))}
            options={namespaceOptions}
            placeholder="Select a namespace"
            disabled={isEdit}
          />
        </Field>

        <Field label={i18next.t("general:Image")} htmlFor="pod-image" required={!isEdit} error={errors.image}>
          <div className="flex gap-2">
            <Input
              id="pod-image"
              value={form.image}
              onChange={(event) => handleImageChange(event.target.value)}
              placeholder="nginx:latest or browse →"
              disabled={isEdit}
            />
            {!isEdit ? (
              <Button type="button" variant="outline" onClick={() => setDockerHubOpen(true)}>
                <Boxes />
                Browse
              </Button>
            ) : null}
          </div>
        </Field>

        <Field label={i18next.t("general:Name")} htmlFor="pod-name" required error={errors.name}>
          <Input
            id="pod-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="auto-filled from image"
            disabled={isEdit}
          />
        </Field>

        {!isEdit ? (
          <Field label="Container Name" htmlFor="pod-container">
            <Input
              id="pod-container"
              value={form.containerName}
              onChange={(event) => setForm((prev) => ({...prev, containerName: event.target.value}))}
              placeholder="auto-filled from image"
            />
          </Field>
        ) : null}

        <Field label="Labels">
          <KeyValueEditor
            value={form.labelEntries}
            onChange={(labelEntries) => setForm((prev) => ({...prev, labelEntries}))}
            addLabel="Add Label"
          />
        </Field>
      </FormDialog>

      <DockerHubDialog
        open={dockerHubOpen}
        onCancel={() => setDockerHubOpen(false)}
        onSelect={(image) => {
          handleImageChange(image);
          setDockerHubOpen(false);
        }}
      />

      <PodLogsSheet pod={logsPod} open={logsPod !== null} onClose={() => setLogsPod(null)} />
      <PodTerminalSheet pod={terminalPod} open={terminalPod !== null} onClose={() => setTerminalPod(null)} />
      <PodFilesSheet pod={filesPod} open={filesPod !== null} onClose={() => setFilesPod(null)} />
      <PodEventsSheet pod={eventsPod} open={eventsPod !== null} onClose={() => setEventsPod(null)} />
    </PageContainer>
  );
}

export default PodListPage;
