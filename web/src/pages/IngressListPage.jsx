import React, {useCallback, useEffect, useState} from "react";
import i18next from "i18next";
import {Lock, LockOpen, Pencil, Plus, RefreshCw, Trash2, X} from "lucide-react";
import * as IngressBackend from "@/backend/IngressBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import * as CertificateBackend from "@/backend/CertificateBackend";
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
import {SearchSelect, SimpleSelect} from "@/components/shared/simple-select";
import {CertificateDialog} from "@/components/shared/certificate-dialog";

const PATH_TYPE_OPTIONS = [
  {label: "Prefix", value: "Prefix"},
  {label: "Exact", value: "Exact"},
  {label: "ImplementationSpecific", value: "ImplementationSpecific"},
];

const emptyRule = {host: "", path: "/", pathType: "Prefix", serviceName: "", servicePort: 80};
const emptyForm = {namespace: "", name: "", ingressClass: "", rules: [emptyRule]};

// The certificate state does not come back with the ingress list; it is a
// per-ingress request. The badge therefore reads from a map keyed by
// namespace/name that is filled in after the list lands.
function TlsBadge({record, cert}) {
  if (!record.tlsEnabled && !cert) {
    return (
      <Badge variant="muted">
        <LockOpen />
        HTTP
      </Badge>
    );
  }
  if (!cert || cert.status === "none") {
    return (
      <Badge variant="muted">
        <Lock />
        HTTPS (no cert)
      </Badge>
    );
  }
  if (cert.status === "pending" || cert.status === "verifying") {
    return (
      <Badge variant="info">
        <Lock />
        HTTPS (issuing…)
      </Badge>
    );
  }
  if (cert.status === "failed") {
    return (
      <SimpleTooltip title={cert.error}>
        <Badge variant="danger">
          <Lock />
          HTTPS (failed)
        </Badge>
      </SimpleTooltip>
    );
  }
  if (cert.status === "issued") {
    return (
      <SimpleTooltip title={`Valid until ${cert.expiry}`}>
        <Badge variant="success">
          <Lock />
          HTTPS · {cert.expiry}
        </Badge>
      </SimpleTooltip>
    );
  }
  return (
    <Badge variant="info">
      <Lock />
      HTTPS
    </Badge>
  );
}

