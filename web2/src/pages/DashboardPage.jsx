import React, {useMemo} from "react";
import {useHistory} from "react-router-dom";
import {useTranslation} from "react-i18next";
import {Boxes, CheckCircle2, Layers, Network, Server, Settings, TriangleAlert} from "lucide-react";
import * as AccountBackend from "@/backend/AccountBackend";
import * as DashboardBackend from "@/backend/DashboardBackend";
import * as MachineBackend from "@/backend/MachineBackend";
import {useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Card, CardContent, CardHeader, CardTitle} from "@/components/ui/card";
import {Alert, AlertTitle} from "@/components/ui/alert";
import {Progress} from "@/components/ui/progress";
import {DataTable} from "@/components/shared/data-table";
import {PageContainer} from "@/components/shared/page-header";
import {StatCard} from "@/components/shared/stat-card";
import {Loading} from "@/components/shared/loading";
import {RadialProgress} from "@/components/shared/radial-progress";
import {FirstRunChecklist} from "@/components/shared/first-run-checklist";
import {CHART_COLORS, EchartsWidget} from "@/components/shared/echarts-widget";
import {getDashboardHealthState} from "@/lib/dashboardHealth";

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

// The two donuts share a shape: a category breakdown with the legend beside it
// rather than under it, so the chart keeps its height in a short card.
function donutOption(data, colors) {
  if (!data) {
    return null;
  }
  return {
    tooltip: {trigger: "item", formatter: "{b}: {c} ({d}%)"},
    legend: {type: "scroll", orient: "vertical", right: 8, left: "56%", top: "center", textStyle: {fontSize: 12}},
    series: [
      {
        type: "pie",
        radius: ["42%", "68%"],
        center: ["28%", "50%"],
        avoidLabelOverlap: true,
        itemStyle: {borderRadius: 5, borderWidth: 2},
        label: {show: false},
        emphasis: {label: {show: true, fontSize: 13, fontWeight: "bold"}},
        data: Object.entries(data).map(([name, value], index) => ({
          name,
          value,
          itemStyle: {color: colors[name] || CHART_COLORS[index % CHART_COLORS.length]},
        })),
      },
    ],
  };
}

// A cluster can have hundreds of namespaces; the bar chart shows the twelve
// busiest, which is what the question "where are my pods" actually means.
function podsByNamespaceOption(podsByNamespace) {
  if (!podsByNamespace) {
    return null;
  }
  const sorted = Object.entries(podsByNamespace)
    .sort((a, b) => b[1] - a[1])
    .slice(0, 12);

  return {
    color: CHART_COLORS,
    tooltip: {
      trigger: "axis",
      axisPointer: {type: "shadow"},
      formatter: (params) => `${params[0].name}<br/>Pods: <b>${params[0].value}</b>`,
    },
    grid: {left: 16, right: 24, top: 8, bottom: 8, containLabel: true},
    xAxis: {type: "value", minInterval: 1, axisLabel: {fontSize: 11}},
    yAxis: {
      type: "category",
      data: sorted.map(([namespace]) => namespace),
      axisLabel: {fontSize: 11, formatter: (value) => (value.length > 20 ? `${value.slice(0, 18)}…` : value)},
    },
    series: [
      {
        type: "bar",
        data: sorted.map(([, count], index) => ({
          value: count,
          itemStyle: {color: CHART_COLORS[index % CHART_COLORS.length], borderRadius: [0, 4, 4, 0]},
        })),
      },
    ],
  };
}

