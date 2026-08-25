import React, {useEffect, useMemo, useRef, useState} from "react";
import i18next from "i18next";
import {useTranslation} from "react-i18next";
import {
  ArrowLeft,
  Boxes,
  Cpu,
  ExternalLink,
  FileText,
  HardDrive,
  MemoryStick,
  Pencil,
  Play,
  RotateCw,
  ScrollText,
  ShieldCheck,
  ShieldOff,
  Square,
  TerminalSquare,
  Trash2,
} from "lucide-react";
import * as CertificateBackend from "@/backend/CertificateBackend";
import * as DeploymentBackend from "@/backend/DeploymentBackend";
import * as ImageBackend from "@/backend/ImageBackend";
import * as MetricsBackend from "@/backend/MetricsBackend";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "@/components/ui/card";
import {Checkbox} from "@/components/ui/checkbox";
import {MessageAlert} from "@/components/ui/alert";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {DataTable} from "@/components/shared/data-table";
import {Loading} from "@/components/shared/loading";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {PodEventsSheet} from "@/components/shared/pod-events-sheet";
import {PodLogsSheet} from "@/components/shared/pod-logs-sheet";
import {PodTerminalSheet} from "@/components/shared/pod-terminal-sheet";
import {StatCard} from "@/components/shared/stat-card";
import {StatusBadge} from "@/components/shared/status-badge";
import {UsageAreaChart} from "@/components/shared/charts";
import {runAction, useResource} from "@/hooks/use-resource";

const POLL_INTERVAL = 10000;
/** ACME issuance settles in seconds to minutes, so it is watched more closely. */
const CERT_POLL_INTERVAL = 4000;
/** Roughly twenty minutes of samples at the poll interval above. */
const MAX_SAMPLES = 120;

function sampleTime() {
  const now = new Date();
  return `${String(now.getHours()).padStart(2, "0")}:${String(now.getMinutes()).padStart(2, "0")}`;
}

/**
 * Whether the app's domains are served over HTTPS, and the button that makes
 * them so. Issuance is asynchronous, so an in-flight request is polled until it
 * settles rather than reported once and forgotten.
 */
