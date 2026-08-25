import React, {useEffect, useState} from "react";
import i18next from "i18next";
import {Pencil, Plus, RefreshCw, Trash2} from "lucide-react";
import * as DaemonSetBackend from "@/backend/DaemonSetBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import * as ConfigMapBackend from "@/backend/ConfigMapBackend";
import * as SecretBackend from "@/backend/SecretBackend";
import * as MetricsBackend from "@/backend/MetricsBackend";
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
import {ReadyBadge} from "@/components/shared/status-badge";
import {EnvVarEditor, envVarsToRows, rowsToEnvVars} from "@/components/shared/env-var-editor";
import {
  ResourceEditor,
  emptyResources,
  resourcesFromRecord,
  resourcesToPayload,
  validateResources,
  workloadUsage,
} from "@/components/shared/resource-editor";

const emptyForm = {namespace: "", name: "", image: "", containerName: "", ...emptyResources, envVars: []};

function DaemonSetListPage() {
  const {data: daemonSets, loading, error, refresh} = useResource(() => DaemonSetBackend.getDaemonSets(), [], {initialData: []});
  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});
  const {data: metrics} = useResource(() => MetricsBackend.getMetrics(), [], {initialData: null, toastOnError: false});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [mode, setMode] = useState("add");
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);

  // Env var references can only point at ConfigMaps and Secrets in the same
  // namespace, so both lists are re-read whenever the namespace changes.
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
      image: record.image,
      containerName: "",
      ...resourcesFromRecord(record),
      envVars: envVarsToRows(record.envVars),
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
    Object.assign(nextErrors, validateResources(form));
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const payload = {
      namespace: form.namespace,
      name: form.name,
      image: form.image,
      containerName: form.containerName ?? "",
      ...resourcesToPayload(form),
      envVars: rowsToEnvVars(form.envVars),
    };

    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(DaemonSetBackend.addDaemonSet(payload), {successMessage: "Daemon Set created"})
        : await runAction(DaemonSetBackend.updateDaemonSet({...payload, resourceVersion: editing.resourceVersion}), {
          successMessage: "Daemon Set updated",
        });
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(DaemonSetBackend.deleteDaemonSet(record.namespace, record.name), {
      successMessage: "Daemon Set deleted",
    });
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 160, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {key: "image", title: i18next.t("general:Image"), dataIndex: "image", ellipsis: true, className: "font-mono text-xs"},
    {
      key: "status",
      title: "Ready / Desired",
      width: 160,
      align: "right",
      render: (_, record) => <ReadyBadge ready={record.numberReady} total={record.desiredNumberScheduled} />,
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
            title={`Delete Daemon Set "${record.name}"?`}
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
      {error ? <MessageAlert title="Failed to fetch Daemon Sets" description={error} /> : null}

      <DataTable
        scopeToWorkspace
        title={i18next.t("general:Daemon Sets")}
        description={`${daemonSets?.length ?? 0} daemon sets`}
        columns={columns}
        dataSource={daemonSets}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No Daemon Sets found"
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
        title={mode === "add" ? "Add Daemon Set" : "Edit Daemon Set"}
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

        <Field label={i18next.t("general:Name")} htmlFor="ds-name" required error={errors.name}>
          <Input
            id="ds-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-daemon-set"
            disabled={mode === "edit"}
          />
        </Field>

        <Field label={i18next.t("general:Image")} htmlFor="ds-image" required error={errors.image}>
          <Input
            id="ds-image"
            value={form.image}
            onChange={(event) => setForm((prev) => ({...prev, image: event.target.value}))}
            placeholder="nginx:latest"
          />
        </Field>

        {mode === "add" ? (
          <Field label="Container Name" htmlFor="ds-container" hint="Leave empty to use the Daemon Set name.">
            <Input
              id="ds-container"
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

export default DaemonSetListPage;
