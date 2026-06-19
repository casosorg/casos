import React from "react";
import {
  Alert, Badge, Button, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Switch, Table, Tag, Tooltip
} from "antd";
import {DeleteOutlined, EditOutlined, LockOutlined, MinusCircleOutlined, PlusOutlined, ReloadOutlined, UnlockOutlined} from "@ant-design/icons";
import * as IngressBackend from "./backend/IngressBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import * as Setting from "./Setting";

const PATH_TYPE_OPTIONS = [
  {label: "Prefix", value: "Prefix"},
  {label: "Exact", value: "Exact"},
  {label: "ImplementationSpecific", value: "ImplementationSpecific"},
];

class IngressListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      ingresses: [],
      namespaces: [],
      loading: true,
      error: null,
      modalVisible: false,
      modalMode: "add",
      submitting: false,
      editingIng: null,
      tlsEnabled: false,
      certStatus: {},
    };
    this.formRef = React.createRef();
  }

  componentDidMount() {
    this.fetchIngresses();
    this.fetchNamespaces();
  }

  fetchNamespaces() {
    NamespaceBackend.getNamespaces().then(res => {
      if (res.status === "ok") {
        this.setState({namespaces: res.data ?? []});
      }
    }).catch(() => {});
  }

  fetchIngresses() {
    this.setState({loading: true, error: null});
    IngressBackend.getIngresses().then(res => {
      if (res.status === "ok") {
        const ingresses = res.data ?? [];
        this.setState({ingresses}, () => {
          this.fetchCertStatuses(ingresses);
        });
      } else {
        Setting.showMessage("error", res.msg);
        this.setState({error: res.msg});
      }
    }).catch(e => {
      Setting.showMessage("error", e.message);
      this.setState({error: e.message});
    }).finally(() => {
      this.setState({loading: false});
    });
  }

  fetchCertStatuses(ingresses) {
    const tlsIngresses = ingresses.filter(ing => ing.tlsEnabled);
    if (tlsIngresses.length === 0) {return;}
    const updates = {};
    const promises = tlsIngresses.map(ing =>
      IngressBackend.getIngressCertStatus(ing.namespace, ing.name)
        .then(res => {
          if (res.status === "ok") {
            updates[`${ing.namespace}/${ing.name}`] = res.data;
          }
        })
        .catch(() => {})
    );
    Promise.all(promises).then(() => {
      this.setState(prev => ({certStatus: {...prev.certStatus, ...updates}}));
    });
  }

  openAddModal() {
    this.setState({modalVisible: true, modalMode: "add", editingIng: null, tlsEnabled: false}, () => {
      const defaultNs = this.state.namespaces.length > 0 ? this.state.namespaces[0].name : "default";
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({
          name: "",
          namespace: defaultNs,
          ingressClass: "",
          tlsEnabled: false,
          clusterIssuer: "letsencrypt-prod",
          rules: [{host: "", path: "/", pathType: "Prefix", serviceName: "", servicePort: 80}],
        });
      }, 0);
    });
  }

  openEditModal(ing) {
    this.setState({modalVisible: true, modalMode: "edit", editingIng: ing, tlsEnabled: ing.tlsEnabled ?? false}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({
          name: ing.name,
          namespace: ing.namespace,
          ingressClass: ing.ingressClass,
          tlsEnabled: ing.tlsEnabled ?? false,
          clusterIssuer: ing.clusterIssuer || "letsencrypt-prod",
          rules: (ing.rules ?? []).map(r => ({
            host: r.host,
            path: r.path,
            pathType: r.pathType || "Prefix",
            serviceName: r.serviceName,
            servicePort: r.servicePort,
          })),
        });
      }, 0);
    });
  }

  closeModal() {
    this.setState({modalVisible: false, editingIng: null, tlsEnabled: false});
  }

  handleSubmit() {
    this.formRef.current?.validateFields().then(values => {
      const payload = {
        name: values.name,
        namespace: values.namespace,
        ingressClass: values.ingressClass ?? "",
        tlsEnabled: values.tlsEnabled ?? false,
        clusterIssuer: values.tlsEnabled ? (values.clusterIssuer || "letsencrypt-prod") : "",
        rules: (values.rules ?? []).map(r => ({
          host: r.host ?? "",
          path: r.path ?? "/",
          pathType: r.pathType ?? "Prefix",
          serviceName: r.serviceName ?? "",
          servicePort: r.servicePort ?? 80,
        })),
      };

      this.setState({submitting: true});

      if (this.state.modalMode === "add") {
        IngressBackend.addIngress(payload).then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", "Ingress created");
            this.closeModal();
            this.fetchIngresses();
          } else {
            Setting.showMessage("error", res.msg);
          }
        }).catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      } else {
        const ing = this.state.editingIng;
        IngressBackend.updateIngress({
          ...payload,
          resourceVersion: ing.resourceVersion,
        }).then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", "Ingress updated");
            this.closeModal();
            this.fetchIngresses();
          } else {
            Setting.showMessage("error", res.msg);
          }
        }).catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      }
    });
  }

  handleDelete(ing) {
    IngressBackend.deleteIngress(ing.namespace, ing.name).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", "Ingress deleted");
        this.fetchIngresses();
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  renderTLSCell(record) {
    if (!record.tlsEnabled) {
      return (
        <Tag icon={<UnlockOutlined />} color="default">HTTP</Tag>
      );
    }
    const key = `${record.namespace}/${record.name}`;
    const cert = this.state.certStatus[key];
    if (!cert) {
      return (
        <Tooltip title="Checking certificate status...">
          <Badge status="processing" text={<Tag icon={<LockOutlined />} color="blue">HTTPS</Tag>} />
        </Tooltip>
      );
    }
    if (cert.status === "pending") {
      return (
        <Tooltip title={`Issuer: ${record.clusterIssuer || "—"} — Certificate is being issued by cert-manager`}>
          <Tag icon={<LockOutlined />} color="orange">HTTPS (pending)</Tag>
        </Tooltip>
      );
    }
    if (cert.status === "ready") {
      return (
        <Tooltip title={`Issuer: ${record.clusterIssuer || "—"} — Certificate valid until ${cert.expiry}`}>
          <Tag icon={<LockOutlined />} color="green">HTTPS · {cert.expiry}</Tag>
        </Tooltip>
      );
    }
    return <Tag icon={<LockOutlined />} color="blue">HTTPS</Tag>;
  }

  render() {
    const {ingresses, namespaces, loading, error, modalVisible, modalMode, submitting, tlsEnabled} = this.state;

    const nsOptions = namespaces.map(ns => ({label: ns.name, value: ns.name}));

    const columns = [
      {title: "Namespace", dataIndex: "namespace", key: "namespace", width: 150},
      {title: "Name", dataIndex: "name", key: "name"},
      {
        title: "Ingress Class",
        dataIndex: "ingressClass",
        key: "ingressClass",
        width: 150,
        render: v => v || <span style={{color: "#bbb"}}>—</span>,
      },
      {
        title: "TLS",
        key: "tls",
        width: 170,
        render: (_, record) => this.renderTLSCell(record),
      },
      {
        title: "Rules",
        dataIndex: "rules",
        key: "rules",
        render: rules => (
          <Space direction="vertical" size={2}>
            {(rules ?? []).map((r, i) => (
              <Tag key={i} style={{fontFamily: "monospace", fontSize: 12}}>
                {r.host || "*"}{r.path} → {r.serviceName}:{r.servicePort}
              </Tag>
            ))}
          </Space>
        ),
      },
      {title: "Created", dataIndex: "createdAt", key: "createdAt", width: 180},
      {
        title: "Actions",
        key: "actions",
        width: 140,
        render: (_, record) => (
          <Space>
            <Button size="small" icon={<EditOutlined />} onClick={() => this.openEditModal(record)}>Edit</Button>
            <Popconfirm
              title={`Delete Ingress "${record.name}"?`}
              okText="Delete"
              okType="danger"
              cancelText="Cancel"
              onConfirm={() => this.handleDelete(record)}
            >
              <Button size="small" danger icon={<DeleteOutlined />}>Delete</Button>
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
            message="Failed to fetch Ingresses"
            description={error}
            style={{marginBottom: 16}}
            showIcon
          />
        )}

        <Table
          rowKey={r => `${r.namespace}/${r.name}`}
          columns={columns}
          dataSource={ingresses}
          loading={loading}
          size="middle"
          pagination={{pageSize: 20}}
          locale={{emptyText: "No Ingresses found"}}
          title={() => (
            <div>
              <span style={{fontWeight: 600}}>Ingresses</span>
              &nbsp;&nbsp;&nbsp;&nbsp;
              <Button icon={<ReloadOutlined />} onClick={() => this.fetchIngresses()} loading={loading} size="small">
                Refresh
              </Button>
              &nbsp;&nbsp;
              <Button type="primary" icon={<PlusOutlined />} size="small" onClick={() => this.openAddModal()}>
                Add
              </Button>
            </div>
          )}
        />

        <Modal
          title={modalMode === "add" ? "Add Ingress" : "Edit Ingress"}
          open={modalVisible}
          onOk={() => this.handleSubmit()}
          onCancel={() => this.closeModal()}
          confirmLoading={submitting}
          okText={modalMode === "add" ? "Create" : "Update"}
          width={700}
          destroyOnHidden
        >
          <Form ref={this.formRef} layout="vertical">
            <Form.Item
              label="Namespace"
              name="namespace"
              rules={[{required: true, message: "Namespace is required"}]}
            >
              <Select
                disabled={modalMode === "edit"}
                options={nsOptions}
                placeholder="Select a namespace"
                showSearch
              />
            </Form.Item>
            <Form.Item
              label="Name"
              name="name"
              rules={[{required: true, message: "Name is required"}]}
            >
              <Input disabled={modalMode === "edit"} placeholder="my-ingress" />
            </Form.Item>
            <Form.Item label="Ingress Class" name="ingressClass">
              <Input placeholder="nginx (optional)" />
            </Form.Item>

            <Form.Item
              label={
                <span>
                  <LockOutlined style={{marginRight: 6, color: tlsEnabled ? "#52c41a" : undefined}} />
                  Enable HTTPS (cert-manager)
                </span>
              }
              name="tlsEnabled"
              valuePropName="checked"
              extra={tlsEnabled ? "cert-manager will automatically issue and renew a TLS certificate" : "Enable to get a free TLS certificate via cert-manager + Let's Encrypt"}
            >
              <Switch
                onChange={v => {
                  this.setState({tlsEnabled: v});
                  this.formRef.current?.setFieldValue("tlsEnabled", v);
                }}
              />
            </Form.Item>

            {tlsEnabled && (
              <Form.Item
                label="Cluster Issuer"
                name="clusterIssuer"
                rules={[{required: true, message: "Cluster issuer is required when HTTPS is enabled"}]}
                extra="The cert-manager ClusterIssuer name to use (must be pre-installed in the cluster)"
              >
                <Input placeholder="letsencrypt-prod" />
              </Form.Item>
            )}

            <Form.List name="rules">
              {(fields, {add, remove}) => (
                <>
                  <div style={{marginBottom: 8, fontWeight: 500}}>Rules</div>
                  {fields.map(({key, name, ...rest}) => (
                    <div key={key} style={{border: "1px solid #f0f0f0", borderRadius: 6, padding: "12px 12px 4px", marginBottom: 10, position: "relative"}}>
                      <MinusCircleOutlined
                        onClick={() => remove(name)}
                        style={{position: "absolute", top: 10, right: 10, color: "#ff4d4f", cursor: "pointer"}}
                      />
                      <Space wrap>
                        <Form.Item
                          {...rest}
                          name={[name, "host"]}
                          label="Host"
                          style={{marginBottom: 8, minWidth: 160}}
                        >
                          <Input placeholder="example.com (leave blank for *)" style={{width: 180}} />
                        </Form.Item>
                        <Form.Item
                          {...rest}
                          name={[name, "path"]}
                          label="Path"
                          rules={[{required: true, message: "Path required"}]}
                          style={{marginBottom: 8}}
                        >
                          <Input placeholder="/" style={{width: 100}} />
                        </Form.Item>
                        <Form.Item
                          {...rest}
                          name={[name, "pathType"]}
                          label="Path Type"
                          style={{marginBottom: 8}}
                        >
                          <Select options={PATH_TYPE_OPTIONS} style={{width: 160}} />
                        </Form.Item>
                        <Form.Item
                          {...rest}
                          name={[name, "serviceName"]}
                          label="Service"
                          rules={[{required: true, message: "Service name required"}]}
                          style={{marginBottom: 8}}
                        >
                          <Input placeholder="my-service" style={{width: 150}} />
                        </Form.Item>
                        <Form.Item
                          {...rest}
                          name={[name, "servicePort"]}
                          label="Port"
                          rules={[{required: true, message: "Port required"}]}
                          style={{marginBottom: 8}}
                        >
                          <InputNumber min={1} max={65535} style={{width: 90}} />
                        </Form.Item>
                      </Space>
                    </div>
                  ))}
                  <Button
                    type="dashed"
                    onClick={() => add({host: "", path: "/", pathType: "Prefix", serviceName: "", servicePort: 80})}
                    icon={<PlusOutlined />}
                    style={{marginTop: 4}}
                    size="small"
                  >
                    Add Rule
                  </Button>
                </>
              )}
            </Form.List>
          </Form>
        </Modal>
      </div>
    );
  }
}

export default IngressListPage;
