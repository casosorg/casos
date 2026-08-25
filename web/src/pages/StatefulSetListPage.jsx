import React, {useEffect, useState} from "react";
import i18next from "i18next";
import {Pencil, Plus, RefreshCw, Trash2} from "lucide-react";
import * as StatefulSetBackend from "@/backend/StatefulSetBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import * as ConfigMapBackend from "@/backend/ConfigMapBackend";
import * as SecretBackend from "@/backend/SecretBackend";
import * as MetricsBackend from "@/backend/MetricsBackend";
import * as Setting from "@/Setting";
import {runAction, useResource} from "@/hooks/use-resource";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {Separator} from "@/components/ui/separator";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {SearchSelect} from "@/components/shared/simple-select";
import {NumberInput} from "@/components/shared/number-input";
import {ReplicasControl} from "@/components/shared/replicas-control";
import {LabelWithTip} from "@/components/shared/misc";
import {EnvVarEditor, envVarsToRows, rowsToEnvVars} from "@/components/shared/env-var-editor";
import {
  ResourceEditor,
  emptyResources,
  resourcesFromRecord,
  resourcesToPayload,
  validateResources,
  workloadUsage,
} from "@/components/shared/resource-editor";

const emptyForm = {
  namespace: "",
  name: "",
  serviceName: "",
  image: "",
  replicas: 1,
  containerName: "",
  ...emptyResources,
  envVars: [],
};

function StatefulSetListPage() {
  const {
    data: statefulSets,
    setData: setStatefulSets,
    loading,
    error,
    refresh,
  } = useResource(() => StatefulSetBackend.getStatefulSets(), [], {initialData: []});
  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});
  const {data: metrics} = useResource(() => MetricsBackend.getMetrics(), [], {initialData: null, toastOnError: false});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [mode, setMode] = useState("add");
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);
  const [configMaps, setConfigMaps] = useState([]);
  const [secrets, setSecrets] = useState([]);

  useEffect(() => {
    if (!dialogOpen || !form.namespace) {
      return;
    }
    let cancelled = false;
    ConfigMapBackend.getConfigMaps(form.namespace)
      .then((res) => {
        if (!cancelled && res.status === "ok") {
          setConfigMaps(res.data ?? []);
        }
      })
      .catch(() => {});
    SecretBackend.getSecrets(form.namespace)
      .then((res) => {
        if (!cancelled && res.status === "ok") {
          setSecrets(res.data ?? []);
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [dialogOpen, form.namespace]);

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
      serviceName: record.serviceName ?? "",
      image: record.image,
      replicas: record.replicas ?? 0,
      containerName: "",
      ...resourcesFromRecord(record),
      envVars: envVarsToRows(record.envVars),
    });
    setErrors({});
    setDialogOpen(true);
  }

  // Scaling from the table patches the one row it changed rather than re-reading
  // the list, so the other rows do not flicker while a rollout is in progress.
  function handleScale(record, next) {
    return StatefulSetBackend.updateStatefulSet({...record, replicas: next})
      .then((res) => {
        if (res.status === "ok") {
          setStatefulSets((previous) =>
            previous.map((item) => (item.namespace === record.namespace && item.name === record.name ? res.data : item))
          );
        } else {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch((e) => Setting.showMessage("error", e.message));
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
    Object.assign(nextErrors, validateResources(form));
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const payload = {
      namespace: form.namespace,
      name: form.name,
      serviceName: form.serviceName ?? "",
      image: form.image,
      // An emptied box means "not set", not zero: sent as null the API keeps the
      // current count on edit and falls back to 1 on create, while an explicit 0
      // is sent as 0 and scales the workload down.
      replicas: form.replicas === "" || form.replicas === null ? null : Number(form.replicas),
      containerName: form.containerName ?? "",
      ...resourcesToPayload(form),
      envVars: rowsToEnvVars(form.envVars),
    };

    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(StatefulSetBackend.addStatefulSet(payload), {successMessage: "StatefulSet created"})
        : await runAction(StatefulSetBackend.updateStatefulSet({...payload, resourceVersion: editing.resourceVersion}), {
          successMessage: "StatefulSet updated",
        });
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(StatefulSetBackend.deleteStatefulSet(record.namespace, record.name), {
      successMessage: "StatefulSet deleted",
    });
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 160, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {key: "serviceName", title: "Service Name", dataIndex: "serviceName", width: 170},
    {key: "image", title: i18next.t("general:Image"), dataIndex: "image", ellipsis: true, className: "font-mono text-xs"},
    {
      key: "replicas",
      title: "Replicas",
      width: 220,
      align: "right",
      render: (_, record) => (
        <ReplicasControl
          readyReplicas={record.readyReplicas ?? 0}
          replicas={record.replicas ?? 0}
          onScale={(next) => handleScale(record, next)}
        />
      ),
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
            title={`Delete StatefulSet "${record.name}"?`}
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
      {error ? <MessageAlert title="Failed to fetch StatefulSets" description={error} /> : null}

      <DataTable
        scopeToWorkspace
        title={i18next.t("general:Stateful Sets")}
        description={`${statefulSets?.length ?? 0} stateful sets`}
        columns={columns}
        dataSource={statefulSets}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No StatefulSets found"
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
        title={mode === "add" ? "Add Stateful Set" : "Edit Stateful Set"}
        submitText={mode === "add" ? "Create" : "Update"}
        submitting={submitting}
        onSubmit={handleSubmit}
        size="lg"
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

        <Field label={i18next.t("general:Name")} htmlFor="sts-name" required error={errors.name}>
          <Input
            id="sts-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-statefulset"
            disabled={mode === "edit"}
          />
        </Field>

        <Field
          label={<LabelWithTip text="Service Name" tooltip="Headless service that governs this StatefulSet" />}
          htmlFor="sts-service"
          hint="Leave empty to use the StatefulSet name."
        >
          <Input
            id="sts-service"
            value={form.serviceName}
            onChange={(event) => setForm((prev) => ({...prev, serviceName: event.target.value}))}
            disabled={mode === "edit"}
          />
        </Field>

        <Field label={i18next.t("general:Image")} htmlFor="sts-image" required error={errors.image}>
          <Input
            id="sts-image"
            value={form.image}
            onChange={(event) => setForm((prev) => ({...prev, image: event.target.value}))}
            placeholder="nginx:latest"
          />
        </Field>

        <Field label="Replicas" required>
          <NumberInput value={form.replicas} onChange={(next) => setForm((prev) => ({...prev, replicas: next}))} min={0} />
        </Field>

        {mode === "add" ? (
          <Field label="Container Name" htmlFor="sts-container" hint="Leave empty to use the StatefulSet name.">
            <Input
              id="sts-container"
              value={form.containerName}
              onChange={(event) => setForm((prev) => ({...prev, containerName: event.target.value}))}
            />
          </Field>
        ) : null}

        <Separator />

        <Field label="CPU & Memory">
          <ResourceEditor
            value={form}
            onChange={(next) => setForm((prev) => ({...prev, ...next}))}
            errors={errors}
            usage={editing ? workloadUsage(metrics?.pods, editing.namespace, editing.name) : null}
          />
        </Field>

        <Separator />

        <Field label="Environment Variables">
          <EnvVarEditor
            value={form.envVars}
            onChange={(envVars) => setForm((prev) => ({...prev, envVars}))}
            configMaps={configMaps}
            secrets={secrets}
          />
        </Field>
      </FormDialog>
    </PageContainer>
  );
}

export default StatefulSetListPage;
