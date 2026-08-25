import React, {useState} from "react";
import i18next from "i18next";
import {Plus, RefreshCw, Trash2} from "lucide-react";
import * as PvcBackend from "@/backend/PvcBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import {runAction, useResource} from "@/hooks/use-resource";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {SearchSelect, SimpleSelect} from "@/components/shared/simple-select";
import {StatusBadge} from "@/components/shared/status-badge";

const ACCESS_MODE_OPTIONS = [
  {label: "ReadWriteOnce (单节点读写)", value: "ReadWriteOnce"},
  {label: "ReadOnlyMany (多节点只读)", value: "ReadOnlyMany"},
  {label: "ReadWriteMany (多节点读写)", value: "ReadWriteMany"},
];

const STATUS_VARIANTS = {Bound: "success", Pending: "warning", Lost: "danger"};

const emptyForm = {namespace: "", name: "", storage: "1Gi", accessMode: "ReadWriteOnce", storageClassName: ""};

function PvcListPage() {
  const {data: pvcs, loading, error, refresh} = useResource(() => PvcBackend.getPvcs(), [], {initialData: []});
  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);

  const namespaceOptions = (namespaces ?? []).map((item) => ({label: item.name, value: item.name}));

  function openAdd() {
    setForm({...emptyForm, namespace: namespaces?.[0]?.name ?? "default"});
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
    if (!form.storage) {
      nextErrors.storage = "Storage size is required";
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    setSubmitting(true);
    const ok = await runAction(PvcBackend.addPvc(form), {successMessage: "PVC created"});
    setSubmitting(false);
    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(PvcBackend.deletePvc(record.namespace, record.name), {successMessage: "PVC deleted"});
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 160, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {
      key: "status",
      title: i18next.t("general:Status"),
      dataIndex: "status",
      width: 120,
      sortable: true,
      render: (value) => <StatusBadge status={value} variants={STATUS_VARIANTS} />,
    },
    {key: "storage", title: "Storage", dataIndex: "storage", width: 110, sortable: true},
    {key: "accessMode", title: "Access Mode", dataIndex: "accessMode", width: 170},
    {key: "storageClassName", title: "Storage Class", dataIndex: "storageClassName", width: 160, sortable: true},
    {key: "volumeName", title: "Volume", dataIndex: "volumeName", ellipsis: true},
    {key: "createdAt", title: i18next.t("general:Created"), dataIndex: "createdAt", width: 190, sortable: true},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 110,
      align: "right",
      render: (_, record) => (
        <ConfirmDialog
          title={`Delete PVC "${record.name}"?`}
          description="Deleting a PVC may cause data loss if it is still mounted."
          confirmText="Delete"
          onConfirm={() => handleDelete(record)}
        >
          <Button variant="outline" size="sm" className="text-destructive">
            <Trash2 />
          </Button>
        </ConfirmDialog>
      ),
    },
  ];

  return (
    <PageContainer>
      {error ? <MessageAlert title="Failed to fetch PVCs" description={error} /> : null}

      <DataTable
        scopeToWorkspace
        title={i18next.t("general:Persistent Volume Claims")}
        description={`${pvcs?.length ?? 0} claims`}
        columns={columns}
        dataSource={pvcs}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No Persistent Volume Claims found"
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
        title="Add Persistent Volume Claim"
        submitText="Create"
        submitting={submitting}
        onSubmit={handleSubmit}
      >
        <Field label={i18next.t("general:Namespace")} required error={errors.namespace}>
          <SearchSelect
            value={form.namespace}
            onChange={(next) => setForm((prev) => ({...prev, namespace: next}))}
            options={namespaceOptions}
            placeholder="Select a namespace"
          />
        </Field>

        <Field label={i18next.t("general:Name")} htmlFor="pvc-name" required error={errors.name}>
          <Input
            id="pvc-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-pvc"
          />
        </Field>

        <Field label="Storage Size" htmlFor="pvc-storage" required error={errors.storage}>
          <Input
            id="pvc-storage"
            value={form.storage}
            onChange={(event) => setForm((prev) => ({...prev, storage: event.target.value}))}
            placeholder="1Gi"
          />
        </Field>

        <Field label="Access Mode" required>
          <SimpleSelect
            value={form.accessMode}
            onChange={(next) => setForm((prev) => ({...prev, accessMode: next}))}
            options={ACCESS_MODE_OPTIONS}
          />
        </Field>

        <Field label="Storage Class" htmlFor="pvc-storageclass" hint="Leave empty to use the cluster default.">
          <Input
            id="pvc-storageclass"
            value={form.storageClassName}
            onChange={(event) => setForm((prev) => ({...prev, storageClassName: event.target.value}))}
            placeholder="Leave empty to use cluster default"
          />
        </Field>
      </FormDialog>
    </PageContainer>
  );
}

export default PvcListPage;
