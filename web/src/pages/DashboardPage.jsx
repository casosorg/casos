import React, {useEffect, useMemo, useState} from "react";
import {useHistory} from "react-router-dom";
import {useTranslation} from "react-i18next";
import {ArrowRight, Boxes, CheckCircle2, Layers, Network, Server, TriangleAlert} from "lucide-react";
import * as AccountBackend from "@/backend/AccountBackend";
import * as DashboardBackend from "@/backend/DashboardBackend";
import * as HelmBackend from "@/backend/HelmBackend";
import * as MachineBackend from "@/backend/MachineBackend";
import {useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {Alert, AlertDescription, AlertTitle, MessageAlert} from "@/components/ui/alert";
import {Progress} from "@/components/ui/progress";
import {DataTable} from "@/components/shared/data-table";
import {PageContainer} from "@/components/shared/page-header";
import {StatCard} from "@/components/shared/stat-card";
import {Loading} from "@/components/shared/loading";
import {FirstRunChecklist} from "@/components/shared/first-run-checklist";
import {CategoryDonut, RadialGauge, RankedBarChart} from "@/components/shared/charts";
import {getDashboardHealthState} from "@/lib/dashboardHealth";
import {getFirstRunChecklist, isFirstRunComplete, markFirstRunChecklistDone, readFirstRunChecklistDone} from "@/lib/firstRunChecklist";
import {useUiMode} from "@/hooks/use-ui-mode";
import SimpleHome from "@/pages/simple/SimpleHome";

function formatMiB(mib) {
  return mib >= 1024 ? `${(mib / 1024).toFixed(1)} GiB` : `${Math.round(mib)} MiB`;
}

function percentOf(used, total) {
  return total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0;
}

/**
 * A chart with its title, its subtitle and a closing line, so every panel on
 * the page has the same three-part shape whatever it draws.
 */
function ChartCard({title, description, footer, children, className}) {
  return (
    <Card className={className}>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
        {description ? <CardDescription>{description}</CardDescription> : null}
      </CardHeader>
      <CardContent>{children}</CardContent>
      {footer ? <CardFooter className="text-muted-foreground text-sm">{footer}</CardFooter> : null}
    </Card>
  );
}

function UtilizationRow({label, percent, detail}) {
  return (
    <div className="grid gap-2">
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-sm font-medium">{label}</span>
        <span className="text-muted-foreground text-sm tabular-nums">{percent}%</span>
      </div>
      <Progress value={percent} className="h-2" />
      <span className="text-muted-foreground text-xs tabular-nums">{detail}</span>
    </div>
  );
}

function DashboardPage({account, accountUpdatedAt, onOpenAccount}) {
  const history = useHistory();
  const {t} = useTranslation();
  const {advanced, resolvePath} = useUiMode();
  const {data: stats, loading} = useResource(() => DashboardBackend.getDashboard(), [], {initialData: null});

  // Setup is a one-time affair, so once it is done these three requests stop
  // being issued and the dashboard goes back to one call per visit.
  const [firstRunDone, setFirstRunDone] = useState(() => readFirstRunChecklistDone());
  const checklistResource = {initialData: null, enabled: !firstRunDone, toastOnError: false};
  // Simple mode puts the machine and release counts on its home page, so those
  // two keep being fetched there even once the checklist is out of the way.
  const countedResource = {initialData: null, enabled: !firstRunDone || !advanced, toastOnError: false};
  const {data: machines, loading: machinesLoading} = useResource(
    () => MachineBackend.getGlobalMachines(), [accountUpdatedAt, advanced], countedResource);
  const {data: signinOptions, loading: signinOptionsLoading} = useResource(
    () => AccountBackend.getSigninOptions(), [accountUpdatedAt], checklistResource);
  const {data: releases, loading: releasesLoading} = useResource(
    () => HelmBackend.getHelmReleases(), [accountUpdatedAt, advanced], countedResource);

  const firstRunSteps = useMemo(
    () => getFirstRunChecklist({account, signinOptions, machines, releases, stats}),
    [account, signinOptions, machines, releases, stats]);
  // Half-loaded state reads as "nothing done yet", which would flash the
  // checklist at every existing cluster on the first visit after an upgrade.
  const firstRunLoading = machinesLoading || signinOptionsLoading || releasesLoading;
  const firstRunComplete = isFirstRunComplete(firstRunSteps);

  useEffect(() => {
    if (firstRunDone || firstRunLoading || !firstRunComplete) {
      return;
    }
    markFirstRunChecklistDone();
    setFirstRunDone(true);
  }, [firstRunDone, firstRunLoading, firstRunComplete]);

  const reasonVariants = {
    CrashLoopBackOff: "danger",
    OOMKilled: "danger",
    ImagePullBackOff: "warning",
    ErrImagePull: "warning",
    ErrImageNeverPull: "warning",
    InvalidImageName: "warning",
    CreateContainerConfigError: "warning",
    CreateContainerError: "warning",
    RunContainerError: "danger",
    Evicted: "secondary",
    Unschedulable: "warning",
    SchedulerError: "danger",
    Failed: "danger",
    Unknown: "muted",
  };

  const reasonLabels = {
    CrashLoopBackOff: t("dashboard:reason CrashLoopBackOff"),
    OOMKilled: t("dashboard:reason OOMKilled"),
    ImagePullBackOff: t("dashboard:reason ImagePullBackOff"),
    ErrImagePull: t("dashboard:reason ImagePullBackOff"),
    ErrImageNeverPull: t("dashboard:reason ErrImageNeverPull"),
    InvalidImageName: t("dashboard:reason InvalidImageName"),
    CreateContainerConfigError: t("dashboard:reason ConfigError"),
    CreateContainerError: t("dashboard:reason ContainerError"),
    RunContainerError: t("dashboard:reason RunContainerError"),
    Evicted: t("dashboard:reason Evicted"),
    Unschedulable: t("dashboard:reason Unschedulable"),
    SchedulerError: t("dashboard:reason SchedulerError"),
    Failed: t("dashboard:reason Failed"),
    Unknown: t("dashboard:reason Unknown"),
  };

  const firstRunChecklist = firstRunDone || firstRunLoading || firstRunComplete ? null : (
    <FirstRunChecklist
      steps={firstRunSteps}
      onAction={(step) => {
        if (step === "password") {
          onOpenAccount?.();
        } else if (step === "machine" || step === "node") {
          history.push(resolvePath("/machines"));
        } else if (step === "app") {
          history.push(resolvePath("/app-store"));
        }
      }}
    />
  );

  if (loading) {
    return <Loading type="page" />;
  }

  if (!advanced) {
    return <SimpleHome stats={stats} releases={releases} machines={machines} checklist={firstRunChecklist} />;
  }

  // There is no dashboard to draw without cluster stats, but that is exactly
  // the state a fresh install is in, so the setup checklist still gets shown.
  if (!stats) {
    return firstRunChecklist ? <PageContainer>{firstRunChecklist}</PageContainer> : null;
  }

  const nodesOffline = stats.nodesTotal - stats.nodesReady;
  const podsOther = stats.podsTotal - stats.podsRunning;
  const deploymentsUnavailable = stats.deploymentsTotal - stats.deploymentsAvailable;
  const nodeReadyRate = percentOf(stats.nodesReady, stats.nodesTotal);
  const podRunningRate = percentOf(stats.podsRunning, stats.podsTotal);
  const cpuPercent = percentOf(stats.clusterCPUUsedM, stats.clusterCPUTotalM);
  const memPercent = percentOf(stats.clusterMemUsedMi, stats.clusterMemTotalMi);
  const unhealthyPods = Array.isArray(stats.unhealthyPods) ? stats.unhealthyPods : [];
  const {healthStatus, notReadyNodes, needsNodes} = getDashboardHealthState(stats);
  const clusterHealthy = healthStatus === "healthy";
  const deploymentsDegraded = deploymentsUnavailable > 0;
  const hasClusterMetrics = stats.clusterCPUTotalM > 0 || stats.clusterMemTotalMi > 0;

  let healthMessage = "";
  if (healthStatus === "unknown") {
    healthMessage = t("dashboard:health data unavailable");
  } else if (healthStatus === "unhealthy") {
    const unhealthyDetails = [];
    if (notReadyNodes > 0) {
      unhealthyDetails.push(t("dashboard:alert nodes not ready", {count: notReadyNodes}));
    }
    if (unhealthyPods.length > 0) {
      unhealthyDetails.push(t("dashboard:alert unhealthy", {count: unhealthyPods.length}));
    }
    healthMessage = unhealthyDetails.join(", ") || t("dashboard:alert unhealthy cluster");
  }

  const unhealthyColumns = [
    {key: "namespace", title: t("dashboard:col Namespace"), dataIndex: "namespace", width: 180},
    {key: "name", title: t("dashboard:col App name"), dataIndex: "name", className: "font-medium"},
    {
      key: "reason",
      title: t("dashboard:col Status"),
      dataIndex: "reason",
      width: 220,
      render: (reason) => <Badge variant={reasonVariants[reason] ?? "muted"}>{reasonLabels[reason] ?? reason}</Badge>,
    },
  ];

  // With no nodes the health alert can only say "unavailable" or list Pods that
  // are pending for that one reason, so the banner replaces it. The checklist
  // already says it better on a fresh install, so the banner waits its turn.
  let clusterAlert = null;
  if (needsNodes) {
    if (!firstRunChecklist && !firstRunLoading) {
      clusterAlert = (
        <MessageAlert
          variant="destructive"
          title={t("dashboard:alert no nodes")}
          description={t("dashboard:alert no nodes description")}
          action={
            <Button size="sm" onClick={() => history.push("/machines")}>
              {t("general:Add Machine")}
              <ArrowRight />
            </Button>
          }
        />
      );
    }
  } else if (!clusterHealthy) {
    const listUnhealthy = healthStatus === "unhealthy" && unhealthyPods.length > 0;
    clusterAlert = (
      <div className="grid gap-4">
        <Alert variant={healthStatus === "unknown" ? "warning" : "destructive"}>
          <TriangleAlert />
          <AlertTitle>{healthMessage}</AlertTitle>
          {listUnhealthy ? <AlertDescription>{t("dashboard:alert inspect pods")}</AlertDescription> : null}
        </Alert>
        {listUnhealthy ? (
          <DataTable
            columns={unhealthyColumns}
            dataSource={unhealthyPods}
            rowKey={(record, index) => `${record.namespace}/${record.name}/${index}`}
            pageSize={5}
            dense
            onRowClick={(record) => history.push(`/pods?namespace=${record.namespace}`)}
          />
        ) : null}
      </div>
    );
  } else {
    clusterAlert = (
      <Alert variant="success">
        <CheckCircle2 />
        <AlertTitle>{t("dashboard:alert healthy")}</AlertTitle>
      </Alert>
    );
  }

  return (
    <PageContainer>
      {firstRunChecklist}
      {clusterAlert}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label={t("dashboard:stat Nodes ready")}
          value={stats.nodesReady}
          suffix={`/ ${stats.nodesTotal}`}
          icon={Server}
          tone={nodesOffline > 0 ? "warning" : "success"}
          hint={nodesOffline > 0
            ? t("dashboard:alert nodes not ready", {count: nodesOffline})
            : t("dashboard:hint all nodes ready")}
        />
        <StatCard
          label={t("dashboard:stat Pods running")}
          value={stats.podsRunning}
          suffix={`/ ${stats.podsTotal}`}
          icon={Boxes}
          tone={unhealthyPods.length > 0 ? "danger" : "success"}
          hint={unhealthyPods.length > 0
            ? t("dashboard:alert unhealthy", {count: unhealthyPods.length})
            : t("dashboard:hint pods other", {n: podsOther})}
        />
        <StatCard
          label={t("dashboard:stat Deployments available")}
          value={stats.deploymentsAvailable}
          suffix={`/ ${stats.deploymentsTotal}`}
          icon={Layers}
          tone={deploymentsDegraded ? "danger" : "success"}
          hint={deploymentsDegraded
            ? t("dashboard:hint deployments degraded", {n: deploymentsUnavailable})
            : t("dashboard:hint all deployments available")}
        />
        <StatCard
          label={t("dashboard:stat Services")}
          value={stats.servicesTotal}
          icon={Network}
          tone="info"
          hint={t("dashboard:hint across namespaces", {n: stats.namespacesTotal})}
        />
      </div>

      {hasClusterMetrics ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("dashboard:chart Cluster utilization")}</CardTitle>
            <CardDescription>{t("dashboard:chart Cluster utilization description")}</CardDescription>
            <CardAction>
              <Badge variant="outline">{t("dashboard:label Live")}</Badge>
            </CardAction>
          </CardHeader>
          <CardContent className="grid gap-6 md:grid-cols-2">
            <UtilizationRow
              label={t("dashboard:label CPU")}
              percent={cpuPercent}
              detail={`${(stats.clusterCPUUsedM / 1000).toFixed(2)} / ${(stats.clusterCPUTotalM / 1000).toFixed(2)} ${t("dashboard:label cores")}`}
            />
            <UtilizationRow
              label={t("dashboard:label Memory")}
              percent={memPercent}
              detail={`${formatMiB(stats.clusterMemUsedMi)} / ${formatMiB(stats.clusterMemTotalMi)}`}
            />
          </CardContent>
        </Card>
      ) : null}

      <div className="grid gap-4 xl:grid-cols-3">
        <ChartCard
          className="xl:col-span-2"
          title={t("dashboard:chart Pods by namespace")}
          description={t("dashboard:chart Pods by namespace description")}
          footer={t("dashboard:hint across namespaces", {n: stats.namespacesTotal})}
        >
          <RankedBarChart
            data={stats.podsByNamespace}
            valueLabel={t("dashboard:stat Pods total")}
            className="aspect-auto h-[300px] w-full"
          />
        </ChartCard>

        <ChartCard title={t("dashboard:chart Pod phase")} description={t("dashboard:chart Pod phase description")}>
          <CategoryDonut data={stats.podsByPhase} centerLabel={t("dashboard:label Pods")} className="max-h-[280px] w-full" />
        </ChartCard>
      </div>

      <div className="grid gap-4 xl:grid-cols-3">
        <ChartCard
          title={t("dashboard:chart Node health")}
          description={t("dashboard:chart Node health description")}
          footer={t("dashboard:hint online offline", {online: stats.nodesReady, offline: nodesOffline})}
        >
          <RadialGauge
            value={nodeReadyRate}
            label={t("dashboard:label Online")}
            caption={t("dashboard:label Online")}
            color="var(--chart-1)"
            className="max-h-[220px] w-full"
          />
        </ChartCard>

        <ChartCard
          title={t("dashboard:chart Pod availability")}
          description={t("dashboard:chart Pod availability description")}
          footer={t("dashboard:hint running other", {running: stats.podsRunning, other: podsOther})}
        >
          <RadialGauge
            value={podRunningRate}
            label={t("dashboard:label Running")}
            caption={t("dashboard:label Running")}
            color="var(--chart-2)"
            className="max-h-[220px] w-full"
          />
        </ChartCard>

        <ChartCard
          title={t("dashboard:chart Service types")}
          description={t("dashboard:chart Service types description")}
        >
          <CategoryDonut
            data={stats.servicesByType}
            centerLabel={t("dashboard:label Services")}
            className="max-h-[220px] w-full"
          />
        </ChartCard>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t("dashboard:chart Node infra")}</CardTitle>
          <CardDescription>{t("dashboard:chart Node infra description")}</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-6 sm:grid-cols-2">
          <div className="grid gap-2">
            <span className="text-muted-foreground text-center text-sm font-medium">
              {t("dashboard:label Operating system")}
            </span>
            <CategoryDonut data={stats.nodesByOS} centerLabel={t("dashboard:label Nodes")} className="max-h-[200px] w-full" />
          </div>
          <div className="grid gap-2">
            <span className="text-muted-foreground text-center text-sm font-medium">
              {t("dashboard:label Architecture")}
            </span>
            <CategoryDonut data={stats.nodesByArch} centerLabel={t("dashboard:label Nodes")} className="max-h-[200px] w-full" />
          </div>
        </CardContent>
      </Card>
    </PageContainer>
  );
}

export default DashboardPage;
