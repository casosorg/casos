import React from "react";
import {useHistory} from "react-router-dom";
import {useTranslation} from "react-i18next";
import {Activity, ArrowRight, BarChart3, Boxes, CheckCircle2, Laptop, Store, TriangleAlert} from "lucide-react";
import {Button} from "@/components/ui/button";
import {Card, CardContent} from "@/components/ui/card";
import {Progress} from "@/components/ui/progress";
import {PageContainer} from "@/components/shared/page-header";
import {getDashboardHealthState} from "@/lib/dashboardHealth";
import {useUiMode} from "@/hooks/use-ui-mode";
import {cn} from "@/lib/utils";

function ActionCard({icon: Icon, title, description, onClick}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="bg-card hover:border-ring/50 hover:bg-accent/40 flex items-start gap-3 rounded-xl border p-4 text-left shadow-sm transition-colors"
    >
      <span className="bg-primary/10 text-primary flex size-10 shrink-0 items-center justify-center rounded-lg">
        <Icon className="size-5" />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block text-sm font-semibold">{title}</span>
        <span className="text-muted-foreground mt-0.5 block text-xs leading-relaxed">{description}</span>
      </span>
      <ArrowRight className="text-muted-foreground mt-2.5 size-4 shrink-0" />
    </button>
  );
}

function CountCard({icon: Icon, value, label, onClick}) {
  return (
    <Card className="cursor-pointer py-4 transition-colors hover:border-ring/50" onClick={onClick}>
      <CardContent className="flex items-center gap-3 px-4">
        <Icon className="text-muted-foreground size-5 shrink-0" />
        <div className="min-w-0">
          <div className="text-2xl leading-none font-semibold tabular-nums">{value}</div>
          <div className="text-muted-foreground mt-1 text-xs">{label}</div>
        </div>
      </CardContent>
    </Card>
  );
}

function UsageBar({label, used, total, unit, tone}) {
  const percent = total > 0 ? Math.min(100, Math.round((used / total) * 100)) : 0;
  return (
    <div className="grid gap-1.5">
      <div className="flex items-baseline justify-between gap-2">
        <span className="text-sm font-medium">{label}</span>
        <span className="text-muted-foreground text-xs tabular-nums">
          {percent}% · {used} / {total} {unit}
        </span>
      </div>
      <Progress value={percent} tone={tone} />
    </div>
  );
}

/**
 * Simple mode's home page. It answers the three questions a non-technical
 * reader actually has — is anything broken, what do I have, what do I do next —
 * and leaves the eight stat tiles and five charts to the advanced dashboard.
 */
function SimpleHome({stats, releases, machines, checklist}) {
  const history = useHistory();
  const {t} = useTranslation();
  const {switchMode} = useUiMode();

  const {healthStatus, needsNodes} = getDashboardHealthState(stats);
  const unhealthyPods = Array.isArray(stats?.unhealthyPods) ? stats.unhealthyPods : [];
  const appCount = Array.isArray(releases) ? releases.length : null;
  const deviceCount = Array.isArray(machines) ? machines.length : stats?.nodesTotal ?? 0;

  let statusTone = "success";
  let statusIcon = CheckCircle2;
  let statusTitle = t("simple:Everything is running");
  let statusText = t("simple:All your apps are working normally.");
  let statusAction = null;

  if (needsNodes) {
    statusTone = "warning";
    statusIcon = Laptop;
    statusTitle = t("simple:No computer has joined yet");
    statusText = t("simple:Add a computer before you install anything — that is where your apps will run.");
    statusAction = {label: t("simple:Add a computer"), to: "/simple/devices"};
  } else if (healthStatus === "unknown") {
    statusTone = "warning";
    statusIcon = TriangleAlert;
    statusTitle = t("simple:Cannot check right now");
    statusText = t("simple:CasOS could not read the cluster status. Try again in a moment.");
  } else if (healthStatus === "unhealthy") {
    statusTone = "danger";
    statusIcon = TriangleAlert;
    statusTitle = unhealthyPods.length > 0
      ? t("simple:{{n}} apps are not working", {n: unhealthyPods.length})
      : t("simple:Something is not healthy");
    statusText = t("simple:Open Health to see what went wrong and read the messages.");
    statusAction = {label: t("simple:See what is wrong"), to: "/simple/health"};
  }

  const toneClass = {
    success: "border-success/40 bg-success/5 text-success",
    warning: "border-warning/40 bg-warning/5 text-warning",
    danger: "border-destructive/40 bg-destructive/5 text-destructive",
  }[statusTone];

  const StatusIcon = statusIcon;
  const cpuTotal = stats?.clusterCPUTotalM ?? 0;
  const memTotal = stats?.clusterMemTotalMi ?? 0;

  return (
    <PageContainer>
      {checklist}

      <Card className={cn("py-5", toneClass)}>
        <CardContent className="flex flex-col gap-4 px-5 sm:flex-row sm:items-center">
          <StatusIcon className="size-10 shrink-0" />
          <div className="min-w-0 flex-1">
            <h2 className="text-xl font-semibold">{statusTitle}</h2>
            <p className="text-foreground/70 mt-1 text-sm">{statusText}</p>
          </div>
          {statusAction ? (
            <Button onClick={() => history.push(statusAction.to)}>
              {statusAction.label}
              <ArrowRight />
            </Button>
          ) : null}
        </CardContent>
      </Card>

      <div className="grid gap-3 sm:grid-cols-3">
        <CountCard
          icon={Boxes}
          value={appCount ?? "—"}
          label={t("simple:Installed apps")}
          onClick={() => history.push("/simple/apps")}
        />
        <CountCard
          icon={Laptop}
          value={deviceCount}
          label={t("simple:Computers")}
          onClick={() => history.push("/simple/devices")}
        />
        <CountCard
          icon={unhealthyPods.length > 0 ? TriangleAlert : CheckCircle2}
          value={unhealthyPods.length}
          label={t("simple:Apps needing attention")}
          onClick={() => history.push("/simple/health")}
        />
      </div>

      {cpuTotal > 0 || memTotal > 0 ? (
        <Card className="py-4">
          <CardContent className="grid gap-4 px-4 md:grid-cols-2">
            <UsageBar
              label={t("simple:Processing power in use")}
              used={(stats.clusterCPUUsedM / 1000).toFixed(1)}
              total={(cpuTotal / 1000).toFixed(1)}
              unit={t("simple:cores")}
              tone="info"
            />
            <UsageBar
              label={t("simple:Memory in use")}
              used={Math.round(stats.clusterMemUsedMi / 1024)}
              total={Math.round(memTotal / 1024)}
              unit="GB"
              tone="success"
            />
          </CardContent>
        </Card>
      ) : null}

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <ActionCard
          icon={Store}
          title={t("simple:Install an app")}
          description={t("simple:Pick something from the App Store and CasOS sets it up for you.")}
          onClick={() => history.push("/simple/app-store")}
        />
        <ActionCard
          icon={Laptop}
          title={t("simple:Add a computer")}
          description={t("simple:Connect another machine so there is more room to run apps.")}
          onClick={() => history.push("/simple/devices")}
        />
        <ActionCard
          icon={Activity}
          title={t("simple:Something is broken")}
          description={t("simple:Check usage and read the messages your apps wrote.")}
          onClick={() => history.push("/simple/health")}
        />
        <ActionCard
          icon={BarChart3}
          title={t("simple:Show me everything")}
          description={t("simple:Switch to advanced mode for the full dashboard and every Kubernetes page.")}
          onClick={() => switchMode("advanced")}
        />
      </div>
    </PageContainer>
  );
}

export default SimpleHome;
