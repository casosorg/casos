import React, {useEffect, useRef, useState} from "react";
import i18next from "i18next";
import {Pencil, Plus, RefreshCw, Trash2} from "lucide-react";
import * as ConfigMapBackend from "@/backend/ConfigMapBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import * as Setting from "@/Setting";
import {runAction, useResource} from "@/hooks/use-resource";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {SearchSelect} from "@/components/shared/simple-select";
import {KeyValueEditor, fromEntries, toEntries} from "@/components/shared/key-value-editor";
import {RestartDeploymentsDialog} from "@/components/shared/restart-deployments-dialog";

const emptyForm = {namespace: "", name: "", entries: []};
const CONFIG_MAP_PAGE_SIZE = 20;

function ConfigMapListPage() {
  const [pageIndex, setPageIndex] = useState(0);
  const [pageTokens, setPageTokens] = useState([""]);
  const pageToken = pageTokens[pageIndex] ?? "";
  const {
    data: configMapPage,
    loading,
    error,
    refresh: refreshPage,
  } = useResource(
    () => ConfigMapBackend.getConfigMapPage("", CONFIG_MAP_PAGE_SIZE, pageToken),
    [pageIndex, pageToken],
    {initialData: {items: [], continueToken: "", remainingItemCount: null}}
  );
  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});

  const configMaps = configMapPage?.items ?? [];
  const hasNextPage = Boolean(configMapPage?.continueToken);
  const remainingItemCount = configMapPage?.remainingItemCount;
  const totalRows = Number.isFinite(remainingItemCount)
    ? pageIndex * CONFIG_MAP_PAGE_SIZE + configMaps.length + remainingItemCount
    : null;

  const [dialogOpen, setDialogOpen] = useState(false);
  const [mode, setMode] = useState("add");
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [restartTarget, setRestartTarget] = useState(null);
  const editRequestRef = useRef(0);

  useEffect(() => () => {
    editRequestRef.current += 1;
  }, []);

  const namespaceOptions = (namespaces ?? []).map((item) => ({label: item.name, value: item.name}));

  function invalidateEditRequest() {
    editRequestRef.current += 1;
    setDetailLoading(false);
  }

  function openAdd() {
    invalidateEditRequest();
    setMode("add");
    setEditing(null);
    setForm({...emptyForm, namespace: namespaces?.[0]?.name ?? "default"});
    setErrors({});
    setDialogOpen(true);
  }

  function openPreviousPage() {
    setPageIndex((current) => Math.max(0, current - 1));
  }

  function openNextPage() {
    if (!hasNextPage) {
      return;
    }
    setPageTokens((current) => {
      const next = current.slice(0, pageIndex + 1);
      next.push(configMapPage.continueToken);
      return next;
    });
    setPageIndex((current) => current + 1);
  }

  function refreshFromFirstPage() {
    if (pageIndex === 0) {
      refreshPage();
      return;
    }
    setPageTokens([""]);
    setPageIndex(0);
  }

  async function openEdit(record) {
    const requestId = ++editRequestRef.current;
    setMode("edit");
    setEditing(record);
    setForm({namespace: record.namespace, name: record.name, entries: []});
    setErrors({});
    setDialogOpen(true);

    setDetailLoading(true);
    try {
      const response = await ConfigMapBackend.getConfigMap(record.namespace, record.name);
      if (requestId !== editRequestRef.current) {
        return;
      }
      if (response?.status !== "ok") {
        Setting.showMessage("error", response?.msg ?? "Request failed");
        return;
      }
      const configMap = response.data ?? {};
      setEditing(configMap);
      setForm({namespace: configMap.namespace, name: configMap.name, entries: toEntries(configMap.data)});
    } catch (error) {
      if (requestId === editRequestRef.current) {
        Setting.showMessage("error", error.message);
      }
    } finally {
      if (requestId === editRequestRef.current) {
        setDetailLoading(false);
      }
    }
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

    const payload = {name: form.name, namespace: form.namespace, data: fromEntries(form.entries)};
    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(ConfigMapBackend.addConfigMap(payload), {successMessage: "ConfigMap created"})
        : await runAction(ConfigMapBackend.updateConfigMap({...payload, resourceVersion: editing.resourceVersion}), {
          successMessage: "ConfigMap updated",
        });
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refreshFromFirstPage();
      if (mode === "edit") {
        // Pods hold the values they started with, so an edit is only live once
        // the deployments referencing it roll.
        setRestartTarget({namespace: editing.namespace, name: editing.name});
      }
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(ConfigMapBackend.deleteConfigMap(record.namespace, record.name), {
      successMessage: "ConfigMap deleted",
    });
    if (ok) {
      refreshFromFirstPage();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 170, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", width: 210, sortable: true, className: "font-medium"},
    {
      key: "data",
      title: "Keys",
      dataIndex: "dataKeys",
      render: (count) => `${count ?? 0}`,
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
            title={`Delete ConfigMap "${record.name}"?`}
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
      {error ? <MessageAlert title="Failed to fetch ConfigMaps" description={error} /> : null}

      <DataTable
        title={i18next.t("general:ConfigMaps")}
        description={`${totalRows ?? pageIndex * CONFIG_MAP_PAGE_SIZE + configMaps.length} config maps`}
        columns={columns}
        dataSource={configMaps}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        pageSize={0}
        manualPagination={{
          pageIndex,
          hasPreviousPage: pageIndex > 0,
          hasNextPage,
          onPreviousPage: openPreviousPage,
          onNextPage: openNextPage,
          label: totalRows === null ? `Page ${pageIndex + 1}` : `${pageIndex * CONFIG_MAP_PAGE_SIZE + 1}–${pageIndex * CONFIG_MAP_PAGE_SIZE + configMaps.length} of ${totalRows}`,
          totalRows,
        }}
        emptyText="No ConfigMaps found"
        toolbar={
          <>
            <Button variant="outline" size="sm" onClick={refreshFromFirstPage} loading={loading}>
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
            invalidateEditRequest();
          }
          setDialogOpen(open);
        }}
        title={mode === "add" ? "Add ConfigMap" : "Edit ConfigMap"}
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

        <Field label={i18next.t("general:Name")} htmlFor="configmap-name" required error={errors.name}>
          <Input
            id="configmap-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-configmap"
            disabled={mode === "edit"}
          />
        </Field>

        <Field label="Data" hint="Key-value pairs mounted or referenced by workloads.">
          <KeyValueEditor
            value={form.entries}
            onChange={(entries) => setForm((prev) => ({...prev, entries}))}
            valueType="textarea"
          />
        </Field>
      </FormDialog>

      <RestartDeploymentsDialog
        open={restartTarget !== null}
        onClose={() => setRestartTarget(null)}
        namespace={restartTarget?.namespace ?? ""}
        configType="configmap"
        configName={restartTarget?.name ?? ""}
      />
    </PageContainer>
  );
}

export default ConfigMapListPage;
