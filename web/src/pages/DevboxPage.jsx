import React, {useState} from "react";
import i18next from "i18next";
import {useTranslation} from "react-i18next";
import {Bot, ChevronRight, Code2, ExternalLink, FolderGit2, GitBranch, Laptop, Pin, Play, Plus, Rocket, ScrollText, Sparkles, Square, Timer, Trash2} from "lucide-react";
import * as DevboxBackend from "@/backend/DevboxBackend";
import * as ImageBackend from "@/backend/ImageBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {Textarea} from "@/components/ui/textarea";
import {Checkbox} from "@/components/ui/checkbox";
import {Collapsible, CollapsibleContent, CollapsibleTrigger} from "@/components/ui/collapsible";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {DataTable} from "@/components/shared/data-table";
import {DevboxEnvironmentPicker, DevboxSourcePicker, presetByKey} from "@/components/shared/devbox-environment";
import {DevboxRunsSheet} from "@/components/shared/devbox-runs-sheet";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {PodLogsSheet} from "@/components/shared/pod-logs-sheet";
import {SimpleSelect} from "@/components/shared/simple-select";
import {StatusBadge} from "@/components/shared/status-badge";
import {CodeBlock, CodeText, DescriptionList} from "@/components/shared/misc";
import {runAction, useResource} from "@/hooks/use-resource";
import {folderOfPath, nameFromPath, nameFromRepo, repoPath} from "@/lib/git";
import {cn} from "@/lib/utils";
import {useUiMode} from "@/hooks/use-ui-mode";
import {useWorkspace} from "@/hooks/use-workspace";

const POLL_INTERVAL = 15000;

const DEVBOX_STATUS_VARIANTS = {
  deployed: "success",
  pending: "warning",
  failed: "danger",
  stopped: "muted",
};

const emptyForm = (namespace = "default") => ({
  name: "",
  nameTouched: false,
  namespace,
  source: "empty",
  repo: "",
  localPath: "",
  branch: "",
  preset: "general",
  image: "",
  setup: "",
  diskSize: "5Gi",
  password: "",
  sshPublicKeys: "",
});

const LOCAL_PATH_PLACEHOLDER = navigator.platform?.startsWith("Win") ? "D:\\code\\my-project" : "/home/me/my-project";

const PREPARE_STEPS = {
  "editor": () => i18next.t("devbox:Installing the editor"),
  "user": () => i18next.t("devbox:Adding the user"),
  "ssh-tools": () => i18next.t("devbox:Installing SSH"),
  "clone": (box) => ({
    local: i18next.t("devbox:Copying the local repository"),
    empty: i18next.t("devbox:Creating the project"),
  })[box.source] ?? i18next.t("devbox:Cloning the repository"),
  "setup": () => i18next.t("devbox:Running the setup script"),
};

function expiresIn(expiresAt) {
  const minutes = Math.round((new Date(expiresAt).getTime() - Date.now()) / 60000);
  if (minutes <= 0) {
    return i18next.t("devbox:Expiring now");
  }
  const format = new Intl.RelativeTimeFormat(i18next.language, {numeric: "auto"});
  const text = minutes < 60 ? format.format(minutes, "minute")
    : minutes < 48 * 60 ? format.format(Math.round(minutes / 60), "hour")
      : format.format(Math.round(minutes / 1440), "day");
  return i18next.t("devbox:Expires {{when}}", {when: text});
}

const hasSsh = (box) => Boolean(box?.sshHost && box?.sshPort);
const sshHostAlias = (box) => `casos-${box.namespace}-${box.name}`;
const sshCommand = (box) => `ssh -p ${box.sshPort} ${box.sshUser}@${box.sshHost}`;
const sshConfigEntry = (box) => [
  `Host ${sshHostAlias(box)}`,
  `  HostName ${box.sshHost}`,
  `  Port ${box.sshPort}`,
  `  User ${box.sshUser}`,
].join("\n");

