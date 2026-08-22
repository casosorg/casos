import React, {useState} from "react";
import {useHistory} from "react-router-dom";
import {useTranslation} from "react-i18next";
import {Check, Rocket, Search, Store} from "lucide-react";
import * as HelmBackend from "@/backend/HelmBackend";
import {useResource} from "@/hooks/use-resource";
import {useUiMode} from "@/hooks/use-ui-mode";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {HelmInstallDialog} from "@/components/shared/helm-install-dialog";
import {APP_CATALOG, APP_CATEGORIES, appTileColor} from "@/lib/appCatalog";
import {cn} from "@/lib/utils";

function AppTile({app}) {
  return (
    <span
      className="flex size-11 shrink-0 items-center justify-center rounded-xl text-lg font-semibold text-white"
      style={{backgroundColor: appTileColor(app.chartName)}}
      aria-hidden="true"
    >
      {app.name[0].toUpperCase()}
    </span>
  );
}

function AppCard({app, installed, onInstall}) {
  const {t} = useTranslation();
  return (
    <div className="bg-card hover:border-ring/50 flex h-full flex-col gap-3 rounded-xl border p-4 shadow-sm transition-colors">
      <div className="flex items-start gap-3">
        <AppTile app={app} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="truncate text-sm font-semibold">{app.name}</span>
            {installed ? (
              <Badge variant="success" className="gap-1">
                <Check className="size-3" />
                {t("simple:Installed")}
              </Badge>
            ) : null}
          </div>
          <p className="text-muted-foreground mt-1 text-xs leading-relaxed">{t(app.description)}</p>
        </div>
      </div>
      <Button size="sm" variant={installed ? "outline" : "default"} className="mt-auto self-end" onClick={onInstall}>
        <Rocket />
        {installed ? t("simple:Install again") : t("simple:Install")}
      </Button>
    </div>
  );
}

/**
 * Simple mode's App Store: a short curated list instead of a search across
 * every chart on ArtifactHub. Picking from it is one click, and the reader
 * never has to know what a repository or a chart version is. The way out to
 * the full catalogue is at the bottom of the page.
 */
function SimpleAppStore() {
  const history = useHistory();
  const {t} = useTranslation();
  const {setMode} = useUiMode();
  const [category, setCategory] = useState("all");
  const [query, setQuery] = useState("");
  const [installTarget, setInstallTarget] = useState(null);

  const {data: releases, refresh} = useResource(() => HelmBackend.getHelmReleases(), [], {
    initialData: [],
    toastOnError: false,
  });
  const installedCharts = new Set((releases ?? []).map((release) => release.chartName).filter(Boolean));

  const needle = query.trim().toLowerCase();
  const visibleApps = APP_CATALOG.filter((app) => {
    if (category !== "all" && app.category !== category) {
      return false;
    }
    if (!needle) {
      return true;
    }
    return app.name.toLowerCase().includes(needle) || t(app.description).toLowerCase().includes(needle);
  });

  return (
    <PageContainer>
      <PageHeader
        title={t("simple:App Store")}
        description={t("simple:Pick an app and CasOS installs and configures it for you.")}
        actions={
          <Button variant="outline" onClick={() => history.push("/helm-releases")}>
            {t("simple:My Apps")}
          </Button>
        }
      />

      <div className="relative max-w-sm">
        <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
        <Input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder={t("simple:Search apps")}
          className="pl-9"
        />
      </div>

      <div className="flex flex-wrap gap-2">
        {APP_CATEGORIES.map((item) => (
          <button
            key={item.key}
            type="button"
            onClick={() => setCategory(item.key)}
            className={cn(
              "rounded-full border px-3 py-1.5 text-sm transition-colors",
              category === item.key
                ? "bg-primary text-primary-foreground border-primary font-medium"
                : "hover:bg-accent text-muted-foreground"
            )}
          >
            {t(item.label)}
          </button>
        ))}
      </div>

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
        {visibleApps.map((app) => (
          <AppCard
            key={app.chartName}
            app={app}
            installed={installedCharts.has(app.chartName)}
            onInstall={() => setInstallTarget({chartName: app.chartName, repoURL: app.repoURL, version: "", displayName: app.name})}
          />
        ))}
      </div>

      {visibleApps.length === 0 ? (
        <p className="text-muted-foreground py-12 text-center text-sm">{t("simple:No app here matches that.")}</p>
      ) : null}

      <div className="bg-muted/40 flex flex-col items-center gap-2 rounded-xl border border-dashed p-6 text-center">
        <Store className="text-muted-foreground size-6" />
        <p className="text-sm font-medium">{t("simple:Looking for something else?")}</p>
        <p className="text-muted-foreground max-w-md text-xs">
          {t("simple:Advanced mode opens the full catalogue with thousands of apps, custom repositories and every install option.")}
        </p>
        <Button variant="outline" size="sm" className="mt-1" onClick={() => setMode("advanced")}>
          {t("simple:Browse all apps")}
        </Button>
      </div>

      <HelmInstallDialog
        open={Boolean(installTarget)}
        chart={installTarget}
        onClose={() => setInstallTarget(null)}
        onInstalled={() => {
          setInstallTarget(null);
          refresh();
        }}
      />
    </PageContainer>
  );
}

export default SimpleAppStore;
