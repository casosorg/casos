import React from "react";
import {Link} from "react-router-dom";
import {
  Alert, Button, Divider, Form, Input, InputNumber, Modal, Popconfirm, Select, Space, Table, Tag, Tooltip, Typography
} from "antd";
import {DeleteOutlined, EditOutlined, HddOutlined, LinkOutlined, PlusOutlined, ReloadOutlined, ShareAltOutlined, SyncOutlined} from "@ant-design/icons";
import * as DeploymentBackend from "./backend/DeploymentBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import * as ConfigMapBackend from "./backend/ConfigMapBackend";
import * as SecretBackend from "./backend/SecretBackend";
import * as ServiceBackend from "./backend/ServiceBackend";
import * as NodeBackend from "./backend/NodeBackend";
import * as IngressBackend from "./backend/IngressBackend";
import * as Setting from "./Setting";
import DeploymentExposeModal from "./DeploymentExposeModal";
import DeploymentUpdateImageModal from "./DeploymentUpdateImageModal";
import DeploymentDomainModal from "./DeploymentDomainModal";
import DeploymentStorageEditor from "./DeploymentStorageEditor";
import EnvVarEditor, {ENV_SOURCE_CONFIGMAP, ENV_SOURCE_PLAIN, ENV_SOURCE_SECRET} from "./EnvVarEditor";
import ReplicasControl from "./ReplicasControl";
import {resourcePath} from "./resourceRoutes";

const {Text} = Typography;

