import React from "react";
import {
  Alert, Button, Form, Input, Modal, Popconfirm, Select, Space, Switch, Table, Tag
} from "antd";
import {DeleteOutlined, EditOutlined, MinusCircleOutlined, PlusOutlined, ReloadOutlined} from "@ant-design/icons";
import * as StorageClassBackend from "./backend/StorageClassBackend";
import * as Setting from "./Setting";

const RECLAIM_POLICIES = ["Delete", "Retain"];
const VOLUME_BINDING_MODES = ["Immediate", "WaitForFirstConsumer"];

function parametersToFormRows(parameters) {
  return Object.entries(parameters ?? {}).map(([key, value]) => ({key, value}));
}

class StorageClassListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      storageclasses: [],
      loading: true,
      error: null,
      modalVisible: false,
      modalMode: "add",
      submitting: false,
      editingSc: null,
    };
    this.formRef = React.createRef();
  }

  componentDidMount() {
    this.fetchStorageClasses();
  }

  fetchStorageClasses() {
    this.setState({loading: true, error: null});
    StorageClassBackend.getStorageClasses().then(res => {
      if (res.status === "ok") {
        this.setState({storageclasses: res.data ?? []});
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
    this.setState({modalVisible: true, modalMode: "add", editingSc: null}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({
          name: "",
          provisioner: "",
          reclaimPolicy: "Delete",
          volumeBindingMode: "Immediate",
          allowVolumeExpansion: false,
          isDefault: false,
          parameters: [],
        });
      }, 0);
    });
  }

  openEditModal(sc) {
    this.setState({modalVisible: true, modalMode: "edit", editingSc: sc}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({
          name: sc.name,
          provisioner: sc.provisioner,
          reclaimPolicy: sc.reclaimPolicy || "Delete",
          volumeBindingMode: sc.volumeBindingMode || "Immediate",
          allowVolumeExpansion: sc.allowVolumeExpansion,
          isDefault: sc.isDefault,
          parameters: parametersToFormRows(sc.parameters),
        });
      }, 0);
    });
  }

  closeModal() {
    this.setState({modalVisible: false, editingSc: null});
  }

  handleSubmit() {
    this.formRef.current?.validateFields().then(values => {
      const parameters = {};
      (values.parameters ?? []).forEach(({key, value}) => {
        if (key) {parameters[key] = value ?? "";}
      });
      this.setState({submitting: true});

      if (this.state.modalMode === "add") {
        StorageClassBackend.addStorageClass({
          name: values.name,
          provisioner: values.provisioner,
          reclaimPolicy: values.reclaimPolicy,
          volumeBindingMode: values.volumeBindingMode,
          allowVolumeExpansion: !!values.allowVolumeExpansion,
          isDefault: !!values.isDefault,
          parameters,
        }).then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", "Storage Class created");
            this.closeModal();
            this.fetchStorageClasses();
          } else {
            Setting.showMessage("error", res.msg);
          }
        }).catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      } else {
        const sc = this.state.editingSc;
        StorageClassBackend.updateStorageClass({
          name: sc.name,
          provisioner: sc.provisioner,
          reclaimPolicy: values.reclaimPolicy,
          volumeBindingMode: sc.volumeBindingMode,
          allowVolumeExpansion: !!values.allowVolumeExpansion,
          isDefault: !!values.isDefault,
          parameters: sc.parameters ?? {},
          resourceVersion: sc.resourceVersion,
        }).then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", "Storage Class updated");
            this.closeModal();
            this.fetchStorageClasses();
          } else {
            Setting.showMessage("error", res.msg);
          }
        }).catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      }
    });
  }

  handleDelete(sc) {
    StorageClassBackend.deleteStorageClass(sc.name).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", "Storage Class deleted");
        this.fetchStorageClasses();
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  render() {
    const {storageclasses, loading, error, modalVisible, modalMode, submitting} = this.state;

    const columns = [
      {
        title: "Name",
        dataIndex: "name",
        key: "name",
        render: (v, record) => (
          <Space>
            {v}
            {record.isDefault && <Tag color="gold">Default</Tag>}
          </Space>
        ),
      },
      {title: "Provisioner", dataIndex: "provisioner", key: "provisioner"},
      {
        title: "Reclaim Policy",
        dataIndex: "reclaimPolicy",
        key: "reclaimPolicy",
        width: 130,
        render: v => <Tag color={v === "Retain" ? "orange" : "blue"}>{v}</Tag>,
      },
      {
        title: "Volume Binding Mode",
        dataIndex: "volumeBindingMode",
        key: "volumeBindingMode",
        width: 190,
      },
      {
        title: "Allow Volume Expansion",
        dataIndex: "allowVolumeExpansion",
        key: "allowVolumeExpansion",
        width: 170,
        render: v => <Tag color={v ? "green" : "default"}>{v ? "Yes" : "No"}</Tag>,
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
              title={`Delete Storage Class "${record.name}"?`}
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
          <Alert type="error" message="Failed to fetch Storage Classes" description={error} style={{marginBottom: 16}} showIcon />
        )}

        <Table
          rowKey="name"
          columns={columns}
          dataSource={storageclasses}
          loading={loading}
          size="middle"
          scroll={{x: 1100}}
          pagination={{pageSize: 20}}
          locale={{emptyText: "No Storage Classes found"}}
          title={() => (
            <div>
              <span style={{fontWeight: 600}}>Storage Classes</span>
              &nbsp;&nbsp;&nbsp;&nbsp;
              <Button icon={<ReloadOutlined />} onClick={() => this.fetchStorageClasses()} loading={loading} size="small">
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
          title={modalMode === "add" ? "Add Storage Class" : "Edit Storage Class"}
          open={modalVisible}
          onOk={() => this.handleSubmit()}
          onCancel={() => this.closeModal()}
          confirmLoading={submitting}
          okText={modalMode === "add" ? "Create" : "Update"}
          width={640}
          destroyOnHidden
        >
          <Form ref={this.formRef} layout="vertical">
            <Form.Item label="Name" name="name" rules={[{required: true, message: "Required"}]}>
              <Input disabled={modalMode === "edit"} placeholder="my-storage-class" />
            </Form.Item>

            <Form.Item
              label="Provisioner"
              name="provisioner"
              rules={[{required: true, message: "Required"}]}
            >
              <Input disabled={modalMode === "edit"} placeholder="casos.io/local-path-provisioner" />
            </Form.Item>

            <Space.Compact style={{width: "100%", marginBottom: 0}}>
              <Form.Item
                label="Reclaim Policy"
                name="reclaimPolicy"
                style={{width: "50%", marginBottom: 24}}
              >
                <Select options={RECLAIM_POLICIES.map(p => ({label: p, value: p}))} />
              </Form.Item>
              <Form.Item
                label="Volume Binding Mode"
                name="volumeBindingMode"
                style={{width: "50%", marginBottom: 24}}
              >
                <Select
                  disabled={modalMode === "edit"}
                  options={VOLUME_BINDING_MODES.map(m => ({label: m, value: m}))}
                />
              </Form.Item>
            </Space.Compact>

            {modalMode === "edit" && (
              <div style={{marginBottom: 8, color: "#888", fontSize: 12}}>
                Note: Provisioner, Volume Binding Mode and Parameters are immutable after creation.
              </div>
            )}

            <Space size={32} style={{marginBottom: 24}}>
              <Form.Item label="Allow Volume Expansion" name="allowVolumeExpansion" valuePropName="checked" style={{marginBottom: 0}}>
                <Switch />
              </Form.Item>
              <Form.Item label="Set As Default" name="isDefault" valuePropName="checked" style={{marginBottom: 0}}>
                <Switch />
              </Form.Item>
            </Space>

            <Form.List name="parameters">
              {(fields, {add, remove}) => (
                <>
                  <div style={{marginBottom: 8, fontWeight: 500}}>Parameters</div>
                  {fields.map(({key, name, ...rest}) => (
                    <Space key={key} align="baseline" style={{display: "flex", marginBottom: 8}}>
                      <Form.Item
                        {...rest}
                        name={[name, "key"]}
                        rules={[{required: true, message: "Key required"}]}
                        style={{marginBottom: 0}}
                      >
                        <Input disabled={modalMode === "edit"} placeholder="key" style={{width: 200}} />
                      </Form.Item>
                      <Form.Item
                        {...rest}
                        name={[name, "value"]}
                        style={{marginBottom: 0}}
                      >
                        <Input disabled={modalMode === "edit"} placeholder="value" style={{width: 280}} />
                      </Form.Item>
                      {modalMode === "add" && (
                        <MinusCircleOutlined onClick={() => remove(name)} style={{color: "#ff4d4f", cursor: "pointer"}} />
                      )}
                    </Space>
                  ))}
                  {modalMode === "add" && (
                    <Button
                      type="dashed"
                      onClick={() => add()}
                      icon={<PlusOutlined />}
                      size="small"
                      style={{marginTop: 4}}
                    >
                      Add Parameter
                    </Button>
                  )}
                </>
              )}
            </Form.List>
          </Form>
        </Modal>
      </div>
    );
  }
}

export default StorageClassListPage;
