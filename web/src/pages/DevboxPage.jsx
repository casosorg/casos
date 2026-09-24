import React, {useState} from "react";
import i18next from "i18next";
import {useTranslation} from "react-i18next";
import {Code2, ExternalLink, Laptop, Play, Plus, Square, Trash2} from "lucide-react";
import * as DevboxBackend from "@/backend/DevboxBackend";
import * as ImageBackend from "@/backend/ImageBackend";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {Textarea} from "@/components/ui/textarea";
import {Checkbox} from "@/components/ui/checkbox";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {DataTable} from "@/components/shared/data-table";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {SimpleSelect} from "@/components/shared/simple-select";
import {StatusBadge} from "@/components/shared/status-badge";
import {CodeBlock, CodeText, DescriptionList} from "@/components/shared/misc";
import {runAction, useResource} from "@/hooks/use-resource";
import {useWorkspace} from "@/hooks/use-workspace";

const POLL_INTERVAL = 15000;

const DEVBOX_STATUS_VARIANTS = {
  deployed: "success",
  pending: "warning",
  failed: "danger",
  stopped: "muted",
};

const emptyForm = (namespace = "default") => ({name: "", namespace, image: "", diskSize: "5Gi", password: "", sshPublicKey: ""});

const hasSsh = (box) => Boolean(box?.sshHost && box?.sshPort);

const sshCommand = (box) => `ssh -p ${box.sshPort} ${box.sshUser}@${box.sshHost}`;
const vscodeLink = (box) => `vscode://vscode-remote/ssh-remote+${box.sshUser}@${box.sshHost}:${box.sshPort}${box.sshPath || ""}`;
const sshConfigEntry = (box) => [
  `Host ${box.name}`,
  `  HostName ${box.sshHost}`,
  `  User ${box.sshUser}`,
  `  Port ${box.sshPort}`,
].join("\n");

