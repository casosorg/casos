import React, {useRef, useState} from "react";
import i18next from "i18next";
import {Pencil, Plus, RefreshCw, Trash2} from "lucide-react";
import * as SecretBackend from "@/backend/SecretBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import * as Setting from "@/Setting";
import {runAction, useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {SearchSelect, SimpleSelect} from "@/components/shared/simple-select";
import {KeyValueEditor, fromEntries, toEntries} from "@/components/shared/key-value-editor";
import {RestartDeploymentsDialog} from "@/components/shared/restart-deployments-dialog";

const SECRET_TYPES = [
  {label: "Opaque", value: "Opaque"},
  {label: "kubernetes.io/tls", value: "kubernetes.io/tls"},
  {label: "kubernetes.io/dockerconfigjson", value: "kubernetes.io/dockerconfigjson"},
  {label: "kubernetes.io/basic-auth", value: "kubernetes.io/basic-auth"},
  {label: "kubernetes.io/ssh-auth", value: "kubernetes.io/ssh-auth"},
  {label: "kubernetes.io/service-account-token", value: "kubernetes.io/service-account-token"},
];

const emptyForm = {namespace: "", name: "", type: "Opaque", entries: []};

function SecretListPage() {
  const {data: secrets, loading, error, refresh} = useResource(() => SecretBackend.getSecrets(), [], {initialData: []});
  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [mode, setMode] = useState("add");
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [restartTarget, setRestartTarget] = useState(null);
  const editRequestRef = useRef(0);

  const namespaceOptions = (namespaces ?? []).map((item) => ({label: item.name, value: item.name}));

  // Makes any in-flight detail response stale, so it cannot land in a dialog
  // the user has since closed or reopened on another Secret.
  function dropPendingDetail() {
    editRequestRef.current += 1;
    setDetailLoading(false);
  }

  function openAdd() {
    dropPendingDetail();
    setMode("add");
    setEditing(null);
    setForm({...emptyForm, namespace: namespaces?.[0]?.name ?? "default"});
    setErrors({});
    setDialogOpen(true);
  }

  async function openEdit(record) {
    const requestId = ++editRequestRef.current;
    setMode("edit");
    setEditing(record);
    setForm({
      namespace: record.namespace,
      name: record.name,
      type: record.type || "Opaque",
      entries: [],
    });
    setErrors({});
    setDetailLoading(true);
    setDialogOpen(true);

    const res = await SecretBackend.getSecret(record.namespace, record.name)
      .catch((e) => ({status: "error", msg: e.message}));
    if (requestId !== editRequestRef.current) {
      return;
    }
    setDetailLoading(false);

    if (res?.status !== "ok") {
      // The editor is still empty here, and submitting it would replace the
      // Secret with no keys at all, so close rather than offer that.
      setDialogOpen(false);
      Setting.showMessage("error", res?.msg ?? "Request failed");
      return;
    }
    setEditing(res.data);
    setForm({
      namespace: res.data.namespace,
      name: res.data.name,
      type: res.data.type || "Opaque",
      entries: toEntries(res.data.stringData),
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
    if (!form.type) {
      nextErrors.type = "Type is required";
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const payload = {
      name: form.name,
      namespace: form.namespace,
      type: form.type,
      stringData: fromEntries(form.entries),
    };
    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(SecretBackend.addSecret(payload), {successMessage: "Secret created"})
        : await runAction(SecretBackend.updateSecret({...payload, resourceVersion: editing.resourceVersion}), {
          successMessage: "Secret updated",
        });
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
      if (mode === "edit") {
        setRestartTarget({namespace: editing.namespace, name: editing.name});
      }
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(SecretBackend.deleteSecret(record.namespace, record.name), {successMessage: "Secret deleted"});
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 170, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", width: 210, sortable: true, className: "font-medium"},
    {key: "type", title: i18next.t("general:Type"), dataIndex: "type", width: 220, sortable: true},
    {
      key: "keys",
      title: "Keys",
      dataIndex: "keys",
      render: (keys) => (
        <div className="flex flex-wrap gap-1">
          {(keys ?? []).map((key) => (
            <Badge key={key} variant="warning" className="font-mono">
              {key}
            </Badge>
          ))}
        </div>
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
            title={`Delete Secret "${record.name}"?`}
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
      {error ? <MessageAlert title="Failed to fetch Secrets" description={error} /> : null}

      <DataTable
        scopeToWorkspace
        title={i18next.t("general:Secrets")}
        description={`${secrets?.length ?? 0} secrets`}
        columns={columns}
        dataSource={secrets}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No Secrets found"
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
        onOpenChange={(open) => {
          if (!open) {
            dropPendingDetail();
          }
          setDialogOpen(open);
        }}
        title={mode === "add" ? "Add Secret" : "Edit Secret"}
        submitText={mode === "add" ? "Create" : "Update"}
        submitting={submitting || detailLoading}
        submitDisabled={detailLoading}
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

        <Field label={i18next.t("general:Name")} htmlFor="secret-name" required error={errors.name}>
          <Input
            id="secret-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-secret"
            disabled={mode === "edit"}
          />
        </Field>

        <Field label={i18next.t("general:Type")} required error={errors.type}>
          <SimpleSelect
            value={form.type}
            onChange={(next) => setForm((prev) => ({...prev, type: next}))}
            options={SECRET_TYPES}
            placeholder="Select secret type"
            disabled={mode === "edit"}
          />
        </Field>

        <Field label="Data" hint="Values are write-only; existing values are never sent back to the browser.">
          <KeyValueEditor
            value={form.entries}
            onChange={(entries) => setForm((prev) => ({...prev, entries}))}
            valueType="password"
          />
        </Field>
      </FormDialog>

      <RestartDeploymentsDialog
        open={restartTarget !== null}
        onClose={() => setRestartTarget(null)}
        namespace={restartTarget?.namespace ?? ""}
        configType="secret"
        configName={restartTarget?.name ?? ""}
      />
    </PageContainer>
  );
}

export default SecretListPage;
