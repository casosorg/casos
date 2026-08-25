import React, {useEffect, useMemo, useState} from "react";
import i18next from "i18next";
import {useTranslation} from "react-i18next";
import {ExternalLink, LayoutGrid, PackageSearch, RefreshCw, Search, Trash2} from "lucide-react";
import * as TemplateBackend from "@/backend/TemplateBackend";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "@/components/ui/card";
import {Checkbox} from "@/components/ui/checkbox";
import {Input} from "@/components/ui/input";
import {Tabs, TabsContent, TabsList, TabsTrigger} from "@/components/ui/tabs";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {DataTable} from "@/components/shared/data-table";
import {EmptyState} from "@/components/shared/empty-state";
import {Loading} from "@/components/shared/loading";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {AppIcon} from "@/components/shared/app-icon";
import {cn} from "@/lib/utils";
import {runAction, useResource} from "@/hooks/use-resource";

const INSTANCE_POLL_INTERVAL = 20000;

function TemplateCard({template, onOpen}) {
  return (
    <Card
      role="button"
      tabIndex={0}
      onClick={() => onOpen(template)}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onOpen(template);
        }
      }}
      className="hover:border-ring/60 focus-visible:border-ring cursor-pointer gap-3 transition-colors outline-none"
      data-testid={`template-card-${template.name}`}
    >
      <CardHeader className="gap-2">
        <div className="flex items-center gap-2.5">
          <AppIcon src={template.icon} name={template.title} chartName={template.name} size="md" />
          <div className="min-w-0">
            <CardTitle className="truncate text-base">{template.title}</CardTitle>
            <p className="text-muted-foreground truncate text-xs">{template.name}</p>
          </div>
        </div>
        <CardDescription className="line-clamp-3 min-h-[3rem]">{template.description}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-wrap gap-1">
        {(template.categories ?? []).slice(0, 3).map((category) => (
          <Badge key={category} variant="secondary">{category}</Badge>
        ))}
      </CardContent>
    </Card>
  );
}

/**
 * The template market: the sealos template repository, browsable and
 * installable here. A card is one published app; opening it leads to the form
 * that fills in whatever that app asks for.
 */