function deploymentEnvVarsToEditorRows(envVars = []) {
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

class DeploymentListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      deployments: [],
      namespaces: [],
      configMaps: [],
      secrets: [],
      services: [],
      ingresses: [],
      nodeIP: null,
      loading: true,
      error: null,
      modalVisible: false,
      modalMode: "add",
      submitting: false,
      editingDeploy: null,
      exposeDeploy: null,
      updateImageDeploy: null,
      domainDeploy: null,
      envVars: [],
      volumes: [],
    };
    this.formRef = React.createRef();
  }

  componentDidMount() {
    this.fetchDeployments();
    this.fetchNamespaces();
    this.fetchServices();
    this.fetchNodeIP();
    this.fetchIngresses();
  }

  fetchNamespaces() {
    NamespaceBackend.getNamespaces().then(res => {
      if (res.status === "ok") {this.setState({namespaces: res.data ?? []});}
    }).catch(() => {});
  }

  fetchServices() {
    ServiceBackend.getServices().then(res => {
      if (res.status === "ok") {this.setState({services: res.data ?? []});}
    }).catch(() => {});
  }

  fetchIngresses() {
    IngressBackend.getIngresses().then(res => {
      if (res.status === "ok") {this.setState({ingresses: res.data ?? []});}
    }).catch(() => {});
  }

  fetchNodeIP() {
    NodeBackend.getNodes().then(res => {
      if (res.status === "ok") {
        const nodes = res.data ?? [];
        for (const node of nodes) {
          if (node.externalIP) {this.setState({nodeIP: node.externalIP}); return;}
        }
        for (const node of nodes) {
          if (node.internalIP) {this.setState({nodeIP: node.internalIP}); return;}
        }
      }
    }).catch(() => {});
  }

  fetchDeployments() {
    this.setState({loading: true, error: null});
    DeploymentBackend.getDeployments().then(res => {
      if (res.status === "ok") {
        this.setState({deployments: res.data ?? []});
      } else {
        Setting.showMessage("error", res.msg);
        this.setState({error: res.msg});
      }
    }).catch(e => {
      Setting.showMessage("error", e.message);
      this.setState({error: e.message});
    }).finally(() => {
      this.setState({loading: false});
      this.fetchServices();
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

  getAccessUrls(deploy) {
    const {services, ingresses, nodeIP} = this.state;
    const urls = [];

    if (nodeIP) {
      const svc = services.find(s => s.name === deploy.name && s.namespace === deploy.namespace && s.type === "NodePort");
      if (svc) {
        (svc.ports ?? []).filter(p => p.nodePort).forEach(p => {
          urls.push({url: `http://${nodeIP}:${p.nodePort}`, type: "nodeport"});
        });
      }
    }

    const deployServiceNames = new Set(
      (services ?? [])
        .filter(s => s.namespace === deploy.namespace && s.selector?.app === deploy.name)
        .map(s => s.name)
    );
    deployServiceNames.add(deploy.name);

    (ingresses ?? [])
      .filter(ing => ing.namespace === deploy.namespace)
      .forEach(ing => {
        (ing.rules ?? []).forEach(rule => {
          if (deployServiceNames.has(rule.serviceName) && rule.host) {
            const path = rule.path && rule.path !== "/" ? rule.path : "";
            urls.push({url: `http://${rule.host}${path}`, type: "domain"});
          }
        });
      });

    return urls;
  }

  openAddModal() {
    const defaultNs = this.state.namespaces.length > 0 ? this.state.namespaces[0].name : "default";
    this.setState({modalVisible: true, modalMode: "add", editingDeploy: null, envVars: [], volumes: []}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({namespace: defaultNs, name: "", replicas: 1, image: "", containerName: ""});
        this.fetchConfigMapsAndSecrets(defaultNs);
      }, 0);
    });
  }

  openEditModal(deploy) {
    this.setState(
      {modalVisible: true, modalMode: "edit", editingDeploy: deploy,
        envVars: deploymentEnvVarsToEditorRows(deploy.envVars), volumes: deploy.volumes ?? []},
      () => {
        setTimeout(() => {
          this.formRef.current?.setFieldsValue({
            namespace: deploy.namespace, name: deploy.name, replicas: deploy.replicas, image: deploy.image,
          });
          this.fetchConfigMapsAndSecrets(deploy.namespace);
        }, 0);
      }
    );
  }

  closeModal() {
    this.setState({modalVisible: false, editingDeploy: null, envVars: [], volumes: []});
  }

  handleSubmit() {
    this.formRef.current?.validateFields().then(values => {
      const payload = {
        namespace: values.namespace,
        name: values.name,
        replicas: values.replicas ?? 1,
        image: values.image,
        containerName: values.containerName ?? "",
        envVars: editorRowsToPayload(this.state.envVars),
        volumes: this.state.modalMode === "add"
          ? (this.state.volumes ?? []).filter(v => v.mountPath)
          : [],
      };

      this.setState({submitting: true});

      if (this.state.modalMode === "add") {
        DeploymentBackend.addDeployment(payload)
          .then(res => {
            if (res.status === "ok") {
              Setting.showMessage("success", "Deployment created");
              this.closeModal();
              this.fetchDeployments();
            } else {
              Setting.showMessage("error", res.msg);
            }
          })
          .catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      } else {
        DeploymentBackend.updateDeployment({...payload, resourceVersion: this.state.editingDeploy.resourceVersion})
          .then(res => {
            if (res.status === "ok") {
              Setting.showMessage("success", "Deployment updated");
              this.closeModal();
              this.fetchDeployments();
            } else {
              Setting.showMessage("error", res.msg);
            }
          })
          .catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      }
    });
  }

  handleDelete(deploy) {
    DeploymentBackend.deleteDeployment(deploy.namespace, deploy.name).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", "Deployment deleted");
        this.fetchDeployments();
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  render() {
    const {deployments, namespaces, configMaps, secrets, services, loading, error,
      modalVisible, modalMode, submitting, exposeDeploy, updateImageDeploy, domainDeploy,
      envVars, volumes} = this.state;

    const nsOptions = namespaces.map(ns => ({label: ns.name, value: ns.name}));

    const columns = [
      {title: "Namespace", dataIndex: "namespace", key: "namespace", width: 160},
      {title: "Name", dataIndex: "name", key: "name", render: (name, record) => <Link to={resourcePath("deployment", record.namespace, name)}>{name}</Link>},
      {
        title: "Image",
        key: "image",
        render: (_, r) => {
          if (!r.image) {return null;}
          const colonIdx = r.image.lastIndexOf(":");
          const repo = colonIdx > 0 ? r.image.slice(0, colonIdx) : r.image;
          const tag = colonIdx > 0 ? r.image.slice(colonIdx + 1) : "latest";
          return (
            <Tooltip title={r.image}>
              <span style={{fontSize: 13, color: "#595959"}}>{repo} </span>
              <Tag color="blue" style={{marginLeft: 2}}>{tag}</Tag>
            </Tooltip>
          );
        },
      },
      {
        title: "Storage",
        key: "storage",
        width: 200,
        render: (_, r) => {
          const vols = r.volumes ?? [];
          if (vols.length === 0) {return <span style={{color: "#bfbfbf", fontSize: 12}}>—</span>;}
          return (
            <Space size={4} wrap>
              {vols.map((v, i) => (
                <Tooltip key={i} title={`PVC: ${v.claimName}`}>
                  <Tag icon={<HddOutlined />} color="geekblue" style={{fontSize: 11}}>{v.mountPath}</Tag>
                </Tooltip>
              ))}
            </Space>
          );
        },
      },
      {
        title: "Replicas",
        key: "replicas",
        width: 200,
        render: (_, r) => (
          <ReplicasControl
            readyReplicas={r.readyReplicas ?? 0}
            replicas={r.replicas ?? 0}
            onScale={n => DeploymentBackend.updateDeployment({...r, replicas: n}).then(res => {
              if (res.status === "ok") {
                this.setState(prev => ({
                  deployments: prev.deployments.map(d =>
                    d.namespace === r.namespace && d.name === r.name ? res.data : d
                  ),
                }));
              } else {
                Setting.showMessage("error", res.msg);
              }
            }).catch(e => Setting.showMessage("error", e.message))}
          />
        ),
      },
      {
        title: "Access URL",
        key: "accessUrl",
        render: (_, record) => {
          const urls = this.getAccessUrls(record);
          if (urls.length === 0) {return null;}
          return urls.map(({url, type}, i) => (
            <div key={i} style={{display: "flex", alignItems: "center", gap: 4, marginBottom: 2}}>
              {type === "domain" && <LinkOutlined style={{color: "#1677ff", fontSize: 11}} />}
              <a href={url} target="_blank" rel="noopener noreferrer" style={{fontSize: 13}}>{url}</a>
            </div>
          ));
        },
      },
      {title: "Created", dataIndex: "createdAt", key: "createdAt", width: 180},
      {
        title: "Actions",
        key: "actions",
        width: 310,
        render: (_, record) => (
          <Space size={4} wrap>
            <Button size="small" icon={<EditOutlined />} onClick={() => this.openEditModal(record)}>Edit</Button>
            <Button size="small" icon={<SyncOutlined />} onClick={() => this.setState({updateImageDeploy: record})}>Update Image</Button>
            <Button size="small" icon={<ShareAltOutlined />} onClick={() => this.setState({exposeDeploy: record})}>Expose</Button>
            <Button size="small" icon={<LinkOutlined />} onClick={() => this.setState({domainDeploy: record})}>Bind Domain</Button>
            <Popconfirm
              title={`Delete Deployment "${record.name}"?`}
              okText="Delete" okType="danger" cancelText="Cancel"
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
          <Alert type="error" message="Failed to fetch Deployments" description={error} style={{marginBottom: 16}} showIcon />
        )}

        <Table
          rowKey={r => `${r.namespace}/${r.name}`}
          columns={columns}
          dataSource={deployments}
          loading={loading}
          size="middle"
          pagination={{pageSize: 20}}
          locale={{emptyText: "No Deployments found"}}
          title={() => (
            <div>
              <span style={{fontWeight: 600}}>Deployments</span>
              &nbsp;&nbsp;&nbsp;&nbsp;
              <Button icon={<ReloadOutlined />} onClick={() => {this.fetchDeployments(); this.fetchIngresses();}} loading={loading} size="small">Refresh</Button>
              &nbsp;&nbsp;
              <Button type="primary" icon={<PlusOutlined />} size="small" onClick={() => this.openAddModal()}>Add</Button>
            </div>
          )}
        />

        <DeploymentUpdateImageModal
          deploy={updateImageDeploy}
          open={updateImageDeploy !== null}
          onClose={() => this.setState({updateImageDeploy: null})}
          onUpdated={() => this.fetchDeployments()}
        />

        <DeploymentExposeModal
          deploy={exposeDeploy}
          open={exposeDeploy !== null}
          onClose={() => this.setState({exposeDeploy: null})}
        />

        <DeploymentDomainModal
          deploy={domainDeploy}
          services={services}
          open={domainDeploy !== null}
          onClose={() => this.setState({domainDeploy: null})}
          onCreated={() => {this.fetchIngresses(); this.fetchServices();}}
        />

        <Modal
          title={modalMode === "add" ? "Add Deployment" : "Edit Deployment"}
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
              <Input disabled={modalMode === "edit"} placeholder="my-deployment" />
            </Form.Item>
            <Form.Item label="Image" name="image" rules={[{required: true, message: "Image is required"}]}>
              <Input placeholder="nginx:latest" />
            </Form.Item>
            <Form.Item label="Replicas" name="replicas" rules={[{required: true}]}>
              <InputNumber min={0} style={{width: "100%"}} />
            </Form.Item>
            {modalMode === "add" && (
              <Form.Item label="Container Name" name="containerName">
                <Input placeholder="Leave empty to use deployment name" />
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

            <Divider orientation="left" orientationMargin={0} style={{marginTop: 8, marginBottom: 12}}>
              <Text style={{fontSize: 13}}>Persistent Storage</Text>
            </Divider>

            <DeploymentStorageEditor
              mode={modalMode}
              value={volumes}
              onChange={v => this.setState({volumes: v})}
            />
          </Form>
        </Modal>
      </div>
    );
  }
}

export default DeploymentListPage;
