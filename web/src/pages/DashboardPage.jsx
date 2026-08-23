import React, {useEffect, useMemo, useState} from "react";
import {useHistory} from "react-router-dom";
import {useTranslation} from "react-i18next";
import {ArrowRight, Boxes, CheckCircle2, Layers, Network, Server, Settings, TriangleAlert} from "lucide-react";
import * as AccountBackend from "@/backend/AccountBackend";
import * as DashboardBackend from "@/backend/DashboardBackend";
import * as HelmBackend from "@/backend/HelmBackend";
import * as MachineBackend from "@/backend/MachineBackend";
import {useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Card, CardContent, CardHeader, CardTitle} from "@/components/ui/card";
import {Alert, AlertTitle, MessageAlert} from "@/components/ui/alert";
import {Progress} from "@/components/ui/progress";
import {DataTable} from "@/components/shared/data-table";
import {PageContainer} from "@/components/shared/page-header";
import {StatCard} from "@/components/shared/stat-card";
import {Loading} from "@/components/shared/loading";
import {RadialProgress} from "@/components/shared/radial-progress";
import {FirstRunChecklist} from "@/components/shared/first-run-checklist";
import {CategoryDonut, DualCategoryPie, RankedBarChart} from "@/components/shared/charts";
import {getDashboardHealthState} from "@/lib/dashboardHealth";
import {getFirstRunChecklist, isFirstRunComplete, markFirstRunChecklistDone, readFirstRunChecklistDone} from "@/lib/firstRunChecklist";
import {useUiMode} from "@/hooks/use-ui-mode";
import SimpleHome from "@/pages/simple/SimpleHome";

const POD_PHASE_COLORS = {
  Running: "#3b82f6",
  Pending: "#0ea5e9",
  Succeeded: "#14b8a6",
  Failed: "#6366f1",
  Unknown: "#8b5cf6",
};

const SVC_TYPE_COLORS = {
  ClusterIP: "#3b82f6",
  NodePort: "#06b6d4",
  LoadBalancer: "#6366f1",
  ExternalName: "#14b8a6",
};

function formatMiB(mib) {
  return mib >= 1024 ? `${(mib / 1024).toFixed(1)} GiB` : `${mib} MiB`;
}

