import React from "react";
import {
  Alert, Button, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Table, Tag
} from "antd";
import {DeleteOutlined, EditOutlined, MinusCircleOutlined, PlusOutlined, ReloadOutlined} from "@ant-design/icons";
import * as ServiceBackend from "./backend/ServiceBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import * as NodeBackend from "./backend/NodeBackend";
import * as Setting from "./Setting";

const SERVICE_TYPES = ["ClusterIP", "NodePort", "LoadBalancer", "ExternalName"];
const PROTOCOLS = ["TCP", "UDP", "SCTP"];

const typeColor = {
  ClusterIP: "blue",
  NodePort: "green",
  LoadBalancer: "purple",
  ExternalName: "orange",
};

function portsToFormRows(ports) {
  return (ports ?? []).map(p => ({
    name: p.name,
    protocol: p.protocol || "TCP",
    port: p.port,
    targetPort: p.targetPort,
    nodePort: p.nodePort || null,
  }));
}

function formRowsToRequest(rows) {
  return (rows ?? []).map(r => ({
    name: r.name ?? "",
    protocol: r.protocol || "TCP",
    port: Number(r.port),
    targetPort: String(r.targetPort ?? r.port ?? ""),
    nodePort: r.nodePort ? Number(r.nodePort) : 0,
  }));
}

function getDefaultPorts(type) {
  if (type === "ExternalName") {
    return [];
  }
  return [{name: "", protocol: "TCP", port: 80, targetPort: "80"}];
}

function selectorToEntries(selector) {
  return Object.entries(selector ?? {}).map(([key, value]) => ({key, value}));
}

function entriesToMap(entries) {
  const m = {};
  (entries ?? []).forEach(({key, value}) => {
    if (key) {
      m[key] = value ?? "";
    }
  });
  return m;
}

class ServiceListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      services: [],
      namespaces: [],
      nodes: [],
      loading: true,
      error: null,
      modalVisible: false,
      modalMode: "add",
      submitting: false,
      editingSvc: null,
    };
    this.formRef = React.createRef();
  }

  componentDidMount() {
    this.fetchServices();
    this.fetchNamespaces();
    this.fetchNodes();
  }

  fetchNamespaces() {
    NamespaceBackend.getNamespaces().then(res => {
      if (res.status === "ok") {
        this.setState({namespaces: res.data ?? []});
      }
    }).catch(() => {});
  }

  fetchNodes() {
    NodeBackend.getNodes().then(res => {
      if (res.status === "ok") {
        this.setState({nodes: res.data ?? []});
      }
    }).catch(() => {});
  }

  getNodeIP() {
    const {nodes} = this.state;
    for (const n of nodes) {
      if (n.externalIP) {
        return n.externalIP;
      }
    }
    for (const n of nodes) {
      if (n.internalIP) {
        return n.internalIP;
      }
    }
    return null;
  }

  fetchServices() {
    this.setState({loading: true, error: null});
    ServiceBackend.getServices().then(res => {
      if (res.status === "ok") {
        this.setState({services: res.data ?? []});
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

  openAddModal() {
    this.setState({modalVisible: true, modalMode: "add", editingSvc: null}, () => {
      const defaultNs = this.state.namespaces.length > 0 ? this.state.namespaces[0].name : "default";
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({
          namespace: defaultNs,
          name: "",
          type: "ClusterIP",
          externalName: "",
          selectorEntries: [],
          ports: getDefaultPorts("ClusterIP"),
        });
      }, 0);
    });
  }

  openEditModal(svc) {
    this.setState({modalVisible: true, modalMode: "edit", editingSvc: svc}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({
          namespace: svc.namespace,
          name: svc.name,
          type: svc.type,
          externalName: svc.externalName,
          selectorEntries: selectorToEntries(svc.selector),
          ports: portsToFormRows(svc.ports),
        });
      }, 0);
    });
  }

  closeModal() {
    this.setState({modalVisible: false, editingSvc: null});
  }

  handleSubmit() {
    this.formRef.current?.validateFields().then(values => {
      const payload = {
        namespace: values.namespace,
        name: values.name,
        type: values.type,
        externalName: values.externalName ?? "",
        selector: values.type === "ExternalName" ? {} : entriesToMap(values.selectorEntries),
        ports: values.type === "ExternalName" ? [] : formRowsToRequest(values.ports),
      };

      this.setState({submitting: true});

      if (this.state.modalMode === "add") {
        ServiceBackend.addService(payload).then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", "Service created");
            this.closeModal();
            this.fetchServices();
          } else {
            Setting.showMessage("error", res.msg);
          }
        }).catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      } else {
        const svc = this.state.editingSvc;
        ServiceBackend.updateService({
          ...payload,
          resourceVersion: svc.resourceVersion,
        }).then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", "Service updated");
            this.closeModal();
            this.fetchServices();
          } else {
            Setting.showMessage("error", res.msg);
          }
        }).catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      }
    });
  }

  handleDelete(svc) {
    ServiceBackend.deleteService(svc.namespace, svc.name).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", "Service deleted");
        this.fetchServices();
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  render() {
    const {services, namespaces, loading, error, modalVisible, modalMode, submitting} = this.state;
    const nodeIP = this.getNodeIP();
    const currentType = this.formRef.current?.getFieldValue("type") || "ClusterIP";
    const isExternalName = currentType === "ExternalName";

    const nsOptions = namespaces.map(ns => ({label: ns.name, value: ns.name}));

    const columns = [
      {title: "Namespace", dataIndex: "namespace", key: "namespace", width: 140},
      {title: "Name", dataIndex: "name", key: "name"},
      {
        title: "Type",
        dataIndex: "type",
        key: "type",
        width: 120,
        render: t => <Tag color={typeColor[t] ?? "default"}>{t}</Tag>,
      },
      {title: "Cluster IP", dataIndex: "clusterIP", key: "clusterIP", width: 130},
      {
        title: "External Name",
        dataIndex: "externalName",
        key: "externalName",
        width: 180,
        render: v => v || null,
      },
      {
        title: "Ports",
        dataIndex: "ports",
        key: "ports",
        render: ports => (ports ?? []).map((p, i) => (
          <Tag key={i}>{`${p.protocol} ${p.port}${p.targetPort !== String(p.port) ? `:${p.targetPort}` : ""}${p.nodePort ? `:${p.nodePort}` : ""}`}</Tag>
        )),
      },
      {
        title: "Access URL",
        key: "accessUrl",
        render: (_, record) => {
          if (record.type !== "NodePort" || !nodeIP) {
            return null;
          }
          return (record.ports ?? []).filter(p => p.nodePort).map((p, i) => {
            const url = `http://${nodeIP}:${p.nodePort}`;
            return (
              <a key={i} href={url} target="_blank" rel="noopener noreferrer" style={{display: "block"}}>
                {url}
              </a>
            );
          });
        },
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
              title={`Delete Service "${record.name}"?`}
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
          <Alert type="error" message="Failed to fetch Services" description={error} style={{marginBottom: 16}} showIcon />
        )}

        <Table
          rowKey={r => `${r.namespace}/${r.name}`}
          columns={columns}
          dataSource={services}
          loading={loading}
          size="middle"
          pagination={{pageSize: 20}}
          locale={{emptyText: "No Services found"}}
          title={() => (
            <div>
              <span style={{fontWeight: 600}}>Services</span>
              &nbsp;&nbsp;&nbsp;&nbsp;
              <Button icon={<ReloadOutlined />} onClick={() => this.fetchServices()} loading={loading} size="small">
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
          title={modalMode === "add" ? "Add Service" : "Edit Service"}
          open={modalVisible}
          onOk={() => this.handleSubmit()}
          onCancel={() => this.closeModal()}
          confirmLoading={submitting}
          okText={modalMode === "add" ? "Create" : "Update"}
          width={640}
          destroyOnHidden
        >
          <Form
            ref={this.formRef}
            layout="vertical"
            onValuesChange={(changedValues) => {
              if (Object.prototype.hasOwnProperty.call(changedValues, "type")) {
                const nextType = changedValues.type;
                if (nextType === "ExternalName") {
                  this.formRef.current?.setFieldsValue({
                    externalName: this.formRef.current?.getFieldValue("externalName") ?? "",
                    selectorEntries: [],
                    ports: [],
                  });
                } else if ((this.formRef.current?.getFieldValue("ports") ?? []).length === 0) {
                  this.formRef.current?.setFieldsValue({ports: getDefaultPorts(nextType)});
                }
              }
              this.forceUpdate();
            }}
          >
            <Form.Item label="Namespace" name="namespace" rules={[{required: true, message: "Required"}]}>
              <Select disabled={modalMode === "edit"} options={nsOptions} placeholder="Select a namespace" showSearch />
            </Form.Item>
            <Form.Item label="Name" name="name" rules={[{required: true, message: "Required"}]}>
              <Input disabled={modalMode === "edit"} placeholder="my-service" />
            </Form.Item>
            <Form.Item label="Type" name="type" rules={[{required: true, message: "Required"}]}>
              <Select options={SERVICE_TYPES.map(t => ({label: t, value: t}))} />
            </Form.Item>

            {isExternalName ? (
              <Form.Item
                label="External Name"
                name="externalName"
                rules={[{required: true, message: "External name is required"}]}
              >
                <Input placeholder="example.com" />
              </Form.Item>
            ) : (
              <>
                <Form.List name="ports">
                  {(fields, {add, remove}) => (
                    <>
                      <div style={{marginBottom: 8, fontWeight: 500}}>Ports</div>
                      {fields.map(({key, name, ...rest}) => (
                        <Space key={key} align="baseline" style={{display: "flex", marginBottom: 4, flexWrap: "wrap"}}>
                          <Form.Item {...rest} name={[name, "protocol"]} style={{marginBottom: 0}}>
                            <Select options={PROTOCOLS.map(p => ({label: p, value: p}))} style={{width: 80}} />
                          </Form.Item>
                          <Form.Item {...rest} name={[name, "port"]} rules={[{required: true, message: "Port required"}]} style={{marginBottom: 0}}>
                            <InputNumber placeholder="port" min={1} max={65535} style={{width: 90}} />
                          </Form.Item>
                          <Form.Item {...rest} name={[name, "targetPort"]} style={{marginBottom: 0}}>
                            <Input placeholder="targetPort" style={{width: 100}} />
                          </Form.Item>
                          {currentType === "NodePort" && (
                            <Form.Item {...rest} name={[name, "nodePort"]} style={{marginBottom: 0}}>
                              <InputNumber placeholder="nodePort" min={30000} max={32767} style={{width: 110}} />
                            </Form.Item>
                          )}
                          <Form.Item {...rest} name={[name, "name"]} style={{marginBottom: 0}}>
                            <Input placeholder="name (opt)" style={{width: 110}} />
                          </Form.Item>
                          <MinusCircleOutlined onClick={() => remove(name)} style={{color: "#ff4d4f", cursor: "pointer"}} />
                        </Space>
                      ))}
                      <Button type="dashed" onClick={() => add({protocol: "TCP", port: 80, targetPort: "80"})} icon={<PlusOutlined />} size="small" style={{marginTop: 4}}>
                        Add Port
                      </Button>
                    </>
                  )}
                </Form.List>

                <Form.List name="selectorEntries">
                  {(fields, {add, remove}) => (
                    <>
                      <div style={{marginBottom: 8, marginTop: 16, fontWeight: 500}}>Selector (key-value)</div>
                      {fields.map(({key, name, ...rest}) => (
                        <Space key={key} align="baseline" style={{display: "flex", marginBottom: 4}}>
                          <Form.Item {...rest} name={[name, "key"]} rules={[{required: true, message: "Key required"}]} style={{marginBottom: 0}}>
                            <Input placeholder="key" style={{width: 180}} />
                          </Form.Item>
                          <Form.Item {...rest} name={[name, "value"]} style={{marginBottom: 0}}>
                            <Input placeholder="value" style={{width: 200}} />
                          </Form.Item>
                          <MinusCircleOutlined onClick={() => remove(name)} style={{color: "#ff4d4f", cursor: "pointer"}} />
                        </Space>
                      ))}
                      <Button type="dashed" onClick={() => add()} icon={<PlusOutlined />} size="small" style={{marginTop: 4}}>
                        Add Selector
                      </Button>
                    </>
                  )}
                </Form.List>
              </>
            )}
          </Form>
        </Modal>
      </div>
    );
  }
}

export default ServiceListPage;