function IngressListPage() {
  const {data: ingresses, loading, error, refresh} = useResource(() => IngressBackend.getIngresses(), [], {initialData: []});
  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});

  const [certStatuses, setCertStatuses] = useState({});
  const [certDialogIngress, setCertDialogIngress] = useState(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [mode, setMode] = useState("add");
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);

  const namespaceOptions = (namespaces ?? []).map((item) => ({label: item.name, value: item.name}));

  const loadCertStatuses = useCallback((list) => {
    if (!list || list.length === 0) {
      return;
    }
    Promise.all(
      list.map((ingress) =>
        CertificateBackend.getCertStatus(ingress.namespace, ingress.name)
          .then((res) =>
            res.status === "ok" && res.data?.status !== "none" ? [`${ingress.namespace}/${ingress.name}`, res.data] : null
          )
          .catch(() => null)
      )
    ).then((results) => {
      const updates = Object.fromEntries(results.filter(Boolean));
      setCertStatuses((previous) => ({...previous, ...updates}));
    });
  }, []);

  useEffect(() => {
    loadCertStatuses(ingresses);
  }, [ingresses, loadCertStatuses]);

  function openAdd() {
    setMode("add");
    setEditing(null);
    setForm({...emptyForm, namespace: namespaces?.[0]?.name ?? "default", rules: [{...emptyRule}]});
    setErrors({});
    setDialogOpen(true);
  }

  function openEdit(record) {
    setMode("edit");
    setEditing(record);
    setForm({
      namespace: record.namespace,
      name: record.name,
      ingressClass: record.ingressClass ?? "",
      rules: (record.rules ?? []).map((rule) => ({
        host: rule.host,
        path: rule.path,
        pathType: rule.pathType || "Prefix",
        serviceName: rule.serviceName,
        servicePort: rule.servicePort,
      })),
    });
    setErrors({});
    setDialogOpen(true);
  }

  function updateRule(index, field, next) {
    setForm((prev) => ({
      ...prev,
      rules: prev.rules.map((rule, ruleIndex) => (ruleIndex === index ? {...rule, [field]: next} : rule)),
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
    if (form.rules.some((rule) => !rule.path || !rule.serviceName || !rule.servicePort)) {
      nextErrors.rules = "Every rule needs a path, a service and a port";
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const payload = {
      name: form.name,
      namespace: form.namespace,
      ingressClass: form.ingressClass ?? "",
      rules: form.rules.map((rule) => ({
        host: rule.host ?? "",
        path: rule.path ?? "/",
        pathType: rule.pathType ?? "Prefix",
        serviceName: rule.serviceName ?? "",
        servicePort: Number(rule.servicePort) || 80,
      })),
    };

    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(IngressBackend.addIngress(payload), {successMessage: "Ingress created"})
        : await runAction(IngressBackend.updateIngress({...payload, resourceVersion: editing.resourceVersion}), {
          successMessage: "Ingress updated",
        });
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(IngressBackend.deleteIngress(record.namespace, record.name), {successMessage: "Ingress deleted"});
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 150, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {
      key: "ingressClass",
      title: "Ingress Class",
      dataIndex: "ingressClass",
      width: 150,
      render: (value) => value || <span className="text-muted-foreground">—</span>,
    },
    {
      key: "tls",
      title: "TLS",
      width: 190,
      render: (_, record) => <TlsBadge record={record} cert={certStatuses[`${record.namespace}/${record.name}`]} />,
    },
    {
      key: "rules",
      title: "Rules",
      dataIndex: "rules",
      render: (rules) => (
        <div className="grid gap-1">
          {(rules ?? []).map((rule, index) => (
            <Badge key={index} variant="muted" className="w-fit font-mono">
              {rule.host || "*"}
              {rule.path} → {rule.serviceName}:{rule.servicePort}
            </Badge>
          ))}
        </div>
      ),
    },
    {key: "createdAt", title: i18next.t("general:Created"), dataIndex: "createdAt", width: 180, sortable: true},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 230,
      align: "right",
      render: (_, record) => (
        <div className="flex justify-end gap-2">
          <Button variant="outline" size="sm" onClick={() => setCertDialogIngress(record)}>
            <Lock />
            HTTPS
          </Button>
          <Button variant="outline" size="sm" onClick={() => openEdit(record)}>
            <Pencil />
            {i18next.t("general:Edit")}
          </Button>
          <ConfirmDialog
            title={`Delete Ingress "${record.name}"?`}
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
      {error ? <MessageAlert title="Failed to fetch Ingresses" description={error} /> : null}

      <DataTable
        scopeToWorkspace
        title={i18next.t("general:Ingresses")}
        description={`${ingresses?.length ?? 0} ingresses`}
        columns={columns}
        dataSource={ingresses}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No Ingresses found"
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
        title={mode === "add" ? "Add Ingress" : "Edit Ingress"}
        submitText={mode === "add" ? "Create" : "Update"}
        submitting={submitting}
        onSubmit={handleSubmit}
        size="xl"
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

        <Field label={i18next.t("general:Name")} htmlFor="ing-name" required error={errors.name}>
          <Input
            id="ing-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-ingress"
            disabled={mode === "edit"}
          />
        </Field>

        <Field label="Ingress Class" htmlFor="ing-class">
          <Input
            id="ing-class"
            value={form.ingressClass}
            onChange={(event) => setForm((prev) => ({...prev, ingressClass: event.target.value}))}
            placeholder="nginx (optional)"
          />
        </Field>

        <Field label="Rules" error={errors.rules}>
          <div className="grid gap-3">
            {form.rules.map((rule, index) => (
              <div key={index} className="relative grid gap-3 rounded-lg border p-3 sm:grid-cols-2">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-xs"
                  onClick={() => setForm((prev) => ({...prev, rules: prev.rules.filter((_, i) => i !== index)}))}
                  className="text-muted-foreground hover:text-destructive absolute top-2 right-2"
                  aria-label="Remove rule"
                >
                  <X className="size-3.5" />
                </Button>

                <Field label="Host">
                  <Input
                    value={rule.host ?? ""}
                    onChange={(event) => updateRule(index, "host", event.target.value)}
                    placeholder="example.com (blank for *)"
                    className="h-8 text-xs"
                  />
                </Field>
                <Field label="Path">
                  <Input
                    value={rule.path ?? ""}
                    onChange={(event) => updateRule(index, "path", event.target.value)}
                    placeholder="/"
                    className="h-8 text-xs"
                  />
                </Field>
                <Field label="Path Type">
                  <SimpleSelect
                    value={rule.pathType}
                    onChange={(next) => updateRule(index, "pathType", next)}
                    options={PATH_TYPE_OPTIONS}
                    size="sm"
                  />
                </Field>
                <div className="grid grid-cols-[minmax(0,1fr)_100px] gap-2">
                  <Field label="Service">
                    <Input
                      value={rule.serviceName ?? ""}
                      onChange={(event) => updateRule(index, "serviceName", event.target.value)}
                      placeholder="my-service"
                      className="h-8 text-xs"
                    />
                  </Field>
                  <Field label="Port">
                    <Input
                      type="number"
                      value={rule.servicePort ?? ""}
                      onChange={(event) => updateRule(index, "servicePort", event.target.value)}
                      min={1}
                      max={65535}
                      className="h-8 text-xs"
                    />
                  </Field>
                </div>
              </div>
            ))}
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="border-dashed"
              onClick={() => setForm((prev) => ({...prev, rules: [...prev.rules, {...emptyRule}]}))}
            >
              <Plus />
              Add Rule
            </Button>
          </div>
        </Field>
      </FormDialog>

      <CertificateDialog
        ingress={certDialogIngress}
        open={certDialogIngress !== null}
        onClose={() => setCertDialogIngress(null)}
        onUpdated={() => refresh()}
      />
    </PageContainer>
  );
}

export default IngressListPage;
