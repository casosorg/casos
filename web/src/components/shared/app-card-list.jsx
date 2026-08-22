import React from "react";
import {useTranslation} from "react-i18next";
import {CircleArrowUp, ExternalLink, Globe, HardDrive, ScrollText, Trash2} from "lucide-react";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Checkbox} from "@/components/ui/checkbox";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {EmptyState} from "@/components/shared/empty-state";
import {AiDots, Loading} from "@/components/shared/loading";
import {appTileColor} from "@/lib/appCatalog";
import {formatBytes, parseQuantity} from "@/lib/quantity";
import {cn} from "@/lib/utils";

// "8Gi" is a Kubernetes quantity, not something to show a reader who just
// wanted to know how big the disk is.
function formatDiskSize(storage) {
  const {value} = parseQuantity(storage);
  return value === null ? storage || "?" : formatBytes(value);
}

const STATUS_TONE = {
  deployed: {dot: "bg-success", label: "simple:Running"},
  failed: {dot: "bg-destructive", label: "simple:Not working"},
  superseded: {dot: "bg-muted-foreground", label: "simple:Replaced"},
  uninstalling: {dot: "bg-info", label: "simple:Removing"},
};

function StatusLine({status, pending}) {
  const {t} = useTranslation();
  const tone = STATUS_TONE[status];
  return (
    <span className="flex items-center gap-1.5 text-xs">
      <span className={cn("size-2 rounded-full", tone?.dot ?? "bg-warning")} />
      {tone ? t(tone.label) : status}
      {pending ? <AiDots size="small" /> : null}
    </span>
  );
}

/**
 * One card per installed app, carrying the two things someone actually wants
 * from an app they installed: the address it answers on, and the disks holding
 * its data. Neither is a page of its own in simple mode — a reader thinks "where
 * is my Nextcloud", not "show me every Service in the cluster".
 */
function AppCard({release, resources, pending, onOpenLogs, onUpgrade, onUninstall, deleteData, onDeleteDataChange}) {
  const {t} = useTranslation();
  const chartName = release.chartName || release.chart || release.name;
  const primaryUrl = resources.urls[0] ?? null;

  return (
    <div className="bg-card flex h-full flex-col gap-3 rounded-xl border p-4 shadow-sm">
      <div className="flex items-start gap-3">
        <span
          className="flex size-11 shrink-0 items-center justify-center rounded-xl text-lg font-semibold text-white"
          style={{backgroundColor: appTileColor(chartName)}}
          aria-hidden="true"
        >
          {(chartName || release.name)[0].toUpperCase()}
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold">{release.name}</div>
          <div className="text-muted-foreground mt-0.5 truncate text-xs">{chartName}</div>
        </div>
        <StatusLine status={release.status} pending={pending} />
      </div>

      <div className="grid gap-2 border-t pt-3">
        <div className="flex items-start gap-2">
          <Globe className="text-muted-foreground mt-0.5 size-4 shrink-0" />
          {primaryUrl ? (
            <div className="min-w-0 flex-1">
              <a
                href={primaryUrl}
                target="_blank"
                rel="noreferrer"
                className="text-info block truncate text-xs hover:underline"
              >
                {primaryUrl}
              </a>
              {resources.urls.length > 1 ? (
                <span className="text-muted-foreground text-[11px]">
                  {t("simple:and {{n}} more addresses", {n: resources.urls.length - 1})}
                </span>
              ) : null}
            </div>
          ) : (
            <span className="text-muted-foreground flex-1 text-xs">{t("simple:No web address yet")}</span>
          )}
        </div>

        <div className="flex items-start gap-2">
          <HardDrive className="text-muted-foreground mt-0.5 size-4 shrink-0" />
          {resources.disks.length > 0 ? (
            <div className="flex min-w-0 flex-1 flex-wrap gap-1">
              {resources.disks.map((disk) => (
                <Badge key={disk.name} variant={disk.status === "Bound" ? "muted" : "warning"} title={disk.name}>
                  {formatDiskSize(disk.storage)}
                </Badge>
              ))}
            </div>
          ) : (
            <span className="text-muted-foreground flex-1 text-xs">{t("simple:Keeps no data of its own")}</span>
          )}
        </div>
      </div>

      <div className="mt-auto grid gap-2 pt-1">
        {primaryUrl ? (
          <Button asChild className="w-full">
            <a href={primaryUrl} target="_blank" rel="noreferrer">
              {t("simple:Open")}
              <ExternalLink />
            </a>
          </Button>
        ) : null}
        <div className="flex justify-end gap-1">
          <Button variant="ghost" size="sm" onClick={() => onUpgrade(release)}>
            <CircleArrowUp />
            {t("simple:Update")}
          </Button>
          <Button variant="ghost" size="sm" onClick={() => onOpenLogs(release)}>
            <ScrollText />
            {t("simple:Logs")}
          </Button>
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
            <Button variant="ghost" size="sm" className="text-destructive">
              <Trash2 />
              {t("simple:Remove")}
            </Button>
          </ConfirmDialog>
        </div>
      </div>
    </div>
  );
}

export function AppCardList({
  releases,
  resources,
  loading,
  isPending,
  onInstallMore,
  deleteDataFor,
  onDeleteDataChange,
  onOpenLogs,
  onUpgrade,
  onUninstall,
}) {
  const {t} = useTranslation();

  if (loading && releases.length === 0) {
    return <Loading />;
  }

  if (releases.length === 0) {
    return (
      <EmptyState
        title={t("simple:You have not installed anything yet")}
        description={t("simple:Everything you install from the App Store shows up here.")}
        action={<Button onClick={onInstallMore}>{t("simple:Install an app")}</Button>}
      />
    );
  }

  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
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
            onUninstall={onUninstall}
          />
        );
      })}
    </div>
  );
}
