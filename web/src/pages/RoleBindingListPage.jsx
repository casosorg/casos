import React, {useState} from "react";
import i18next from "i18next";
import {Pencil, Plus, RefreshCw, Trash2} from "lucide-react";
import * as RoleBindingBackend from "@/backend/RoleBindingBackend";
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
import {SearchSelect, SimpleSelect} from "@/components/shared/simple-select";
import {SubjectBadges, SubjectsEditor, subjectsToRows} from "@/components/shared/subjects-editor";

const ROLE_REF_KINDS = [
  {label: "Role", value: "Role"},
  {label: "Cluster Role", value: "ClusterRole"},
];

const emptyForm = {namespace: "", name: "", roleRef: "", roleRefKind: "Role", subjects: []};

function RoleBindingListPage() {
  const {data: bindings, loading, error, refresh} = useResource(() => RoleBindingBackend.getRoleBindings(), [], {initialData: []});
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
      roleRef: record.roleRef,
      roleRefKind: record.roleRefKind || "Role",
      subjects: subjectsToRows(record.subjects),
    });
    setErrors({});
    setDialogOpen(true);
  }

  async function handleSubmit() {
    const nextErrors = {};
    if (!form.namespace) {
      nextErrors.namespace = "Required";
    }
    if (!form.name) {
      nextErrors.name = "Required";
    }
    if (!form.roleRef) {
      nextErrors.roleRef = "Required";
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const subjects = form.subjects.filter((subject) => subject && subject.name);
    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(
          RoleBindingBackend.addRoleBinding({
            namespace: form.namespace,
            name: form.name,
            roleRef: form.roleRef,
            roleRefKind: form.roleRefKind || "Role",
            subjects,
          }),
          {successMessage: "Role Binding created"}
        )
        : await runAction(
          RoleBindingBackend.updateRoleBinding({
            namespace: editing.namespace,
            name: editing.name,
            roleRef: editing.roleRef,
            subjects,
            resourceVersion: editing.resourceVersion,
          }),
          {successMessage: "Role Binding updated"}
        );
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(RoleBindingBackend.deleteRoleBinding(record.namespace, record.name), {
      successMessage: "Role Binding deleted",
    });
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 160, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {
      key: "roleRefKind",
      title: "Role Ref Kind",
      dataIndex: "roleRefKind",
      width: 140,
      sortable: true,
      render: (value) => <Badge variant="info">{value}</Badge>,
    },
    {
      key: "roleRef",
      title: "Role Ref",
      dataIndex: "roleRef",
      width: 210,
      sortable: true,
      render: (value) => <Badge variant="danger">{value}</Badge>,
    },
    {
      key: "subjects",
      title: "Subjects",
      dataIndex: "subjects",
      render: (subjects) => <SubjectBadges subjects={subjects} />,
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
            title={`Delete Role Binding "${record.name}"?`}
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
      {error ? <MessageAlert title="Failed to fetch Role Bindings" description={error} /> : null}

      <DataTable
        scopeToWorkspace
        title={i18next.t("general:Role Bindings")}
        description={`${bindings?.length ?? 0} bindings`}
        columns={columns}
        dataSource={bindings}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No Role Bindings found"
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
        title={mode === "add" ? "Add Role Binding" : "Edit Role Binding"}
        description={mode === "edit" ? "Role Ref is immutable after creation — only subjects can be updated." : undefined}
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

        <Field label={i18next.t("general:Name")} htmlFor="rb-name" required error={errors.name}>
          <Input
            id="rb-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-role-binding"
            disabled={mode === "edit"}
          />
        </Field>

        <div className="grid grid-cols-[40%_minmax(0,1fr)] gap-3">
          <Field label="Role Ref Kind">
            <SimpleSelect
              value={form.roleRefKind}
              onChange={(next) => setForm((prev) => ({...prev, roleRefKind: next}))}
              options={ROLE_REF_KINDS}
              disabled={mode === "edit"}
            />
          </Field>
          <Field label="Role Ref Name" htmlFor="rb-roleref" required error={errors.roleRef}>
            <Input
              id="rb-roleref"
              value={form.roleRef}
              onChange={(event) => setForm((prev) => ({...prev, roleRef: event.target.value}))}
              placeholder="my-role"
              disabled={mode === "edit"}
            />
          </Field>
        </div>

        <Field label="Subjects">
          <SubjectsEditor value={form.subjects} onChange={(subjects) => setForm((prev) => ({...prev, subjects}))} />
        </Field>
      </FormDialog>
    </PageContainer>
  );
}

export default RoleBindingListPage;
