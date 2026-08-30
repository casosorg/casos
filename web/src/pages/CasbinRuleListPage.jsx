import React, {useState} from "react";
import {useTranslation} from "react-i18next";
import i18next from "i18next";
import {Plus, RefreshCw, ShieldCheck, Trash2} from "lucide-react";
import * as CasbinRuleBackend from "@/backend/CasbinRuleBackend";
import * as Setting from "@/Setting";
import {runAction, useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {SearchSelect, SimpleSelect} from "@/components/shared/simple-select";
import {CodeText} from "@/components/shared/misc";
import {SimpleTooltip} from "@/components/ui/tooltip";

const RESOURCES = [
  "*",
  "pods",
  "deployments",
  "statefulsets",
  "services",
  "ingresses",
  "configmaps",
  "secrets",
  "persistentvolumeclaims",
  "nodes",
  "namespaces",
  "serviceaccounts",
  "clusterrolebindings",
].map((resource) => ({label: resource, value: resource}));

const ADMISSION_ACTIONS = ["*", "CREATE", "UPDATE", "DELETE", "CONNECT"];
const AUTHORIZATION_VERBS = ["*", "get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"];

const emptyForm = {pType: "p", v0: "", v1: "*", v2: "*", v3: "*", v4: "allow"};

function Dash() {
  return <span className="text-muted-foreground">—</span>;
}

/**
 * Casbin rules for one enforcer. Both the admission and the authorization
 * screens are this page with a different `scope`; the only things that differ
 * are the vocabulary for the action column and which enforcer gets reloaded.
 */
function CasbinRuleListPage({scope, title, description}) {
  useTranslation();

  const {data: rules, loading, refresh} = useResource(() => CasbinRuleBackend.getCasbinRules(scope), [scope], {initialData: []});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);

  const isPolicy = form.pType === "p";
  const actionOptions = (scope === "authorization" ? AUTHORIZATION_VERBS : ADMISSION_ACTIONS).map((action) => ({
    label: action,
    value: action,
  }));
  const actionLabel = scope === "authorization" ? i18next.t("policy:Verb") : i18next.t("general:Action");

  function openAdd() {
    setForm(emptyForm);
    setErrors({});
    setDialogOpen(true);
  }

  async function handleAdd() {
    const nextErrors = {};
    if (!form.v0) {
      nextErrors.v0 = i18next.t("general:required");
    }
    if (!isPolicy && !form.v1) {
      nextErrors.v1 = i18next.t("general:required");
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    setSubmitting(true);
    const ok = await runAction(
      CasbinRuleBackend.addCasbinRule({
        scope,
        pType: form.pType,
        v0: form.v0,
        v1: form.v1 || "",
        v2: isPolicy ? form.v2 || "" : "",
        v3: isPolicy ? form.v3 || "" : "",
        // The effect column only exists on policy rules; a grouping rule leaves
        // it blank rather than defaulting to allow.
        v4: isPolicy ? form.v4 || "allow" : "",
      }),
      {successMessage: i18next.t("policy:Rule added")}
    );
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(id) {
    const ok = await runAction(CasbinRuleBackend.deleteCasbinRule(id, scope), {
      successMessage: i18next.t("policy:Rule deleted"),
    });
    if (ok) {
      refresh();
    }
  }

  function handleReload() {
    CasbinRuleBackend.reloadCasbinEnforcer(scope).then((res) => {
      if (res.status === "ok") {
        Setting.showMessage("success", i18next.t("policy:Enforcer reloaded"));
      } else {
        Setting.showMessage("error", res.msg);
      }
    });
  }

  const columns = [
    {
      key: "pType",
      title: i18next.t("general:Type"),
      dataIndex: "pType",
      width: 100,
      render: (value) =>
        value === "g" ? (
          <Badge variant="info">{i18next.t("policy:g role")}</Badge>
        ) : (
          <Badge variant="success">{i18next.t("policy:p policy")}</Badge>
        ),
    },
    {
      key: "v0",
      title: i18next.t("policy:Subject column"),
      dataIndex: "v0",
      render: (value) => <CodeText>{value}</CodeText>,
    },
    {
      key: "v1",
      title: i18next.t("policy:Namespace column"),
      dataIndex: "v1",
      render: (value) => (value ? <CodeText>{value}</CodeText> : <Dash />),
    },
    {
      key: "v2",
      title: i18next.t("policy:Resource"),
      dataIndex: "v2",
      render: (value) => (value ? <CodeText>{value}</CodeText> : <Dash />),
    },
    {
      key: "v3",
      title: actionLabel,
      dataIndex: "v3",
      render: (value) => (value ? <Badge variant="muted">{value}</Badge> : <Dash />),
    },
    {
      key: "v4",
      title: i18next.t("policy:Effect"),
      dataIndex: "v4",
      width: 100,
      render: (value, record) =>
        record.pType === "p" ? (
          <Badge variant={value === "deny" ? "danger" : "success"}>
            {value === "deny" ? i18next.t("policy:deny") : i18next.t("policy:allow")}
          </Badge>
        ) : (
          <Dash />
        ),
    },
    {
      key: "actions",
      title: i18next.t("policy:Op column"),
      width: 80,
      align: "right",
      render: (_, record) => (
        <ConfirmDialog title={i18next.t("policy:Delete this rule?")} confirmText="Delete" onConfirm={() => handleDelete(record.id)}>
          <Button variant="ghost" size="icon-sm" className="text-muted-foreground hover:text-destructive">
            <Trash2 className="size-4" />
          </Button>
        </ConfirmDialog>
      ),
    },
  ];

  return (
    <PageContainer>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-2">
          <ShieldCheck className="text-info size-5" />
          <h1 className="text-lg font-semibold">{title}</h1>
        </div>
        <div className="flex items-center gap-2">
          <SimpleTooltip title={i18next.t("policy:Reload Enforcer tooltip")}>
            <Button variant="outline" size="sm" onClick={handleReload}>
              <RefreshCw />
              {i18next.t("policy:Reload Enforcer")}
            </Button>
          </SimpleTooltip>
          <Button size="sm" onClick={openAdd}>
            <Plus />
            {i18next.t("policy:Add Rule")}
          </Button>
        </div>
      </div>

      {description ? (
        <div className="border-info/20 bg-info/5 text-muted-foreground rounded-lg border px-3 py-2 text-xs">{description}</div>
      ) : null}

      <DataTable columns={columns} dataSource={rules} rowKey="id" loading={loading} searchable emptyText="No rules yet" />

      <FormDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={i18next.t("policy:Add Rule")}
        submitText={i18next.t("policy:Add Rule")}
        submitting={submitting}
        onSubmit={handleAdd}
      >
        <Field label={i18next.t("general:Type")} required>
          <SimpleSelect
            value={form.pType}
            onChange={(next) => setForm((prev) => ({...prev, pType: next}))}
            options={[
              {label: i18next.t("policy:p policy"), value: "p"},
              {label: i18next.t("policy:g role"), value: "g"},
            ]}
          />
        </Field>

        <Field
          label={isPolicy ? i18next.t("policy:Subject") : i18next.t("policy:User Group")}
          htmlFor="casbin-v0"
          required
          error={errors.v0}
        >
          <Input
            id="casbin-v0"
            value={form.v0}
            onChange={(event) => setForm((prev) => ({...prev, v0: event.target.value}))}
            placeholder={isPolicy ? i18next.t("policy:subject placeholder") : i18next.t("policy:user placeholder")}
          />
        </Field>

        {isPolicy ? (
          <>
            <Field label={i18next.t("general:Namespace")} htmlFor="casbin-v1">
              <Input
                id="casbin-v1"
                value={form.v1}
                onChange={(event) => setForm((prev) => ({...prev, v1: event.target.value}))}
                placeholder={i18next.t("policy:namespace placeholder")}
              />
            </Field>

            <Field label={i18next.t("policy:Resource")}>
              <SearchSelect
                value={form.v2}
                onChange={(next) => setForm((prev) => ({...prev, v2: next}))}
                options={RESOURCES}
                placeholder={i18next.t("policy:resource placeholder")}
                allowClear
              />
            </Field>

            <Field label={actionLabel}>
              <SimpleSelect
                value={form.v3}
                onChange={(next) => setForm((prev) => ({...prev, v3: next}))}
                options={actionOptions}
                placeholder={i18next.t("policy:action placeholder")}
              />
            </Field>

            <Field label={i18next.t("policy:Effect")} required>
              <SimpleSelect
                value={form.v4}
                onChange={(next) => setForm((prev) => ({...prev, v4: next}))}
                options={[
                  {label: i18next.t("policy:allow"), value: "allow"},
                  {label: i18next.t("policy:deny"), value: "deny"},
                ]}
              />
            </Field>
          </>
        ) : (
          <Field label={i18next.t("policy:Role")} htmlFor="casbin-role" required error={errors.v1}>
            <Input
              id="casbin-role"
              value={form.v1}
              onChange={(event) => setForm((prev) => ({...prev, v1: event.target.value}))}
              placeholder={i18next.t("policy:role placeholder")}
            />
          </Field>
        )}
      </FormDialog>
    </PageContainer>
  );
}

export default CasbinRuleListPage;