function TemplateMarketPage(props) {
  useTranslation();
  const {history} = props;

  const [search, setSearch] = useState("");
  const [debounced, setDebounced] = useState("");
  const [category, setCategory] = useState("all");
  const [syncing, setSyncing] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState(null);
  const [deleteData, setDeleteData] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => setDebounced(search.trim()), 250);
    return () => clearTimeout(timer);
  }, [search]);

  const {data: market, loading, refresh} = useResource(
    () => TemplateBackend.getTemplates({search: debounced, category}),
    [debounced, category],
    {initialData: {templates: [], categories: [], status: {}}}
  );
  const {data: instances, refresh: refreshInstances} = useResource(
    () => TemplateBackend.getTemplateInstances(),
    [],
    {initialData: [], pollInterval: INSTANCE_POLL_INTERVAL, toastOnError: false}
  );

  const categories = useMemo(() => ["all", ...(market?.categories ?? [])], [market]);
  const templates = market?.templates ?? [];
  const status = market?.status ?? {};

  function sync() {
    setSyncing(true);
    runAction(TemplateBackend.syncTemplates(), {
      successMessage: i18next.t("template:Market updated"),
      onSuccess: () => refresh(),
    }).finally(() => setSyncing(false));
  }

  function remove() {
    if (!deleteTarget) {
      return;
    }
    runAction(
      TemplateBackend.deleteTemplateInstance({
        namespace: deleteTarget.namespace,
        name: deleteTarget.name,
        deleteData,
      }),
      {
        successMessage: i18next.t("template:App removed"),
        onSuccess: () => {
          setDeleteTarget(null);
          setDeleteData(false);
          refreshInstances({silent: true});
        },
      }
    );
  }

  const instanceColumns = [
    {
      key: "name",
      title: i18next.t("template:App"),
      dataIndex: "name",
      minWidth: 220,
      sortable: true,
      render: (value, record) => (
        <div className="flex items-center gap-2">
          <AppIcon src={record.icon} name={record.title} chartName={record.template} size="sm" />
          <div className="min-w-0">
            <div className="truncate font-medium">{record.title || value}</div>
            <div className="text-muted-foreground truncate text-xs">{value}</div>
          </div>
        </div>
      ),
    },
    {key: "namespace", title: i18next.t("general:Namespace"), dataIndex: "namespace", width: 150},
    {key: "template", title: i18next.t("template:Template"), dataIndex: "template", width: 160, ellipsis: true},
    {
      key: "apps",
      title: i18next.t("template:Address"),
      minWidth: 220,
      render: (_value, record) => {
        const urls = (record.apps ?? []).map((app) => app.url).filter(Boolean);
        if (urls.length === 0) {
          return <span className="text-muted-foreground text-xs">—</span>;
        }
        return (
          <div className="flex flex-wrap gap-1">
            {urls.map((url) => (
              <Button key={url} size="sm" variant="outline" asChild onClick={(event) => event.stopPropagation()}>
                <a href={url} target="_blank" rel="noreferrer">
                  <ExternalLink />
                  {url.replace(/^https?:\/\//, "")}
                </a>
              </Button>
            ))}
          </div>
        );
      },
    },
    {
      key: "unsupported",
      title: i18next.t("template:Missing pieces"),
      width: 150,
      render: (_value, record) => (
        (record.unsupported ?? []).length > 0
          ? <Badge variant="warning">{record.unsupported.length}</Badge>
          : <span className="text-muted-foreground text-xs">—</span>
      ),
    },
    {key: "createdAt", title: i18next.t("launchpad:Created"), dataIndex: "createdAt", width: 170, sortable: true},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 90,
      align: "right",
      render: (_value, record) => (
        <Button
          size="icon-sm"
          variant="ghost"
          aria-label={i18next.t("launchpad:Delete")}
          title={i18next.t("launchpad:Delete")}
          onClick={(event) => {
            event.stopPropagation();
            setDeleteTarget(record);
          }}
        >
          <Trash2 />
        </Button>
      ),
    },
  ];

  return (
    <PageContainer>
      <PageHeader
        title={i18next.t("template:Template market")}
        description={i18next.t("template:Ready-made apps from the sealos template repository, deployed into this cluster.")}
        actions={
          <Button variant="outline" onClick={sync} disabled={syncing}>
            <RefreshCw className={cn(syncing && "animate-spin")} />
            {syncing ? i18next.t("template:Updating…") : i18next.t("template:Update market")}
          </Button>
        }
      />

      <Tabs defaultValue="market">
        <TabsList>
          <TabsTrigger value="market" data-testid="template-tab-market">
            <LayoutGrid />
            {i18next.t("template:Market")}
            {templates.length > 0 ? <Badge variant="secondary">{templates.length}</Badge> : null}
          </TabsTrigger>
          <TabsTrigger value="installed" data-testid="template-tab-installed">
            <PackageSearch />
            {i18next.t("template:Installed")}
            {instances.length > 0 ? <Badge variant="secondary">{instances.length}</Badge> : null}
          </TabsTrigger>
        </TabsList>

        <TabsContent value="market" className="grid gap-4">
          <div className="flex flex-wrap items-center gap-2">
            <div className="relative w-full max-w-sm">
              <Search className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
              <Input
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder={i18next.t("template:Search apps")}
                className="pl-9"
                data-testid="template-search"
              />
            </div>
            <div className="flex flex-wrap gap-1.5">
              {categories.map((item) => (
                <Button
                  key={item}
                  size="sm"
                  variant={category === item ? "default" : "outline"}
                  onClick={() => setCategory(item)}
                >
                  {item === "all" ? i18next.t("template:All") : item}
                </Button>
              ))}
            </div>
            {status.updatedAt ? (
              <span className="text-muted-foreground ml-auto text-xs">
                {i18next.t("template:Updated")} {new Date(status.updatedAt).toLocaleString()} · {status.count} {i18next.t("template:apps")}
              </span>
            ) : null}
          </div>

          {loading ? (
            <Loading type="section" />
          ) : templates.length === 0 ? (
            <EmptyState
              icon={PackageSearch}
              title={i18next.t("template:No apps found")}
              description={i18next.t("template:Update the market to fetch the published templates.")}
              action={<Button onClick={sync}>{i18next.t("template:Update market")}</Button>}
            />
          ) : (
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
              {templates.map((template) => (
                <TemplateCard
                  key={template.name}
                  template={template}
                  onOpen={() => history.push(`/templates/${template.name}`)}
                />
              ))}
            </div>
          )}
        </TabsContent>

        <TabsContent value="installed">
          <DataTable
            scopeToWorkspace
            testId="template-instances-table"
            columns={instanceColumns}
            dataSource={instances}
            rowKey={(record) => `${record.namespace}/${record.name}`}
            searchable
            onRowClick={(record) => history.push(`/templates/instances/${record.namespace}/${record.name}`)}
            emptyIcon={PackageSearch}
            emptyText={i18next.t("template:Nothing installed from the market yet.")}
          />
        </TabsContent>
      </Tabs>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null);
            setDeleteData(false);
          }
        }}
        title={`${i18next.t("launchpad:Delete")} ${deleteTarget?.title || deleteTarget?.name || ""}`}
        description={i18next.t("template:Everything this app created is removed. Databases keep their data unless you say otherwise.")}
        confirmText={i18next.t("launchpad:Delete")}
        onConfirm={remove}
        extra={
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={deleteData} onCheckedChange={(checked) => setDeleteData(Boolean(checked))} />
            {i18next.t("database:Also delete its data and backups")}
          </label>
        }
      />
    </PageContainer>
  );
}

export default TemplateMarketPage;
