import React from "react";
import {useHistory} from "react-router-dom";
import {useTranslation} from "react-i18next";
import {Activity, ArrowRight, BarChart3, Boxes, CheckCircle2, ChevronRight, Laptop, Store, TriangleAlert} from "lucide-react";
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
import {Progress} from "@/components/ui/progress";
import {PageContainer} from "@/components/shared/page-header";
import {getDashboardHealthState} from "@/lib/dashboardHealth";
import {useUiMode} from "@/hooks/use-ui-mode";
import {cn} from "@/lib/utils";

function ActionCard({icon: Icon, title, description, onClick}) {
  return (
    <Card
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onClick?.();
        }
      }}
      className="hover:border-ring/60 focus-visible:border-ring focus-visible:ring-ring/50 cursor-pointer transition-colors outline-none focus-visible:ring-[3px]"
    >
      <CardHeader>
        <span className="bg-muted text-foreground mb-1 flex size-9 items-center justify-center rounded-lg">
          <Icon className="size-4.5" />
        </span>
        <CardTitle className="text-base">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
        <CardAction>
          <ChevronRight className="text-muted-foreground size-4" />
        </CardAction>
      </CardHeader>
    </Card>
  );
}

function CountCard({icon: Icon, value, label, caption, onClick}) {
  return (
    <Card
      onClick={onClick}
      className="@container/card from-primary/5 to-card dark:bg-card hover:border-ring/60 cursor-pointer gap-0 bg-gradient-to-t transition-colors"
    >
      <CardHeader>
        <CardDescription>{label}</CardDescription>
        <CardTitle className="text-3xl font-semibold tracking-tight tabular-nums">{value}</CardTitle>
        <CardAction>
          <span className="bg-muted text-muted-foreground flex size-8 items-center justify-center rounded-lg">
            <Icon className="size-4" />
          </span>
        </CardAction>
      </CardHeader>
      <CardFooter className="text-muted-foreground gap-1 pt-4 text-sm">
        {caption}
        <ChevronRight className="size-4" />
      </CardFooter>
    </Card>
  );
}

function UsageRow({label, percent, detail}) {
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

/**
 * Simple mode's home page. It answers the three questions a non-technical
 * reader actually has — is anything broken, what do I have, what do I do next —
 * and leaves the stat tiles and the charts to the advanced dashboard.
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

  // The status card keeps the neutral card surface and lets one tinted chip
  // carry the state, rather than washing the whole panel in a colour.
  const iconClass = {
    success: "border-success/25 bg-success/10 text-success",
    warning: "border-warning/30 bg-warning/12 text-warning",
    danger: "border-destructive/25 bg-destructive/10 text-destructive",
  }[statusTone];

  const badgeVariant = {success: "success", warning: "warning", danger: "danger"}[statusTone];
  const badgeLabel = {
    success: t("simple:Running"),
    warning: t("simple:Needs attention"),
    danger: t("simple:Not working"),
  }[statusTone];

  const StatusIcon = statusIcon;
  const cpuTotal = stats?.clusterCPUTotalM ?? 0;
  const memTotal = stats?.clusterMemTotalMi ?? 0;
  const cpuPercent = cpuTotal > 0 ? Math.min(100, Math.round((stats.clusterCPUUsedM / cpuTotal) * 100)) : 0;
  const memPercent = memTotal > 0 ? Math.min(100, Math.round((stats.clusterMemUsedMi / memTotal) * 100)) : 0;

  return (
    <PageContainer>
      {checklist}

      <Card>
        <CardHeader>
          <span className={cn("mb-1 flex size-11 items-center justify-center rounded-xl border", iconClass)}>
            <StatusIcon className="size-5.5" />
          </span>
          <CardTitle className="text-xl">{statusTitle}</CardTitle>
          <CardDescription className="text-base">{statusText}</CardDescription>
          <CardAction>
            <Badge variant={badgeVariant}>{badgeLabel}</Badge>
          </CardAction>
        </CardHeader>
        {statusAction ? (
          <CardFooter>
            <Button onClick={() => history.push(statusAction.to)}>
              {statusAction.label}
              <ArrowRight />
            </Button>
          </CardFooter>
        ) : null}
      </Card>

      <div className="grid gap-4 sm:grid-cols-3">
        <CountCard
          icon={Boxes}
          value={appCount ?? "—"}
          label={t("simple:Installed apps")}
          caption={t("simple:My Apps")}
          onClick={() => history.push("/simple/apps")}
        />
        <CountCard
          icon={Laptop}
          value={deviceCount}
          label={t("simple:Computers")}
          caption={t("simple:Devices")}
          onClick={() => history.push("/simple/devices")}
        />
        <CountCard
          icon={unhealthyPods.length > 0 ? TriangleAlert : CheckCircle2}
          value={unhealthyPods.length}
          label={t("simple:Apps needing attention")}
          caption={t("simple:Health")}
          onClick={() => history.push("/simple/health")}
        />
      </div>

      {cpuTotal > 0 || memTotal > 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>{t("simple:Usage")}</CardTitle>
            <CardDescription>{t("simple:How much processing power and memory each app is using.")}</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-6 md:grid-cols-2">
            <UsageRow
              label={t("simple:Processing power in use")}
              percent={cpuPercent}
              detail={`${(stats.clusterCPUUsedM / 1000).toFixed(1)} / ${(cpuTotal / 1000).toFixed(1)} ${t("simple:cores")}`}
            />
            <UsageRow
              label={t("simple:Memory in use")}
              percent={memPercent}
              detail={`${(stats.clusterMemUsedMi / 1024).toFixed(1)} / ${(memTotal / 1024).toFixed(1)} GB`}
            />
          </CardContent>
        </Card>
      ) : null}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
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
