import React, {useState} from "react";
import i18next from "i18next";
import {FileText, GitCommitHorizontal, Hammer, RefreshCw} from "lucide-react";
import * as GitBuildBackend from "@/backend/GitBuildBackend";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {DataTable} from "@/components/shared/data-table";
import {PodLogsSheet} from "@/components/shared/pod-logs-sheet";
import {ResourceSheet} from "@/components/shared/resource-sheet";
import {runAction, useResource} from "@/hooks/use-resource";
import {repoPath} from "@/lib/git";

const POLL_INTERVAL = 4000;

const BUILD_STATUS = {
  queued: {variant: "warning", label: "general:Queued"},
  building: {variant: "info", label: "launchpad:Building"},
  deploying: {variant: "info", label: "launchpad:Deploying"},
  deployed: {variant: "success", label: "launchpad:Deployed"},
  failed: {variant: "danger", label: "general:Failed"},
};

const isActive = (build) => ["queued", "building", "deploying"].includes(build.status);

/**
 * The builds of an app deployed from a Git repository: what each one built,
 * how it went, its log, and a way to build the latest commit again.
 */
export function GitBuildsSheet({app, open, onClose, onChanged}) {
  const [logsBuild, setLogsBuild] = useState(null);
  const [rebuilding, setRebuilding] = useState(false);

  const {data: builds, loading, refresh} = useResource(
    () => GitBuildBackend.getGitBuilds(app.namespace, app.name),
    [app?.namespace, app?.name, open],
    {initialData: [], enabled: open && Boolean(app), pollInterval: open ? POLL_INTERVAL : 0}
  );
  const latest = builds?.[0];

  async function rebuild() {
    setRebuilding(true);
    try {
      await runAction(GitBuildBackend.deployGitApp({namespace: app.namespace, name: app.name}), {
        successMessage: i18next.t("launchpad:Build started"),
        onSuccess: () => {
          refresh({silent: true});
          onChanged?.();
        },
      });
    } finally {
      setRebuilding(false);
    }
  }

  const columns = [
    {
      key: "status",
      title: i18next.t("general:Status"),
      dataIndex: "status",
      width: 240,
      render: (value, record) => {
        const status = BUILD_STATUS[value] ?? BUILD_STATUS.queued;
        return (
          <div className="min-w-0 space-y-1">
            <Badge variant={status.variant}>{i18next.t(status.label)}</Badge>
            {record.message ? (
              <div className="text-muted-foreground line-clamp-2 text-xs" title={record.message}>{record.message}</div>
            ) : null}
          </div>
        );
      },
    },
    {
      key: "commit",
      title: i18next.t("launchpad:Commit"),
      dataIndex: "commit",
      width: 170,
      render: (value, record) => (
        <div className="min-w-0 text-xs">
          <div className="font-mono">
            <GitCommitHorizontal className="mr-1 inline size-3 align-[-2px]" />
            {value || "—"}
          </div>
          {record.kind ? <div className="text-muted-foreground">{record.kind}</div> : null}
        </div>
      ),
    },
    {key: "createdAt", title: i18next.t("general:Created"), dataIndex: "createdAt", width: 160},
    {key: "finishedAt", title: i18next.t("devbox:Finished"), dataIndex: "finishedAt", width: 160, render: (value) => value || "—"},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 100,
      align: "right",
      render: (_value, record) => (
        <Button size="sm" variant="outline" disabled={!record.podName} onClick={() => setLogsBuild(record)}>
          <FileText />
          {i18next.t("general:Logs")}
        </Button>
      ),
    },
  ];

  const source = latest ? `${repoPath(latest.repo)}${latest.branch ? ` @ ${latest.branch}` : ""}${latest.path ? ` / ${latest.path}` : ""}` : "";

  return (
    <>
      <ResourceSheet
        open={open}
        onOpenChange={(next) => (next ? null : onClose())}
        title={i18next.t("launchpad:Builds of {{name}}", {name: app?.name ?? ""})}
        description={source}
        size="xl"
        toolbar={
          <div className="flex items-center gap-2">
            <Button
              size="sm"
              data-testid="git-rebuild"
              onClick={rebuild}
              loading={rebuilding}
              disabled={!latest || isActive(latest)}
              title={latest && isActive(latest) ? i18next.t("launchpad:A build is already running") : undefined}
            >
              <Hammer />
              {i18next.t("launchpad:Rebuild latest commit")}
            </Button>
            <Button variant="outline" size="sm" onClick={() => refresh()}>
              <RefreshCw />
              {i18next.t("general:Refresh")}
            </Button>
          </div>
        }
      >
        <DataTable
          testId="git-builds-table"
          columns={columns}
          dataSource={builds}
          rowKey="name"
          loading={loading}
          dense
          emptyIcon={Hammer}
          emptyText={i18next.t("launchpad:No builds yet.")}
        />
      </ResourceSheet>

      <PodLogsSheet
        pod={logsBuild ? {namespace: logsBuild.namespace, name: logsBuild.podName, containers: ["build"]} : null}
        open={Boolean(logsBuild)}
        onClose={() => setLogsBuild(null)}
      />
    </>
  );
}

export default GitBuildsSheet;
