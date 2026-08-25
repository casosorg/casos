import React, {useEffect, useState} from "react";
import {useTranslation} from "react-i18next";
import {CircleArrowUp, History, Play, Plus, RefreshCw, RotateCcw, ScrollText, Search, Square, Trash2} from "lucide-react";
import {useHistory} from "react-router-dom";
import * as HelmBackend from "@/backend/HelmBackend";
import * as ImageBackend from "@/backend/ImageBackend";
import * as IngressBackend from "@/backend/IngressBackend";
import * as NodeBackend from "@/backend/NodeBackend";
import * as PodBackend from "@/backend/PodBackend";
import * as PvcBackend from "@/backend/PvcBackend";
import * as ServiceBackend from "@/backend/ServiceBackend";
import * as Setting from "@/Setting";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Checkbox} from "@/components/ui/checkbox";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {AppIcon} from "@/components/shared/app-icon";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {ResourceSheet} from "@/components/shared/resource-sheet";
import {SimpleSelect} from "@/components/shared/simple-select";
import {AiDots, Loading} from "@/components/shared/loading";
import {HelmInstallDialog} from "@/components/shared/helm-install-dialog";
import {ImageInstallDialog} from "@/components/shared/image-install-dialog";
import {PodLogsSheet} from "@/components/shared/pod-logs-sheet";
import {AppCardList} from "@/components/shared/app-card-list";
import {PageHeader} from "@/components/shared/page-header";
import {useResource} from "@/hooks/use-resource";
import {useUiMode} from "@/hooks/use-ui-mode";
import {appResourcesOf, groupAppResources} from "@/lib/appAccess";
import {chartIconUrl} from "@/lib/appCatalog";
import {cn} from "@/lib/utils";

function releaseKey(release) {
  return `${release.namespace}/${release.name}`;
}

// An app installed from a container image is listed beside the Helm releases:
// the two are installed differently, but a reader has one list of apps. The
// record is shaped like a release so one table, one card and one set of filters
// serve both, and carries the app itself for the actions that differ.
function toAppRecord(app) {
  const shortName = app.repository.split("/").pop();
  return {
    kind: "image",
    name: app.name,
    namespace: app.namespace,
    chart: `${app.repository}:${app.tag}`,
    chartName: app.repository,
    chartVersion: app.tag,
    app_version: app.tag,
    status: app.status,
    description: app.description,
    updated: app.createdAt,
    // Docker Hub only hands out a logo while searching it, so an installed app
    // falls back to the catalogue's icon for an app of that name.
    icon: chartIconUrl(shortName),
    image: app,
  };
}

const STATUS_VARIANTS = {
  deployed: "success",
  stopped: "muted",
  failed: "danger",
  pending: "warning",
  "pending-install": "warning",
  "pending-upgrade": "warning",
  "pending-rollback": "warning",
  superseded: "muted",
  uninstalling: "info",
};

const OPERATION_STATUS_VARIANTS = {
  succeeded: "success",
  failed: "danger",
  running: "info",
  pending: "warning",
};

// How often the table re-reads a release that Helm still calls pending. Such a
// release is mid-operation, and its row only becomes truthful on a later read.
const RELEASE_POLL_INTERVAL = 5000;
const OPERATION_POLL_INTERVAL = 3000;

const isPendingRelease = (status) => typeof status === "string" && status.startsWith("pending");
const isActiveOperation = (status) => status === "running" || status === "pending";

// Go marshals a time.Time that was never set as year one; that is "never", not
// a date to show.
function formatTimestamp(value) {
  if (!value || value.startsWith("0001-")) {
    return "-";
  }
  return value.slice(0, 19).replace("T", " ");
}

function logLineClass(line) {
  if (line.startsWith("ERROR")) {
    return "text-red-400";
  }
  if (line.startsWith("WARNING")) {
    return "text-amber-400";
  }
  return "text-neutral-300";
}

