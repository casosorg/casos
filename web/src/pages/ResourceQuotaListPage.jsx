import React, {useState} from "react";
import i18next from "i18next";
import {Pencil, Plus, RefreshCw, Trash2, X} from "lucide-react";
import * as ResourceQuotaBackend from "@/backend/ResourceQuotaBackend";
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

const COMMON_RESOURCES = [
  "requests.cpu",
  "requests.memory",
  "limits.cpu",
  "limits.memory",
  "pods",
  "services",
  "configmaps",
  "secrets",
  "persistentvolumeclaims",
  "services.loadbalancers",
  "services.nodeports",
].map((resource) => ({label: resource, value: resource}));

const emptyForm = {namespace: "", name: "", hardEntries: []};

// A quota can carry a dozen resources; showing them all would make every row as
// tall as a paragraph. Three plus a hover-revealed remainder keeps the table
// scannable without hiding anything outright.
function QuantityBadges({entries, variant = "muted", limit = 3}) {
  const list = Object.entries(entries ?? {});
  if (list.length === 0) {
    return <span className="text-muted-foreground">—</span>;
  }
  const visible = list.slice(0, limit);
  const rest = list.slice(limit);

  return (
    <div className="flex flex-wrap gap-1">
      {visible.map(([key, value]) => (
        <Badge key={key} variant={variant} className="font-mono">
          {key}: {value}
        </Badge>
      ))}
      {rest.length > 0 ? (
        <SimpleTooltip title={rest.map(([key, value]) => `${key}: ${value}`).join("\n")}>
          <Badge variant="outline">+{rest.length} more</Badge>
        </SimpleTooltip>
      ) : null}
    </div>
  );
}

function ResourceQuotaListPage() {
  const {
    data: quotas,
    loading,
    error,
    refresh,
  } = useResource(() => ResourceQuotaBackend.getResourceQuotas(), [], {initialData: []});
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
      hardEntries: Object.entries(record.hard ?? {}).map(([resource, quantity]) => ({resource, quantity})),
    });
    setErrors({});
    setDialogOpen(true);
  }

  function updateEntry(index, field, next) {
    setForm((prev) => ({
      ...prev,
      hardEntries: prev.hardEntries.map((entry, entryIndex) => (entryIndex === index ? {...entry, [field]: next} : entry)),
    }));
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

    const hard = {};
    form.hardEntries.forEach(({resource, quantity}) => {
      if (resource && quantity) {
        hard[resource] = quantity;
      }
    });
    const payload = {name: form.name, namespace: form.namespace, hard};

    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(ResourceQuotaBackend.addResourceQuota(payload), {successMessage: "Resource Quota created"})
        : await runAction(ResourceQuotaBackend.updateResourceQuota({...payload, resourceVersion: editing.resourceVersion}), {
          successMessage: "Resource Quota updated",
        });
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(ResourceQuotaBackend.deleteResourceQuota(record.namespace, record.name), {
      successMessage: "Resource Quota deleted",
    });
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 160, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", width: 210, sortable: true, className: "font-medium"},
    {key: "hard", title: "Hard Limits", dataIndex: "hard", render: (hard) => <QuantityBadges entries={hard} />},
    {key: "used", title: "Used", dataIndex: "used", render: (used) => <QuantityBadges entries={used} variant="info" />},
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
            title={`Delete Resource Quota "${record.name}"?`}
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
      {error ? <MessageAlert title="Failed to fetch Resource Quotas" description={error} /> : null}

      <DataTable
        scopeToWorkspace
        title={i18next.t("general:Resource Quotas")}
        description={`${quotas?.length ?? 0} quotas`}
        columns={columns}
        dataSource={quotas}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No Resource Quotas found"
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
        title={mode === "add" ? "Add Resource Quota" : "Edit Resource Quota"}
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

        <Field label={i18next.t("general:Name")} htmlFor="rq-name" required error={errors.name}>
          <Input
            id="rq-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-resource-quota"
            disabled={mode === "edit"}
          />
        </Field>

        <Field label="Hard Limits">
          <div className="grid gap-2">
            {form.hardEntries.map((entry, index) => (
              <div key={index} className="grid grid-cols-[minmax(0,1fr)_170px_auto] items-center gap-2">
                <SearchSelect
                  value={entry.resource}
                  onChange={(next) => updateEntry(index, "resource", next)}
                  options={COMMON_RESOURCES}
                  placeholder="Resource"
                  className="h-8 text-xs"
                />
                <Input
                  value={entry.quantity ?? ""}
                  onChange={(event) => updateEntry(index, "quantity", event.target.value)}
                  placeholder="e.g. 2, 4Gi, 500m"
                  className="h-8 font-mono text-xs"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  onClick={() =>
                    setForm((prev) => ({...prev, hardEntries: prev.hardEntries.filter((_, i) => i !== index)}))
                  }
                  className="text-muted-foreground hover:text-destructive"
                  aria-label="Remove limit"
                >
                  <X className="size-4" />
                </Button>
              </div>
            ))}
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="border-dashed"
              onClick={() => setForm((prev) => ({...prev, hardEntries: [...prev.hardEntries, {resource: "", quantity: ""}]}))}
            >
              <Plus />
              Add Limit
            </Button>
          </div>
        </Field>
      </FormDialog>
    </PageContainer>
  );
}

export default ResourceQuotaListPage;
