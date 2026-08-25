import React, {useMemo, useState} from "react";
import i18next from "i18next";
import {useTranslation} from "react-i18next";
import {Pencil, Play, Rocket, Square, Trash2} from "lucide-react";
import * as ImageBackend from "@/backend/ImageBackend";
import * as MetricsBackend from "@/backend/MetricsBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Checkbox} from "@/components/ui/checkbox";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {DataTable} from "@/components/shared/data-table";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {SimpleSelect} from "@/components/shared/simple-select";
import {StatusBadge} from "@/components/shared/status-badge";
import {AppIcon} from "@/components/shared/app-icon";
import {runAction, useResource} from "@/hooks/use-resource";

const POLL_INTERVAL = 15000;

const APP_STATUS_VARIANTS = {
  deployed: "success",
  pending: "warning",
  failed: "danger",
  stopped: "muted",
};

function formatCpu(millicores) {
  if (!millicores) {
    return "—";
  }
  return millicores >= 1000 ? `${(millicores / 1000).toFixed(2)} ${i18next.t("launchpad:cores")}` : `${Math.round(millicores)}m`;
}

function formatMemory(mebibytes) {
  if (!mebibytes) {
    return "—";
  }
  return mebibytes >= 1024 ? `${(mebibytes / 1024).toFixed(2)} GiB` : `${Math.round(mebibytes)} MiB`;
}

/**
 * The App Launchpad's list: every app deployed from a container image, what it
 * is doing, and what it is using right now.
 *
 * Usage is read per pod and summed per app rather than taken from the workload
 * spec, because what an app asked for and what it uses are different questions
 * and only the second one tells anybody whether it is sized right.
 */
