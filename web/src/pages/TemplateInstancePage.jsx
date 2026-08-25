import React, {useEffect, useState} from "react";
import i18next from "i18next";
import {useTranslation} from "react-i18next";
import {ArrowLeft, Boxes, Database, ExternalLink, Trash2, TriangleAlert} from "lucide-react";
import * as TemplateBackend from "@/backend/TemplateBackend";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "@/components/ui/card";
import {Checkbox} from "@/components/ui/checkbox";
import {MessageAlert} from "@/components/ui/alert";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {DataTable} from "@/components/shared/data-table";
import {Loading} from "@/components/shared/loading";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {AppIcon} from "@/components/shared/app-icon";
import {runAction} from "@/hooks/use-resource";
import {useUiMode} from "@/hooks/use-ui-mode";

const POLL_INTERVAL = 20000;

/**
 * One app installed from the market: where to reach it, what it created, and
 * what the cluster could not provide. That last part is the honest half — a
 * template written for sealos may ask for an operator casos does not run, and
 * the app is only as installed as the pieces that went in.
 */
function TemplateInstancePage(props) {
  useTranslation();
  const {history, match} = props;
  const {resolvePath} = useUiMode();
  const {namespace, name} = match.params;

  const [instance, setInstance] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleteData, setDeleteData] = useState(false);

  function load({background = false} = {}) {
    if (!background) {
      setLoading(true);
    }
    return TemplateBackend.getTemplateInstance(namespace, name)
      .then((res) => {
        if (res.status === "ok") {
          setInstance(res.data);
          setError(null);
        } else {
          setError(res.msg);
        }
      })
      .catch((requestError) => setError(requestError.message))
      .finally(() => {
        if (!background) {
          setLoading(false);
        }
      });
  }

  useEffect(() => {
    load();
    const timer = setInterval(() => load({background: true}), POLL_INTERVAL);
    return () => clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [namespace, name]);

  function remove() {
    runAction(TemplateBackend.deleteTemplateInstance({namespace, name, deleteData}), {
      successMessage: i18next.t("template:App removed"),
      onSuccess: () => history.push("/helm-releases"),
    });
  }

  if (loading) {
    return <Loading type="page" />;
  }

  if (!instance) {
    return (
      <PageContainer>
        <MessageAlert title={error ?? i18next.t("template:App not found")} />
        <div>
          <Button variant="outline" onClick={() => history.push("/helm-releases")}>
            <ArrowLeft />
            {i18next.t("launchpad:Back")}
          </Button>
        </div>
      </PageContainer>
    );
  }

  const objectColumns = [
    {key: "kind", title: i18next.t("template:Kind"), dataIndex: "kind", width: 180, sortable: true},
    {key: "name", title: i18next.t("template:Name"), dataIndex: "name", minWidth: 220, ellipsis: true},
    {
      key: "group",
      title: i18next.t("template:API group"),
      dataIndex: "group",
      width: 220,
      render: (value, record) => <span className="font-mono text-xs">{value ? `${value}/${record.version}` : record.version}</span>,
    },
  ];

  return (
    <PageContainer>
      <PageHeader
        title={
          <span className="flex items-center gap-2">
            <AppIcon src={instance.icon} name={instance.title} chartName={instance.template} size="md" />
            {instance.title || instance.name}
          </span>
        }
        description={`${instance.template} · ${instance.namespace}/${instance.name}`}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" onClick={() => history.push("/helm-releases")}>
              <ArrowLeft />
              {i18next.t("launchpad:Back")}
            </Button>
            <Button variant="destructive" onClick={() => setDeleteOpen(true)}>
              <Trash2 />
              {i18next.t("launchpad:Delete")}
            </Button>
          </div>
        }
      />

      {error ? <MessageAlert title={error} /> : null}

      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{i18next.t("template:Address")}</CardTitle>
            <CardDescription>{i18next.t("template:Where this app answers once its pods are up.")}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-2">
            {(instance.apps ?? []).length > 0 ? (
              instance.apps.map((app) => (
                <Button key={app.url || app.name} size="sm" variant="outline" asChild>
                  <a href={app.url} target="_blank" rel="noreferrer">
                    <ExternalLink />
                    {app.url ? app.url.replace(/^https?:\/\//, "") : app.name}
                  </a>
                </Button>
              ))
            ) : (
              <p className="text-muted-foreground text-sm">{i18next.t("template:This app publishes no address of its own.")}</p>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">{i18next.t("database:Databases")}</CardTitle>
            <CardDescription>{i18next.t("template:Created for this app and managed like any other database here.")}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-2">
            {(instance.databases ?? []).length > 0 ? (
              instance.databases.map((database) => (
                <Button
                  key={database}
                  size="sm"
                  variant="outline"
                  onClick={() => history.push(resolvePath(`/databases/${instance.namespace}/${database}`))}
                >
                  <Database />
                  {database}
                </Button>
              ))
            ) : (
              <p className="text-muted-foreground text-sm">{i18next.t("template:This app brought no database.")}</p>
            )}
          </CardContent>
        </Card>
      </div>

      {(instance.unsupported ?? []).length > 0 ? (
        <Card className="border-warning/40">
          <CardHeader>
            <CardTitle className="flex items-center gap-2 text-base">
              <TriangleAlert className="text-warning size-4" />
              {i18next.t("template:What this cluster could not provide")}
            </CardTitle>
            <CardDescription>{i18next.t("template:The app is installed without these. Install what provides them and deploy again.")}</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-2">
            {instance.unsupported.map((item, index) => (
              <div key={`${item.kind}-${item.name}-${index}`} className="flex items-start gap-2 text-sm">
                <Badge variant="warning" className="shrink-0">{item.kind || i18next.t("template:Document")}</Badge>
                <span className="min-w-0">
                  <span className="block truncate font-medium">{item.name}</span>
                  <span className="text-muted-foreground block text-xs">{item.reason}</span>
                </span>
              </div>
            ))}
          </CardContent>
        </Card>
      ) : null}

      <DataTable
        testId="template-objects-table"
        title={i18next.t("template:What it created")}
        description={i18next.t("template:Deleting the app removes exactly these.")}
        columns={objectColumns}
        dataSource={instance.objects ?? []}
        rowKey={(record, index) => `${record.kind}-${record.name}-${index}`}
        pageSize={15}
        emptyIcon={Boxes}
        emptyText={i18next.t("template:Nothing was applied.")}
      />

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={`${i18next.t("launchpad:Delete")} ${instance.title || instance.name}`}
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

export default TemplateInstancePage;
