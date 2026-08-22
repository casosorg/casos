import React, {useState} from "react";
import i18next from "i18next";
import {ExternalLink, Pencil, Plus, RefreshCw, Trash2, X} from "lucide-react";
import * as ServiceBackend from "@/backend/ServiceBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import * as NodeBackend from "@/backend/NodeBackend";
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
import {clusterNodeAddress, serviceAccessUrls} from "@/lib/appAccess";

const SERVICE_TYPES = ["ClusterIP", "NodePort", "LoadBalancer", "ExternalName"].map((type) => ({label: type, value: type}));
const PROTOCOLS = ["TCP", "UDP", "SCTP"].map((protocol) => ({label: protocol, value: protocol}));

const typeVariant = {
  ClusterIP: "info",
  NodePort: "success",
  LoadBalancer: "secondary",
  ExternalName: "warning",
};

const emptyForm = {
  namespace: "",
  name: "",
  type: "ClusterIP",
  selectorEntries: [],
  ports: [{name: "", protocol: "TCP", port: 80, targetPort: "80"}],
};

function portsToRows(ports) {
  return (ports ?? []).map((port) => ({
    name: port.name,
    protocol: port.protocol || "TCP",
    port: port.port,
    targetPort: port.targetPort,
    nodePort: port.nodePort || "",
  }));
}

function rowsToRequest(rows) {
  return (rows ?? []).map((row) => ({
    name: row.name ?? "",
    protocol: row.protocol || "TCP",
    port: Number(row.port),
    targetPort: String(row.targetPort ?? row.port ?? ""),
    nodePort: row.nodePort ? Number(row.nodePort) : 0,
  }));
}

