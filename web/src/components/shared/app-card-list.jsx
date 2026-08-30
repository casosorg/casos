import React from "react";
import {useTranslation} from "react-i18next";
import {CircleArrowUp, ExternalLink, Globe, HardDrive, Play, ScrollText, SearchX, Square, Trash2} from "lucide-react";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Checkbox} from "@/components/ui/checkbox";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {AppIcon} from "@/components/shared/app-icon";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {EmptyState} from "@/components/shared/empty-state";
import {AiDots, Loading} from "@/components/shared/loading";
import {formatBytes, parseQuantity} from "@/lib/quantity";
import {cn} from "@/lib/utils";

// "8Gi" is a Kubernetes quantity, not something to show a reader who just
// wanted to know how big the disk is.
function formatDiskSize(storage) {
  const {value} = parseQuantity(storage);
  return value === null ? storage || "?" : formatBytes(value);
}

// The scheme is what the link is for; on a card it only costs width.
function displayUrl(url) {
  return url.replace(/^https?:\/\//, "");
}

const STATUS_TONE = {
  deployed: {label: "simple:Running", dot: "bg-success", pill: "border-success/25 bg-success/10 text-success", bar: "bg-success/70"},
  stopped: {label: "simple:Stopped", dot: "bg-muted-foreground", pill: "border-border bg-muted text-muted-foreground", bar: "bg-border"},
  failed: {label: "simple:Not working", dot: "bg-destructive", pill: "border-destructive/25 bg-destructive/10 text-destructive", bar: "bg-destructive/70"},
  superseded: {label: "simple:Replaced", dot: "bg-muted-foreground", pill: "border-border bg-muted text-muted-foreground", bar: "bg-border"},
  uninstalling: {label: "simple:Removing", dot: "bg-info", pill: "border-info/25 bg-info/10 text-info", bar: "bg-info/70"},
};

const PENDING_TONE = {
  label: "simple:Updating",
  dot: "bg-warning",
  pill: "border-warning/30 bg-warning/10 text-warning",
  bar: "bg-warning/70",
};

function toneOf(status, pending) {
  return pending ? PENDING_TONE : (STATUS_TONE[status] ?? PENDING_TONE);
}

function StatusPill({status, pending}) {
  const {t} = useTranslation();
  const tone = toneOf(status, pending);
  const known = pending || STATUS_TONE[status];
  return (
    <span className={cn("inline-flex shrink-0 items-center gap-1.5 rounded-full border px-2 py-0.5 text-[11px] font-medium", tone.pill)}>
      <span className={cn("size-1.5 rounded-full", tone.dot)} />
      {known ? t(tone.label) : status}
      {pending ? <AiDots size="small" className="ml-0.5" /> : null}
    </span>
  );
}

// One line of the card's detail block: an icon, and whatever the app has of
// that kind — an address, a disk — or a plain sentence saying it has none.
function DetailRow({icon: Icon, empty, children, hasValue}) {
  return (
    <div className="bg-muted/40 flex min-h-8 items-center gap-2 rounded-lg px-2.5 py-1.5">
      <Icon className={cn("size-3.5 shrink-0", hasValue ? "text-muted-foreground" : "text-muted-foreground/50")} />
      {hasValue ? children : <span className="text-muted-foreground/70 truncate text-xs">{empty}</span>}
    </div>
  );
}

/**
 * One card per installed app, carrying the two things someone actually wants
 * from an app they installed: the address it answers on, and the disks holding
 * its data. Neither is a page of its own in simple mode — a reader thinks "where
 * is my Nextcloud", not "show me every Service in the cluster".
 */
function AppCard({release, resources, pending, onOpenLogs, onUpgrade, onToggleRunning, onUninstall, deleteData, onDeleteDataChange}) {
  const {t} = useTranslation();
  const chartName = release.chartName || release.chart || release.name;
  const primaryUrl = resources.urls[0] ?? null;
  const tone = toneOf(release.status, pending);

  return (
    <div
      data-testid="app-card"
      className="group bg-card hover:border-ring/40 relative flex h-full flex-col overflow-hidden rounded-xl border shadow-sm transition-all duration-200 hover:-translate-y-0.5 hover:shadow-md"
    >
      {/* A card only wears the accent stripe when something is off: sixteen
          green stripes would decorate the grid without saying anything. */}
      {release.status === "deployed" && !pending ? null : (
        <span aria-hidden="true" className={cn("absolute inset-x-0 top-0 h-1", tone.bar)} />
      )}

      <div className="flex items-start gap-3 px-4 pt-5 pb-3">
        <AppIcon src={release.icon} chartName={release.chartName} name={release.name} />
        <div className="min-w-0 flex-1">
          <div className="truncate text-[15px] leading-tight font-semibold" title={release.name}>
            {release.name}
          </div>
          <div className="text-muted-foreground mt-1 flex min-w-0 items-center gap-1.5 text-xs">
            <span className="truncate" title={`${chartName} · ${release.namespace}`}>
              {chartName}
            </span>
            {release.chartVersion ? <span className="shrink-0 font-mono text-[11px] opacity-70">{release.chartVersion}</span> : null}
          </div>
        </div>
        <StatusPill status={release.status} pending={pending} />
      </div>

      <div className="grid gap-1.5 px-4 pb-4">
        <DetailRow icon={Globe} hasValue={Boolean(primaryUrl)} empty={t("simple:No web address yet")}>
          <SimpleTooltip title={primaryUrl} className="max-w-sm">
            <a
              href={primaryUrl}
              target="_blank"
              rel="noreferrer"
              className="text-info min-w-0 flex-1 truncate text-xs hover:underline"
            >
              {primaryUrl ? displayUrl(primaryUrl) : null}
            </a>
          </SimpleTooltip>
          {resources.urls.length > 1 ? (
            <SimpleTooltip title={resources.urls.slice(1).join(", ")} className="max-w-sm">
              {/* Badge does not forward a ref, so the tooltip anchors to a span of its own. */}
              <span className="shrink-0">
                <Badge variant="muted" className="px-1.5 text-[10px]">
                  +{resources.urls.length - 1}
                </Badge>
              </span>
            </SimpleTooltip>
          ) : null}
        </DetailRow>

        <DetailRow icon={HardDrive} hasValue={resources.disks.length > 0} empty={t("simple:Keeps no data of its own")}>
          <div className="flex min-w-0 flex-1 flex-wrap gap-1">
            {resources.disks.map((disk) => (
              <SimpleTooltip key={disk.name} title={disk.name}>
                <span>
                  <Badge variant={disk.status === "Bound" ? "muted" : "warning"} className="px-1.5 text-[10px]">
                    {formatDiskSize(disk.storage)}
                  </Badge>
                </span>
              </SimpleTooltip>
            ))}
          </div>
        </DetailRow>
      </div>

      <div className={cn("bg-muted/30 mt-auto flex items-center gap-1 border-t px-3 py-2.5", !primaryUrl && "justify-end")}>
        {primaryUrl ? (
          <Button asChild size="sm" className="flex-1">
            <a href={primaryUrl} target="_blank" rel="noreferrer">
              {t("simple:Open")}
              <ExternalLink />
            </a>
          </Button>
        ) : null}
        {release.kind === "image" ? (
          <SimpleTooltip title={release.status === "stopped" ? t("general:Start") : t("general:Stop")}>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => onToggleRunning(release)}
              aria-label={release.status === "stopped" ? t("general:Start") : t("general:Stop")}
            >
              {release.status === "stopped" ? <Play /> : <Square />}
            </Button>
          </SimpleTooltip>
        ) : null}
        <SimpleTooltip title={t("simple:Update")}>
          <Button variant="ghost" size="icon-sm" onClick={() => onUpgrade(release)} aria-label={t("simple:Update")}>
            <CircleArrowUp />
          </Button>
        </SimpleTooltip>
        <SimpleTooltip title={t("general:Logs")}>
          <Button variant="ghost" size="icon-sm" onClick={() => onOpenLogs(release)} aria-label={t("general:Logs")}>
            <ScrollText />
          </Button>
        </SimpleTooltip>
        <ConfirmDialog
          title={t("simple:Remove {{name}}?", {name: release.name})}
          description={t("simple:The app stops running and disappears from this list.")}
          extra={
            <label className="hover:bg-accent/50 flex cursor-pointer items-start gap-2 rounded-md px-2 py-1.5 text-sm">
              <Checkbox className="mt-0.5" checked={deleteData} onCheckedChange={(checked) => onDeleteDataChange(checked === true)} />
              <span>
                {t("simple:Also delete its data")}
                <span className="text-muted-foreground block text-xs">
                  {t("simple:Leave this off and the data is kept, ready for when you install it again.")}
                </span>
              </span>
            </label>
          }
          confirmText={t("general:Delete")}
          cancelText={t("general:Cancel")}
          onConfirm={() => onUninstall(release)}
        >
          <Button
            variant="ghost"
            size="icon-sm"
            className="text-muted-foreground hover:text-destructive hover:bg-destructive/10"
            aria-label={t("general:Remove")}
          >
            <Trash2 />
          </Button>
        </ConfirmDialog>
      </div>
    </div>
  );
}

