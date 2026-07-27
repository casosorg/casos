import React from "react";
import {Link} from "react-router-dom";
import {
  Alert, Button, Form, Input, Modal, Popconfirm, Space, Table, Tag, Tooltip, Typography
} from "antd";

const {Text} = Typography;
import {
  CheckCircleOutlined, DeleteOutlined, EditOutlined, KeyOutlined,
  MinusCircleOutlined, PlusOutlined, ReloadOutlined, StopOutlined
} from "@ant-design/icons";
import * as NodeBackend from "./backend/NodeBackend";
import * as Setting from "./Setting";
import {resourcePath} from "./resourceRoutes";

const statusColor = {
  Ready: "green",
  NotReady: "red",
  Unknown: "default",
};

class NodeListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      nodes: [],
      loading: true,
      error: null,
      // label-edit modal
      editModalVisible: false,
      editingNode: null,
      submitting: false,
      // kubeconfig modal
      kubeconfigModalVisible: false,
      kubeconfigNodeName: "",
      kubeconfigContent: "",
      kubeconfigLoading: false,
    };
    this.formRef = React.createRef();
  }

  componentDidMount() {
    this.fetchNodes();
  }

  fetchNodes() {
    this.setState({loading: true, error: null});
    NodeBackend.getNodes().then(res => {
      if (res.status === "ok") {
        this.setState({nodes: res.data ?? []});
      } else {
        Setting.showMessage("error", res.msg);
        this.setState({error: res.msg});
      }
    }).catch(e => {
      Setting.showMessage("error", e.message);
      this.setState({error: e.message});
    }).finally(() => this.setState({loading: false}));
  }

  // ── Cordon / Uncordon ──────────────────────────────────────────────────────

  handleCordon(node, unschedulable) {
    NodeBackend.updateNode({
      name: node.name,
      labels: node.labels,
      unschedulable,
      resourceVersion: node.resourceVersion,
    }).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", unschedulable ? "Node cordoned" : "Node uncordoned");
        this.fetchNodes();
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  // ── Label edit modal ───────────────────────────────────────────────────────

  openEditModal(node) {
    const labelEntries = Object.entries(node.labels ?? {}).map(([key, value]) => ({key, value}));
    this.setState({editModalVisible: true, editingNode: node}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({labelEntries});
      }, 0);
    });
  }

  closeEditModal() {
    this.setState({editModalVisible: false, editingNode: null});
  }

  handleEditSubmit() {
    this.formRef.current?.validateFields().then(values => {
      const labels = {};
      (values.labelEntries ?? []).forEach(({key, value}) => {
        if (key) {
          labels[key] = value ?? "";
        }
      });
      const node = this.state.editingNode;
      this.setState({submitting: true});
      NodeBackend.updateNode({
        name: node.name,
        labels,
        unschedulable: node.unschedulable,
        resourceVersion: node.resourceVersion,
      }).then(res => {
        if (res.status === "ok") {
          Setting.showMessage("success", "Node labels updated");
          this.closeEditModal();
          this.fetchNodes();
        } else {
          Setting.showMessage("error", res.msg);
        }
      }).catch(e => Setting.showMessage("error", e.message))
        .finally(() => this.setState({submitting: false}));
    });
  }

  // ── Delete ─────────────────────────────────────────────────────────────────

  handleDelete(name) {
    NodeBackend.deleteNode(name).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", "Node removed from cluster");
        this.fetchNodes();
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  // ── Worker kubeconfig ──────────────────────────────────────────────────────

  openKubeconfigModal(nodeName) {
    this.setState({kubeconfigModalVisible: true, kubeconfigNodeName: nodeName, kubeconfigContent: "", kubeconfigLoading: true});
    NodeBackend.getWorkerKubeconfig(nodeName).then(res => {
      if (res.status === "ok") {
        this.setState({kubeconfigContent: res.data?.kubeconfig ?? ""});
      } else {
        Setting.showMessage("error", res.msg);
        this.setState({kubeconfigContent: ""});
      }
    }).catch(e => {
      Setting.showMessage("error", e.message);
      this.setState({kubeconfigContent: ""});
    }).finally(() => this.setState({kubeconfigLoading: false}));
  }

  closeKubeconfigModal() {
    this.setState({kubeconfigModalVisible: false, kubeconfigContent: ""});
  }

  copyKubeconfig() {
    navigator.clipboard.writeText(this.state.kubeconfigContent).then(() => {
      Setting.showMessage("success", "Copied to clipboard");
    });
  }

  // ── Render ─────────────────────────────────────────────────────────────────

  render() {
    const {
      nodes, loading, error,
      editModalVisible, submitting,
      kubeconfigModalVisible, kubeconfigNodeName, kubeconfigContent, kubeconfigLoading,
    } = this.state;

    const columns = [
      {
        title: "Name",
        dataIndex: "name",
        key: "name",
        render: (name, record) => (
          <Space>
            <Link to={resourcePath("node", "", name)}>{name}</Link>
            {record.unschedulable && <Tag color="orange">SchedulingDisabled</Tag>}
          </Space>
        ),
      },
      {
        title: "Status",
        dataIndex: "status",
        key: "status",
        width: 110,
        render: v => <Tag color={statusColor[v] ?? "default"}>{v}</Tag>,
      },
      {
        title: "Roles",
        dataIndex: "roles",
        key: "roles",
        width: 130,
        render: roles => (roles ?? []).map(r => <Tag key={r}>{r}</Tag>),
      },
      {title: "Kubelet", dataIndex: "kubeletVersion", key: "kubeletVersion", width: 120},
      {
        title: "OS / Arch",
        key: "osArch",
        width: 130,
        render: (_, r) => r.os ? `${r.os} / ${r.arch}` : "—",
      },
      {title: "Created", dataIndex: "createdAt", key: "createdAt", width: 180},
      {
        title: "Actions",
        key: "actions",
        width: 240,
        render: (_, record) => (
          <Space>
            <Tooltip title={record.unschedulable ? "Uncordon (re-enable scheduling)" : "Cordon (disable scheduling)"}>
              <Button
                size="small"
                icon={record.unschedulable ? <CheckCircleOutlined /> : <StopOutlined />}
                onClick={() => this.handleCordon(record, !record.unschedulable)}
              >
                {record.unschedulable ? "Uncordon" : "Cordon"}
              </Button>
            </Tooltip>
            <Button
              size="small"
              icon={<EditOutlined />}
              onClick={() => this.openEditModal(record)}
            >
              Labels
            </Button>
            <Tooltip title="Generate kubeconfig for this node">
              <Button
                size="small"
                icon={<KeyOutlined />}
                onClick={() => this.openKubeconfigModal(record.name)}
              >
                Kubeconfig
              </Button>
            </Tooltip>
            <Popconfirm
              title={`Remove node "${record.name}" from cluster?`}
              description="This removes the node record. The kubelet process is not stopped."
              okText="Remove"
              okType="danger"
              cancelText="Cancel"
              onConfirm={() => this.handleDelete(record.name)}
            >
              <Button size="small" danger icon={<DeleteOutlined />}>Remove</Button>
            </Popconfirm>
          </Space>
        ),
      },
    ];

    return (
      <div style={{padding: "24px"}}>
        {error && (
          <Alert
            type="error"
            message="Failed to fetch nodes"
            description={error}
            style={{marginBottom: 16}}
            showIcon
          />
        )}

        <Table
          rowKey="name"
          columns={columns}
          dataSource={nodes}
          loading={loading}
          size="middle"
          pagination={{pageSize: 20}}
          locale={{emptyText: "No nodes registered. Start kubelet on a worker to join the cluster."}}
          title={() => (
            <div>
              <span style={{fontWeight: 600}}>Nodes</span>
              &nbsp;&nbsp;&nbsp;&nbsp;
              <Button
                icon={<ReloadOutlined />}
                onClick={() => this.fetchNodes()}
                loading={loading}
                size="small"
              >
                Refresh
              </Button>
            </div>
          )}
        />

        {/* Label edit modal */}
        <Modal
          title={`Edit Labels — ${this.state.editingNode?.name ?? ""}`}
          open={editModalVisible}
          onOk={() => this.handleEditSubmit()}
          onCancel={() => this.closeEditModal()}
          confirmLoading={submitting}
          okText="Save"
          width={560}
          destroyOnHidden
        >
          <Form ref={this.formRef} layout="vertical">
            <Form.List name="labelEntries">
              {(fields, {add, remove}) => (
                <>
                  <div style={{marginBottom: 8, fontWeight: 500}}>Labels (key=value)</div>
                  {fields.map(({key, name, ...rest}) => (
                    <Space key={key} align="baseline" style={{display: "flex", marginBottom: 4}}>
                      <Form.Item
                        {...rest}
                        name={[name, "key"]}
                        rules={[{required: true, message: "Key required"}]}
                        style={{marginBottom: 0}}
                      >
                        <Input placeholder="key" style={{width: 200}} />
                      </Form.Item>
                      <Form.Item
                        {...rest}
                        name={[name, "value"]}
                        style={{marginBottom: 0}}
                      >
                        <Input placeholder="value" style={{width: 200}} />
                      </Form.Item>
                      <MinusCircleOutlined
                        onClick={() => remove(name)}
                        style={{color: "#ff4d4f", cursor: "pointer"}}
                      />
                    </Space>
                  ))}
                  <Button
                    type="dashed"
                    onClick={() => add()}
                    icon={<PlusOutlined />}
                    style={{marginTop: 4}}
                    size="small"
                  >
                    Add Label
                  </Button>
                </>
              )}
            </Form.List>
          </Form>
        </Modal>

        {/* Worker kubeconfig modal */}
        <Modal
          title={`Worker Kubeconfig — ${kubeconfigNodeName}`}
          open={kubeconfigModalVisible}
          onCancel={() => this.closeKubeconfigModal()}
          footer={[
            <Button key="copy" type="primary" onClick={() => this.copyKubeconfig()} disabled={!kubeconfigContent}>
              Copy
            </Button>,
            <Button key="close" onClick={() => this.closeKubeconfigModal()}>Close</Button>,
          ]}
          width={680}
          destroyOnHidden
        >
          <Text type="secondary" style={{display: "block", marginBottom: 8}}>
            Save this as <Text code>/etc/kubernetes/worker.kubeconfig</Text> on the worker node, then start kubelet with <Text code>--kubeconfig=/etc/kubernetes/worker.kubeconfig</Text>.
          </Text>
          <Input.TextArea
            value={kubeconfigLoading ? "Loading…" : kubeconfigContent}
            readOnly
            autoSize={{minRows: 12, maxRows: 20}}
            style={{fontFamily: "monospace", fontSize: 12}}
          />
        </Modal>
      </div>
    );
  }
}

export default NodeListPage;
