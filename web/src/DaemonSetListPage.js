import React from "react";
import {Link} from "react-router-dom";
import {
  Alert, Button, Divider, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Typography
} from "antd";
import {DeleteOutlined, EditOutlined, PlusOutlined, ReloadOutlined} from "@ant-design/icons";
import * as DaemonSetBackend from "./backend/DaemonSetBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import * as ConfigMapBackend from "./backend/ConfigMapBackend";
import * as SecretBackend from "./backend/SecretBackend";
import * as Setting from "./Setting";
import EnvVarEditor, {ENV_SOURCE_CONFIGMAP, ENV_SOURCE_PLAIN, ENV_SOURCE_SECRET} from "./EnvVarEditor";
import {resourcePath} from "./resourceRoutes";

const {Text} = Typography;

function dsEnvVarsToEditorRows(envVars = []) {
  return envVars.map(e => {
    if (e.configMapName) {
      return {source: ENV_SOURCE_CONFIGMAP, name: e.name, configMapName: e.configMapName, configMapKey: e.configMapKey};
    }
    if (e.secretName) {
      return {source: ENV_SOURCE_SECRET, name: e.name, secretName: e.secretName, secretKey: e.secretKey};
    }
    return {source: ENV_SOURCE_PLAIN, name: e.name, value: e.value};
  });
}

function editorRowsToPayload(rows = []) {
  return rows
    .filter(e => e.name)
    .map(e => {
      if (e.source === ENV_SOURCE_CONFIGMAP) {
        return {name: e.name, configMapName: e.configMapName ?? "", configMapKey: e.configMapKey ?? ""};
      }
      if (e.source === ENV_SOURCE_SECRET) {
        return {name: e.name, secretName: e.secretName ?? "", secretKey: e.secretKey ?? ""};
      }
      return {name: e.name, value: e.value ?? ""};
    });
}

class DaemonSetListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      daemonsets: [],
      namespaces: [],
      configMaps: [],
      secrets: [],
      loading: true,
      error: null,
      modalVisible: false,
      modalMode: "add",
      submitting: false,
      editingDs: null,
      envVars: [],
    };
    this.formRef = React.createRef();
  }

  componentDidMount() {
    this.fetchDaemonSets();
    this.fetchNamespaces();
  }

  fetchNamespaces() {
    NamespaceBackend.getNamespaces().then(res => {
      if (res.status === "ok") {
        this.setState({namespaces: res.data ?? []});
      }
    }).catch(() => {});
  }

  fetchDaemonSets() {
    this.setState({loading: true, error: null});
    DaemonSetBackend.getDaemonSets().then(res => {
      if (res.status === "ok") {
        this.setState({daemonsets: res.data ?? []});
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

  fetchConfigMapsAndSecrets(namespace) {
    if (!namespace) {return;}
    ConfigMapBackend.getConfigMaps(namespace).then(res => {
      if (res.status === "ok") {this.setState({configMaps: res.data ?? []});}
    }).catch(() => {});
    SecretBackend.getSecrets(namespace).then(res => {
      if (res.status === "ok") {this.setState({secrets: res.data ?? []});}
    }).catch(() => {});
  }

  openAddModal() {
    const defaultNs = this.state.namespaces.length > 0 ? this.state.namespaces[0].name : "default";
    this.setState({modalVisible: true, modalMode: "add", editingDs: null, envVars: []}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({namespace: defaultNs, name: "", image: "", containerName: ""});
        this.fetchConfigMapsAndSecrets(defaultNs);
      }, 0);
    });
  }

  openEditModal(ds) {
    this.setState(
      {modalVisible: true, modalMode: "edit", editingDs: ds, envVars: dsEnvVarsToEditorRows(ds.envVars)},
      () => {
        setTimeout(() => {
          this.formRef.current?.setFieldsValue({
            namespace: ds.namespace,
            name: ds.name,
            image: ds.image,
          });
          this.fetchConfigMapsAndSecrets(ds.namespace);
        }, 0);
      }
    );
  }

  closeModal() {
    this.setState({modalVisible: false, editingDs: null, envVars: []});
  }

  handleSubmit() {
    this.formRef.current?.validateFields().then(values => {
      const payload = {
        namespace: values.namespace,
        name: values.name,
        image: values.image,
        containerName: values.containerName ?? "",
        envVars: editorRowsToPayload(this.state.envVars),
      };

      this.setState({submitting: true});

      if (this.state.modalMode === "add") {
        DaemonSetBackend.addDaemonSet(payload)
          .then(res => {
            if (res.status === "ok") {
              Setting.showMessage("success", "Daemon Set created");
              this.closeModal();
              this.fetchDaemonSets();
            } else {
              Setting.showMessage("error", res.msg);
            }
          })
          .catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      } else {
        DaemonSetBackend.updateDaemonSet({...payload, resourceVersion: this.state.editingDs.resourceVersion})
          .then(res => {
            if (res.status === "ok") {
              Setting.showMessage("success", "Daemon Set updated");
              this.closeModal();
              this.fetchDaemonSets();
            } else {
              Setting.showMessage("error", res.msg);
            }
          })
          .catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      }
    });
  }

  handleDelete(ds) {
    DaemonSetBackend.deleteDaemonSet(ds.namespace, ds.name).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", "Daemon Set deleted");
        this.fetchDaemonSets();
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  render() {
    const {daemonsets, namespaces, configMaps, secrets, loading, error, modalVisible, modalMode, submitting, envVars} = this.state;

    const nsOptions = namespaces.map(ns => ({label: ns.name, value: ns.name}));

    const columns = [
      {title: "Namespace", dataIndex: "namespace", key: "namespace", width: 160},
      {title: "Name", dataIndex: "name", key: "name", render: (name, record) => <Link to={resourcePath("daemonset", record.namespace, name)}>{name}</Link>},
      {title: "Image", dataIndex: "image", key: "image", ellipsis: true},
      {
        title: "Desired / Ready",
        key: "status",
        width: 140,
        render: (_, r) => (
          <Space size={4}>
            <Tag color="blue">{r.desiredNumberScheduled ?? 0}</Tag>
            <span style={{color: "#999"}}>/</span>
            <Tag color={r.numberReady >= r.desiredNumberScheduled && r.desiredNumberScheduled > 0 ? "green" : "orange"}>
              {r.numberReady ?? 0}
            </Tag>
          </Space>
        ),
      },
      {title: "Created", dataIndex: "createdAt", key: "createdAt", width: 180},
      {
        title: "Actions",
        key: "actions",
        width: 160,
        render: (_, record) => (
          <Space size={4}>
            <Button size="small" icon={<EditOutlined />} onClick={() => this.openEditModal(record)}>Edit</Button>
            <Popconfirm
              title={`Delete Daemon Set "${record.name}"?`}
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
          <Alert type="error" message="Failed to fetch Daemon Sets" description={error} style={{marginBottom: 16}} showIcon />
        )}

        <Table
          rowKey={r => `${r.namespace}/${r.name}`}
          columns={columns}
          dataSource={daemonsets}
          loading={loading}
          size="middle"
          pagination={{pageSize: 20}}
          locale={{emptyText: "No Daemon Sets found"}}
          title={() => (
            <div>
              <span style={{fontWeight: 600}}>Daemon Sets</span>
              &nbsp;&nbsp;&nbsp;&nbsp;
              <Button icon={<ReloadOutlined />} onClick={() => this.fetchDaemonSets()} loading={loading} size="small">Refresh</Button>
              &nbsp;&nbsp;
              <Button type="primary" icon={<PlusOutlined />} size="small" onClick={() => this.openAddModal()}>Add</Button>
            </div>
          )}
        />

        <Modal
          title={modalMode === "add" ? "Add Daemon Set" : "Edit Daemon Set"}
          open={modalVisible}
          onOk={() => this.handleSubmit()}
          onCancel={() => this.closeModal()}
          confirmLoading={submitting}
          okText={modalMode === "add" ? "Create" : "Update"}
          width={640}
          destroyOnHidden
        >
          <Form ref={this.formRef} layout="vertical">
            <Form.Item label="Namespace" name="namespace" rules={[{required: true, message: "Namespace is required"}]}>
              <Select
                disabled={modalMode === "edit"}
                options={nsOptions}
                placeholder="Select a namespace"
                showSearch
                onChange={ns => this.fetchConfigMapsAndSecrets(ns)}
              />
            </Form.Item>
            <Form.Item label="Name" name="name" rules={[{required: true, message: "Name is required"}]}>
              <Input disabled={modalMode === "edit"} placeholder="my-daemon-set" />
            </Form.Item>
            <Form.Item label="Image" name="image" rules={[{required: true, message: "Image is required"}]}>
              <Input placeholder="nginx:latest" />
            </Form.Item>
            {modalMode === "add" && (
              <Form.Item label="Container Name" name="containerName">
                <Input placeholder="Leave empty to use Daemon Set name" />
              </Form.Item>
            )}

            <Divider orientation="left" orientationMargin={0} style={{marginTop: 8, marginBottom: 12}}>
              <Text style={{fontSize: 13}}>Environment Variables</Text>
            </Divider>

            <EnvVarEditor
              value={envVars}
              onChange={rows => this.setState({envVars: rows})}
              configMaps={configMaps}
              secrets={secrets}
            />
          </Form>
        </Modal>
      </div>
    );
  }
}

export default DaemonSetListPage;