function LaunchpadPage(props) {
  useTranslation();
  const {history} = props;
  const [namespace, setNamespace] = useState("all");
  const [deleteTarget, setDeleteTarget] = useState(null);
  const [deleteData, setDeleteData] = useState(false);

  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});
  const {data: apps, loading, refresh} = useResource(
    () => ImageBackend.getImageApps(namespace === "all" ? "" : namespace),
    [namespace],
    {initialData: [], pollInterval: POLL_INTERVAL}
  );
  const {data: metrics} = useResource(() => MetricsBackend.getMetrics(), [], {
    initialData: {nodes: [], pods: []},
    toastOnError: false,
    pollInterval: POLL_INTERVAL,
  });

  // A pod belongs to the app whose name it carries as a prefix; the workload
  // names every pod it owns after itself, which is what makes this exact
  // enough without a second request per app.
  const usageByApp = useMemo(() => {
    const usage = new Map();
    (metrics?.pods ?? []).forEach((pod) => {
      const key = `${pod.namespace}/${String(pod.name).replace(/-[a-z0-9]+-[a-z0-9]+$|-\d+$/, "")}`;
      const current = usage.get(key) ?? {cpu: 0, memory: 0};
      usage.set(key, {
        cpu: current.cpu + (pod.cpuM ?? 0),
        memory: current.memory + (pod.memMi ?? 0),
      });
    });
    return usage;
  }, [metrics]);

  function usageOf(app) {
    return usageByApp.get(`${app.namespace}/${app.name}`) ?? {cpu: 0, memory: 0};
  }

  function toggleRunning(app, running) {
    runAction(ImageBackend.scaleApp({namespace: app.namespace, name: app.name, running}), {
      successMessage: running ? i18next.t("launchpad:App started") : i18next.t("launchpad:App stopped"),
      onSuccess: () => refresh({silent: true}),
    });
  }

  function uninstall() {
    if (!deleteTarget) {
      return;
    }
    runAction(
      ImageBackend.uninstallApp({namespace: deleteTarget.namespace, name: deleteTarget.name, deleteData}),
      {
        successMessage: i18next.t("launchpad:App deleted"),
        onSuccess: () => {
          setDeleteTarget(null);
          setDeleteData(false);
          refresh({silent: true});
        },
      }
    );
  }

  const columns = [
    {
      key: "name",
      title: i18next.t("launchpad:App"),
      dataIndex: "name",
      minWidth: 200,
      sortable: true,
      render: (value, record) => (
        <div className="flex items-center gap-2">
          <AppIcon chartName={record.repository} name={value} size="sm" />
          <div className="min-w-0">
            <div className="truncate font-medium">{value}</div>
            <div className="text-muted-foreground truncate text-xs">{record.namespace}</div>
          </div>
        </div>
      ),
    },
    {
      key: "status",
      title: i18next.t("general:Status"),
      dataIndex: "status",
      width: 120,
      sortable: true,
      render: (value) => <StatusBadge status={value} variants={APP_STATUS_VARIANTS} />,
    },
    {
      key: "image",
      title: i18next.t("launchpad:Image"),
      dataIndex: "image",
      minWidth: 180,
      ellipsis: true,
      render: (value) => <span className="font-mono text-xs">{value}</span>,
    },
    {
      key: "replicas",
      title: i18next.t("launchpad:Copies"),
      width: 110,
      render: (_value, record) => (
        <span className="tabular-nums">{record.readyReplicas ?? 0} / {record.replicas ?? 0}</span>
      ),
    },
    {
      key: "cpu",
      title: i18next.t("dashboard:label CPU"),
      width: 120,
      render: (_value, record) => <span className="tabular-nums">{formatCpu(usageOf(record).cpu)}</span>,
    },
    {
      key: "memory",
      title: i18next.t("dashboard:label Memory"),
      width: 130,
      render: (_value, record) => <span className="tabular-nums">{formatMemory(usageOf(record).memory)}</span>,
    },
    {
      key: "createdAt",
      title: i18next.t("launchpad:Created"),
      dataIndex: "createdAt",
      width: 170,
      sortable: true,
    },
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 190,
      align: "right",
      render: (_value, record) => (
        <div className="flex items-center justify-end gap-0.5">
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={i18next.t("launchpad:Edit app")}
            title={i18next.t("launchpad:Edit app")}
            onClick={(event) => {
              event.stopPropagation();
              history.push(`/launchpad/${record.namespace}/${record.name}/edit`);
            }}
          >
            <Pencil />
          </Button>
          {record.status === "stopped" ? (
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={i18next.t("launchpad:Start")}
              title={i18next.t("launchpad:Start")}
              onClick={(event) => {
                event.stopPropagation();
                toggleRunning(record, true);
              }}
            >
              <Play />
            </Button>
          ) : (
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={i18next.t("launchpad:Stop")}
              title={i18next.t("launchpad:Stop")}
              onClick={(event) => {
                event.stopPropagation();
                toggleRunning(record, false);
              }}
            >
              <Square />
            </Button>
          )}
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={i18next.t("launchpad:Delete")}
            title={i18next.t("launchpad:Delete")}
            onClick={(event) => {
              event.stopPropagation();
              setDeleteTarget(record);
            }}
          >
            <Trash2 />
          </Button>
        </div>
      ),
    },
  ];

  return (
    <PageContainer>
      <PageHeader
        title={i18next.t("launchpad:App Launchpad")}
        description={i18next.t("launchpad:Run a container image as an application — sized, reachable and kept up.")}
        actions={
          <Button onClick={() => history.push("/launchpad/new")} data-testid="launchpad-create">
            <Rocket />
            {i18next.t("launchpad:Deploy app")}
          </Button>
        }
      />

      <DataTable
        scopeToWorkspace
        testId="launchpad-table"
        columns={columns}
        dataSource={apps}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        onRowClick={(record) => history.push(`/launchpad/${record.namespace}/${record.name}`)}
        emptyIcon={Rocket}
        emptyText={i18next.t("launchpad:No apps yet. Deploy one from an image, or install one from the App Store.")}
        toolbar={
          <div className="flex items-center gap-2">
            <SimpleSelect
              value={namespace}
              onChange={setNamespace}
              options={[
                {label: i18next.t("launchpad:All namespaces"), value: "all"},
                ...namespaces.map((item) => ({label: item.name, value: item.name})),
              ]}
              size="sm"
              className="w-52"
            />
            <Button variant="outline" size="sm" onClick={() => refresh()}>
              {i18next.t("general:Refresh")}
            </Button>
          </div>
        }
      />

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null);
            setDeleteData(false);
          }
        }}
        title={`${i18next.t("launchpad:Delete")} ${deleteTarget?.name ?? ""}`}
        description={i18next.t("launchpad:The app, its address and its autoscaler are removed. Its disks are kept unless you say otherwise.")}
        confirmText={i18next.t("launchpad:Delete")}
        onConfirm={uninstall}
        extra={
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={deleteData} onCheckedChange={(checked) => setDeleteData(Boolean(checked))} />
            {i18next.t("launchpad:Also delete its disks")}
            <Badge variant="danger">{i18next.t("launchpad:Cannot be undone")}</Badge>
          </label>
        }
      />
    </PageContainer>
  );
}

export default LaunchpadPage;
