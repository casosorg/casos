import React, {useState} from "react";
import i18next from "i18next";
import {Pencil, Plus, RefreshCw, Trash2} from "lucide-react";
import * as HPABackend from "@/backend/HPABackend";
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
import {NumberInput} from "@/components/shared/number-input";
import {SearchSelect, SimpleSelect} from "@/components/shared/simple-select";
import {LabelWithTip} from "@/components/shared/misc";

const SCALE_TARGET_KINDS = ["Deployment", "StatefulSet", "ReplicaSet"].map((kind) => ({label: kind, value: kind}));

const emptyForm = {
  namespace: "",
  name: "",
  scaleTargetKind: "Deployment",
  scaleTargetName: "",
  minReplicas: 1,
  maxReplicas: 10,
  cpuTargetUtilization: 80,
};

function HPAListPage() {
  const {data: hpas, loading, error, refresh} = useResource(() => HPABackend.getHPAs(), [], {initialData: []});
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
      scaleTargetKind: record.scaleTargetKind,
      scaleTargetName: record.scaleTargetName,
      minReplicas: record.minReplicas,
      maxReplicas: record.maxReplicas,
      cpuTargetUtilization: record.cpuTargetUtilization ?? "",
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
    if (!form.scaleTargetKind) {
      nextErrors.scaleTargetKind = "Scale target kind is required";
    }
    if (!form.scaleTargetName) {
      nextErrors.scaleTargetName = "Scale target name is required";
    }
    if (!form.maxReplicas) {
      nextErrors.maxReplicas = "Required";
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const payload = {
      name: form.name,
      namespace: form.namespace,
      scaleTargetKind: form.scaleTargetKind,
      scaleTargetName: form.scaleTargetName,
      minReplicas: form.minReplicas ?? 1,
      maxReplicas: form.maxReplicas,
      // An empty field means "do not scale on CPU", which the API spells null
      // rather than 0.
      cpuTargetUtilization: form.cpuTargetUtilization === "" ? null : form.cpuTargetUtilization,
    };

    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(HPABackend.addHPA(payload), {successMessage: "Horizontal Pod Autoscaler created"})
        : await runAction(HPABackend.updateHPA({...payload, resourceVersion: editing.resourceVersion}), {
          successMessage: "Horizontal Pod Autoscaler updated",
        });
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(HPABackend.deleteHPA(record.namespace, record.name), {
      successMessage: "Horizontal Pod Autoscaler deleted",
    });
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 150, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {
      key: "scaleTargetRef",
      title: "Scale Target",
      dataIndex: "scaleTargetRef",
      width: 210,
      render: (value) => <Badge variant="muted">{value}</Badge>,
    },
    {key: "minReplicas", title: "Min", dataIndex: "minReplicas", width: 80, align: "right", sortable: true},
    {key: "maxReplicas", title: "Max", dataIndex: "maxReplicas", width: 80, align: "right", sortable: true},
    {
      key: "replicas",
      title: "Current / Desired",
      width: 150,
      align: "right",
      render: (_, record) => (
        <span className="tabular-nums">
          {record.currentReplicas} / {record.desiredReplicas}
        </span>
      ),
    },
    {
      key: "cpuTargetUtilization",
      title: "CPU Target",
      dataIndex: "cpuTargetUtilization",
      width: 120,
      align: "right",
      render: (value) => (value !== null && value !== undefined ? `${value}%` : "—"),
    },
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
            title={`Delete Horizontal Pod Autoscaler "${record.name}"?`}
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
      {error ? <MessageAlert title="Failed to fetch Horizontal Pod Autoscalers" description={error} /> : null}

      <DataTable
        scopeToWorkspace
        title={i18next.t("general:Horizontal Pod Autoscaler")}
        description={`${hpas?.length ?? 0} autoscalers`}
        columns={columns}
        dataSource={hpas}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No Horizontal Pod Autoscalers found"
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
        title={mode === "add" ? "Add Horizontal Pod Autoscaler" : "Edit Horizontal Pod Autoscaler"}
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

        <Field label={i18next.t("general:Name")} htmlFor="hpa-name" required error={errors.name}>
          <Input
            id="hpa-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-hpa"
            disabled={mode === "edit"}
          />
        </Field>

        <div className="grid grid-cols-2 gap-3">
          <Field label="Scale Target Kind" required error={errors.scaleTargetKind}>
            <SimpleSelect
              value={form.scaleTargetKind}
              onChange={(next) => setForm((prev) => ({...prev, scaleTargetKind: next}))}
              options={SCALE_TARGET_KINDS}
            />
          </Field>
          <Field label="Scale Target Name" htmlFor="hpa-target" required error={errors.scaleTargetName}>
            <Input
              id="hpa-target"
              value={form.scaleTargetName}
              onChange={(event) => setForm((prev) => ({...prev, scaleTargetName: event.target.value}))}
              placeholder="my-deployment"
            />
          </Field>
        </div>

        <div className="flex flex-wrap gap-6">
          <Field label="Min Replicas">
            <NumberInput
              value={form.minReplicas}
              onChange={(next) => setForm((prev) => ({...prev, minReplicas: next}))}
              min={1}
            />
          </Field>
          <Field label="Max Replicas" error={errors.maxReplicas}>
            <NumberInput
              value={form.maxReplicas}
              onChange={(next) => setForm((prev) => ({...prev, maxReplicas: next}))}
              min={1}
            />
          </Field>
          <Field
            label={
              <LabelWithTip
                text="CPU Target %"
                tooltip="Target average CPU utilization. Leave empty to disable CPU-based scaling."
              />
            }
          >
            <NumberInput
              value={form.cpuTargetUtilization}
              onChange={(next) => setForm((prev) => ({...prev, cpuTargetUtilization: next}))}
              min={1}
              max={100}
              placeholder="80"
            />
          </Field>
        </div>
      </FormDialog>
    </PageContainer>
  );
}

export default HPAListPage;