export function AppCardList({
  releases,
  resources,
  loading,
  isPending,
  emptyTitle,
  emptyDescription,
  emptyAction,
  onInstallMore,
  deleteDataFor,
  onDeleteDataChange,
  onOpenLogs,
  onUpgrade,
  onToggleRunning,
  onUninstall,
}) {
  const {t} = useTranslation();

  if (loading && releases.length === 0) {
    return <Loading />;
  }

  // A caller that names its own empty state — "nothing matched that search" —
  // replaces all three parts of it, rather than inheriting a description and a
  // button written for the reader who has installed nothing at all.
  if (releases.length === 0) {
    return emptyTitle === undefined ? (
      <EmptyState
        title={t("simple:You have not installed anything yet")}
        description={t("simple:Everything you install from the App Store shows up here.")}
        action={<Button onClick={onInstallMore}>{t("simple:Install an app")}</Button>}
      />
    ) : (
      <EmptyState icon={SearchX} title={emptyTitle} description={emptyDescription} action={emptyAction} />
    );
  }

  return (
    <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
      {releases.map((release) => {
        const key = `${release.namespace}/${release.name}`;
        return (
          <AppCard
            key={key}
            release={release}
            resources={resources(release)}
            pending={isPending(release.status)}
            deleteData={deleteDataFor[key] ?? false}
            onDeleteDataChange={(checked) => onDeleteDataChange(release, checked)}
            onOpenLogs={onOpenLogs}
            onUpgrade={onUpgrade}
            onToggleRunning={onToggleRunning}
            onUninstall={onUninstall}
          />
        );
      })}
    </div>
  );
}
