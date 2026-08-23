import React, {useCallback, useEffect, useRef, useState} from "react";
import {Link} from "react-router-dom";
import {useTranslation} from "react-i18next";
import {Plus, RefreshCw, Rocket, Search, Store, Trash2} from "lucide-react";
import * as HelmBackend from "@/backend/HelmBackend";
import * as Setting from "@/Setting";
import {runAction} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {MessageAlert} from "@/components/ui/alert";
import {Separator} from "@/components/ui/separator";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {cn} from "@/lib/utils";
import {AppIcon} from "@/components/shared/app-icon";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {Loading} from "@/components/shared/loading";
import {HelmInstallDialog} from "@/components/shared/helm-install-dialog";
import {useUiMode} from "@/hooks/use-ui-mode";
import SimpleAppStore from "@/pages/simple/SimpleAppStore";

const PRESET_REPOS = [
  {name: "ArtifactHub", url: null, desc: "artifacthub.io — 8 000+ charts"},
  {name: "Bitnami", url: "https://charts.bitnami.com/bitnami", desc: "~200 curated charts"},
  {name: "Rancher", url: "https://charts.rancher.io", desc: "Rancher Charts"},
  {name: "ingress-nginx", url: "https://kubernetes.github.io/ingress-nginx", desc: "Official ingress-nginx"},
];

const ARTIFACT_HUB_PAGE_SIZE = 20;

function ChartCard({chart, onInstall}) {
  const {t} = useTranslation();
  return (
    <div data-testid="chart-card" className="bg-card hover:border-ring/50 flex gap-3 rounded-xl border p-3 shadow-sm transition-colors">
      <AppIcon src={chart.icon} chartName={chart.chartName} name={chart.displayName} />
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <div className="flex items-start gap-2">
          <SimpleTooltip title={chart.displayName} className="max-w-xs">
            <span className="min-w-0 flex-1 truncate text-sm font-semibold">{chart.displayName}</span>
          </SimpleTooltip>
          {chart.version ? <Badge variant="muted">{chart.version}</Badge> : null}
        </div>
        {/* The card only has room for two lines, so the full text lives in a tooltip. */}
        <SimpleTooltip title={chart.description} className="max-w-sm text-left text-wrap">
          <p className="text-muted-foreground line-clamp-2 text-xs leading-relaxed">{chart.description || ""}</p>
        </SimpleTooltip>
        <div className="flex justify-end">
          <Button size="sm" onClick={onInstall}>
            <Rocket />
            {t("helm:Install")}
          </Button>
        </div>
      </div>
    </div>
  );
}

function AddRepoDialog({open, onClose, onAdded}) {
  const {t} = useTranslation();
  const [form, setForm] = useState({name: "", url: ""});
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (open) {
      setForm({name: "", url: ""});
      setErrors({});
    }
  }, [open]);

  async function handleSubmit() {
    const nextErrors = {};
    if (!form.name) {
      nextErrors.name = t("policy:required");
    }
    if (!form.url) {
      nextErrors.url = t("policy:required");
    } else if (!/^(https?|oci):\/\//.test(form.url)) {
      // OCI registries (e.g. "oci://registry-1.docker.io/casbin/casdoor-helm-charts")
      // host charts just like a classic index.yaml repo, so they are valid here too.
      nextErrors.url = t("helm:Repo URL pattern");
    }
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) {
      return;
    }

    setSubmitting(true);
    const ok = await runAction(HelmBackend.addHelmRepo(form), {successMessage: t("helm:Add Helm Repo")});
    setSubmitting(false);
    if (ok) {
      onAdded();
      onClose();
    }
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={(next) => (next ? null : onClose())}
      title={t("helm:Add Helm Repo")}
      submitText={t("general:Add")}
      submitting={submitting}
      onSubmit={handleSubmit}
    >
      <Field label={t("helm:Repo name")} htmlFor="repo-name" required error={errors.name}>
        <Input
          id="repo-name"
          value={form.name}
          onChange={(event) => setForm((prev) => ({...prev, name: event.target.value}))}
          placeholder="my-charts"
        />
      </Field>
      <Field label={t("helm:Repo URL")} htmlFor="repo-url" required error={errors.url}>
        <Input
          id="repo-url"
          value={form.url}
          onChange={(event) => setForm((prev) => ({...prev, url: event.target.value}))}
          placeholder="https://example.com/charts"
        />
      </Field>
    </FormDialog>
  );
}

