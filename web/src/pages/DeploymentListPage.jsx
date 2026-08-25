import React, {useEffect, useRef, useState} from "react";
import {useHistory, useLocation} from "react-router-dom";
import i18next from "i18next";
import {HardDrive, Link2, Pencil, Plus, RefreshCcwDot, RefreshCw, RotateCw, Share2, Trash2} from "lucide-react";
import * as DeploymentBackend from "@/backend/DeploymentBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import * as ConfigMapBackend from "@/backend/ConfigMapBackend";
import * as SecretBackend from "@/backend/SecretBackend";
import * as ServiceBackend from "@/backend/ServiceBackend";
import * as NodeBackend from "@/backend/NodeBackend";
import * as MetricsBackend from "@/backend/MetricsBackend";
import * as IngressBackend from "@/backend/IngressBackend";
import * as Setting from "@/Setting";
import {runAction, useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {Separator} from "@/components/ui/separator";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {SearchSelect} from "@/components/shared/simple-select";
import {NumberInput} from "@/components/shared/number-input";
import {ReplicasControl} from "@/components/shared/replicas-control";
import {EnvVarEditor, envVarsToRows, rowsToEnvVars} from "@/components/shared/env-var-editor";
import {DeploymentStorageEditor} from "@/components/shared/deployment-storage-editor";
import {
  ResourceEditor,
  emptyResources,
  resourcesFromRecord,
  resourcesToPayload,
  validateResources,
  workloadUsage,
} from "@/components/shared/resource-editor";
import {
  DeploymentDomainDialog,
  DeploymentExposeDialog,
  DeploymentUpdateImageDialog,
} from "@/components/shared/deployment-dialogs";

const emptyForm = {
  namespace: "",
  name: "",
  image: "",
  replicas: 1,
  containerName: "",
  ...emptyResources,
  envVars: [],
  volumes: [],
};

function splitImage(image) {
  if (!image) {
    return null;
  }
  const colonIndex = image.lastIndexOf(":");
  return {
    repo: colonIndex > 0 ? image.slice(0, colonIndex) : image,
    tag: colonIndex > 0 ? image.slice(colonIndex + 1) : "latest",
  };
}

/**
 * Every way a deployment can be reached from outside: through a NodePort or
 * LoadBalancer Service in front of it, and through any Ingress rule that routes
 * to one of its Services.
 */
function accessUrls(deploy, services, ingresses, nodeIP) {
  const urls = [];

  const service = services.find(
    (item) =>
      item.name === deploy.name &&
      item.namespace === deploy.namespace &&
      (item.type === "NodePort" || item.type === "LoadBalancer")
  );

  if (service) {
    const isLoadBalancer = service.type === "LoadBalancer";
    const addresses = isLoadBalancer ? service.loadBalancerAddresses ?? [] : nodeIP ? [nodeIP] : [];
    const ports = (service.ports ?? []).filter((port) => (isLoadBalancer ? port.port : port.nodePort));
    addresses.forEach((address) => {
      ports.forEach((port) => {
        const exposed = isLoadBalancer ? port.port : port.nodePort;
        const host = address.includes(":") ? `[${address}]` : address;
        const portName = String(port.name ?? "").toLowerCase();
        const scheme = port.port === 443 || portName.includes("https") || portName.includes("websecure") ? "https" : "http";
        urls.push({url: `${scheme}://${host}:${exposed}`, type: isLoadBalancer ? "loadbalancer" : "nodeport"});
      });
    });
  }

  const serviceNames = new Set(
    (services ?? [])
      .filter((item) => item.namespace === deploy.namespace && item.selector?.app === deploy.name)
      .map((item) => item.name)
  );
  serviceNames.add(deploy.name);

  (ingresses ?? [])
    .filter((ingress) => ingress.namespace === deploy.namespace)
    .forEach((ingress) => {
      (ingress.rules ?? []).forEach((rule) => {
        if (!serviceNames.has(rule.serviceName)) {
          return;
        }
        const path = rule.path && rule.path !== "/" ? rule.path : "";
        const scheme = ingress.tlsEnabled ? "https" : "http";
        if (rule.host) {
          urls.push({url: `${scheme}://${rule.host}${path}`, type: "domain"});
          return;
        }
        (ingress.loadBalancerAddresses ?? []).forEach((address) => {
          const host = address.includes(":") ? `[${address}]` : address;
          urls.push({url: `${scheme}://${host}${path}`, type: "ingress"});
        });
      });
    });

  return urls;
}

function DeploymentListPage() {
  const history = useHistory();
  const location = useLocation();

  const {
    data: deployments,
    setData: setDeployments,
    loading,
    error,
    refresh,
  } = useResource(() => DeploymentBackend.getDeployments(), [], {initialData: []});
  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});
  const {data: services, refresh: refreshServices} = useResource(() => ServiceBackend.getServices(), [], {
    initialData: [],
    toastOnError: false,
  });
  const {data: ingresses, refresh: refreshIngresses} = useResource(() => IngressBackend.getIngresses(), [], {
    initialData: [],
    toastOnError: false,
  });
  const {data: nodes} = useResource(() => NodeBackend.getNodes(), [], {initialData: [], toastOnError: false});
  const {data: metrics} = useResource(() => MetricsBackend.getMetrics(), [], {initialData: null, toastOnError: false});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [mode, setMode] = useState("add");
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);
  const [configMaps, setConfigMaps] = useState([]);
  const [secrets, setSecrets] = useState([]);

  const [exposeDeploy, setExposeDeploy] = useState(null);
  const [domainDeploy, setDomainDeploy] = useState(null);
  const [updateImageDeploy, setUpdateImageDeploy] = useState(null);

  const podMetrics = metrics?.pods ?? [];
  const metricsAvailable = podMetrics.length > 0;
  const nodeIP =
    (nodes ?? []).find((node) => node.externalIP)?.externalIP ?? (nodes ?? []).find((node) => node.internalIP)?.internalIP ?? null;
  const namespaceOptions = (namespaces ?? []).map((item) => ({label: item.name, value: item.name}));

  useEffect(() => {
    if (!dialogOpen || !form.namespace) {
      return;
    }
    let cancelled = false;
    ConfigMapBackend.getConfigMaps(form.namespace)
      .then((res) => {
        if (!cancelled && res.status === "ok") {
          setConfigMaps(res.data ?? []);
        }
      })
      .catch(() => {});
    SecretBackend.getSecrets(form.namespace)
      .then((res) => {
        if (!cancelled && res.status === "ok") {
          setSecrets(res.data ?? []);
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [dialogOpen, form.namespace]);

  function openEdit(record) {
    setMode("edit");
    setEditing(record);
    setForm({
      namespace: record.namespace,
      name: record.name,
      image: record.image,
      replicas: record.replicas,
      containerName: "",
      ...resourcesFromRecord(record),
      envVars: envVarsToRows(record.envVars),
      volumes: record.volumes ?? [],
    });
    setErrors({});
    setDialogOpen(true);
  }

  // The pod list links here as /deployments?namespace=&name= to open one
  // deployment's editor directly. Handled once per mount, and the query is
  // dropped so a refresh does not reopen the dialog.
  const queryHandledRef = useRef(false);
  useEffect(() => {
    if (queryHandledRef.current || !deployments || deployments.length === 0) {
      return;
    }
    const params = new URLSearchParams(location.search ?? "");
    const namespace = params.get("namespace");
    const name = params.get("name");
    if (!namespace || !name) {
      return;
    }
    queryHandledRef.current = true;
    history.replace("/deployments");

    const match = deployments.find((item) => item.namespace === namespace && item.name === name);
    if (match) {
      openEdit(match);
    } else {
      Setting.showMessage("error", `Deployment "${namespace}/${name}" not found`);
    }
    // openEdit is stable for this purpose; re-running on it would reopen the
    // dialog every time the list refreshes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deployments, location.search, history]);

  function openAdd() {
    setMode("add");
    setEditing(null);
    setForm({...emptyForm, namespace: namespaces?.[0]?.name ?? "default"});
    setErrors({});
    setDialogOpen(true);
  }

  function handleScale(record, next) {
    return DeploymentBackend.updateDeployment({...record, replicas: next})
      .then((res) => {
        if (res.status === "ok") {
          setDeployments((previous) =>
            previous.map((item) => (item.namespace === record.namespace && item.name === record.name ? res.data : item))
          );
        } else {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch((e) => Setting.showMessage("error", e.message));
  }

  async function handleSubmit() {
    const nextErrors = {};
    if (!form.namespace) {
      nextErrors.namespace = "Namespace is required";
    }
    if (!form.name) {
      nextErrors.name = "Name is required";
    }
    if (!form.image) {
      nextErrors.image = "Image is required";
    }
    Object.assign(nextErrors, validateResources(form));
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    const payload = {
      namespace: form.namespace,
      name: form.name,
      // An emptied box means "not set", not zero: sent as null the API keeps the
      // current count on edit and falls back to 1 on create, while an explicit 0
      // is sent as 0 and scales the workload down.
      replicas: form.replicas === "" || form.replicas === null ? null : Number(form.replicas),
      image: form.image,
      containerName: form.containerName ?? "",
      ...resourcesToPayload(form),
      envVars: rowsToEnvVars(form.envVars),
      // Volumes are only sent on create; the API rejects mount changes on an
      // existing deployment, and the editor is read-only there for that reason.
      volumes: mode === "add" ? (form.volumes ?? []).filter((volume) => volume.mountPath) : [],
    };

    setSubmitting(true);
    const ok =
      mode === "add"
        ? await runAction(DeploymentBackend.addDeployment(payload), {successMessage: "Deployment created"})
        : await runAction(DeploymentBackend.updateDeployment({...payload, resourceVersion: editing.resourceVersion}), {
          successMessage: "Deployment updated",
        });
    setSubmitting(false);

    if (ok) {
      setDialogOpen(false);
      refresh();
    }
  }

  // A rolling restart is the usual way to get a workload to pick up a changed
  // ConfigMap, Secret or mutable image tag. The ConfigMap and Secret pages offer
  // it right after an edit; this is the same action for a single deployment.
  async function handleRestart(record) {
    const ok = await runAction(DeploymentBackend.restartDeployment(record.namespace, record.name), {
      successMessage: "Rolling restart started",
    });
    if (ok) {
      refresh();
    }
  }

  async function handleDelete(record) {
    const ok = await runAction(DeploymentBackend.deleteDeployment(record.namespace, record.name), {
      successMessage: "Deployment deleted",
    });
    if (ok) {
      refresh();
    }
  }

  const columns = [
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 160, sortable: true},
    {key: "name", title: i18next.t("general:Name"), dataIndex: "name", sortable: true, className: "font-medium"},
    {
      key: "image",
      title: i18next.t("general:Image"),
      render: (_, record) => {
        const image = splitImage(record.image);
        if (!image) {
          return null;
        }
        return (
          <SimpleTooltip title={record.image}>
            <span className="flex items-center gap-1.5">
              <span className="text-muted-foreground truncate text-xs">{image.repo}</span>
              <Badge variant="info">{image.tag}</Badge>
            </span>
          </SimpleTooltip>
        );
      },
    },
    {
      key: "storage",
      title: "Storage",
      width: 210,
      render: (_, record) => {
        const volumes = record.volumes ?? [];
        if (volumes.length === 0) {
          return <span className="text-muted-foreground text-xs">—</span>;
        }
        return (
          <div className="flex flex-wrap gap-1">
            {volumes.map((volume, index) => (
              <SimpleTooltip key={index} title={`PVC: ${volume.claimName}`}>
                <Badge variant="info">
                  <HardDrive />
                  {volume.mountPath}
                </Badge>
              </SimpleTooltip>
            ))}
          </div>
        );
      },
    },
    {
      key: "replicas",
      title: "Replicas",
      width: 220,
      align: "right",
      render: (_, record) => (
        <ReplicasControl
          readyReplicas={record.readyReplicas ?? 0}
          replicas={record.replicas ?? 0}
          onScale={(next) => handleScale(record, next)}
        />
      ),
    },
    {
      key: "metrics",
      title: "CPU / Memory",
      width: 170,
      align: "right",
      render: (_, record) => {
        if (!metricsAvailable) {
          return <span className="text-muted-foreground text-xs">—</span>;
        }
        const totals = workloadUsage(podMetrics, record.namespace, record.name);
        const memory = totals.memMi >= 1024 ? `${(totals.memMi / 1024).toFixed(1)}G` : `${totals.memMi}M`;
        const budget = (
          <span className="grid gap-0.5">
            <span>{`Requests: ${record.cpuRequest || "—"} CPU, ${record.memoryRequest || "—"} memory`}</span>
            <span>{`Limits: ${record.cpuLimit || "—"} CPU, ${record.memoryLimit || "—"} memory`}</span>
          </span>
        );
        return (
          <SimpleTooltip title={budget}>
            <span className="text-muted-foreground font-mono text-xs">
              {(totals.cpuM / 1000).toFixed(3)}c {memory}
            </span>
          </SimpleTooltip>
        );
      },
    },
    {
      key: "accessUrl",
      title: "Access URL",
      render: (_, record) => {
        const urls = accessUrls(record, services ?? [], ingresses ?? [], nodeIP);
        if (urls.length === 0) {
          return null;
        }
        return (
          <div className="grid gap-0.5">
            {urls.map(({url, type}) => (
              <a
                key={url}
                href={url}
                target="_blank"
                rel="noopener noreferrer"
                className="text-info flex items-center gap-1 text-xs hover:underline"
              >
                {type === "domain" ? <Link2 className="size-3 shrink-0" /> : null}
                {url}
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
      width: 280,
      align: "right",
      render: (_, record) => (
        <div className="flex justify-end gap-1">
          <SimpleTooltip title={i18next.t("general:Edit")}>
            <Button variant="outline" size="icon-sm" onClick={() => openEdit(record)} aria-label="Edit">
              <Pencil className="size-4" />
            </Button>
          </SimpleTooltip>
          <ConfirmDialog
            title={`Restart Deployment "${record.name}"?`}
            description={`Rolls the pods in ${record.namespace} one at a time, so the deployment keeps serving through the restart.`}
            confirmText="Restart"
            variant="default"
            onConfirm={() => handleRestart(record)}
          >
            <Button variant="outline" size="icon-sm" aria-label="Restart">
              <RotateCw className="size-4" />
            </Button>
          </ConfirmDialog>
          <SimpleTooltip title="Update image">
            <Button variant="outline" size="icon-sm" onClick={() => setUpdateImageDeploy(record)} aria-label="Update image">
              <RefreshCcwDot className="size-4" />
            </Button>
          </SimpleTooltip>
          <SimpleTooltip title="Expose as a Service">
            <Button variant="outline" size="icon-sm" onClick={() => setExposeDeploy(record)} aria-label="Expose">
              <Share2 className="size-4" />
            </Button>
          </SimpleTooltip>
          <SimpleTooltip title="Bind a domain">
            <Button variant="outline" size="icon-sm" onClick={() => setDomainDeploy(record)} aria-label="Bind domain">
              <Link2 className="size-4" />
            </Button>
          </SimpleTooltip>
          <ConfirmDialog
            title={`Delete Deployment "${record.name}"?`}
            description={`In namespace ${record.namespace}.`}
            confirmText="Delete"
            onConfirm={() => handleDelete(record)}
          >
            <Button variant="outline" size="icon-sm" className="text-destructive" aria-label="Delete">
              <Trash2 className="size-4" />
            </Button>
          </ConfirmDialog>
        </div>
      ),
    },
  ];

  return (
    <PageContainer>
      {error ? <MessageAlert title="Failed to fetch Deployments" description={error} /> : null}

      <DataTable
        scopeToWorkspace
        title={i18next.t("general:Deployments")}
        description={`${deployments?.length ?? 0} deployments`}
        columns={columns}
        dataSource={deployments}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyText="No Deployments found"
        toolbar={
          <>
            <Button
              variant="outline"
              size="sm"
              loading={loading}
              onClick={() => {
                refresh();
                refreshServices();
                refreshIngresses();
              }}
            >
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

      <DeploymentUpdateImageDialog
        deploy={updateImageDeploy}
        open={updateImageDeploy !== null}
        onClose={() => setUpdateImageDeploy(null)}
        onUpdated={() => refresh()}
      />

      <DeploymentExposeDialog
        deploy={exposeDeploy}
        open={exposeDeploy !== null}
        onClose={() => {
          setExposeDeploy(null);
          refreshServices();
        }}
      />

      <DeploymentDomainDialog
        deploy={domainDeploy}
        services={services ?? []}
        open={domainDeploy !== null}
        onClose={() => setDomainDeploy(null)}
        onCreated={() => {
          refreshIngresses();
          refreshServices();
        }}
      />

      <FormDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={mode === "add" ? "Add Deployment" : "Edit Deployment"}
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

        <Field label={i18next.t("general:Name")} htmlFor="deploy-name" required error={errors.name}>
          <Input
            id="deploy-name"
            value={form.name}
            onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
            placeholder="my-deployment"
            disabled={mode === "edit"}
          />
        </Field>

        <Field label={i18next.t("general:Image")} htmlFor="deploy-image" required error={errors.image}>
          <Input
            id="deploy-image"
            value={form.image}
            onChange={(event) => setForm((prev) => ({...prev, image: event.target.value}))}
            placeholder="nginx:latest"
          />
        </Field>

        <Field label="Replicas" required>
          <NumberInput value={form.replicas} onChange={(next) => setForm((prev) => ({...prev, replicas: next}))} min={0} />
        </Field>

        {mode === "add" ? (
          <Field label="Container Name" htmlFor="deploy-container" hint="Leave empty to use the deployment name.">
            <Input
              id="deploy-container"
              value={form.containerName}
              onChange={(event) => setForm((prev) => ({...prev, containerName: event.target.value}))}
            />
          </Field>
        ) : null}

        <Separator />

        <Field label="CPU & Memory">
          <ResourceEditor
            value={form}
            onChange={(next) => setForm((prev) => ({...prev, ...next}))}
            errors={errors}
            usage={editing && metricsAvailable ? workloadUsage(podMetrics, editing.namespace, editing.name) : null}
          />
        </Field>

        <Separator />

        <Field label="Environment Variables">
          <EnvVarEditor
            value={form.envVars}
            onChange={(envVars) => setForm((prev) => ({...prev, envVars}))}
            configMaps={configMaps}
            secrets={secrets}
          />
        </Field>

        <Separator />

        <Field label="Persistent Storage">
          <DeploymentStorageEditor
            mode={mode}
            value={form.volumes}
            onChange={(volumes) => setForm((prev) => ({...prev, volumes}))}
          />
        </Field>
      </FormDialog>
    </PageContainer>
  );
}

export default DeploymentListPage;
