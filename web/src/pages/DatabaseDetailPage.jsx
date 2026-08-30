import React, {useEffect, useState} from "react";
import i18next from "i18next";
import {useTranslation} from "react-i18next";
import {
  ArrowLeft,
  Boxes,
  Copy,
  Database,
  Download,
  Eye,
  EyeOff,
  HardDrive,
  Pencil,
  Play,
  RotateCcw,
  Save,
  ScrollText,
  SlidersHorizontal,
  Square,
  TerminalSquare,
  Trash2,
} from "lucide-react";
import * as DatabaseBackend from "@/backend/DatabaseBackend";
import * as Setting from "@/Setting";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "@/components/ui/card";
import {Checkbox} from "@/components/ui/checkbox";
import {MessageAlert} from "@/components/ui/alert";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {DataTable} from "@/components/shared/data-table";
import {Loading} from "@/components/shared/loading";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {DatabaseParamsDialog} from "@/components/shared/database-params-dialog";
import {PodLogsSheet} from "@/components/shared/pod-logs-sheet";
import {PodShell} from "@/components/shared/pod-shell";
import {ResourceSheet} from "@/components/shared/resource-sheet";
import {StatCard} from "@/components/shared/stat-card";
import {StatusBadge} from "@/components/shared/status-badge";
import {engineTint} from "@/lib/database";
import {cn} from "@/lib/utils";
import {runAction} from "@/hooks/use-resource";
import {useUiMode} from "@/hooks/use-ui-mode";

const POLL_INTERVAL = 15000;

function CopyField({label, value, secret}) {
  const [revealed, setRevealed] = useState(false);
  if (!value) {
    return null;
  }
  const shown = secret && !revealed ? "•".repeat(Math.min(String(value).length, 24)) : value;

  return (
    <div className="grid gap-1">
      <span className="text-muted-foreground text-xs">{label}</span>
      <div className="flex items-center gap-1.5">
        <code className="bg-muted/60 min-w-0 flex-1 truncate rounded-md px-2 py-1.5 font-mono text-xs">{shown}</code>
        {secret ? (
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={revealed ? i18next.t("database:Hide") : i18next.t("database:Reveal")}
            onClick={() => setRevealed((open) => !open)}
          >
            {revealed ? <EyeOff /> : <Eye />}
          </Button>
        ) : null}
        <Button
          size="icon-sm"
          variant="ghost"
          aria-label={i18next.t("launchpad:Copy")}
          onClick={() => {
            navigator.clipboard?.writeText(String(value));
            Setting.showMessage("success", i18next.t("launchpad:Copied"));
          }}
        >
          <Copy />
        </Button>
      </div>
    </div>
  );
}

/**
 * One database: how to connect to it, what it is doing, and the backups it
 * holds. The console and the backups both run inside the database's own pod,
 * which is why both are unavailable while it is stopped — and say so.
 */