function DevboxPage() {
  useTranslation();
  const {workspace} = useWorkspace();
  const defaultNamespace = workspace && workspace !== "all" ? workspace : "default";
  const [namespace, setNamespace] = useState("all");
  const [createOpen, setCreateOpen] = useState(false);
  const [form, setForm] = useState(emptyForm);
  const [submitting, setSubmitting] = useState(false);
  const [created, setCreated] = useState(null);
  const [connectTarget, setConnectTarget] = useState(null);
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

  function openCreate() {
    setForm(emptyForm(defaultNamespace));
    setCreateOpen(true);
  }

  async function submitCreate() {
    if (!form.name.trim()) {
      return;
    }
    setSubmitting(true);
    try {
      await runAction(
        DevboxBackend.deployDevbox({
          name: form.name.trim(),
          namespace: form.namespace,
          image: form.image.trim(),
          diskSize: form.diskSize.trim(),
          password: form.password.trim(),
          sshPublicKey: form.sshPublicKey.trim(),
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

  const columns = [
    {
      key: "name",
      title: i18next.t("devbox:DevBox"),
      dataIndex: "name",
      minWidth: 200,
      sortable: true,
      render: (value, record) => (
        <div className="flex items-center gap-2">
          <Code2 className="text-muted-foreground size-4 shrink-0" />
          <div className="min-w-0">
            <div className="truncate font-medium">{value}</div>
            <div className="text-muted-foreground truncate text-xs">{record.namespace}</div>
          </div>
        </div>
      ),
    },
    {
      key: "status",
      title: i18next.t("general:Status"),
      dataIndex: "status",
      width: 120,
      sortable: true,
      render: (value) => <StatusBadge status={value} variants={DEVBOX_STATUS_VARIANTS} />,
    },
    {
      key: "image",
      title: i18next.t("general:Image"),
      dataIndex: "image",
      minWidth: 180,
      ellipsis: true,
      render: (value) => <span className="font-mono text-xs">{value}</span>,
    },
    {
      key: "ready",
      title: i18next.t("launchpad:Copies"),
      width: 90,
      render: (_value, record) => <span className="tabular-nums">{record.ready ?? 0} / {record.replicas ?? 0}</span>,
    },
    {
      key: "createdAt",
      title: i18next.t("general:Created"),
      dataIndex: "createdAt",
      width: 170,
      sortable: true,
    },
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 240,
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
        description={i18next.t("devbox:A code-server container, reachable over a node port. Leave a field blank for its default.")}
        onSubmit={submitCreate}
        submitText={i18next.t("general:Create")}
        submitting={submitting}
        submitDisabled={!form.name.trim()}
      >
        <Field label={i18next.t("general:Name")} htmlFor="devbox-name" required>
          <Input
            id="devbox-name"
            value={form.name}
            onChange={(e) => setField("name", e.target.value)}
            placeholder="my-devbox"
            data-testid="devbox-name-input"
          />
        </Field>
        <Field label={i18next.t("general:Namespace")} htmlFor="devbox-namespace">
          <SimpleSelect
            value={form.namespace}
            onChange={(value) => setField("namespace", value)}
            options={namespaces.map((item) => ({label: item.name, value: item.name}))}
            className="w-full"
          />
        </Field>
        <Field
          label={i18next.t("general:Image")}
          htmlFor="devbox-image"
          hint={i18next.t("devbox:Defaults to codercom/code-server:latest.")}
        >
          <Input
            id="devbox-image"
            value={form.image}
            onChange={(e) => setField("image", e.target.value)}
            placeholder="codercom/code-server:latest"
          />
        </Field>
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
        <Field
          label={i18next.t("devbox:SSH public key")}
          htmlFor="devbox-ssh-key"
          hint={i18next.t("devbox:Paste a .pub key to also reach this box from the VS Code on your machine, over Remote-SSH. Blank keeps it browser-only.")}
        >
          <Textarea
            id="devbox-ssh-key"
            rows={3}
            className="font-mono text-xs"
            value={form.sshPublicKey}
            onChange={(e) => setField("sshPublicKey", e.target.value)}
            placeholder="ssh-ed25519 AAAA... you@laptop"
            data-testid="devbox-ssh-key-input"
          />
        </Field>
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
            ...(hasSsh(created) ? [{label: i18next.t("devbox:SSH"), value: <CodeText copyable>{sshCommand(created)}</CodeText>}] : []),
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
        description={i18next.t("devbox:Open this workspace in the VS Code on your machine. It needs the Remote - SSH extension and the private key that matches the one you gave the box.")}
        footer={
          <>
            <Button variant="outline" onClick={() => setConnectTarget(null)}>
              {i18next.t("general:Close")}
            </Button>
            {connectTarget && hasSsh(connectTarget) ? (
              <Button
                data-testid="devbox-open-desktop"
                onClick={() => {
                  // A vscode:// link is handed to the OS, not opened in a tab.
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
        {connectTarget && hasSsh(connectTarget) ? (
          <div className="space-y-4">
            <DescriptionList
              items={[
                {label: i18next.t("general:Name"), value: connectTarget.name},
                {label: i18next.t("devbox:SSH"), value: <CodeText copyable>{sshCommand(connectTarget)}</CodeText>},
                {label: i18next.t("devbox:Workspace folder"), value: <CodeText copyable>{connectTarget.sshPath}</CodeText>},
              ]}
            />
            <div className="space-y-1.5">
              <div className="text-muted-foreground text-xs">
                {i18next.t("devbox:If the button does not reach it, add this to ~/.ssh/config and pick the host in VS Code.")}
              </div>
              <CodeBlock copyable>{sshConfigEntry(connectTarget)}</CodeBlock>
            </div>
          </div>
        ) : (
          <div className="text-muted-foreground text-sm">
            {i18next.t("devbox:Created without an SSH key, so this one is browser-only.")}
          </div>
        )}
      </FormDialog>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null);
            setDeleteData(false);
          }
        }}
        title={`${i18next.t("general:Delete")} ${deleteTarget?.name ?? ""}`}
        description={i18next.t("devbox:The workspace and its address are removed. Its home disk is kept unless you say otherwise.")}
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
