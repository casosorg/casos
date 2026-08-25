import React, {useState} from "react";
import i18next from "i18next";
import {Pencil, Plus, RefreshCw, Trash2} from "lucide-react";
import * as NetworkPolicyBackend from "@/backend/NetworkPolicyBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import {runAction, useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Checkbox} from "@/components/ui/checkbox";
import {Input} from "@/components/ui/input";
import {Label} from "@/components/ui/label";
import {MessageAlert} from "@/components/ui/alert";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {SearchSelect} from "@/components/shared/simple-select";
import {KeyValueEditor, fromEntries} from "@/components/shared/key-value-editor";

const POLICY_TYPES = ["Ingress", "Egress"];

const emptyForm = {namespace: "", name: "", policyTypes: ["Ingress"], podSelectorEntries: []};

// The API returns the selector as a JSON string rather than an object, so both
// the table cell and the edit form have to parse it — and tolerate it being
// malformed rather than blanking the row.
function parseSelector(raw) {
  try {
    return JSON.parse(raw || "{}");
  } catch {
    return null;
  }
}

function NetworkPolicyListPage() {
  const {
    data: policies,
    loading,
    error,
    refresh,
  } = useResource(() => NetworkPolicyBackend.getNetworkPolicies(), [], {initialData: []});
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
    const selector = parseSelector(record.podSelector) ?? {};
    setMode("edit");
    setEditing(record);
    setForm({
      namespace: record.namespace,
      name: record.name,
      policyTypes: record.policyTypes ?? ["Ingress"],
      podSelectorEntries: Object.entries(selector).map(([key, value]) => ({key, value})),
    });
    setErrors({});
    setDialogOpen(true);
  }

  function togglePolicyType(type) {
    setForm((prev) => ({
      ...prev,
      policyTypes: prev.policyTypes.includes(type)
        ? prev.policyTypes.filter((item) => item !== type)
        : [...prev.policyTypes, type],
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
    if (form.policyTypes.length === 0) {
      nextErrors.policyTypes = "At least one policy type is required";
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const payload = {
      name: form.name,
      namespace: form.namespace,
      policyTypes: form.policyTypes,
      podSelectorLabels: fromEntries(form.podSelectorEntries),
      ingress: [],
      egress: [],
    };

    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(NetworkPolicyBackend.addNetworkPolicy(payload), {successMessage: "Network Policy created"})
        : await runAction(NetworkPolicyBackend.updateNetworkPolicy({...payload, resourceVersion: editing.resourceVersion}), {
          successMessage: "Network Policy updated",
        });
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(NetworkPolicyBackend.deleteNetworkPolicy(record.namespace, record.name), {
      successMessage: "Network Policy deleted",
    });
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 160, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {
      key: "podSelector",
      title: "Pod Selector",
      dataIndex: "podSelector",
      render: (value) => {
        const selector = parseSelector(value);
        if (selector === null) {
          return <Badge variant="muted">{value}</Badge>;
        }
        const entries = Object.entries(selector);
        if (entries.length === 0) {
          return <Badge variant="muted">All Pods</Badge>;
        }
        return (
          <div className="flex flex-wrap gap-1">
            {entries.map(([key, labelValue]) => (
              <Badge key={key} variant="muted" className="font-mono">
                {key}: {labelValue}
              </Badge>
            ))}
          </div>
        );
      },
    },
    {
      key: "policyTypes",
      title: "Policy Types",
      dataIndex: "policyTypes",
      width: 170,
      render: (types) => (
        <div className="flex flex-wrap gap-1">
          {(types ?? []).map((type) => (
            <Badge key={type} variant={type === "Ingress" ? "info" : "warning"}>
              {type}
            </Badge>
          ))}
        </div>
      ),
    },
    {key: "ingressRules", title: "Ingress Rules", dataIndex: "ingressRules", width: 130, align: "right"},
    {key: "egressRules", title: "Egress Rules", dataIndex: "egressRules", width: 130, align: "right"},
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
            title={`Delete Network Policy "${record.name}"?`}
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
      {error ? <MessageAlert title="Failed to fetch Network Policies" description={error} /> : null}

      <DataTable
        scopeToWorkspace
        title={i18next.t("general:Network Policies")}
        description={`${policies?.length ?? 0} policies`}
        columns={columns}
        dataSource={policies}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No Network Policies found"
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
        title={mode === "add" ? "Add Network Policy" : "Edit Network Policy"}
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

        <Field label={i18next.t("general:Name")} htmlFor="np-name" required error={errors.name}>
          <Input
            id="np-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-network-policy"
            disabled={mode === "edit"}
          />
        </Field>

        <Field label="Policy Types" required error={errors.policyTypes}>
          <div className="flex gap-6">
            {POLICY_TYPES.map((type) => (
              <div key={type} className="flex items-center gap-2">
                <Checkbox
                  id={`np-type-${type}`}
                  checked={form.policyTypes.includes(type)}
                  onCheckedChange={() => togglePolicyType(type)}
                />
                <Label htmlFor={`np-type-${type}`}>{type}</Label>
              </div>
            ))}
          </div>
        </Field>

        <Field label="Pod Selector Labels" hint="Leave empty to select all pods in the namespace.">
          <KeyValueEditor
            value={form.podSelectorEntries}
            onChange={(podSelectorEntries) => setForm((prev) => ({...prev, podSelectorEntries}))}
            addLabel="Add Label"
          />
        </Field>
      </FormDialog>
    </PageContainer>
  );
}

export default NetworkPolicyListPage;
