import React from "react";
import {
  Alert, Button, Drawer, Form, Input, Modal, Popconfirm, Select, Space, Switch, Table, Tag, Tooltip
} from "antd";
import {AppstoreOutlined, CodeOutlined, DeleteOutlined, DesktopOutlined, EditOutlined, FileTextOutlined, GlobalOutlined, MinusCircleOutlined, PlusOutlined, ReloadOutlined, UnorderedListOutlined} from "@ant-design/icons";
import * as PodBackend from "./backend/PodBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import * as Setting from "./Setting";
import DockerHubModal from "./DockerHubModal";

const phaseColor = {
  Running: "green",
  Pending: "gold",
  Succeeded: "blue",
  Failed: "red",
  Unknown: "default",
};

const uiModeButton = {
  web: {icon: <GlobalOutlined />, label: "Open"},
  vnc: {icon: <DesktopOutlined />, label: "Open noVNC"},
  terminal: {icon: <CodeOutlined />, label: "Open Terminal"},
  unknown: {icon: <GlobalOutlined />, label: "Open"},
};

class PodListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      pods: [],
      namespaces: [],
      loading: true,
      error: null,
      modalVisible: false,
      modalMode: "add",
      submitting: false,
      editingPod: null,
      eventsDrawerVisible: false,
      eventsPod: null,
      events: [],
      eventsLoading: false,
      eventsError: null,
      logsDrawerVisible: false,
      logsPod: null,
      logs: "",
      logsLoading: false,
      logsError: null,
      marketplaceVisible: false,
      modalInitialValues: {},
      openingPodKey: null,
    };
    this.formRef = React.createRef();
  }

  componentDidMount() {
    this.fetchPods();
    this.fetchNamespaces();
  }

  componentWillUnmount() {
    this.stopEventsPolling();
    this.stopLogsPolling();
  }

  openEventsDrawer(pod) {
    this.setState({eventsDrawerVisible: true, eventsPod: pod, events: [], eventsError: null}, () => {
      this.fetchEvents();
      this._eventsTimer = setInterval(() => this.fetchEvents(), 3000);
    });
  }

  closeEventsDrawer() {
    this.stopEventsPolling();
    this.setState({eventsDrawerVisible: false, eventsPod: null, events: []});
  }

  stopEventsPolling() {
    if (this._eventsTimer) {
      clearInterval(this._eventsTimer);
      this._eventsTimer = null;
    }
  }

  fetchEvents() {
    const {eventsPod} = this.state;
    if (!eventsPod) {
      return;
    }
    this.setState({eventsLoading: true});
    PodBackend.getPodEvents(eventsPod.namespace, eventsPod.name).then(res => {
      if (res.status === "ok") {
        this.setState({events: res.data ?? [], eventsError: null});
      } else {
        this.setState({eventsError: res.msg});
      }
    }).catch(e => {
      this.setState({eventsError: e.message});
    }).finally(() => {
      this.setState({eventsLoading: false});
    });
  }

  openLogsDrawer(pod) {
    this.setState({logsDrawerVisible: true, logsPod: pod, logs: "", logsError: null}, () => {
      this.fetchLogs();
      this._logsTimer = setInterval(() => this.fetchLogs(), 3000);
    });
  }

  closeLogsDrawer() {
    this.stopLogsPolling();
    this.setState({logsDrawerVisible: false, logsPod: null, logs: ""});
  }

  stopLogsPolling() {
    if (this._logsTimer) {
      clearInterval(this._logsTimer);
      this._logsTimer = null;
    }
  }

  fetchLogs() {
    const {logsPod} = this.state;
    if (!logsPod) {
      return;
    }
    this.setState({logsLoading: true});
    PodBackend.getPodLogs(logsPod.namespace, logsPod.name).then(res => {
      if (res.status === "ok") {
        this.setState({logs: res.data ?? "", logsError: null}, () => {
          if (this._logsEndRef) {
            this._logsEndRef.scrollIntoView({behavior: "smooth"});
          }
        });
      } else {
        this.setState({logsError: res.msg});
      }
    }).catch(e => {
      this.setState({logsError: e.message});
    }).finally(() => {
      this.setState({logsLoading: false});
    });
  }

  fetchNamespaces() {
    NamespaceBackend.getNamespaces().then(res => {
      if (res.status === "ok") {
        this.setState({namespaces: res.data ?? []});
      }
    }).catch(() => {});
  }

  fetchPods() {
    this.setState({loading: true, error: null});
    PodBackend.getPods().then(res => {
      if (res.status === "ok") {
        this.setState({pods: res.data ?? []});
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
    const nsList = this.state.namespaces.map(n => n.name);
    const defaultNs = nsList.includes("default") ? "default" : (nsList[0] ?? "default");
    this.setState({modalVisible: true, modalMode: "add", editingPod: null, modalInitialValues: {
      namespace: defaultNs,
      name: "",
      image: "",
      containerName: "",
      labelEntries: [],
      portEntries: [],
      injectTtydSidecar: true,
    }});
  }

  deriveNameFromImage(image) {
    const base = image.split(":")[0].split("/").pop();
    const safe = base.toLowerCase().replace(/[^a-z0-9-]/g, "-").replace(/-+/g, "-").replace(/^-|-$/g, "");
    this.formRef.current?.setFieldsValue({
      name: safe,
      containerName: safe,
    });
  }

  openEditModal(pod) {
    const labelEntries = Object.entries(pod.labels ?? {}).map(([key, value]) => ({key, value}));
    const values = {namespace: pod.namespace, name: pod.name, image: pod.image, containerName: "", labelEntries};
    this.setState({modalVisible: true, modalMode: "edit", editingPod: pod, modalInitialValues: values}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue(values);
      }, 0);
    });
  }

  closeModal() {
    this.setState({modalVisible: false, editingPod: null});
  }

  handleSubmit() {
    this.formRef.current?.validateFields().then(values => {
      const labels = {};
      (values.labelEntries ?? []).forEach(({key, value}) => {
        if (key) {
          labels[key] = value ?? "";
        }
      });

      const ports = (values.portEntries ?? [])
        .map(p => Number(p?.port))
        .filter(p => Number.isInteger(p) && p > 0 && p <= 65535);

      this.setState({submitting: true});

      if (this.state.modalMode === "add") {
        PodBackend.addPod({
          namespace: values.namespace,
          name: values.name,
          image: values.image,
          containerName: values.containerName || "app",
          labels,
          ports,
          injectTtydSidecar: !!values.injectTtydSidecar,
        }).then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", "Pod created");
            this.closeModal();
            this.fetchPods();
          } else {
            Setting.showMessage("error", res.msg);
          }
        }).catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      } else {
        const pod = this.state.editingPod;
        PodBackend.updatePod({
          namespace: pod.namespace,
          name: pod.name,
          labels,
          resourceVersion: pod.resourceVersion,
        }).then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", "Pod labels updated");
            this.closeModal();
            this.fetchPods();
          } else {
            Setting.showMessage("error", res.msg);
          }
        }).catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      }
    });
  }

  handleDelete(pod) {
    PodBackend.deletePod(pod.namespace, pod.name).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", "Pod deleted");
        this.fetchPods();
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  handleOpenUI(pod) {
    const key = `${pod.namespace}/${pod.name}`;
    this.setState({openingPodKey: key});
    PodBackend.openPodUI(pod.namespace, pod.name).then(res => {
      if (res.status === "ok") {
        const path = res.data?.url ?? `/p/${pod.namespace}/${pod.name}/`;
        const full = path.startsWith("http") ? path : (window.location.origin + path);
        window.open(full, "_blank", "noopener,noreferrer");
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message))
      .finally(() => this.setState({openingPodKey: null}));
  }

  render() {
    const {pods, namespaces, loading, error, modalVisible, modalMode, submitting,
      eventsDrawerVisible, eventsPod, events, eventsLoading, eventsError,
      logsDrawerVisible, logsPod, logs, logsLoading, logsError,
      marketplaceVisible, modalInitialValues,
      openingPodKey} = this.state;

    const nsOptions = namespaces.map(ns => ({label: ns.name, value: ns.name}));

    const columns = [
      {title: "Namespace", dataIndex: "namespace", key: "namespace", width: 140},
      {title: "Name", dataIndex: "name", key: "name"},
      {title: "Image", dataIndex: "image", key: "image"},
      {
        title: "Node",
        dataIndex: "nodeName",
        key: "nodeName",
        width: 160,
        render: v => v || <span style={{color: "#999"}}>—</span>,
      },
      {
        title: "Phase",
        dataIndex: "phase",
        key: "phase",
        width: 110,
        render: phase => (
          <Tag color={phaseColor[phase] ?? "default"}>{phase || "Unknown"}</Tag>
        ),
      },
      {title: "Created", dataIndex: "createdAt", key: "createdAt", width: 180},
      {
        title: "Actions",
        key: "actions",
        width: 140,
        render: (_, record) => (
          <Space>
            <Button
              size="small"
              icon={<FileTextOutlined />}
              onClick={() => this.openLogsDrawer(record)}
            >
              Logs
            </Button>
            <Button
              size="small"
              icon={<UnorderedListOutlined />}
              onClick={() => this.openEventsDrawer(record)}
            >
              Events
            </Button>
            {record.phase === "Running" && (() => {
              const cfg = uiModeButton[record.uiMode] ?? uiModeButton.web;
              const podKey = `${record.namespace}/${record.name}`;
              const disabled = record.uiMode === "unknown";
              const tip = disabled
                ? "Pod has no usable port and no sidecar — delete and recreate with ports or ttyd sidecar"
                : `Open ${record.uiMode} session`;
              return (
                <Tooltip title={tip}>
                  <Button
                    size="small"
                    icon={cfg.icon}
                    loading={openingPodKey === podKey}
                    disabled={disabled}
                    onClick={() => this.handleOpenUI(record)}
                  >
                    {cfg.label}
                  </Button>
                </Tooltip>
              );
            })()}
            <Button
              size="small"
              icon={<EditOutlined />}
              onClick={() => this.openEditModal(record)}
            >
              Edit
            </Button>
            <Popconfirm
              title={`Delete Pod "${record.name}"?`}
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
            message="Failed to fetch pods"
            description={error}
            style={{marginBottom: 16}}
            showIcon
          />
        )}

        <Table
          rowKey={r => `${r.namespace}/${r.name}`}
          columns={columns}
          dataSource={pods}
          loading={loading}
          size="middle"
          pagination={{pageSize: 20}}
          locale={{emptyText: "No pods found"}}
          title={() => (
            <div>
              <span style={{fontWeight: 600}}>Pods</span>
              &nbsp;&nbsp;&nbsp;&nbsp;
              <Button icon={<ReloadOutlined />} onClick={() => this.fetchPods()} loading={loading} size="small">
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
          title={modalMode === "add" ? "Add Pod" : "Edit Pod Labels"}
          open={modalVisible}
          onOk={() => this.handleSubmit()}
          onCancel={() => this.closeModal()}
          confirmLoading={submitting}
          okText={modalMode === "add" ? "Create" : "Update"}
          width={580}
          destroyOnHidden
        >
          <Form ref={this.formRef} layout="vertical" initialValues={modalInitialValues}>
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
            <Form.Item label="Image" required={modalMode === "add"} style={{marginBottom: 0}}>
              <Space.Compact style={{width: "100%"}}>
                <Form.Item
                  name="image"
                  noStyle
                  rules={modalMode === "add" ? [{required: true, message: "Image is required"}] : []}
                >
                  <Input
                    disabled={modalMode === "edit"}
                    placeholder="nginx:latest or browse →"
                    style={{flex: 1}}
                    onChange={e => {
                      const v = e.target.value.trim();
                      if (v) {this.deriveNameFromImage(v);}
                    }}
                  />
                </Form.Item>
                {modalMode === "add" && (
                  <Tooltip title="Browse Docker Hub">
                    <Button
                      icon={<AppstoreOutlined />}
                      onClick={() => this.setState({marketplaceVisible: true})}
                    >
                      Browse
                    </Button>
                  </Tooltip>
                )}
              </Space.Compact>
            </Form.Item>
            <div style={{marginBottom: 16}} />
            <Form.Item
              label="Name"
              name="name"
              rules={[{required: true, message: "Name is required"}]}
            >
              <Input disabled={modalMode === "edit"} placeholder="auto-filled from image" />
            </Form.Item>
            {modalMode === "add" && (
              <Form.Item label="Container Name" name="containerName">
                <Input placeholder="auto-filled from image" />
              </Form.Item>
            )}
            {modalMode === "edit" && (
              <div style={{marginBottom: 8, color: "#888", fontSize: 12}}>
                Note: pod spec (image, containers) is immutable after creation. Only labels can be updated.
              </div>
            )}

            <Form.List name="labelEntries">
              {(fields, {add, remove}) => (
                <>
                  <div style={{marginBottom: 8, fontWeight: 500}}>Labels</div>
                  {fields.map(({key, name, ...rest}) => (
                    <Space key={key} align="baseline" style={{display: "flex", marginBottom: 4}}>
                      <Form.Item
                        {...rest}
                        name={[name, "key"]}
                        rules={[{required: true, message: "Key required"}]}
                        style={{marginBottom: 0}}
                      >
                        <Input placeholder="key" style={{width: 180}} />
                      </Form.Item>
                      <Form.Item
                        {...rest}
                        name={[name, "value"]}
                        style={{marginBottom: 0}}
                      >
                        <Input placeholder="value" style={{width: 200}} />
                      </Form.Item>
                      <MinusCircleOutlined onClick={() => remove(name)} style={{color: "#ff4d4f", cursor: "pointer"}} />
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

            {modalMode === "add" && (
              <>
                <div style={{marginTop: 16, marginBottom: 8, fontWeight: 500}}>
                  Container Ports
                  <span style={{color: "#999", fontWeight: 400, marginLeft: 8, fontSize: 12}}>
                    leave empty for CLI containers (ttyd sidecar handles the entry)
                  </span>
                </div>
                <Form.List name="portEntries">
                  {(fields, {add, remove}) => (
                    <>
                      {fields.map(({key, name, ...rest}) => (
                        <Space key={key} align="baseline" style={{display: "flex", marginBottom: 4}}>
                          <Form.Item
                            {...rest}
                            name={[name, "port"]}
                            rules={[{
                              validator: (_, v) => {
                                const n = Number(v);
                                if (!Number.isInteger(n) || n < 1 || n > 65535) {
                                  return Promise.reject(new Error("port 1-65535"));
                                }
                                return Promise.resolve();
                              },
                            }]}
                            style={{marginBottom: 0}}
                          >
                            <Input placeholder="80" style={{width: 120}} />
                          </Form.Item>
                          <MinusCircleOutlined onClick={() => remove(name)} style={{color: "#ff4d4f", cursor: "pointer"}} />
                        </Space>
                      ))}
                      <Button
                        type="dashed"
                        onClick={() => add()}
                        icon={<PlusOutlined />}
                        style={{marginTop: 4}}
                        size="small"
                      >
                        Add Port
                      </Button>
                    </>
                  )}
                </Form.List>

                <Form.Item
                  name="injectTtydSidecar"
                  valuePropName="checked"
                  style={{marginTop: 16, marginBottom: 0}}
                  tooltip="Inject a ttyd sidecar container (casos/ttyd:latest) so the Open button can launch a browser-based terminal. Disable for Web/VNC pods that already expose a usable port."
                >
                  <Switch checkedChildren="Inject ttyd sidecar" unCheckedChildren="No ttyd sidecar" />
                </Form.Item>
              </>
            )}
          </Form>
        </Modal>

        <DockerHubModal
          open={marketplaceVisible}
          onCancel={() => this.setState({marketplaceVisible: false})}
          onSelect={image => {
            this.formRef.current?.setFieldValue("image", image);
            this.deriveNameFromImage(image);
            this.setState({marketplaceVisible: false});
          }}
        />

        <Drawer
          title={logsPod ? `Logs — ${logsPod.namespace}/${logsPod.name}` : "Logs"}
          open={logsDrawerVisible}
          onClose={() => this.closeLogsDrawer()}
          width={800}
          extra={
            <Tag color={logsLoading ? "processing" : "success"}>
              {logsLoading ? "refreshing…" : "live · 3s"}
            </Tag>
          }
        >
          {logsError && (
            <Alert type="error" message={logsError} style={{marginBottom: 12}} showIcon />
          )}
          <div style={{
            background: "#0d1117",
            borderRadius: 6,
            padding: "12px 16px",
            fontFamily: "'Cascadia Code', 'Fira Mono', 'Consolas', monospace",
            fontSize: 13,
            lineHeight: 1.7,
            minHeight: 200,
            maxHeight: "calc(100vh - 160px)",
            overflowY: "auto",
            color: "#c9d1d9",
            whiteSpace: "pre-wrap",
            wordBreak: "break-all",
          }}>
            {!logs && !logsLoading && (
              <span style={{color: "#6e7681"}}>No logs yet…</span>
            )}
            {logs}
            <div ref={el => {this._logsEndRef = el;}} />
          </div>
        </Drawer>

        <Drawer
          title={eventsPod ? `Events — ${eventsPod.namespace}/${eventsPod.name}` : "Events"}
          open={eventsDrawerVisible}
          onClose={() => this.closeEventsDrawer()}
          width={720}
          extra={
            <Tag color={eventsLoading ? "processing" : "success"}>
              {eventsLoading ? "refreshing…" : "live · 3s"}
            </Tag>
          }
        >
          {eventsError && (
            <Alert type="error" message={eventsError} style={{marginBottom: 12}} showIcon />
          )}
          <div style={{
            background: "#0d1117",
            borderRadius: 6,
            padding: "12px 16px",
            fontFamily: "'Cascadia Code', 'Fira Mono', 'Consolas', monospace",
            fontSize: 13,
            lineHeight: 1.7,
            minHeight: 200,
            maxHeight: "calc(100vh - 160px)",
            overflowY: "auto",
            color: "#c9d1d9",
          }}>
            {events.length === 0 && !eventsLoading && (
              <span style={{color: "#6e7681"}}>No events yet…</span>
            )}
            {events.map((e, i) => {
              const typeColor = e.type === "Warning" ? "#f85149" : "#3fb950";
              const reasonColor = "#79c0ff";
              return (
                <div key={i} style={{marginBottom: 6}}>
                  <span style={{color: "#6e7681"}}>{e.lastTimestamp}</span>
                  {" "}
                  <span style={{color: typeColor, fontWeight: 600}}>{e.type}</span>
                  {" "}
                  <span style={{color: reasonColor}}>{e.reason}</span>
                  {e.count > 1 && <span style={{color: "#6e7681"}}> (×{e.count})</span>}
                  {" — "}
                  <span>{e.message}</span>
                </div>
              );
            })}
          </div>
        </Drawer>
      </div>
    );
  }
}

export default PodListPage;
