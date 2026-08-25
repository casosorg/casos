import React, {useState} from "react";
import i18next from "i18next";
import {Pencil, Plus, RefreshCw, Trash2} from "lucide-react";
import * as ServiceAccountBackend from "@/backend/ServiceAccountBackend";
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
import {StringListEditor} from "@/components/shared/key-value-editor";

const emptyForm = {namespace: "", name: "", imagePullSecrets: []};

function ServiceAccountListPage() {
  const {
    data: serviceAccounts,
    loading,
    error,
    refresh,
  } = useResource(() => ServiceAccountBackend.getServiceAccounts(), [], {initialData: []});
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
    setForm({namespace: record.namespace, name: record.name, imagePullSecrets: [...(record.imagePullSecrets ?? [])]});
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
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const payload = {
      namespace: form.namespace,
      name: form.name,
      imagePullSecrets: form.imagePullSecrets.filter(Boolean),
    };
    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(ServiceAccountBackend.addServiceAccount(payload), {successMessage: "ServiceAccount created"})
        : await runAction(ServiceAccountBackend.updateServiceAccount({...payload, resourceVersion: editing.resourceVersion}), {
          successMessage: "ServiceAccount updated",
        });
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(ServiceAccountBackend.deleteServiceAccount(record.namespace, record.name), {
      successMessage: "ServiceAccount deleted",
    });
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 170, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {
      key: "secrets",
      title: i18next.t("general:Secrets"),
      dataIndex: "secrets",
      width: 100,
      align: "right",
      sortable: true,
      render: (value) => <span className="tabular-nums">{value ?? 0}</span>,
    },
    {
      key: "imagePullSecrets",
      title: "Image Pull Secrets",
      dataIndex: "imagePullSecrets",
      render: (list) =>
        list && list.length > 0 ? (
          <div className="flex flex-wrap gap-1">
            {list.map((secret) => (
              <Badge key={secret} variant="muted" className="font-mono">
                {secret}
              </Badge>
            ))}
          </div>
        ) : (
          <span className="text-muted-foreground">—</span>
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
            title={`Delete ServiceAccount "${record.name}"?`}
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
      {error ? <MessageAlert title="Failed to fetch ServiceAccounts" description={error} /> : null}

      <DataTable
        scopeToWorkspace
        title={i18next.t("general:ServiceAccounts")}
        description={`${serviceAccounts?.length ?? 0} service accounts`}
        columns={columns}
        dataSource={serviceAccounts}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No ServiceAccounts found"
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
        title={mode === "add" ? "Add Service Account" : "Edit Service Account"}
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

        <Field label={i18next.t("general:Name")} htmlFor="serviceaccount-name" required error={errors.name}>
          <Input
            id="serviceaccount-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-service-account"
            disabled={mode === "edit"}
          />
        </Field>

        <Field label="Image Pull Secrets" hint="Secrets used to pull private images for pods running as this account.">
          <StringListEditor
            value={form.imagePullSecrets}
            onChange={(next) => setForm((prev) => ({...prev, imagePullSecrets: next}))}
            placeholder="secret-name"
            addLabel="Add Secret"
          />
        </Field>
      </FormDialog>
    </PageContainer>
  );
}

export default ServiceAccountListPage;