function AdvancedAppStore() {
  const {t} = useTranslation();
  const [source, setSource] = useState(PRESET_REPOS[0]);
  const [query, setQuery] = useState("");
  const [charts, setCharts] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [page, setPage] = useState(1);
  const [hasMore, setHasMore] = useState(true);
  const [customRepos, setCustomRepos] = useState([]);
  const [addRepoOpen, setAddRepoOpen] = useState(false);
  const [installTarget, setInstallTarget] = useState(null);
  const sentinelRef = useRef(null);

  // ArtifactHub is a search API with server-side paging; a plain repo returns its
  // whole index at once and is filtered in the browser.
  const isArtifactHub = !source.url;

  const loadCustomRepos = useCallback(() => {
    HelmBackend.getHelmRepos().then((res) => {
      if (res.status === "ok") {
        setCustomRepos(res.data ?? []);
      }
    });
  }, []);

  useEffect(() => {
    loadCustomRepos();
  }, [loadCustomRepos]);

  const fetchCharts = useCallback((activeSource, activeQuery, activePage) => {
    setLoading(true);
    setError(null);

    const remote = !activeSource.url;
    const request = remote
      ? HelmBackend.searchArtifactHub(activeQuery, activePage)
      : HelmBackend.getRepoCharts(activeSource.url);

    request
      .then((res) => {
        if (res.status !== "ok") {
          setError(res.msg);
          return;
        }
        const data = res.data ?? [];
        if (remote) {
          setCharts((previous) => (activePage === 1 ? data : [...previous, ...data]));
          setHasMore(data.length === ARTIFACT_HUB_PAGE_SIZE);
        } else {
          setCharts(data);
          setHasMore(false);
        }
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    setCharts([]);
    setPage(1);
    setHasMore(true);
    fetchCharts(source, query, 1);
  }, [source, query, fetchCharts]);

  useEffect(() => {
    if (page > 1) {
      fetchCharts(source, query, page);
    }
  }, [page, fetchCharts, source, query]);

  // Infinite scroll: the sentinel sits below the grid, so the next ArtifactHub
  // page is requested as soon as it scrolls close to the viewport. Re-running
  // once "loading" flips back to false re-arms the observer, which also keeps
  // paging when a short page does not fill the screen.
  useEffect(() => {
    if (!isArtifactHub || !hasMore || loading) {
      return;
    }
    const sentinel = sentinelRef.current;
    if (!sentinel) {
      return;
    }
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting) {
          observer.disconnect();
          setPage((previous) => previous + 1);
        }
      },
      {rootMargin: "400px"}
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [isArtifactHub, hasMore, loading, charts.length]);

  // ArtifactHub and a plain Helm index describe a chart with different field
  // names; normalising here keeps the card and the install dialog from each
  // having to know which source they came from.
  function normalize(chart) {
    if (isArtifactHub) {
      return {
        chartName: chart.name,
        repoURL: chart.repository?.url ?? "",
        artifactHubRepository: chart.repository?.name ?? "",
        version: chart.version ?? "",
        displayName: chart.display_name || chart.name,
        description: chart.description,
        icon: chart.logo_image_id ? `https://artifacthub.io/image/${chart.logo_image_id}` : chart.logo_url ?? null,
      };
    }
    return {
      chartName: chart.name,
      repoURL: source.url,
      version: chart.version ?? "",
      displayName: chart.name,
      description: chart.description,
      icon: chart.icon,
    };
  }

  const visibleCharts = (isArtifactHub
    ? charts
    : charts.filter((chart) => {
      const needle = query.toLowerCase();
      return (
        !needle ||
          (chart.name || "").toLowerCase().includes(needle) ||
          (chart.description || "").toLowerCase().includes(needle)
      );
    })
  ).map(normalize);

  function deleteCustomRepo(id) {
    HelmBackend.deleteHelmRepo(id)
      .then((res) => {
        if (res.status !== "ok") {
          Setting.showMessage("error", `${t("helm:Delete repo failed")}: ${res.msg}`);
          return;
        }
        loadCustomRepos();
        if (source.id === id) {
          setSource(PRESET_REPOS[0]);
        }
      })
      .catch((e) => Setting.showMessage("error", `${t("helm:Delete repo failed")}: ${e.message}`));
  }

  function sourceClass(active) {
    return cn(
      "mx-2 flex cursor-pointer items-center rounded-md px-3 py-1.5 text-sm transition-colors",
      active ? "bg-accent text-accent-foreground font-medium" : "hover:bg-accent/50"
    );
  }

  return (
    <div className="flex min-h-0 flex-1 overflow-hidden">
      <aside className="bg-muted/30 w-52 shrink-0 overflow-y-auto border-r py-4">
        <div className="text-muted-foreground px-4 pb-2 text-[11px] font-semibold tracking-wider uppercase">
          {t("helm:Sources")}
        </div>
        {PRESET_REPOS.map((repo) => (
          <SimpleTooltip key={repo.name} title={repo.desc} side="right">
            <div
              onClick={() => {
                setSource(repo);
                setQuery("");
              }}
              className={sourceClass(source.name === repo.name)}
            >
              {repo.name}
            </div>
          </SimpleTooltip>
        ))}

        {customRepos.length > 0 ? (
          <>
            <Separator className="my-2" />
            <div className="text-muted-foreground px-4 pb-1.5 text-[11px] font-semibold tracking-wider uppercase">
              {t("helm:My Repos")}
            </div>
            {customRepos.map((repo) => (
              <div key={repo.id} className={sourceClass(source.id === repo.id)}>
                <span
                  className="flex-1 truncate"
                  onClick={() => {
                    setSource({...repo, url: repo.url});
                    setQuery("");
                  }}
                >
                  {repo.name}
                </span>
                <ConfirmDialog
                  title={t("helm:Delete repo?")}
                  confirmText={t("general:Delete")}
                  cancelText={t("general:Cancel")}
                  onConfirm={() => deleteCustomRepo(repo.id)}
                >
                  <button
                    type="button"
                    onClick={(event) => event.stopPropagation()}
                    className="text-muted-foreground hover:text-destructive"
                    aria-label="Delete repo"
                  >
                    <Trash2 className="size-3.5" />
                  </button>
                </ConfirmDialog>
              </div>
            ))}
          </>
        ) : null}

        <div className="px-3 pt-3">
          <Button variant="outline" size="sm" className="w-full border-dashed" onClick={() => setAddRepoOpen(true)}>
            <Plus />
            {t("helm:Add Repo")}
          </Button>
        </div>
      </aside>

      <div className="flex-1 overflow-y-auto p-5">
        <div className="mb-4 flex items-center gap-3">
          <Store className="size-5" />
          <h1 className="text-lg font-semibold">{source.name}</h1>
          <div className="flex-1" />
          <Button variant="outline" size="sm" asChild>
            <Link to="/helm-releases">{t("helm:My Releases")} →</Link>
          </Button>
        </div>

        <div className="mb-4 flex flex-wrap gap-2">
          <div className="relative">
            <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
            <Input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={t("helm:Search charts")}
              className="w-72 pl-9"
            />
          </div>
          <Button
            variant="outline"
            loading={loading}
            onClick={() => {
              setCharts([]);
              setPage(1);
              fetchCharts(source, query, 1);
            }}
          >
            <RefreshCw />
            {t("general:Refresh")}
          </Button>
        </div>

        {error ? <MessageAlert title={error} className="mb-4" /> : null}

        <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
          {visibleCharts.map((chart, index) => (
            <ChartCard key={`${chart.chartName}-${index}`} chart={chart} onInstall={() => setInstallTarget(chart)} />
          ))}
        </div>

        {loading ? <Loading /> : null}

        {isArtifactHub && hasMore ? <div ref={sentinelRef} className="h-px" aria-hidden="true" /> : null}

        {!loading && visibleCharts.length === 0 && !error ? (
          <p className="text-muted-foreground py-16 text-center text-sm">{t("helm:No charts found")}</p>
        ) : null}
      </div>

      <AddRepoDialog open={addRepoOpen} onClose={() => setAddRepoOpen(false)} onAdded={loadCustomRepos} />

      <HelmInstallDialog
        open={Boolean(installTarget)}
        chart={installTarget}
        onClose={() => setInstallTarget(null)}
        onInstalled={() => setInstallTarget(null)}
      />
    </div>
  );
}

// Simple mode gets a curated shortlist instead of a search over every chart on
// ArtifactHub; the repository sidebar and the version fields only appear once
// the reader has asked for advanced mode.
export default function AppStorePage() {
  const {advanced} = useUiMode();
  return advanced ? <AdvancedAppStore /> : <SimpleAppStore />;
}