// Helm reports the chart as "name-version" in one string. Splitting on the first
// segment that starts with a digit is what separates "ingress-nginx" from
// "4.11.2" — chart names routinely contain dashes themselves.
function parseChartName(chart) {
  const parts = chart?.split("-") ?? [];
  const versionIndex = parts.findIndex((part) => /^\d/.test(part));
  return versionIndex > 0 ? parts.slice(0, versionIndex).join("-") : chart;
}

function parseChartVersion(chart) {
  const parts = chart?.split("-") ?? [];
  const versionIndex = parts.findIndex((part) => /^\d/.test(part));
  return versionIndex > 0 ? parts.slice(versionIndex).join("-") : "";
}

export function helmReleaseUpgradeTarget(release) {
  return {
    releaseName: release.name,
    namespace: release.namespace,
    chartName: release.chartName || parseChartName(release.chart),
    repoURL: release.repoURL,
    version: release.chartVersion || parseChartVersion(release.chart),
  };
}

function StatusBadge({status, description}) {
  const badge = (
    <Badge variant={STATUS_VARIANTS[status] ?? "muted"} className="gap-1.5">
      <span className="size-1.5 rounded-full bg-current" />
      {status}
    </Badge>
  );
  if (status === "failed" && description) {
    // Badge does not forward a ref; the tooltip anchors to a span of its own.
    return (
      <SimpleTooltip title={description}>
        <span>{badge}</span>
      </SimpleTooltip>
    );
  }
  return badge;
}

// Simple mode sorts an app into one of these, so the reader can ask "what is
// broken" without knowing that Helm calls it "failed" or "pending-upgrade".
function statusBucket(release) {
  if (isPendingRelease(release.status)) {
    return "updating";
  }
  if (release.status === "failed") {
    return "attention";
  }
  if (release.status === "deployed") {
    return "running";
  }
  return "other";
}

function FilterChip({active, label, count, onClick}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-3 py-1.5 text-xs transition-colors",
        active
          ? "bg-primary text-primary-foreground border-primary font-medium"
          : "hover:bg-accent text-muted-foreground"
      )}
    >
      {label}
      <span
        className={cn(
          "rounded-full px-1.5 text-[10px] tabular-nums",
          active ? "bg-primary-foreground/20" : "bg-muted text-muted-foreground"
        )}
      >
        {count}
      </span>
    </button>
  );
}