function nodeInfraOption(nodesByOS, nodesByArch) {
  const osData = Object.entries(nodesByOS || {}).map(([name, value], index) => ({
    name,
    value,
    itemStyle: {color: CHART_COLORS[index % CHART_COLORS.length]},
  }));
  const archData = Object.entries(nodesByArch || {}).map(([name, value], index) => ({
    name,
    value,
    itemStyle: {color: CHART_COLORS[(index + 4) % CHART_COLORS.length]},
  }));

  return {
    tooltip: {trigger: "item", formatter: "{a}<br/>{b}: {c} ({d}%)"},
    legend: {data: [...osData, ...archData].map((item) => item.name), bottom: 0, textStyle: {fontSize: 11}},
    series: [
      {
        name: "OS",
        type: "pie",
        radius: ["20%", "40%"],
        center: ["30%", "45%"],
        label: {position: "inner", fontSize: 11, color: "#fff"},
        itemStyle: {borderRadius: 4, borderWidth: 2},
        data: osData,
      },
      {
        name: "Arch",
        type: "pie",
        radius: ["20%", "40%"],
        center: ["70%", "45%"],
        label: {position: "inner", fontSize: 11, color: "#fff"},
        itemStyle: {borderRadius: 4, borderWidth: 2},
        data: archData,
      },
    ],
  };
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
  const {data: stats, loading} = useResource(() => DashboardBackend.getDashboard(), [], {initialData: null});
  const {data: machines} = useResource(() => MachineBackend.getGlobalMachines(), [accountUpdatedAt], {
    initialData: null,
    toastOnError: false,
  });
  const {data: signinOptions} = useResource(() => AccountBackend.getSigninOptions(), [accountUpdatedAt], {
    initialData: null,
    toastOnError: false,
  });

  const podPhase = useMemo(() => donutOption(stats?.podsByPhase, POD_PHASE_COLORS), [stats]);
  const serviceTypes = useMemo(() => donutOption(stats?.servicesByType, SVC_TYPE_COLORS), [stats]);
  const podsByNamespace = useMemo(() => podsByNamespaceOption(stats?.podsByNamespace), [stats]);
  const nodeInfra = useMemo(() => nodeInfraOption(stats?.nodesByOS, stats?.nodesByArch), [stats]);

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

  if (loading) {
    return <Loading type="page" />;
  }

  if (!stats) {
    return null;
  }

  const nodeReadyRate = stats.nodesTotal > 0 ? Number(((stats.nodesReady / stats.nodesTotal) * 100).toFixed(1)) : 0;
  const podRunningRate = stats.podsTotal > 0 ? Number(((stats.podsRunning / stats.podsTotal) * 100).toFixed(1)) : 0;
  const unhealthyPods = Array.isArray(stats.unhealthyPods) ? stats.unhealthyPods : [];
  const {healthStatus, notReadyNodes} = getDashboardHealthState(stats);
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

  return (
    <PageContainer>
      <FirstRunChecklist
        account={account}
        signinOptions={signinOptions}
        machines={machines}
        stats={stats}
        onAction={(step) => {
          if (step === "password") {
            onOpenAccount?.();
          } else if (step === "machine" || step === "node") {
            history.push("/machines");
          } else if (step === "app") {
            history.push("/app-store");
          }
        }}
      />
      {!clusterHealthy ? (
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
      ) : (
        <Alert variant="success">
          <CheckCircle2 />
          <AlertTitle>{t("dashboard:alert healthy")}</AlertTitle>
        </Alert>
      )}

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
            <EchartsWidget option={podsByNamespace} style={{height: 280}} />
          </CardContent>
        </Card>

        <Card className="gap-3 py-4">
          <CardHeader className="px-4">
            <CardTitle className="text-sm">{t("dashboard:chart Pod phase")}</CardTitle>
          </CardHeader>
          <CardContent className="px-2">
            <EchartsWidget option={podPhase} style={{height: 280}} />
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
            <EchartsWidget option={serviceTypes} style={{height: 200}} />
          </CardContent>
        </Card>
      </div>

      <Card className="gap-3 py-4">
        <CardHeader className="px-4">
          <CardTitle className="text-sm">{t("dashboard:chart Node infra")}</CardTitle>
        </CardHeader>
        <CardContent className="px-2">
          <EchartsWidget option={nodeInfra} style={{height: 220}} />
        </CardContent>
      </Card>
    </PageContainer>
  );
}

export default DashboardPage;
