import React, {useState} from "react";
import i18next from "i18next";
import {Ban, CheckCircle2, KeyRound, Pencil, RefreshCw, Trash2} from "lucide-react";
import * as NodeBackend from "@/backend/NodeBackend";
import * as Setting from "@/Setting";
import {runAction, useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Textarea} from "@/components/ui/textarea";
import {MessageAlert} from "@/components/ui/alert";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from "@/components/ui/dialog";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {KeyValueEditor, fromEntries, toEntries} from "@/components/shared/key-value-editor";
import {CodeText} from "@/components/shared/misc";

const STATUS_VARIANTS = {Ready: "success", NotReady: "danger", Unknown: "muted"};

function NodeListPage() {
  const {data: nodes, loading, error, refresh} = useResource(() => NodeBackend.getNodes(), [], {initialData: []});

  const [labelDialogOpen, setLabelDialogOpen] = useState(false);
  const [editingNode, setEditingNode] = useState(null);
  const [labelEntries, setLabelEntries] = useState([]);
  const [submitting, setSubmitting] = useState(false);

  const [kubeconfigOpen, setKubeconfigOpen] = useState(false);
  const [kubeconfigNode, setKubeconfigNode] = useState("");
  const [kubeconfig, setKubeconfig] = useState("");
  const [kubeconfigLoading, setKubeconfigLoading] = useState(false);

  async function handleCordon(node, unschedulable) {
    const ok = await runAction(
      NodeBackend.updateNode({
        name: node.name,
        labels: node.labels,
        unschedulable,
        resourceVersion: node.resourceVersion,
      }),
      {successMessage: unschedulable ? "Node cordoned" : "Node uncordoned"}
    );
    if (ok) {
      refresh();
    }
  }

  function openLabelDialog(node) {
    setEditingNode(node);
    setLabelEntries(toEntries(node.labels));
    setLabelDialogOpen(true);
  }

  async function handleLabelSubmit() {
    setSubmitting(true);
    const ok = await runAction(
      NodeBackend.updateNode({
        name: editingNode.name,
        labels: fromEntries(labelEntries),
        unschedulable: editingNode.unschedulable,
        resourceVersion: editingNode.resourceVersion,
      }),
      {successMessage: "Node labels updated"}
    );
    setSubmitting(false);
    if (ok) {
      setLabelDialogOpen(false);
      refresh();
    }
  }

  async function handleDelete(name) {
    const ok = await runAction(NodeBackend.deleteNode(name), {successMessage: "Node removed from cluster"});
    if (ok) {
      refresh();
    }
  }

  function openKubeconfig(nodeName) {
    setKubeconfigNode(nodeName);
    setKubeconfig("");
    setKubeconfigLoading(true);
    setKubeconfigOpen(true);
    NodeBackend.getWorkerKubeconfig(nodeName)
      .then((res) => {
        setKubeconfig(res.status === "ok" ? res.data?.kubeconfig ?? "" : "");
        if (res.status !== "ok") {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch(() => setKubeconfig(""))
      .finally(() => setKubeconfigLoading(false));
  }

  const columns = [
    {
      key: "name",
      title: i18next.t("general:Name"),
      dataIndex: "name",
      sortable: true,
      render: (name, record) => (
        <span className="flex items-center gap-2">
          <span className="font-medium">{name}</span>
          {record.unschedulable ? <Badge variant="warning">SchedulingDisabled</Badge> : null}
        </span>
      ),
    },
    {
      key: "status",
      title: i18next.t("general:Status"),
      dataIndex: "status",
      width: 120,
      sortable: true,
      render: (value) => <Badge variant={STATUS_VARIANTS[value] ?? "muted"}>{value}</Badge>,
    },
    {
      key: "roles",
      title: "Roles",
      dataIndex: "roles",
      width: 150,
      render: (roles) => (
        <div className="flex flex-wrap gap-1">
          {(roles ?? []).map((role) => (
            <Badge key={role} variant="muted">
              {role}
            </Badge>
          ))}
        </div>
      ),
    },
    {key: "kubeletVersion", title: "Kubelet", dataIndex: "kubeletVersion", width: 130, sortable: true},
    {
      key: "osArch",
      title: "OS / Arch",
      width: 150,
      render: (_, record) => (record.os ? `${record.os} / ${record.arch}` : "—"),
    },
    {key: "createdAt", title: i18next.t("general:Created"), dataIndex: "createdAt", width: 190, sortable: true},
    {
      key: "actions",
      title: i18next.t("general:Action"),
      width: 330,
      align: "right",
      render: (_, record) => (
        <div className="flex justify-end gap-2">
          <SimpleTooltip title={record.unschedulable ? "Re-enable scheduling" : "Disable scheduling"}>
            <Button variant="outline" size="sm" onClick={() => handleCordon(record, !record.unschedulable)}>
              {record.unschedulable ? <CheckCircle2 /> : <Ban />}
              {record.unschedulable ? "Uncordon" : "Cordon"}
            </Button>
          </SimpleTooltip>
          <Button variant="outline" size="sm" onClick={() => openLabelDialog(record)}>
            <Pencil />
            Labels
          </Button>
          <SimpleTooltip title="Generate kubeconfig for this node">
            <Button variant="outline" size="sm" onClick={() => openKubeconfig(record.name)}>
              <KeyRound />
              Kubeconfig
            </Button>
          </SimpleTooltip>
          <ConfirmDialog
            title={`Remove node "${record.name}" from cluster?`}
            description="This removes the node record. The kubelet process is not stopped."
            confirmText="Remove"
            onConfirm={() => handleDelete(record.name)}
          >
            <Button variant="outline" size="sm" className="text-destructive">
              <Trash2 />
            </Button>
          </ConfirmDialog>
        </div>
      ),
    },
  ];

  return (
    <PageContainer>
      {error ? <MessageAlert title="Failed to fetch nodes" description={error} /> : null}

      <DataTable
        testId="nodes-table"
        title={i18next.t("general:Nodes")}
        description={`${nodes?.length ?? 0} nodes`}
        columns={columns}
        dataSource={nodes}
        rowKey="name"
        loading={loading}
        searchable
        emptyText={i18next.t("node:No nodes registered. Add a machine and deploy it as a node from Machines.")}
        toolbar={
          <Button variant="outline" size="sm" onClick={() => refresh()} loading={loading}>
            <RefreshCw />
            {i18next.t("general:Refresh")}
          </Button>
        }
      />

      <FormDialog
        open={labelDialogOpen}
        onOpenChange={setLabelDialogOpen}
        title={`Edit Labels — ${editingNode?.name ?? ""}`}
        submitText={i18next.t("general:Save")}
        submitting={submitting}
        onSubmit={handleLabelSubmit}
      >
        <Field label="Labels" hint="Node labels drive scheduling constraints such as nodeSelector and affinity.">
          <KeyValueEditor value={labelEntries} onChange={setLabelEntries} addLabel="Add Label" />
        </Field>
      </FormDialog>

      <Dialog open={kubeconfigOpen} onOpenChange={setKubeconfigOpen}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Worker Kubeconfig — {kubeconfigNode}</DialogTitle>
            <DialogDescription>
              Save this as <CodeText>/etc/kubernetes/worker.kubeconfig</CodeText> on the worker node, then start kubelet with{" "}
              <CodeText>--kubeconfig=/etc/kubernetes/worker.kubeconfig</CodeText>.
            </DialogDescription>
          </DialogHeader>

          <Textarea
            value={kubeconfigLoading ? "Loading…" : kubeconfig}
            readOnly
            rows={14}
            className="scrollbar-thin font-mono text-xs"
          />

          <DialogFooter>
            <Button variant="outline" onClick={() => setKubeconfigOpen(false)}>
              Close
            </Button>
            <Button
              disabled={!kubeconfig}
              onClick={() => {
                navigator.clipboard.writeText(kubeconfig).then(() => Setting.showMessage("success", "Copied to clipboard"));
              }}
            >
              Copy
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </PageContainer>
  );
}

export default NodeListPage;