function GaugeCard({title, percent, tone, primaryValue, primaryLabel, secondaryValue, secondaryLabel}) {
  return (
    <Card className="h-full gap-3 py-4">
      <CardHeader className="px-4">
        <CardTitle className="text-sm">{title}</CardTitle>
      </CardHeader>
      <CardContent className="flex items-center justify-center gap-8 px-4 pb-2">
        <RadialProgress value={percent} tone={tone} label={primaryLabel} />
        <div className="grid gap-4">
          <div>
            <div className={tone === "info" ? "text-info text-2xl font-semibold" : "text-2xl font-semibold"}>{primaryValue}</div>
            <div className="text-muted-foreground text-sm">{primaryLabel}</div>
          </div>
          <div>
            <div className="text-2xl font-semibold text-violet-500">{secondaryValue}</div>
            <div className="text-muted-foreground text-sm">{secondaryLabel}</div>
          </div>
        </div>
      </CardContent>
    </Card>
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

  const nodeReadyRate = stats.nodesTotal > 0 ? Number(((stats.nodesReady / stats.nodesTotal) * 100).toFixed(1)) : 0;
  const podRunningRate = stats.podsTotal > 0 ? Number(((stats.podsRunning / stats.podsTotal) * 100).toFixed(1)) : 0;
  const unhealthyPods = Array.isArray(stats.unhealthyPods) ? stats.unhealthyPods : [];
  const {healthStatus, notReadyNodes, needsNodes} = getDashboardHealthState(stats);
  const clusterHealthy = healthStatus === "healthy";
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
  const deploymentsDegraded = stats.deploymentsAvailable < stats.deploymentsTotal;
  const hasClusterMetrics = stats.clusterCPUTotalM > 0 || stats.clusterMemTotalMi > 0;

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
    clusterAlert = (
      <div className="grid gap-2">
        <Alert variant={healthStatus === "unknown" ? "warning" : "destructive"}>
          <TriangleAlert />
          <AlertTitle className="line-clamp-none break-words">{healthMessage}</AlertTitle>
        </Alert>
        {healthStatus === "unhealthy" && unhealthyPods.length > 0 ? (
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

      <div className="grid grid-cols-2 gap-3 md:grid-cols-4 xl:grid-cols-8">
        <StatCard label={t("dashboard:stat Nodes total")} value={stats.nodesTotal} icon={Server} tone="info" />
        <StatCard label={t("dashboard:stat Nodes ready")} value={stats.nodesReady} icon={CheckCircle2} tone="success" />
        <StatCard label={t("dashboard:stat Pods total")} value={stats.podsTotal} icon={Boxes} tone="info" />
        <StatCard label={t("dashboard:stat Pods running")} value={stats.podsRunning} icon={Boxes} tone="info" />
        <StatCard label={t("dashboard:stat Namespaces")} value={stats.namespacesTotal} icon={Settings} />
        <StatCard label={t("dashboard:stat Services")} value={stats.servicesTotal} icon={Network} />
        <StatCard label={t("dashboard:stat Deployments total")} value={stats.deploymentsTotal} icon={Layers} />
        <StatCard
          label={t("dashboard:stat Deployments available")}
          value={stats.deploymentsAvailable}
          suffix={`/ ${stats.deploymentsTotal}`}
          icon={CheckCircle2}
          tone={deploymentsDegraded ? "danger" : "success"}
          className={deploymentsDegraded ? "border-destructive/40" : undefined}
        />
      </div>

      {hasClusterMetrics ? (
        <div className="grid gap-3 md:grid-cols-2">
          <Card className="gap-2 py-4">
            <CardContent className="grid gap-2 px-4">
              <span className="text-muted-foreground text-sm font-medium">CPU</span>
              <div className="flex items-center gap-3">
                <Progress
                  value={stats.clusterCPUTotalM > 0 ? Math.round((stats.clusterCPUUsedM / stats.clusterCPUTotalM) * 100) : 0}
                  tone="info"
                  className="flex-1"
                />
                <span className="text-muted-foreground text-xs whitespace-nowrap tabular-nums">
                  {(stats.clusterCPUUsedM / 1000).toFixed(2)} / {(stats.clusterCPUTotalM / 1000).toFixed(2)} cores
                </span>
              </div>
            </CardContent>
          </Card>

          <Card className="gap-2 py-4">
            <CardContent className="grid gap-2 px-4">
              <span className="text-muted-foreground text-sm font-medium">Memory</span>
              <div className="flex items-center gap-3">
                <Progress
                  value={stats.clusterMemTotalMi > 0 ? Math.round((stats.clusterMemUsedMi / stats.clusterMemTotalMi) * 100) : 0}
                  tone="success"
                  className="flex-1"
                />
                <span className="text-muted-foreground text-xs whitespace-nowrap tabular-nums">
                  {formatMiB(stats.clusterMemUsedMi)} / {formatMiB(stats.clusterMemTotalMi)}
                </span>
              </div>
            </CardContent>
          </Card>
        </div>
      ) : null}

      <div className="grid gap-3 xl:grid-cols-[7fr_5fr]">
        <Card className="gap-3 py-4">
          <CardHeader className="px-4">
            <CardTitle className="text-sm">{t("dashboard:chart Pods by namespace")}</CardTitle>
          </CardHeader>
          <CardContent className="px-2">
            <RankedBarChart data={stats.podsByNamespace} valueLabel="Pods" className="aspect-auto h-[280px] w-full" />
          </CardContent>
        </Card>

        <Card className="gap-3 py-4">
          <CardHeader className="px-4">
            <CardTitle className="text-sm">{t("dashboard:chart Pod phase")}</CardTitle>
          </CardHeader>
          <CardContent className="px-2">
            <CategoryDonut data={stats.podsByPhase} colors={POD_PHASE_COLORS} className="aspect-auto h-[280px] w-full" />
          </CardContent>
        </Card>
      </div>

      <div className="grid gap-3 xl:grid-cols-[7fr_7fr_10fr]">
        <GaugeCard
          title={t("dashboard:chart Node health")}
          percent={nodeReadyRate}
          tone="info"
          primaryValue={stats.nodesReady}
          primaryLabel={t("dashboard:label Online")}
          secondaryValue={stats.nodesTotal - stats.nodesReady}
          secondaryLabel={t("dashboard:label Offline")}
        />
        <GaugeCard
          title={t("dashboard:chart Pod availability")}
          percent={podRunningRate}
          tone="info"
          primaryValue={stats.podsRunning}
          primaryLabel={t("dashboard:label Running")}
          secondaryValue={stats.podsTotal - stats.podsRunning}
          secondaryLabel={t("dashboard:label Other")}
        />
        <Card className="gap-3 py-4">
          <CardHeader className="px-4">
            <CardTitle className="text-sm">{t("dashboard:chart Service types")}</CardTitle>
          </CardHeader>
          <CardContent className="px-2">
            <CategoryDonut data={stats.servicesByType} colors={SVC_TYPE_COLORS} className="aspect-auto h-[200px] w-full" />
          </CardContent>
        </Card>
      </div>

      <Card className="gap-3 py-4">
        <CardHeader className="px-4">
          <CardTitle className="text-sm">{t("dashboard:chart Node infra")}</CardTitle>
        </CardHeader>
        <CardContent className="px-2">
          <DualCategoryPie left={stats.nodesByOS} right={stats.nodesByArch} className="aspect-auto h-[220px] w-full" />
        </CardContent>
      </Card>
    </PageContainer>
  );
}

export default DashboardPage;
