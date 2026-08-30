import React, {useState} from "react";
import i18next from "i18next";
import {useTranslation} from "react-i18next";
import {Database, Pencil, Play, Square, Trash2} from "lucide-react";
import * as DatabaseBackend from "@/backend/DatabaseBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Checkbox} from "@/components/ui/checkbox";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {DataTable} from "@/components/shared/data-table";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {SimpleSelect} from "@/components/shared/simple-select";
import {StatusBadge} from "@/components/shared/status-badge";
import {DATABASE_STATUS_VARIANTS, engineTint} from "@/lib/database";
import {cn} from "@/lib/utils";
import {runAction, useResource} from "@/hooks/use-resource";
import {useUiMode} from "@/hooks/use-ui-mode";

const POLL_INTERVAL = 15000;

/** Every database casos runs, and what state each one is in. */
function DatabasePage(props) {
  useTranslation();
  const {history} = props;
  const {resolvePath} = useUiMode();
  const [namespace, setNamespace] = useState("all");
  const [deleteTarget, setDeleteTarget] = useState(null);
  const [deleteData, setDeleteData] = useState(false);

  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});
  const {data: databases, loading, refresh} = useResource(
    () => DatabaseBackend.getDatabases(namespace === "all" ? "" : namespace),
    [namespace],
    {initialData: [], pollInterval: POLL_INTERVAL}
  );

  function toggleRunning(record, running) {
    runAction(DatabaseBackend.scaleDatabase({namespace: record.namespace, name: record.name, running}), {
      successMessage: running ? i18next.t("database:Database started") : i18next.t("database:Database stopped"),
      onSuccess: () => refresh({silent: true}),
    });
  }

  function remove() {
    if (!deleteTarget) {
      return;
    }
    runAction(
      DatabaseBackend.deleteDatabase({namespace: deleteTarget.namespace, name: deleteTarget.name, deleteData}),
      {
        successMessage: i18next.t("database:Database deleted"),
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
      title: i18next.t("database:Database"),
      dataIndex: "name",
      minWidth: 200,
      sortable: true,
      render: (value, record) => (
        <div className="flex items-center gap-2">
          <span className={cn("flex size-8 shrink-0 items-center justify-center rounded-lg text-white", engineTint(record.engine))}>
            <Database className="size-4" />
          </span>
          <div className="min-w-0">
            <div className="truncate font-medium">{value}</div>
            <div className="text-muted-foreground truncate text-xs">{record.namespace}</div>
          </div>
        </div>
      ),
    },
    {
      key: "engine",
      title: i18next.t("database:Engine"),
      dataIndex: "engineLabel",
      width: 160,
      sortable: true,
      render: (value, record) => (
        <span className="flex items-center gap-1.5">
          {value}
          <Badge variant="secondary">{record.version}</Badge>
        </span>
      ),
    },
    {
      key: "status",
      title: i18next.t("general:Status"),
      dataIndex: "status",
      width: 120,
      sortable: true,
      render: (value) => <StatusBadge status={value} variants={DATABASE_STATUS_VARIANTS} />,
    },
    {
      key: "replicas",
      title: i18next.t("database:Instances"),
      width: 110,
      render: (_value, record) => (
        <span className="tabular-nums">{record.readyReplicas ?? 0} / {record.replicas ?? 0}</span>
      ),
    },
    {key: "cpuLimit", title: i18next.t("launchpad:CPU limit"), dataIndex: "cpuLimit", width: 110},
    {key: "memoryLimit", title: i18next.t("launchpad:Memory limit"), dataIndex: "memoryLimit", width: 130},
    {key: "storage", title: i18next.t("general:Storage"), dataIndex: "storage", width: 110},
    {key: "createdAt", title: i18next.t("general:Created"), dataIndex: "createdAt", width: 170, sortable: true},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 150,
      align: "right",
      render: (_value, record) => (
        <div className="flex items-center justify-end gap-0.5">
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={i18next.t("database:Edit database")}
            title={i18next.t("database:Edit database")}
            onClick={(event) => {
              event.stopPropagation();
              history.push(resolvePath(`/databases/${record.namespace}/${record.name}/edit`));
            }}
          >
            <Pencil />
          </Button>
          {record.status === "stopped" ? (
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={i18next.t("general:Start")}
              title={i18next.t("general:Start")}
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
              aria-label={i18next.t("general:Stop")}
              title={i18next.t("general:Stop")}
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
            aria-label={i18next.t("general:Delete")}
            title={i18next.t("general:Delete")}
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
        title={i18next.t("database:Databases")}
        description={i18next.t("database:Managed PostgreSQL, MySQL, MongoDB and Redis, with credentials and backups handled for you.")}
        actions={
          <Button onClick={() => history.push(resolvePath("/databases/new"))} data-testid="database-create">
            <Database />
            {i18next.t("database:New database")}
          </Button>
        }
      />

      <DataTable
        scopeToWorkspace
        testId="databases-table"
        columns={columns}
        dataSource={databases}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        onRowClick={(record) => history.push(resolvePath(`/databases/${record.namespace}/${record.name}`))}
        emptyIcon={Database}
        emptyText={i18next.t("database:No databases yet. Create one and its connection details are generated for you.")}
        toolbar={
          <div className="flex items-center gap-2">
            <SimpleSelect
              value={namespace}
              onChange={setNamespace}
              options={[
                {label: i18next.t("general:All namespaces"), value: "all"},
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
        title={`${i18next.t("general:Delete")} ${deleteTarget?.name ?? ""}`}
        description={i18next.t("database:The engine and its address are removed. Its data and backups are kept unless you say otherwise.")}
        confirmText={i18next.t("general:Delete")}
        onConfirm={remove}
        extra={
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={deleteData} onCheckedChange={(checked) => setDeleteData(Boolean(checked))} />
            {i18next.t("database:Also delete its data and backups")}
            <Badge variant="danger">{i18next.t("launchpad:Cannot be undone")}</Badge>
          </label>
        }
      />
    </PageContainer>
  );
}

export default DatabasePage;
