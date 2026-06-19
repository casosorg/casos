import React from "react";
import {
  Alert, Button, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag
} from "antd";
import {DeleteOutlined, EditOutlined, MinusCircleOutlined, PlusOutlined, ReloadOutlined} from "@ant-design/icons";
import * as ServiceAccountBackend from "./backend/ServiceAccountBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import * as Setting from "./Setting";

class ServiceAccountListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      serviceAccounts: [],
      namespaces: [],
      loading: true,
      error: null,
      modalVisible: false,
      modalMode: "add",
      submitting: false,
      editingSa: null,
    };
    this.formRef = React.createRef();
  }

  componentDidMount() {
    this.fetchServiceAccounts();
    this.fetchNamespaces();
  }

  fetchNamespaces() {
    NamespaceBackend.getNamespaces().then(res => {
      if (res.status === "ok") {
        this.setState({namespaces: res.data ?? []});
      }
    }).catch(() => {});
  }

  fetchServiceAccounts() {
    this.setState({loading: true, error: null});
    ServiceAccountBackend.getServiceAccounts().then(res => {
      if (res.status === "ok") {
        this.setState({serviceAccounts: res.data ?? []});
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
    this.setState({modalVisible: true, modalMode: "add", editingSa: null}, () => {
      const defaultNs = this.state.namespaces.length > 0 ? this.state.namespaces[0].name : "default";
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({namespace: defaultNs, name: "", imagePullSecrets: []});
      }, 0);
    });
  }

  openEditModal(sa) {
    const imagePullSecrets = (sa.imagePullSecrets ?? []).map(s => ({secret: s}));
    this.setState({modalVisible: true, modalMode: "edit", editingSa: sa}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({
          namespace: sa.namespace,
          name: sa.name,
          imagePullSecrets,
        });
      }, 0);
    });
  }

  closeModal() {
    this.setState({modalVisible: false, editingSa: null});
  }

  handleSubmit() {
    this.formRef.current?.validateFields().then(values => {
      const imagePullSecrets = (values.imagePullSecrets ?? [])
        .map(item => item.secret)
        .filter(Boolean);

      const payload = {
        namespace: values.namespace,
        name: values.name,
        imagePullSecrets,
      };

      this.setState({submitting: true});

      if (this.state.modalMode === "add") {
        ServiceAccountBackend.addServiceAccount(payload).then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", "ServiceAccount created");
            this.closeModal();
            this.fetchServiceAccounts();
          } else {
            Setting.showMessage("error", res.msg);
          }
        }).catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      } else {
        const sa = this.state.editingSa;
        ServiceAccountBackend.updateServiceAccount({
          ...payload,
          resourceVersion: sa.resourceVersion,
        }).then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", "ServiceAccount updated");
            this.closeModal();
            this.fetchServiceAccounts();
          } else {
            Setting.showMessage("error", res.msg);
          }
        }).catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      }
    });
  }

  handleDelete(sa) {
    ServiceAccountBackend.deleteServiceAccount(sa.namespace, sa.name).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", "ServiceAccount deleted");
        this.fetchServiceAccounts();
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  render() {
    const {serviceAccounts, namespaces, loading, error, modalVisible, modalMode, submitting} = this.state;

    const nsOptions = namespaces.map(ns => ({label: ns.name, value: ns.name}));

    const columns = [
      {title: "Namespace", dataIndex: "namespace", key: "namespace", width: 160},
      {title: "Name", dataIndex: "name", key: "name"},
      {
        title: "Secrets",
        dataIndex: "secrets",
        key: "secrets",
        width: 90,
        render: v => v ?? 0,
      },
      {
        title: "Image Pull Secrets",
        dataIndex: "imagePullSecrets",
        key: "imagePullSecrets",
        render: list => list && list.length > 0
          ? list.map(s => <Tag key={s}>{s}</Tag>)
          : <span style={{color: "#999"}}>—</span>,
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
              icon={<EditOutlined />}
              onClick={() => this.openEditModal(record)}
            >
              Edit
            </Button>
            <Popconfirm
              title={`Delete ServiceAccount "${record.name}"?`}
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
            message="Failed to fetch ServiceAccounts"
            description={error}
            style={{marginBottom: 16}}
            showIcon
          />
        )}

        <Table
          rowKey={r => `${r.namespace}/${r.name}`}
          columns={columns}
          dataSource={serviceAccounts}
          loading={loading}
          size="middle"
          pagination={{pageSize: 20}}
          locale={{emptyText: "No ServiceAccounts found"}}
          title={() => (
            <div>
              <span style={{fontWeight: 600}}>Service Accounts</span>
              &nbsp;&nbsp;&nbsp;&nbsp;
              <Button icon={<ReloadOutlined />} onClick={() => this.fetchServiceAccounts()} loading={loading} size="small">
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
          title={modalMode === "add" ? "Add Service Account" : "Edit Service Account"}
          open={modalVisible}
          onOk={() => this.handleSubmit()}
          onCancel={() => this.closeModal()}
          confirmLoading={submitting}
          okText={modalMode === "add" ? "Create" : "Update"}
          width={560}
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
              <Input disabled={modalMode === "edit"} placeholder="my-service-account" />
            </Form.Item>

            <Form.List name="imagePullSecrets">
              {(fields, {add, remove}) => (
                <>
                  <div style={{marginBottom: 8, fontWeight: 500}}>Image Pull Secrets</div>
                  {fields.map(({key, name, ...rest}) => (
                    <Space key={key} align="baseline" style={{display: "flex", marginBottom: 4}}>
                      <Form.Item
                        {...rest}
                        name={[name, "secret"]}
                        rules={[{required: true, message: "Secret name required"}]}
                        style={{marginBottom: 0}}
                      >
                        <Input placeholder="secret-name" style={{width: 360}} />
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
                    Add Secret
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

export default ServiceAccountListPage;