// Remote-SSH reads a hex-encoded JSON authority, which carries the port and an IPv6 host intact.
function vscodeLink(box) {
  const authority = JSON.stringify({hostName: box.sshHost, user: box.sshUser, port: box.sshPort});
  const hex = Array.from(new TextEncoder().encode(authority), (byte) => byte.toString(16).padStart(2, "0")).join("");
  return `vscode://vscode-remote/ssh-remote+${hex}${box.sshPath}`;
}

/**
 * A DevBox is a browser VS Code (code-server) running in a container on the
 * cluster: pick a name, get an editor at a URL, connect and work. It is a
 * one-click preset over the launchpad's deploy pipeline, so starting, stopping
 * and deleting one reuse the image-app actions the launchpad already has.
 */
function DevboxPage() {
  useTranslation();
  const {workspace} = useWorkspace();
  const {advanced} = useUiMode();
  const defaultNamespace = workspace && workspace !== "all" ? workspace : "default";
  const [namespace, setNamespace] = useState("all");
  const [createOpen, setCreateOpen] = useState(false);
  const [form, setForm] = useState(emptyForm);
  const [submitting, setSubmitting] = useState(false);
  const [created, setCreated] = useState(null);
  const [connectTarget, setConnectTarget] = useState(null);
  const [runsTarget, setRunsTarget] = useState(null);
  const [logsTarget, setLogsTarget] = useState(null);
  const [deleteTarget, setDeleteTarget] = useState(null);
  const [deleteData, setDeleteData] = useState(false);

  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});
  const {data: devboxes, loading, refresh} = useResource(
    () => DevboxBackend.getDevboxes(namespace === "all" ? "" : namespace),
    [namespace],
    {initialData: [], pollInterval: POLL_INTERVAL}
  );

  function setField(key, value) {
    setForm((prev) => ({...prev, [key]: value}));
  }

  function setRepo(repo) {
    setForm((prev) => ({...prev, repo, name: prev.nameTouched ? prev.name : nameFromRepo(repo)}));
  }

  function setLocalPath(localPath) {
    setForm((prev) => ({...prev, localPath, name: prev.nameTouched ? prev.name : nameFromPath(localPath)}));
  }

  function chooseSource(source) {
    setForm((prev) => {
      const name = prev.nameTouched ? prev.name
        : source === "git" ? nameFromRepo(prev.repo)
          : source === "local" ? nameFromPath(prev.localPath) : "";
      return {...prev, source, name};
    });
  }

  function choosePreset(preset) {
    setForm((prev) => ({...prev, preset: preset.key, image: preset.image, setup: preset.setup}));
  }

  function openCreate() {
    setForm(emptyForm(defaultNamespace));
    setCreateOpen(true);
  }

  const sourceMissing = (form.source === "git" && !form.repo.trim()) || (form.source === "local" && !form.localPath.trim());

  async function submitCreate() {
    if (!form.name.trim() || sourceMissing) {
      return;
    }
    setSubmitting(true);
    try {
      await runAction(
        DevboxBackend.deployDevbox({
          name: form.name.trim(),
          namespace: form.namespace,
          source: form.source,
          repo: form.source === "git" ? form.repo.trim() : "",
          localPath: form.source === "local" ? form.localPath.trim() : "",
          branch: form.source === "empty" ? "" : form.branch.trim(),
          image: form.image.trim(),
          setup: form.setup.trim(),
          diskSize: form.diskSize.trim(),
          password: form.password.trim(),
          sshPublicKeys: form.sshPublicKeys.trim(),
        }),
        {
          onSuccess: (res) => {
            setCreateOpen(false);
            setCreated(res.data);
            refresh({silent: true});
          },
        }
      );
    } finally {
      setSubmitting(false);
    }
  }

  function toggleRunning(box, running) {
    runAction(ImageBackend.scaleApp({namespace: box.namespace, name: box.name, running}), {
      successMessage: running ? i18next.t("devbox:DevBox started") : i18next.t("devbox:DevBox stopped"),
      onSuccess: () => refresh({silent: true}),
    });
  }

  function keep(box) {
    runAction(DevboxBackend.keepDevbox({namespace: box.namespace, name: box.name}), {
      successMessage: i18next.t("devbox:Kept: it will no longer be deleted automatically"),
      onSuccess: () => refresh({silent: true}),
    });
  }

  function remove() {
    if (!deleteTarget) {
      return;
    }
    runAction(
      ImageBackend.uninstallApp({namespace: deleteTarget.namespace, name: deleteTarget.name, deleteData}),
      {
        successMessage: i18next.t("devbox:DevBox deleted"),
        onSuccess: () => {
          setDeleteTarget(null);
          setDeleteData(false);
          refresh({silent: true});
        },
      }
    );
  }

  const namespaceField = (
    <Field label={i18next.t("general:Namespace")} htmlFor="devbox-namespace">
      <SimpleSelect
        value={form.namespace}
        onChange={(value) => setField("namespace", value)}
        options={namespaces.map((item) => ({label: item.name, value: item.name}))}
        className="w-full"
      />
    </Field>
  );

  const sshKeysField = (
    <Field
      label={i18next.t("devbox:SSH public keys")}
      htmlFor="devbox-ssh-keys"
      hint={i18next.t("devbox:Paste a .pub key, one per line, to also open this box from the VS Code on your machine over Remote-SSH. Blank keeps it browser-only.")}
    >
      <Textarea
        id="devbox-ssh-keys"
        rows={2}
        className="font-mono text-xs"
        value={form.sshPublicKeys}
        onChange={(e) => setField("sshPublicKeys", e.target.value)}
        placeholder="ssh-ed25519 AAAA... you@laptop"
        data-testid="devbox-ssh-keys-input"
      />
    </Field>
  );

  const columns = [
    {
      key: "name",
      title: i18next.t("devbox:DevBox"),
      dataIndex: "name",
      minWidth: 160,
      sortable: true,
      render: (value, record) => (
        <div className="flex items-center gap-2">
          <Code2 className="text-muted-foreground size-4 shrink-0" />
          <div className="min-w-0">
            <div className="truncate font-medium">{value}</div>
            <div className="text-muted-foreground truncate text-xs">{record.namespace}</div>
            {record.agent ? (
              <div
                className="text-muted-foreground flex items-center gap-1 truncate text-xs"
                title={i18next.t("devbox:Created by an AI agent with the access token {{token}}", {token: record.agent})}
              >
                <Bot className="size-3 shrink-0" />
                {record.agent}
              </div>
            ) : null}
          </div>
        </div>
      ),
    },
    {
      key: "status",
      title: i18next.t("general:Status"),
      dataIndex: "status",
      width: 184,
      sortable: true,
      render: (value, record) => (
        <div className="flex flex-wrap items-center gap-1.5">
          <StatusBadge status={value} variants={DEVBOX_STATUS_VARIANTS} />
          {record.prepareStep ? (
            <Badge variant="muted">{PREPARE_STEPS[record.prepareStep]?.(record) ?? record.prepareStep}</Badge>
          ) : null}
          {record.cloneError ? (
            <Badge variant="danger" title={record.cloneError}>{i18next.t("devbox:Clone failed")}</Badge>
          ) : null}
          {record.setupFailed ? <Badge variant="danger">{i18next.t("devbox:Setup failed")}</Badge> : null}
          {record.activeRuns > 0 ? (
            <Badge variant="info">{i18next.t("devbox:{{count}} active", {count: record.activeRuns})}</Badge>
          ) : null}
          {record.expiresAt ? (
            <Badge variant="warning" title={new Date(record.expiresAt).toLocaleString()} data-testid="devbox-expiry">
              <Timer />
              {expiresIn(record.expiresAt)}
            </Badge>
          ) : null}
        </div>
      ),
    },
    {
      key: "image",
      title: i18next.t("general:Environment"),
      dataIndex: "image",
      minWidth: 180,
      ellipsis: true,
      render: (value, record) => (
        <div className="min-w-0">
          {record.repo ? (
            <div className="truncate font-medium" title={record.repo}>
              {repoPath(record.repo)}
            </div>
          ) : record.localRepo ? (
            <div className="truncate font-medium" title={record.localRepo}>
              <FolderGit2 className="text-muted-foreground mr-1 inline size-3.5 align-[-2px]" />
              {folderOfPath(record.localRepo)}
            </div>
          ) : record.source === "empty" ? (
            <div className="truncate font-medium" title={record.folder}>
              <Sparkles className="text-muted-foreground mr-1 inline size-3.5 align-[-2px]" />
              {i18next.t("devbox:New project")}
            </div>
          ) : null}
          <div className={cn("truncate font-mono text-xs", record.source && "text-muted-foreground")}>
            {record.commit ? (
              <>
                <GitBranch className="mr-0.5 inline size-3 align-[-2px]" />
                {record.branch ? `${record.branch} @ ` : ""}
                {record.commit}
                {" · "}
              </>
            ) : null}
            {value}
          </div>
        </div>
      ),
    },
    {
      key: "createdAt",
      title: i18next.t("general:Created"),
      dataIndex: "createdAt",
      width: 170,
      className: "whitespace-nowrap",
      sortable: true,
    },
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 312,
      align: "right",
      render: (_value, record) => (
        <div className="flex items-center justify-end gap-0.5">
          <Button
            size="sm"
            variant="outline"
            disabled={!record.url || record.status === "stopped"}
            title={record.url || i18next.t("devbox:Not reachable yet")}
            onClick={(event) => {
              event.stopPropagation();
              if (record.url) {
                window.open(record.url, "_blank", "noopener,noreferrer");
              }
            }}
          >
            <ExternalLink />
            {i18next.t("general:Open")}
          </Button>
          <Button
            size="icon-sm"
            variant="ghost"
            disabled={!hasSsh(record)}
            aria-label={i18next.t("devbox:Connect from desktop VS Code")}
            title={hasSsh(record) ? i18next.t("devbox:Connect from desktop VS Code") : i18next.t("devbox:Created without an SSH key, so this one is browser-only.")}
            data-testid="devbox-connect"
            onClick={(event) => {
              event.stopPropagation();
              setConnectTarget(record);
            }}
          >
            <Laptop />
          </Button>
          <Button
            size="icon-sm"
            variant="ghost"
            disabled={!record.podName || !record.logContainers?.length}
            aria-label={i18next.t("devbox:Environment log")}
            title={i18next.t("devbox:Environment log")}
            data-testid="devbox-environment-log"
            onClick={(event) => {
              event.stopPropagation();
              setLogsTarget(record);
            }}
          >
            <ScrollText />
          </Button>
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={i18next.t("devbox:Runs")}
            title={i18next.t("devbox:Runs")}
            data-testid="devbox-runs"
            onClick={(event) => {
              event.stopPropagation();
              setRunsTarget(record);
            }}
          >
            <Rocket />
          </Button>
          {record.status === "stopped" ? (
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={i18next.t("general:Start")}
              title={i18next.t("general:Start")}
              onClick={(event) => {
                event.stopPropagation();
                toggleRunning(record, true);
              }}
            >
              <Play />
            </Button>
          ) : (
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={i18next.t("general:Stop")}
              title={i18next.t("general:Stop")}
              onClick={(event) => {
                event.stopPropagation();
                toggleRunning(record, false);
              }}
            >
              <Square />
            </Button>
          )}
          {record.expiresAt ? (
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={i18next.t("devbox:Keep")}
              title={i18next.t("devbox:Keep this sandbox, so it is not deleted when its time runs out")}
              data-testid="devbox-keep"
              onClick={(event) => {
                event.stopPropagation();
                keep(record);
              }}
            >
              <Pin />
            </Button>
          ) : null}
          <Button
            size="icon-sm"
            variant="ghost"
            aria-label={i18next.t("general:Delete")}
            title={i18next.t("general:Delete")}
            onClick={(event) => {
              event.stopPropagation();
              setDeleteTarget(record);
            }}
          >
            <Trash2 />
          </Button>
        </div>
      ),
    },
  ];

  return (
    <PageContainer>
      <PageHeader
        title={i18next.t("devbox:DevBox")}
        description={i18next.t("devbox:Spin up a VS Code workspace in a container and connect from your browser — no Kubernetes to learn.")}
        actions={
          <Button onClick={openCreate} data-testid="devbox-create">
            <Plus />
            {i18next.t("devbox:New DevBox")}
          </Button>
        }
      />

      <DataTable
        scopeToWorkspace
        testId="devbox-table"
        columns={columns}
        dataSource={devboxes}
        rowKey={(record) => `${record.namespace}/${record.name}`}
        loading={loading}
        searchable
        emptyIcon={Code2}
        emptyText={i18next.t("devbox:No DevBoxes yet. Create one to get a VS Code workspace on the cluster.")}
        toolbar={
          <div className="flex items-center gap-2">
            {advanced ? (
              <SimpleSelect
                value={namespace}
                onChange={setNamespace}
                options={[
                  {label: i18next.t("general:All namespaces"), value: "all"},
                  ...namespaces.map((item) => ({label: item.name, value: item.name})),
                ]}
                size="sm"
                className="w-52"
              />
            ) : null}
            <Button variant="outline" size="sm" onClick={() => refresh()}>
              {i18next.t("general:Refresh")}
            </Button>
          </div>
        }
      />

      <FormDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        title={i18next.t("devbox:New DevBox")}
        description={i18next.t("devbox:A VS Code workspace in the image you pick, with your repository checked out and its dependencies installed.")}
        onSubmit={submitCreate}
        submitText={i18next.t("general:Create")}
        submitting={submitting}
        submitDisabled={!form.name.trim() || sourceMissing}
        size="lg"
      >
        <Field label={i18next.t("devbox:Project")}>
          <DevboxSourcePicker value={form.source} onChange={chooseSource} />
        </Field>
        {form.source === "git" ? (
          <div className="grid gap-4 sm:grid-cols-[1fr_12rem]">
            <Field
              label={i18next.t("general:Repository")}
              htmlFor="devbox-repo"
              required
              hint={i18next.t("devbox:A public Git repository, cloned into the home disk on first start.")}
            >
              <Input
                id="devbox-repo"
                value={form.repo}
                onChange={(e) => setRepo(e.target.value)}
                placeholder="https://github.com/owner/project.git"
                data-testid="devbox-repo-input"
              />
            </Field>
            <Field label={i18next.t("general:Branch")} htmlFor="devbox-branch">
              <Input
                id="devbox-branch"
                value={form.branch}
                onChange={(e) => setField("branch", e.target.value)}
                placeholder={i18next.t("devbox:Default branch")}
              />
            </Field>
          </div>
        ) : null}
        {form.source === "local" ? (
          <div className="grid gap-4 sm:grid-cols-[1fr_12rem]">
            <Field
              label={i18next.t("general:Folder")}
              htmlFor="devbox-local-path"
              required
              hint={i18next.t("devbox:The full path of a Git repository on the computer running casos. Every committed branch is copied in; uncommitted changes stay where they are.")}
            >
              <Input
                id="devbox-local-path"
                value={form.localPath}
                onChange={(e) => setLocalPath(e.target.value)}
                placeholder={LOCAL_PATH_PLACEHOLDER}
                className="font-mono"
                data-testid="devbox-local-path-input"
              />
            </Field>
            <Field label={i18next.t("general:Branch")} htmlFor="devbox-local-branch">
              <Input
                id="devbox-local-branch"
                value={form.branch}
                onChange={(e) => setField("branch", e.target.value)}
                placeholder={i18next.t("devbox:Current branch")}
              />
            </Field>
          </div>
        ) : null}
        <div className={cn("grid gap-4", advanced && "sm:grid-cols-2")}>
          <Field
            label={i18next.t("general:Name")}
            htmlFor="devbox-name"
            required
            hint={form.source === "empty" && form.name.trim()
              ? i18next.t("devbox:The project is created in ~/{{name}}.", {name: form.name.trim()})
              : null}
          >
            <Input
              id="devbox-name"
              value={form.name}
              onChange={(e) => setForm((prev) => ({...prev, name: e.target.value, nameTouched: true}))}
              placeholder="my-devbox"
              data-testid="devbox-name-input"
            />
          </Field>
          {advanced ? namespaceField : null}
        </div>
        <Field label={i18next.t("general:Environment")} hint={presetByKey(form.preset).hint?.()}>
          <DevboxEnvironmentPicker value={form.preset} onChange={choosePreset} />
        </Field>
        {form.preset === "custom" ? (
          <Field
            label={i18next.t("general:Image")}
            htmlFor="devbox-image"
            hint={i18next.t("devbox:Any glibc-based image, such as a Debian or Ubuntu one. The editor is added to it.")}
          >
            <Input
              id="devbox-image"
              value={form.image}
              onChange={(e) => setField("image", e.target.value)}
              placeholder="registry.example.com/lab/env:1.0"
              data-testid="devbox-image-input"
            />
          </Field>
        ) : null}
        <Field
          label={i18next.t("devbox:Setup script")}
          htmlFor="devbox-setup"
          hint={i18next.t("devbox:Runs once in the repository folder before the editor opens, as a normal user. Only the home disk survives a restart, so install there: pip and conda already do.")}
        >
          <Textarea
            id="devbox-setup"
            rows={3}
            className="font-mono text-xs"
            value={form.setup}
            onChange={(e) => setField("setup", e.target.value)}
            placeholder="pip install -r requirements.txt"
            data-testid="devbox-setup-input"
          />
        </Field>
        {advanced ? sshKeysField : null}
        <Collapsible>
          <CollapsibleTrigger className="text-muted-foreground hover:text-foreground group flex items-center gap-1 text-sm">
            <ChevronRight className="size-4 transition-transform group-data-[state=open]:rotate-90" />
            {advanced ? i18next.t("devbox:Disk and password") : i18next.t("general:More options")}
          </CollapsibleTrigger>
          <CollapsibleContent className="grid gap-4 pt-4 sm:grid-cols-2">
            {advanced ? null : namespaceField}
            <Field
              label={i18next.t("devbox:Home disk")}
              htmlFor="devbox-disk"
              hint={i18next.t("devbox:Keeps /home/coder across restarts. Use 0 for a stateless box.")}
            >
              <Input
                id="devbox-disk"
                value={form.diskSize}
                onChange={(e) => setField("diskSize", e.target.value)}
                placeholder="5Gi"
              />
            </Field>
            <Field
              label={i18next.t("general:Password")}
              htmlFor="devbox-password"
              hint={i18next.t("devbox:Leave blank to have one generated.")}
            >
              <Input
                id="devbox-password"
                value={form.password}
                onChange={(e) => setField("password", e.target.value)}
                placeholder="••••••••"
              />
            </Field>
            {advanced ? null : <div className="sm:col-span-2">{sshKeysField}</div>}
          </CollapsibleContent>
        </Collapsible>
      </FormDialog>

      <FormDialog
        open={Boolean(created)}
        onOpenChange={(open) => {
          if (!open) {
            setCreated(null);
          }
        }}
        title={i18next.t("devbox:DevBox created")}
        description={i18next.t("devbox:Copy the password now — it is not shown again.")}
        footer={
          <>
            <Button variant="outline" onClick={() => setCreated(null)}>
              {i18next.t("general:Close")}
            </Button>
            {created?.url ? (
              <Button
                onClick={() => {
                  window.open(created.url, "_blank", "noopener,noreferrer");
                  setCreated(null);
                }}
              >
                <ExternalLink />
                {i18next.t("devbox:Open VS Code")}
              </Button>
            ) : null}
          </>
        }
      >
        <DescriptionList
          items={[
            {label: i18next.t("general:Name"), value: created?.name},
            {
              label: i18next.t("general:URL"),
              value: created?.url ? <CodeText copyable>{created.url}</CodeText> : i18next.t("devbox:Assigned shortly"),
            },
            {label: i18next.t("general:Password"), value: <CodeText copyable>{created?.password}</CodeText>},
            hasSsh(created) ? {label: i18next.t("devbox:SSH"), value: <CodeText copyable>{sshCommand(created)}</CodeText>} : null,
          ]}
        />
      </FormDialog>

      <FormDialog
        open={Boolean(connectTarget)}
        onOpenChange={(open) => {
          if (!open) {
            setConnectTarget(null);
          }
        }}
        title={i18next.t("devbox:Connect from desktop VS Code")}
        description={i18next.t("devbox:Needs the Remote - SSH extension, and the private key that matches a public key given to this box.")}
        footer={
          <>
            <Button variant="outline" onClick={() => setConnectTarget(null)}>
              {i18next.t("general:Close")}
            </Button>
            {hasSsh(connectTarget) ? (
              <Button
                data-testid="devbox-open-desktop"
                onClick={() => {
                  window.location.href = vscodeLink(connectTarget);
                  setConnectTarget(null);
                }}
              >
                <Laptop />
                {i18next.t("devbox:Open in VS Code")}
              </Button>
            ) : null}
          </>
        }
      >
        {hasSsh(connectTarget) ? (
          <div className="space-y-4">
            <DescriptionList
              items={[
                {label: i18next.t("devbox:SSH"), value: <CodeText copyable>{sshCommand(connectTarget)}</CodeText>},
                {label: i18next.t("general:Folder"), value: <CodeText copyable>{connectTarget.sshPath}</CodeText>},
              ]}
            />
            <div className="space-y-1.5">
              <div className="text-muted-foreground text-xs">
                {i18next.t("devbox:Or add this to ~/.ssh/config and pick the host under Remote Explorer.")}
              </div>
              <CodeBlock copyable>{sshConfigEntry(connectTarget)}</CodeBlock>
            </div>
          </div>
        ) : null}
      </FormDialog>

      <PodLogsSheet
        pod={logsTarget ? {namespace: logsTarget.namespace, name: logsTarget.podName, containers: logsTarget.logContainers} : null}
        open={Boolean(logsTarget)}
        onClose={() => setLogsTarget(null)}
      />

      <DevboxRunsSheet
        devbox={runsTarget}
        open={Boolean(runsTarget)}
        onClose={() => setRunsTarget(null)}
        onChanged={() => refresh({silent: true})}
      />

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null);
            setDeleteData(false);
          }
        }}
        title={`${i18next.t("general:Delete")} ${deleteTarget?.name ?? ""}`}
        description={i18next.t("devbox:The workspace, its address and its runs are removed. Its home disk is kept unless you say otherwise.")}
        confirmText={i18next.t("general:Delete")}
        onConfirm={remove}
        extra={
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={deleteData} onCheckedChange={(checked) => setDeleteData(Boolean(checked))} />
            {i18next.t("devbox:Also delete its home disk")}
          </label>
        }
      />
    </PageContainer>
  );
}

export default DevboxPage;