function ServiceListPage() {
  const {data: services, loading, error, refresh} = useResource(() => ServiceBackend.getServices(), [], {initialData: []});
  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});
  const {data: nodes} = useResource(() => NodeBackend.getNodes(), [], {initialData: [], toastOnError: false});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [mode, setMode] = useState("add");
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);

  const namespaceOptions = (namespaces ?? []).map((item) => ({label: item.name, value: item.name}));
  const nodeIP = clusterNodeAddress(nodes);

  function openAdd() {
    setMode("add");
    setEditing(null);
    setForm({...emptyForm, namespace: namespaces?.[0]?.name ?? "default", ports: [{name: "", protocol: "TCP", port: 80, targetPort: "80"}]});
    setErrors({});
    setDialogOpen(true);
  }

  function openEdit(record) {
    setMode("edit");
    setEditing(record);
    setForm({
      namespace: record.namespace,
      name: record.name,
      type: record.type,
      selectorEntries: toEntries(record.selector),
      ports: portsToRows(record.ports),
    });
    setErrors({});
    setDialogOpen(true);
  }

  function updatePort(index, field, next) {
    setForm((prev) => ({
      ...prev,
      ports: prev.ports.map((port, portIndex) => (portIndex === index ? {...port, [field]: next} : port)),
    }));
  }

  async function handleSubmit() {
    const nextErrors = {};
    if (!form.namespace) {
      nextErrors.namespace = "Required";
    }
    if (!form.name) {
      nextErrors.name = "Required";
    }
    if (form.ports.some((port) => !port.port)) {
      nextErrors.ports = "Every port row needs a port number";
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const payload = {
      namespace: form.namespace,
      name: form.name,
      type: form.type,
      selector: fromEntries(form.selectorEntries),
      ports: rowsToRequest(form.ports),
    };

    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(ServiceBackend.addService(payload), {successMessage: "Service created"})
        : await runAction(ServiceBackend.updateService({...payload, resourceVersion: editing.resourceVersion}), {
          successMessage: "Service updated",
        });
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(ServiceBackend.deleteService(record.namespace, record.name), {successMessage: "Service deleted"});
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 150, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {
      key: "type",
      title: i18next.t("general:Type"),
      dataIndex: "type",
      width: 130,
      sortable: true,
      render: (value) => <Badge variant={typeVariant[value] ?? "muted"}>{value}</Badge>,
    },
    {key: "clusterIP", title: "Cluster IP", dataIndex: "clusterIP", width: 140, className: "font-mono text-xs"},
    {
      key: "ports",
      title: "Ports",
      dataIndex: "ports",
      render: (ports) => (
        <div className="flex flex-wrap gap-1">
          {(ports ?? []).map((port, index) => (
            <Badge key={index} variant="muted" className="font-mono">
              {port.protocol} {port.port}
              {port.targetPort !== String(port.port) ? `:${port.targetPort}` : ""}
              {port.nodePort ? `:${port.nodePort}` : ""}
            </Badge>
          ))}
        </div>
      ),
    },
    {
      key: "accessUrl",
      title: "Access URL",
      render: (_, record) => {
        const urls = serviceAccessUrls(record, nodeIP);
        if (urls.length === 0) {
          return null;
        }
        return (
          <div className="grid gap-0.5">
            {urls.map((url) => (
              <a
                key={url}
                href={url}
                target="_blank"
                rel="noopener noreferrer"
                className="text-info inline-flex items-center gap-1 text-xs hover:underline"
              >
                {url}
                <ExternalLink className="size-3" />
              </a>
            ))}
          </div>
        );
      },
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
            title={`Delete Service "${record.name}"?`}
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
      {error ? <MessageAlert title="Failed to fetch Services" description={error} /> : null}

      <DataTable
        testId="services-table"
        title={i18next.t("general:Services")}
        description={`${services?.length ?? 0} services`}
        columns={columns}
        dataSource={services}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No Services found"
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
        title={mode === "add" ? "Add Service" : "Edit Service"}
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

        <Field label={i18next.t("general:Name")} htmlFor="svc-name" required error={errors.name}>
          <Input
            id="svc-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-service"
            disabled={mode === "edit"}
          />
        </Field>

        <Field label={i18next.t("general:Type")} required>
          <SimpleSelect
            value={form.type}
            onChange={(next) => setForm((prev) => ({...prev, type: next}))}
            options={SERVICE_TYPES}
          />
        </Field>

        {form.type === "LoadBalancer" ? (
          <MessageAlert
            variant="warning"
            title="LoadBalancer ports must be free on worker nodes"
            description="Traefik reserves host ports 80 and 443. Another LoadBalancer Service using the same host port cannot be scheduled on those workers; use ClusterIP behind Ingress for shared HTTP or HTTPS access."
          />
        ) : null}

        <Field label="Ports" error={errors.ports}>
          <div className="grid gap-2">
            {form.ports.map((port, index) => (
              <div key={index} className="flex flex-wrap items-center gap-2">
                <SimpleSelect
                  value={port.protocol}
                  onChange={(next) => updatePort(index, "protocol", next)}
                  options={PROTOCOLS}
                  size="sm"
                  className="w-24"
                />
                <Input
                  type="number"
                  value={port.port ?? ""}
                  onChange={(event) => updatePort(index, "port", event.target.value)}
                  placeholder="port"
                  min={1}
                  max={65535}
                  className="h-8 w-24 text-xs"
                />
                <Input
                  value={port.targetPort ?? ""}
                  onChange={(event) => updatePort(index, "targetPort", event.target.value)}
                  placeholder="targetPort"
                  className="h-8 w-28 text-xs"
                />
                {form.type === "NodePort" ? (
                  <Input
                    type="number"
                    value={port.nodePort ?? ""}
                    onChange={(event) => updatePort(index, "nodePort", event.target.value)}
                    placeholder="nodePort"
                    min={30000}
                    max={32767}
                    className="h-8 w-28 text-xs"
                  />
                ) : null}
                <Input
                  value={port.name ?? ""}
                  onChange={(event) => updatePort(index, "name", event.target.value)}
                  placeholder="name (opt)"
                  className="h-8 w-28 text-xs"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => setForm((prev) => ({...prev, ports: prev.ports.filter((_, i) => i !== index)}))}
                  className="text-muted-foreground hover:text-destructive"
                  aria-label="Remove port"
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
              onClick={() =>
                setForm((prev) => ({...prev, ports: [...prev.ports, {name: "", protocol: "TCP", port: 80, targetPort: "80"}]}))
              }
            >
              <Plus />
              Add Port
            </Button>
          </div>
        </Field>

        <Field label="Selector" hint="Labels the Service matches to find its backing pods.">
          <KeyValueEditor
            value={form.selectorEntries}
            onChange={(selectorEntries) => setForm((prev) => ({...prev, selectorEntries}))}
            addLabel="Add Selector"
          />
        </Field>
      </FormDialog>
    </PageContainer>
  );
}

export default ServiceListPage;