function DatabaseDetailPage(props) {
  useTranslation();
  const {history, match} = props;
  const {resolvePath} = useUiMode();
  const {namespace, name} = match.params;

  const [detail, setDetail] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [consoleOpen, setConsoleOpen] = useState(false);
  const [logsPod, setLogsPod] = useState(null);
  const [paramsOpen, setParamsOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteData, setDeleteData] = useState(false);
  const [restoreTarget, setRestoreTarget] = useState(null);
  const [backupTarget, setBackupTarget] = useState(null);
  const [working, setWorking] = useState(false);

  function load({background = false} = {}) {
    if (!background) {
      setLoading(true);
    }
    return DatabaseBackend.getDatabase(namespace, name)
      .then((res) => {
        if (res.status === "ok") {
          setDetail(res.data);
          setError(null);
        } else {
          setError(res.msg);
        }
      })
      .catch((requestError) => setError(requestError.message))
      .finally(() => {
        if (!background) {
          setLoading(false);
        }
      });
  }

  useEffect(() => {
    load();
    const timer = setInterval(() => load({background: true}), POLL_INTERVAL);
    return () => clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [namespace, name]);

  function toggleRunning(running) {
    runAction(DatabaseBackend.scaleDatabase({namespace, name, running}), {
      successMessage: running ? i18next.t("database:Database started") : i18next.t("database:Database stopped"),
      onSuccess: () => load({background: true}),
    });
  }

  function backupNow() {
    setWorking(true);
    // A dump of a large database takes a while, and the button has to say so
    // rather than look ignored.
    runAction(DatabaseBackend.backupDatabase({namespace, name}), {
      successMessage: i18next.t("database:Backup written"),
      onSuccess: () => load({background: true}),
    }).finally(() => setWorking(false));
  }

  function restore() {
    if (!restoreTarget) {
      return;
    }
    setWorking(true);
    runAction(DatabaseBackend.restoreDatabase({namespace, name, file: restoreTarget.name}), {
      successMessage: i18next.t("database:Backup restored"),
      onSuccess: () => {
        setRestoreTarget(null);
        load({background: true});
      },
    }).finally(() => setWorking(false));
  }

  function removeBackup() {
    if (!backupTarget) {
      return;
    }
    runAction(DatabaseBackend.deleteDatabaseBackup({namespace, name, file: backupTarget.name}), {
      successMessage: i18next.t("database:Backup deleted"),
      onSuccess: () => {
        setBackupTarget(null);
        load({background: true});
      },
    });
  }

  function remove() {
    runAction(DatabaseBackend.deleteDatabase({namespace, name, deleteData}), {
      successMessage: i18next.t("database:Database deleted"),
      onSuccess: () => history.push(resolvePath("/databases")),
    });
  }

  if (loading) {
    return <Loading type="page" />;
  }

  if (!detail) {
    return (
      <PageContainer>
        <MessageAlert title={error ?? i18next.t("database:Database not found")} />
        <div>
          <Button variant="outline" onClick={() => history.push(resolvePath("/databases"))}>
            <ArrowLeft />
            {i18next.t("launchpad:Back")}
          </Button>
        </div>
      </PageContainer>
    );
  }

  const running = detail.status === "running";
  // The engine's own log is the log of whichever pod is actually up; a reader
  // asking for it should not have to know there is a pod at all.
  const enginePod = (detail.pods ?? []).find((pod) => pod.phase === "Running") ?? null;

  const podColumns = [
    {key: "name", title: i18next.t("launchpad:Pod"), dataIndex: "name", minWidth: 220, ellipsis: true},
    {
      key: "phase",
      title: i18next.t("general:Status"),
      dataIndex: "phase",
      width: 120,
      render: (value) => <StatusBadge status={value} variants={{Running: "success", Pending: "warning", Failed: "danger"}} />,
    },
    {key: "ready", title: i18next.t("launchpad:Ready"), dataIndex: "ready", width: 90},
    {key: "restarts", title: i18next.t("launchpad:Restarts"), dataIndex: "restarts", width: 100},
    {key: "nodeName", title: i18next.t("general:Node"), dataIndex: "nodeName", minWidth: 140, ellipsis: true},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 90,
      align: "right",
      render: (_value, record) => (
        <Button size="icon-sm" variant="ghost" title={i18next.t("general:Logs")} aria-label={i18next.t("general:Logs")} onClick={() => setLogsPod(record)}>
          <ScrollText />
        </Button>
      ),
    },
  ];

  const backupColumns = [
    {key: "name", title: i18next.t("database:Backup"), dataIndex: "name", minWidth: 240, ellipsis: true},
    {
      key: "size",
      title: i18next.t("database:Size"),
      dataIndex: "size",
      width: 120,
      render: (value) => (value >= 1048576 ? `${(value / 1048576).toFixed(1)} MiB` : `${Math.max(1, Math.round((value ?? 0) / 1024))} KiB`),
    },
    {key: "modTime", title: i18next.t("database:Written"), dataIndex: "modTime", width: 150},
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
            title={i18next.t("database:Restore")}
            aria-label={i18next.t("database:Restore")}
            onClick={() => setRestoreTarget(record)}
          >
            <RotateCcw />
          </Button>
          <Button size="icon-sm" variant="ghost" title={i18next.t("database:Download")} aria-label={i18next.t("database:Download")} asChild>
            <a href={DatabaseBackend.backupDownloadUrl(namespace, detail.podName, record.name)} download>
              <Download />
            </a>
          </Button>
          <Button
            size="icon-sm"
            variant="ghost"
            title={i18next.t("general:Delete")}
            aria-label={i18next.t("general:Delete")}
            onClick={() => setBackupTarget(record)}
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
        title={
          <span className="flex items-center gap-2">
            <span className={cn("flex size-8 items-center justify-center rounded-lg text-white", engineTint(detail.engine))}>
              <Database className="size-4" />
            </span>
            {detail.name}
          </span>
        }
        description={`${detail.engineLabel} ${detail.version}`}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" onClick={() => history.push(resolvePath("/databases"))}>
              <ArrowLeft />
              {i18next.t("launchpad:Back")}
            </Button>
            <Button variant="outline" disabled={!running} onClick={() => setConsoleOpen(true)} data-testid="database-console">
              <TerminalSquare />
              {i18next.t("database:Console")}
            </Button>
            <Button variant="outline" disabled={!enginePod} onClick={() => setLogsPod(enginePod)} data-testid="database-engine-log">
              <ScrollText />
              {i18next.t("database:Engine log")}
            </Button>
            <Button variant="outline" onClick={() => setParamsOpen(true)} data-testid="database-params">
              <SlidersHorizontal />
              {i18next.t("database:Engine settings")}
            </Button>
            <Button variant="outline" onClick={() => history.push(resolvePath(`/databases/${namespace}/${name}/edit`))}>
              <Pencil />
              {i18next.t("database:Edit database")}
            </Button>
            {detail.status === "stopped" ? (
              <Button variant="outline" onClick={() => toggleRunning(true)}>
                <Play />
                {i18next.t("general:Start")}
              </Button>
            ) : (
              <Button variant="outline" onClick={() => toggleRunning(false)}>
                <Square />
                {i18next.t("general:Stop")}
              </Button>
            )}
            <Button variant="destructive" onClick={() => setDeleteOpen(true)}>
              <Trash2 />
              {i18next.t("general:Delete")}
            </Button>
          </div>
        }
      />

      {error ? <MessageAlert title={error} /> : null}

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label={i18next.t("general:Status")}
          value={detail.status}
          icon={Boxes}
          tone={running ? "success" : detail.status === "failed" ? "danger" : "default"}
        />
        <StatCard label={i18next.t("database:Instances")} value={`${detail.readyReplicas ?? 0} / ${detail.replicas ?? 0}`} icon={Boxes} />
        <StatCard label={i18next.t("general:Storage")} value={detail.storage || "—"} icon={HardDrive} />
        <StatCard label={i18next.t("launchpad:Memory limit")} value={detail.memoryLimit || "—"} icon={Save} />
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{i18next.t("database:Connection")}</CardTitle>
          <CardDescription>{i18next.t("database:Applications in this cluster connect with the internal address.")}</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          <div className="grid gap-3">
            <CopyField label={i18next.t("database:Host")} value={detail.internalHost} />
            <CopyField label={i18next.t("database:Port")} value={String(detail.port ?? "")} />
            <CopyField label={i18next.t("general:Username")} value={detail.user} />
            <CopyField label={i18next.t("general:Password")} value={detail.password} secret />
          </div>
          <div className="grid gap-3">
            <CopyField label={i18next.t("database:Connection string")} value={detail.internalUri} secret />
            {detail.externalHost ? (
              <>
                <CopyField label={i18next.t("database:External address")} value={detail.externalHost} />
                <CopyField label={i18next.t("database:External connection string")} value={detail.externalUri} secret />
              </>
            ) : (
              <div className="grid gap-1">
                <span className="text-muted-foreground text-xs">{i18next.t("database:External address")}</span>
                <p className="text-muted-foreground text-sm">{i18next.t("database:Not published outside the cluster.")}</p>
              </div>
            )}
            {detail.database ? <CopyField label={i18next.t("database:Database name")} value={detail.database} /> : null}
          </div>
        </CardContent>
      </Card>

      <DataTable
        testId="database-backups-table"
        title={i18next.t("database:Backups")}
        description={i18next.t("database:Dumps written by the engine itself, kept on a disk of their own.")}
        columns={backupColumns}
        dataSource={detail.backups ?? []}
        rowKey="name"
        pageSize={10}
        emptyText={detail.backupsError ? detail.backupsError : i18next.t("database:No backups yet.")}
        toolbar={
          <Button size="sm" disabled={!running || working} onClick={backupNow} data-testid="database-backup">
            <Save />
            {working ? i18next.t("database:Working…") : i18next.t("database:Back up now")}
          </Button>
        }
      />

      <DataTable
        testId="database-pods-table"
        title={i18next.t("general:Pods")}
        columns={podColumns}
        dataSource={detail.pods ?? []}
        rowKey="name"
        pageSize={10}
        emptyText={i18next.t("database:The database is not running.")}
      />

      <ResourceSheet
        open={consoleOpen}
        onOpenChange={(open) => (open ? null : setConsoleOpen(false))}
        title={`${detail.engineLabel} — ${detail.name}`}
        size="xl"
        bodyClassName="bg-neutral-950 p-3"
        toolbar={<Badge variant="secondary">{detail.podName}</Badge>}
      >
        {consoleOpen ? (
          <PodShell
            namespace={namespace}
            name={name}
            endpoint="/api/database-console"
            params={{namespace, name}}
            openDelay={250}
          />
        ) : null}
      </ResourceSheet>

      <DatabaseParamsDialog
        namespace={namespace}
        name={name}
        open={paramsOpen}
        onOpenChange={setParamsOpen}
        onSaved={() => load({background: true})}
      />

      <PodLogsSheet pod={logsPod} open={Boolean(logsPod)} onClose={() => setLogsPod(null)} />

      <ConfirmDialog
        open={Boolean(restoreTarget)}
        onOpenChange={(open) => (open ? null : setRestoreTarget(null))}
        title={i18next.t("database:Restore this backup")}
        description={i18next.t("database:The database is overwritten with the contents of this dump. Anything written since it was taken is lost.")}
        confirmText={i18next.t("database:Restore")}
        onConfirm={restore}
        extra={restoreTarget ? <code className="font-mono text-xs">{restoreTarget.name}</code> : null}
      />

      <ConfirmDialog
        open={Boolean(backupTarget)}
        onOpenChange={(open) => (open ? null : setBackupTarget(null))}
        title={i18next.t("database:Delete this backup")}
        description={backupTarget?.name}
        confirmText={i18next.t("general:Delete")}
        onConfirm={removeBackup}
      />

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={`${i18next.t("general:Delete")} ${detail.name}`}
        description={i18next.t("database:The engine and its address are removed. Its data and backups are kept unless you say otherwise.")}
        confirmText={i18next.t("general:Delete")}
        onConfirm={remove}
        extra={
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={deleteData} onCheckedChange={(checked) => setDeleteData(Boolean(checked))} />
            {i18next.t("database:Also delete its data and backups")}
          </label>
        }
      />
    </PageContainer>
  );
}

export default DatabaseDetailPage;