export default function HelmReleasePage() {
  const {t} = useTranslation();
  const {advanced, resolvePath} = useUiMode();
  const router = useHistory();
  const [namespace, setNamespace] = useState("all");
  const [releases, setReleases] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [query, setQuery] = useState("");
  const [bucket, setBucket] = useState("all");
  // Keyed per release so the choice cannot leak from one row's dialog to another.
  const [deleteDataFor, setDeleteDataFor] = useState({});

  const [historyRelease, setHistoryRelease] = useState(null);
  const [history, setHistory] = useState([]);
  const [historyLoading, setHistoryLoading] = useState(false);

  const [upgradeTarget, setUpgradeTarget] = useState(null);
  const [imageUpgradeTarget, setImageUpgradeTarget] = useState(null);
  const [logsPod, setLogsPod] = useState(null);

  const [logsRelease, setLogsRelease] = useState(null);
  const [operation, setOperation] = useState(null);
  const [operationLogs, setOperationLogs] = useState([]);
  const [operationLoading, setOperationLoading] = useState(false);
  const [operationError, setOperationError] = useState(null);

  // Only simple mode draws an app's address and disks, so the three lists
  // behind them are not requested in advanced mode, where the same objects
  // have list pages of their own.
  const perAppResources = {initialData: [], enabled: !advanced, toastOnError: false};
  const {data: services} = useResource(() => ServiceBackend.getServices(), [advanced], perAppResources);
  const {data: ingresses} = useResource(() => IngressBackend.getIngresses(), [advanced], perAppResources);
  const {data: pvcs} = useResource(() => PvcBackend.getPvcs(), [advanced], perAppResources);
  const {data: nodes} = useResource(() => NodeBackend.getNodes(), [advanced], perAppResources);

  // A background refresh leaves the table alone: it is the page keeping itself
  // current, not the operator asking for something, and a spinner every few
  // seconds would say otherwise.
  function fetchReleases({background = false} = {}) {
    if (!background) {
      setLoading(true);
      setError(null);
    }
    // One list, two installers. Either side failing leaves the other showing:
    // a broken Helm repository is no reason to hide the apps that are running.
    return Promise.all([HelmBackend.getHelmReleases(namespace), ImageBackend.getImageApps(namespace)])
      .then(([helmRes, imageRes]) => {
        const next = [];
        if (helmRes.status === "ok") {
          next.push(...(helmRes.data ?? []));
        }
        if (imageRes.status === "ok") {
          next.push(...(imageRes.data ?? []).map(toAppRecord));
        }
        setReleases(next);
        const failure = helmRes.status === "ok" ? imageRes : helmRes;
        if (failure.status !== "ok" && !background) {
          setError(failure.msg);
        }
      })
      .catch((e) => {
        if (!background) {
          setError(e.message);
        }
      })
      .finally(() => {
        if (!background) {
          setLoading(false);
        }
      });
  }

  useEffect(() => {
    fetchReleases();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [namespace]);

  // A pending release is one Helm has not finished with. Left alone the row
  // keeps its stale status until someone presses Refresh, which is what makes
  // an install that is merely slow look like one that has died.
  const hasPendingRelease = releases.some((release) => isPendingRelease(release.status));
  useEffect(() => {
    if (!hasPendingRelease) {
      return undefined;
    }
    const timer = setInterval(() => fetchReleases({background: true}), RELEASE_POLL_INTERVAL);
    return () => clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasPendingRelease, namespace]);

  // The operation behind the open log sheet, re-read while it is still running
  // so its output arrives as it is produced.
  useEffect(() => {
    if (!logsRelease) {
      return undefined;
    }
    let cancelled = false;
    let timer = null;

    function load(initial) {
      if (initial) {
        setOperationLoading(true);
      }
      HelmBackend.getHelmReleaseOperation(logsRelease.name, logsRelease.namespace)
        .then((res) => {
          if (cancelled) {
            return;
          }
          if (res.status !== "ok") {
            setOperationError(res.msg);
            return;
          }
          const task = res.data ?? null;
          setOperationError(null);
          setOperation(task);
          setOperationLogs((res.data2 ?? []).map((entry) => entry?.message).filter(Boolean));
          if (isActiveOperation(task?.status)) {
            timer = setTimeout(() => load(false), OPERATION_POLL_INTERVAL);
          }
        })
        .catch((e) => {
          if (!cancelled) {
            setOperationError(e.message);
          }
        })
        .finally(() => {
          if (!cancelled && initial) {
            setOperationLoading(false);
          }
        });
    }

    load(true);
    return () => {
      cancelled = true;
      if (timer) {
        clearTimeout(timer);
      }
    };
  }, [logsRelease]);

  // An image app has no install log to show: nothing recorded the install
  // beyond the workload itself. What its reader wants is the container output,
  // so that is what the same button opens.
  function openAppPodLogs(release) {
    PodBackend.getPods(release.namespace).then((res) => {
      if (res.status !== "ok") {
        Setting.showMessage("error", res.msg);
        return;
      }
      const pods = (res.data ?? []).filter(
        (pod) => pod.labels?.["app.kubernetes.io/instance"] === release.name && !pod.labels?.["app.kubernetes.io/component"]
      );
      const pod = pods.find((item) => item.phase === "Running") ?? pods[0];
      if (!pod) {
        Setting.showMessage("error", t("image:This app has no container running yet"));
        return;
      }
      setLogsPod(pod);
    });
  }

  function openLogs(release) {
    if (release.kind === "image") {
      openAppPodLogs(release);
      return;
    }
    setOperation(null);
    setOperationLogs([]);
    setOperationError(null);
    setLogsRelease(release);
  }

  function openUpgrade(release) {
    if (release.kind === "image") {
      setImageUpgradeTarget(release.image);
      return;
    }
    setUpgradeTarget(helmReleaseUpgradeTarget(release));
  }

  function closeLogs() {
    setLogsRelease(null);
    // The list is refreshed on the way out: the sheet is usually closed just
    // after an operation it was watching has finished.
    fetchReleases({background: true});
  }

  function openHistory(release) {
    setHistoryRelease(release);
    setHistory([]);
    setHistoryLoading(true);
    HelmBackend.getHelmReleaseHistory(release.name, release.namespace)
      .then((res) => {
        if (res.status === "ok") {
          setHistory(res.data ?? []);
        }
      })
      .finally(() => setHistoryLoading(false));
  }

  function handleRollback(release, revision) {
    HelmBackend.rollbackHelmRelease({releaseName: release.name, namespace: release.namespace, revision}).then((res) => {
      if (res.status === "ok") {
        Setting.showMessage("success", `Rolled back to revision ${revision}`);
        setHistoryRelease(null);
        fetchReleases();
      } else {
        Setting.showMessage("error", res.msg);
      }
    });
  }

  // Stopping an app is not uninstalling it: the workload scales to zero and
  // its volumes stay, so starting it again finds everything where it was.
  function handleToggleRunning(release) {
    const running = release.status === "stopped";
    return ImageBackend.scaleApp({namespace: release.namespace, name: release.name, running}).then((res) => {
      if (res.status !== "ok") {
        Setting.showMessage("error", res.msg);
        return;
      }
      fetchReleases();
    });
  }

  function handleUninstall(release) {
    const deleteData = deleteDataFor[releaseKey(release)] ?? false;
    const uninstalled =
      release.kind === "image"
        ? ImageBackend.uninstallApp({namespace: release.namespace, name: release.name, deleteData})
        : HelmBackend.uninstallHelmRelease({releaseName: release.name, namespace: release.namespace, deleteData});
    return uninstalled.then((res) => {
      if (res.status === "ok") {
        Setting.showMessage("success", `Uninstalled ${release.name}`);
        fetchReleases();
      } else {
        setError(res.msg);
        Setting.showMessage("error", res.msg);
      }
      setDeleteDataFor((previous) => ({...previous, [releaseKey(release)]: false}));
    });
  }

  const columns = [
    {
      key: "name",
      title: t("helm:Release name"),
      dataIndex: "name",
      sortable: true,
      minWidth: 220,
      render: (value, release) => (
        <span className="flex items-center gap-2.5">
          <AppIcon src={release.icon} chartName={release.chartName} name={value} size="sm" />
          <span className="truncate font-medium">{value}</span>
        </span>
      ),
    },
    {
      key: "chart",
      title: t("helm:Chart"),
      dataIndex: "chart",
      render: (value, release) => (
        <span className="flex items-center gap-1.5">
          {release.kind === "image" ? <Badge variant="info">{t("image:Image")}</Badge> : null}
          <span className="truncate">{release.chartName || parseChartName(value)}</span>
          <Badge variant="muted">{release.chartVersion || parseChartVersion(value)}</Badge>
        </span>
      ),
    },
    {
      key: "namespace",
      title: t("general:Namespaces"),
      dataIndex: "namespace",
      width: 170,
      sortable: true,
      render: (value) => <Badge variant="muted">{value}</Badge>,
    },
    {
      key: "status",
      title: t("general:Status"),
      dataIndex: "status",
      width: 150,
      sortable: true,
      render: (value, record) => (
        <span className="flex items-center gap-1.5">
          <StatusBadge status={value} description={record.description} />
          {isPendingRelease(value) ? <AiDots size="small" /> : null}
        </span>
      ),
    },
    {key: "app_version", title: t("helm:App version"), dataIndex: "app_version", width: 140},
    {
      key: "updated",
      title: t("helm:Last deployed"),
      dataIndex: "updated",
      width: 190,
      sortable: true,
      render: (value) => (value ? <span className="text-xs">{value.slice(0, 19).replace("T", " ")}</span> : "-"),
    },
    {
      key: "action",
      title: t("general:Action"),
      width: 190,
      align: "right",
      render: (_, release) => (
        <div className="flex justify-end gap-1">
          <SimpleTooltip title={t("helm:Logs")}>
            <Button variant="outline" size="icon-sm" onClick={() => openLogs(release)} aria-label="Logs">
              <ScrollText className="size-4" />
            </Button>
          </SimpleTooltip>
          {release.kind === "image" ? (
            <SimpleTooltip title={release.status === "stopped" ? t("image:Start") : t("image:Stop")}>
              <Button
                variant="outline"
                size="icon-sm"
                onClick={() => handleToggleRunning(release)}
                aria-label={release.status === "stopped" ? "Start" : "Stop"}
              >
                {release.status === "stopped" ? <Play className="size-4" /> : <Square className="size-4" />}
              </Button>
            </SimpleTooltip>
          ) : null}
          <SimpleTooltip title={t("helm:Upgrade")}>
            <Button variant="outline" size="icon-sm" onClick={() => openUpgrade(release)} aria-label="Upgrade">
              <CircleArrowUp className="size-4" />
            </Button>
          </SimpleTooltip>
          {release.kind === "image" ? null : (
            <SimpleTooltip title={t("helm:History")}>
              <Button variant="outline" size="icon-sm" onClick={() => openHistory(release)} aria-label="History">
                <History className="size-4" />
              </Button>
            </SimpleTooltip>
          )}
          <ConfirmDialog
            title={t("helm:Uninstall release?")}
            description={`${release.name} (${release.namespace})`}
            extra={
              <label className="hover:bg-accent/50 flex cursor-pointer items-start gap-2 rounded-md px-2 py-1.5 text-sm">
                <Checkbox
                  className="mt-0.5"
                  checked={deleteDataFor[releaseKey(release)] ?? false}
                  onCheckedChange={(checked) =>
                    setDeleteDataFor((previous) => ({...previous, [releaseKey(release)]: checked === true}))
                  }
                />
                <span>
                  {t("helm:Also delete this app's data")}
                  <span className="text-muted-foreground block text-xs">
                    {t("helm:Its volumes are kept by default, and reinstalling this app will reuse them")}
                  </span>
                </span>
              </label>
            }
            confirmText={t("general:Delete")}
            cancelText={t("general:Cancel")}
            onConfirm={() => handleUninstall(release)}
          >
            <Button variant="outline" size="icon-sm" className="text-destructive" aria-label="Uninstall">
              <Trash2 className="size-4" />
            </Button>
          </ConfirmDialog>
        </div>
      ),
    },
  ];

  // The status the cluster reports now, not the one the row carried when the
  // sheet was opened.
  const logsReleaseStatus =
    releases.find((release) => release.name === logsRelease?.name && release.namespace === logsRelease?.namespace)
      ?.status ?? logsRelease?.status;
  // Helm leaves a release pending when the operation that was writing it never
  // reached an end — a CasOS restart mid-install is the usual way. Nothing in
  // the cluster will move it now, so say so instead of showing a status that
  // reads like work in progress.
  const stuckHint =
    logsRelease && isPendingRelease(logsReleaseStatus) && !isActiveOperation(operation?.status)
      ? t("helm:Release stuck in a pending status", {status: logsReleaseStatus})
      : null;

  const appResources = groupAppResources({services, ingresses, pvcs, nodes});

  const counts = {all: releases.length, running: 0, attention: 0, updating: 0, other: 0};
  releases.forEach((release) => {
    counts[statusBucket(release)] += 1;
  });
  const needle = query.trim().toLowerCase();
  const visibleReleases = releases.filter((release) => {
    if (bucket !== "all" && statusBucket(release) !== bucket) {
      return false;
    }
    if (!needle) {
      return true;
    }
    return [release.name, release.chartName, release.chart, release.namespace]
      .filter(Boolean)
      .some((field) => field.toLowerCase().includes(needle));
  });
  const filters = [
    {key: "all", label: t("simple:All apps")},
    {key: "running", label: t("simple:Running")},
    {key: "attention", label: t("simple:Needs attention")},
    {key: "updating", label: t("simple:Updating")},
  ].filter((filter) => filter.key === "all" || (counts[filter.key] ?? 0) > 0);

  return (
    <PageContainer>
      {error ? <MessageAlert title={error} /> : null}

      {advanced ? (
        <DataTable
          scopeToWorkspace
          title={t("helm:Installed Apps")}
          description={`${releases.length} apps`}
          columns={columns}
          dataSource={releases}
          rowKey="name"
          loading={loading}
          searchable
          emptyText={t("helm:No releases")}
          toolbar={
            <>
              <SimpleSelect
                value={namespace}
                onChange={setNamespace}
                options={[{value: "all", label: t("helm:All namespaces")}]}
                className="w-44"
              />
              <Button variant="outline" size="sm" onClick={() => fetchReleases()} loading={loading}>
                <RefreshCw />
                {t("general:Refresh")}
              </Button>
              <Button size="sm" onClick={() => router.push(resolvePath("/app-store"))}>
                <Plus />
                {t("helm:Install")}
              </Button>
            </>
          }
        />
      ) : (
        <>
          <PageHeader
            title={t("simple:My Apps")}
            description={t("simple:Everything you have installed, with the address it answers on.")}
            actions={
              <>
                <Button variant="outline" onClick={() => fetchReleases()} loading={loading}>
                  <RefreshCw />
                  {t("general:Refresh")}
                </Button>
                <Button onClick={() => router.push(resolvePath("/app-store"))}>
                  <Plus />
                  {t("simple:Install an app")}
                </Button>
              </>
            }
          />

          {releases.length > 0 ? (
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="relative w-full sm:max-w-xs">
                <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
                <Input
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder={t("simple:Search apps")}
                  className="pl-9"
                />
              </div>
              <div className="flex flex-wrap gap-1.5">
                {filters.map((filter) => (
                  <FilterChip
                    key={filter.key}
                    label={filter.label}
                    count={counts[filter.key] ?? 0}
                    active={bucket === filter.key}
                    onClick={() => setBucket(filter.key)}
                  />
                ))}
              </div>
            </div>
          ) : null}

          <AppCardList
            releases={visibleReleases}
            resources={(release) => appResourcesOf(appResources, release)}
            loading={loading}
            isPending={isPendingRelease}
            emptyTitle={releases.length > 0 ? t("simple:No app here matches that.") : undefined}
            emptyAction={
              releases.length > 0 ? (
                <Button
                  variant="outline"
                  onClick={() => {
                    setQuery("");
                    setBucket("all");
                  }}
                >
                  {t("simple:Show all apps")}
                </Button>
              ) : undefined
            }
            deleteDataFor={deleteDataFor}
            onDeleteDataChange={(release, checked) =>
              setDeleteDataFor((previous) => ({...previous, [releaseKey(release)]: checked}))
            }
            onOpenLogs={openLogs}
            onUpgrade={openUpgrade}
            onToggleRunning={handleToggleRunning}
            onUninstall={handleUninstall}
            onInstallMore={() => router.push(resolvePath("/app-store"))}
          />
        </>
      )}

      <ResourceSheet
        open={Boolean(historyRelease)}
        onOpenChange={(next) => (next ? null : setHistoryRelease(null))}
        title={historyRelease ? `${t("helm:History")}: ${historyRelease.name}` : ""}
        size="md"
        bodyClassName="overflow-y-auto scrollbar-thin"
      >
        {historyLoading ? (
          <Loading />
        ) : (
          <ol className="relative grid gap-3 border-l pl-5">
            {history.map((revision) => (
              <li key={revision.revision} className="bg-card relative rounded-lg border p-3 shadow-sm">
                <span
                  className={`ring-background absolute top-4 -left-[27px] size-2.5 rounded-full ring-4 ${
                    revision.status === "deployed"
                      ? "bg-success"
                      : revision.status === "failed"
                        ? "bg-destructive"
                        : "bg-info"
                  }`}
                />
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-semibold">#{revision.revision}</span>
                  <Badge variant="muted">{revision.chart}</Badge>
                  <Badge variant={STATUS_VARIANTS[revision.status] ?? "muted"}>{revision.status}</Badge>
                  <div className="flex-1" />
                  {revision.status !== "deployed" ? (
                    <Button variant="outline" size="sm" onClick={() => handleRollback(historyRelease, revision.revision)}>
                      <RotateCcw />
                      {t("helm:Rollback")}
                    </Button>
                  ) : null}
                </div>
                <div className="text-muted-foreground mt-1 text-xs">{revision.updated?.slice(0, 19).replace("T", " ")}</div>
                {revision.description ? <div className="mt-1 text-xs">{revision.description}</div> : null}
              </li>
            ))}
          </ol>
        )}
      </ResourceSheet>

      <ResourceSheet
        open={Boolean(logsRelease)}
        onOpenChange={(next) => (next ? null : closeLogs())}
        title={logsRelease ? `${t("helm:Release logs")}: ${logsRelease.name}` : ""}
        description={logsRelease ? `${logsRelease.namespace} · ${logsRelease.chart ?? ""}` : ""}
        size="lg"
        bodyClassName="gap-3 overflow-y-auto scrollbar-thin"
      >
        {operationLoading ? (
          <Loading />
        ) : (
          <>
            {operationError ? <MessageAlert title={operationError} /> : null}
            {operation ? (
              <div className="bg-muted/40 flex flex-wrap items-center gap-2 rounded-lg border px-3 py-2">
                <Badge variant="muted">{operation.operation}</Badge>
                <Badge variant={OPERATION_STATUS_VARIANTS[operation.status] ?? "muted"}>{operation.status}</Badge>
                <Badge variant="muted">{operation.phase}</Badge>
                <span className="text-muted-foreground text-xs">
                  {formatTimestamp(operation.startedAt)} → {formatTimestamp(operation.finishedAt)}
                </span>
              </div>
            ) : null}
            {!operation && !operationError ? (
              <MessageAlert variant="info" description={t("helm:No operation recorded for this release")} />
            ) : null}
            {operation?.errorMsg ? (
              <MessageAlert
                title={t("helm:Helm operation failed")}
                description={<span className="break-all whitespace-pre-wrap">{operation.errorMsg}</span>}
              />
            ) : null}
            {isActiveOperation(operation?.status) ? (
              <MessageAlert variant="info" description={t("helm:This operation is still running")} />
            ) : null}
            {stuckHint ? <MessageAlert variant="warning" description={stuckHint} /> : null}
            {operationLogs.length > 0 ? (
              <div className="scrollbar-thin min-h-0 flex-1 overflow-y-auto rounded-lg bg-neutral-950 p-3 font-mono text-xs leading-relaxed">
                {operationLogs.map((line, index) => (
                  <div key={index} className={logLineClass(line)}>
                    {line}
                  </div>
                ))}
                {isActiveOperation(operation?.status) ? (
                  <span className="mt-1 inline-flex items-center text-neutral-500">
                    <AiDots size="small" />
                  </span>
                ) : null}
              </div>
            ) : null}
          </>
        )}
      </ResourceSheet>

      <HelmInstallDialog
        open={Boolean(upgradeTarget)}
        action="upgrade"
        chart={upgradeTarget}
        onClose={() => setUpgradeTarget(null)}
        onInstalled={() => {
          setUpgradeTarget(null);
          fetchReleases();
        }}
      />

      <ImageInstallDialog
        open={Boolean(imageUpgradeTarget)}
        action="upgrade"
        app={imageUpgradeTarget}
        onClose={() => setImageUpgradeTarget(null)}
        onInstalled={() => {
          setImageUpgradeTarget(null);
          fetchReleases();
        }}
      />

      <PodLogsSheet pod={logsPod} open={Boolean(logsPod)} onClose={() => setLogsPod(null)} />
    </PageContainer>
  );
}