function CertificateLine({namespace, name, domains}) {
  const [status, setStatus] = useState(null);
  const [requesting, setRequesting] = useState(false);

  useEffect(() => {
    let cancelled = false;
    let timer = null;

    function poll() {
      CertificateBackend.getCertStatus(namespace, name).then((res) => {
        if (cancelled || res.status !== "ok") {
          return;
        }
        setStatus(res.data);
        if (res.data?.status === "pending" || res.data?.status === "verifying") {
          timer = setTimeout(poll, CERT_POLL_INTERVAL);
        }
      });
    }

    poll();
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [namespace, name, requesting]);

  if (domains.length === 0) {
    return null;
  }

  const state = status?.status ?? "none";

  function request() {
    setRequesting(true);
    runAction(CertificateBackend.requestLECert({namespace, ingressName: name, domain: domains[0].host}), {
      successMessage: i18next.t("launchpad:Requesting a certificate"),
      onSuccess: () => setRequesting(false),
      onError: () => setRequesting(false),
    });
  }

  return (
    <div className="flex flex-wrap items-center gap-2 text-sm">
      {state === "issued" ? (
        <Badge variant="success">
          <ShieldCheck />
          {i18next.t("launchpad:HTTPS")}
        </Badge>
      ) : (
        <Badge variant="secondary">
          <ShieldOff />
          {i18next.t("launchpad:No certificate")}
        </Badge>
      )}
      {state === "issued" && status?.expiry ? (
        <span className="text-muted-foreground text-xs">{i18next.t("launchpad:Expires")} {status.expiry}</span>
      ) : null}
      {state === "pending" || state === "verifying" ? (
        <span className="text-muted-foreground text-xs">{i18next.t("launchpad:Checking the domain with Let's Encrypt…")}</span>
      ) : null}
      {state === "failed" ? <span className="text-destructive text-xs">{status?.error}</span> : null}
      {state !== "issued" && state !== "pending" && state !== "verifying" ? (
        <Button size="sm" variant="outline" onClick={request} disabled={requesting}>
          <ShieldCheck />
          {i18next.t("launchpad:Enable HTTPS")}
        </Button>
      ) : null}
    </div>
  );
}

/**
 * One app, in full: what it is doing, what it is using, where it answers and
 * which pods are behind it.
 *
 * The usage series is collected in the page rather than fetched: the cluster
 * reports what is happening now, so a chart over time can only be built by
 * keeping the samples as they arrive. That is also why it starts empty and
 * fills in — an honest gap is better than a flat line invented for it.
 */
function LaunchpadDetailPage(props) {
  useTranslation();
  const {history, match} = props;
  const {namespace, name} = match.params;

  const [detail, setDetail] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [series, setSeries] = useState([]);
  const [logsPod, setLogsPod] = useState(null);
  const [terminalPod, setTerminalPod] = useState(null);
  const [eventsPod, setEventsPod] = useState(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteData, setDeleteData] = useState(false);

  const podNamesRef = useRef([]);

  function load({background = false} = {}) {
    if (!background) {
      setLoading(true);
    }
    return ImageBackend.getImageApp(namespace, name)
      .then((res) => {
        if (res.status === "ok") {
          setDetail(res.data);
          setError(null);
          podNamesRef.current = (res.data.pods ?? []).map((pod) => pod.name);
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

  const {data: metrics} = useResource(() => MetricsBackend.getMetrics(), [], {
    initialData: {nodes: [], pods: []},
    toastOnError: false,
    pollInterval: POLL_INTERVAL,
  });

  // Every metrics poll adds one point, summed over the app's own pods.
  useEffect(() => {
    const podNames = new Set(podNamesRef.current);
    if (podNames.size === 0) {
      return;
    }
    const totals = (metrics?.pods ?? []).reduce(
      (accumulator, pod) => {
        if (pod.namespace !== namespace || !podNames.has(pod.name)) {
          return accumulator;
        }
        return {cpu: accumulator.cpu + (pod.cpuM ?? 0), memory: accumulator.memory + (pod.memMi ?? 0)};
      },
      {cpu: 0, memory: 0}
    );
    setSeries((current) => [...current, {time: sampleTime(), ...totals}].slice(-MAX_SAMPLES));
  }, [metrics, namespace]);

  const cpuSeries = useMemo(() => series.map((point) => ({time: point.time, value: point.cpu})), [series]);
  const memorySeries = useMemo(() => series.map((point) => ({time: point.time, value: point.memory})), [series]);
  const latest = series[series.length - 1];

  function toggleRunning(running) {
    runAction(ImageBackend.scaleApp({namespace, name, running}), {
      successMessage: running ? i18next.t("launchpad:App started") : i18next.t("launchpad:App stopped"),
      onSuccess: () => load({background: true}),
    });
  }

  function restart() {
    runAction(DeploymentBackend.restartDeployment(namespace, name), {
      successMessage: i18next.t("launchpad:Restarting"),
      onSuccess: () => load({background: true}),
    });
  }

  function uninstall() {
    runAction(ImageBackend.uninstallApp({namespace, name, deleteData}), {
      successMessage: i18next.t("launchpad:App deleted"),
      onSuccess: () => history.push("/launchpad"),
    });
  }

  if (loading) {
    return <Loading type="page" />;
  }

  if (!detail) {
    return (
      <PageContainer>
        <MessageAlert title={error ?? i18next.t("launchpad:App not found")} />
        <div>
          <Button variant="outline" onClick={() => history.push("/launchpad")}>
            <ArrowLeft />
            {i18next.t("launchpad:Back")}
          </Button>
        </div>
      </PageContainer>
    );
  }

  const podColumns = [
    {key: "name", title: i18next.t("launchpad:Pod"), dataIndex: "name", minWidth: 220, ellipsis: true},
    {
      key: "phase",
      title: i18next.t("general:Status"),
      dataIndex: "phase",
      width: 120,
      render: (value) => <StatusBadge status={value} variants={{Running: "success", Pending: "warning", Failed: "danger", Succeeded: "info"}} />,
    },
    {key: "ready", title: i18next.t("launchpad:Ready"), dataIndex: "ready", width: 90},
    {key: "restarts", title: i18next.t("launchpad:Restarts"), dataIndex: "restarts", width: 100},
    {key: "nodeName", title: i18next.t("general:Node"), dataIndex: "nodeName", minWidth: 140, ellipsis: true},
    {key: "createdAt", title: i18next.t("launchpad:Created"), dataIndex: "createdAt", width: 170},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 150,
      align: "right",
      render: (_value, record) => (
        <div className="flex items-center justify-end gap-0.5">
          <Button size="icon-sm" variant="ghost" title={i18next.t("launchpad:Logs")} aria-label={i18next.t("launchpad:Logs")} onClick={() => setLogsPod(record)}>
            <ScrollText />
          </Button>
          <Button size="icon-sm" variant="ghost" title={i18next.t("desktop:Terminal")} aria-label={i18next.t("desktop:Terminal")} onClick={() => setTerminalPod(record)}>
            <TerminalSquare />
          </Button>
          <Button size="icon-sm" variant="ghost" title={i18next.t("launchpad:Events")} aria-label={i18next.t("launchpad:Events")} onClick={() => setEventsPod(record)}>
            <FileText />
          </Button>
        </div>
      ),
    },
  ];

  return (
    <PageContainer>
      <PageHeader
        title={detail.name}
        description={detail.image}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" onClick={() => history.push("/launchpad")}>
              <ArrowLeft />
              {i18next.t("launchpad:Back")}
            </Button>
            <Button variant="outline" onClick={() => history.push(`/launchpad/${namespace}/${name}/edit`)}>
              <Pencil />
              {i18next.t("launchpad:Edit app")}
            </Button>
            {detail.status === "stopped" ? (
              <Button variant="outline" onClick={() => toggleRunning(true)}>
                <Play />
                {i18next.t("launchpad:Start")}
              </Button>
            ) : (
              <>
                <Button variant="outline" onClick={restart}>
                  <RotateCw />
                  {i18next.t("launchpad:Restart")}
                </Button>
                <Button variant="outline" onClick={() => toggleRunning(false)}>
                  <Square />
                  {i18next.t("launchpad:Stop")}
                </Button>
              </>
            )}
            <Button variant="destructive" onClick={() => setDeleteOpen(true)}>
              <Trash2 />
              {i18next.t("launchpad:Delete")}
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
          tone={detail.status === "deployed" ? "success" : detail.status === "failed" ? "danger" : "default"}
          hint={detail.description}
        />
        <StatCard
          label={i18next.t("launchpad:Copies")}
          value={`${detail.readyReplicas ?? 0} / ${detail.replicas ?? 0}`}
          icon={Boxes}
          hint={detail.hpa ? i18next.t("launchpad:Autoscaling on CPU") : null}
        />
        <StatCard label={i18next.t("launchpad:CPU limit")} value={detail.cpuLimit || "—"} icon={Cpu} />
        <StatCard label={i18next.t("launchpad:Memory limit")} value={detail.memoryLimit || "—"} icon={MemoryStick} />
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{i18next.t("launchpad:CPU usage")}</CardTitle>
            <CardDescription>
              {i18next.t("launchpad:Millicores across every copy")}
              {latest ? ` · ${i18next.t("launchpad:now")} ${Math.round(latest.cpu)}m` : ""}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {cpuSeries.length > 1 ? (
              <UsageAreaChart data={cpuSeries} label={i18next.t("dashboard:label CPU")} unit="m" />
            ) : (
              <p className="text-muted-foreground py-10 text-center text-sm">{i18next.t("launchpad:Collecting samples…")}</p>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{i18next.t("launchpad:Memory usage")}</CardTitle>
            <CardDescription>
              {i18next.t("launchpad:MiB across every copy")}
              {latest ? ` · ${i18next.t("launchpad:now")} ${Math.round(latest.memory)} MiB` : ""}
            </CardDescription>
          </CardHeader>
          <CardContent>
            {memorySeries.length > 1 ? (
              <UsageAreaChart data={memorySeries} label={i18next.t("dashboard:label Memory")} unit="Mi" color="var(--chart-2)" />
            ) : (
              <p className="text-muted-foreground py-10 text-center text-sm">{i18next.t("launchpad:Collecting samples…")}</p>
            )}
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{i18next.t("launchpad:Where it answers")}</CardTitle>
            <CardDescription>{detail.serviceType || i18next.t("launchpad:Not exposed")}</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3">
            {(detail.urls ?? []).length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {detail.urls.map((url) => (
                  <Button key={url} size="sm" variant="outline" asChild>
                    <a href={url} target="_blank" rel="noreferrer">
                      <ExternalLink />
                      {url}
                    </a>
                  </Button>
                ))}
              </div>
            ) : (
              <p className="text-muted-foreground text-sm">{i18next.t("launchpad:No address yet.")}</p>
            )}
            <CertificateLine namespace={namespace} name={name} domains={detail.domains ?? []} />
            {(detail.ports ?? []).length > 0 ? (
              <div className="flex flex-wrap gap-1.5">
                {detail.ports.map((port) => (
                  <Badge key={`${port.name}-${port.containerPort}`} variant="secondary">
                    {port.containerPort}/{port.protocol}
                  </Badge>
                ))}
              </div>
            ) : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{i18next.t("launchpad:Storage")}</CardTitle>
            <CardDescription>{i18next.t("launchpad:Disks that survive a restart")}</CardDescription>
          </CardHeader>
          <CardContent>
            {(detail.volumes ?? []).length > 0 ? (
              <div className="flex flex-wrap gap-1.5">
                {detail.volumes.map((volume) => (
                  <Badge key={volume.claimName} variant="info">
                    <HardDrive />
                    {volume.mountPath}
                  </Badge>
                ))}
              </div>
            ) : (
              <p className="text-muted-foreground text-sm">{i18next.t("launchpad:No persistent storage.")}</p>
            )}
          </CardContent>
        </Card>
      </div>

      <DataTable
        testId="launchpad-pods-table"
        title={i18next.t("launchpad:Pods")}
        columns={podColumns}
        dataSource={detail.pods ?? []}
        rowKey="name"
        pageSize={10}
        emptyText={i18next.t("launchpad:No pods are running for this app.")}
      />

      <PodLogsSheet pod={logsPod} open={Boolean(logsPod)} onClose={() => setLogsPod(null)} />
      <PodTerminalSheet pod={terminalPod} open={Boolean(terminalPod)} onClose={() => setTerminalPod(null)} />
      <PodEventsSheet pod={eventsPod} open={Boolean(eventsPod)} onClose={() => setEventsPod(null)} />

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={`${i18next.t("launchpad:Delete")} ${detail.name}`}
        description={i18next.t("launchpad:The app, its address and its autoscaler are removed. Its disks are kept unless you say otherwise.")}
        confirmText={i18next.t("launchpad:Delete")}
        onConfirm={uninstall}
        extra={
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={deleteData} onCheckedChange={(checked) => setDeleteData(Boolean(checked))} />
            {i18next.t("launchpad:Also delete its disks")}
          </label>
        }
      />
    </PageContainer>
  );
}

export default LaunchpadDetailPage;
