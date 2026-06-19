import React from "react";
import {
  Alert, Button, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag
} from "antd";
import {DeleteOutlined, EditOutlined, MinusCircleOutlined, PlusOutlined, ReloadOutlined} from "@ant-design/icons";
import * as ClusterRoleBindingBackend from "./backend/ClusterRoleBindingBackend";
import * as Setting from "./Setting";

const SUBJECT_KINDS = ["ServiceAccount", "User", "Group"];

const kindColor = {
  ServiceAccount: "blue",
  User: "green",
  Group: "purple",
};

function subjectsToFormRows(subjects) {
  return (subjects ?? []).map(s => ({
    kind: s.kind,
    name: s.name,
    namespace: s.namespace || "",
  }));
}

class ClusterRoleBindingListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      crbs: [],
      loading: true,
      error: null,
      modalVisible: false,
      modalMode: "add",
      submitting: false,
      editingCrb: null,
    };
    this.formRef = React.createRef();
  }

  componentDidMount() {
    this.fetchCrbs();
  }

  fetchCrbs() {
    this.setState({loading: true, error: null});
    ClusterRoleBindingBackend.getClusterRoleBindings().then(res => {
      if (res.status === "ok") {
        this.setState({crbs: res.data ?? []});
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
    this.setState({modalVisible: true, modalMode: "add", editingCrb: null}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({
          name: "",
          roleRef: "",
          roleRefKind: "ClusterRole",
          subjects: [],
        });
      }, 0);
    });
  }

  openEditModal(crb) {
    this.setState({modalVisible: true, modalMode: "edit", editingCrb: crb}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({
          name: crb.name,
          roleRef: crb.roleRef,
          roleRefKind: "ClusterRole",
          subjects: subjectsToFormRows(crb.subjects),
        });
      }, 0);
    });
  }

  closeModal() {
    this.setState({modalVisible: false, editingCrb: null});
  }

  handleSubmit() {
    this.formRef.current?.validateFields().then(values => {
      const subjects = (values.subjects ?? []).filter(s => s && s.name);
      this.setState({submitting: true});

      if (this.state.modalMode === "add") {
        ClusterRoleBindingBackend.addClusterRoleBinding({
          name: values.name,
          roleRef: values.roleRef,
          roleRefKind: values.roleRefKind || "ClusterRole",
          subjects,
        }).then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", "ClusterRoleBinding created");
            this.closeModal();
            this.fetchCrbs();
          } else {
            Setting.showMessage("error", res.msg);
          }
        }).catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      } else {
        const crb = this.state.editingCrb;
        ClusterRoleBindingBackend.updateClusterRoleBinding({
          name: crb.name,
          roleRef: crb.roleRef,
          subjects,
          resourceVersion: crb.resourceVersion,
        }).then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", "ClusterRoleBinding updated");
            this.closeModal();
            this.fetchCrbs();
          } else {
            Setting.showMessage("error", res.msg);
          }
        }).catch(e => Setting.showMessage("error", e.message))
          .finally(() => this.setState({submitting: false}));
      }
    });
  }

  handleDelete(crb) {
    ClusterRoleBindingBackend.deleteClusterRoleBinding(crb.name).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", "ClusterRoleBinding deleted");
        this.fetchCrbs();
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  render() {
    const {crbs, loading, error, modalVisible, modalMode, submitting} = this.state;

    const columns = [
      {title: "Name", dataIndex: "name", key: "name"},
      {
        title: "Role Ref",
        dataIndex: "roleRef",
        key: "roleRef",
        width: 220,
        render: v => <Tag color="volcano">{v}</Tag>,
      },
      {
        title: "Subjects",
        dataIndex: "subjects",
        key: "subjects",
        render: subjects => (subjects ?? []).map((s, i) => (
          <Tag key={i} color={kindColor[s.kind] ?? "default"}>
            {s.kind}/{s.name}{s.namespace ? ` (${s.namespace})` : ""}
          </Tag>
        )),
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
              title={`Delete ClusterRoleBinding "${record.name}"?`}
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
          <Alert type="error" message="Failed to fetch ClusterRoleBindings" description={error} style={{marginBottom: 16}} showIcon />
        )}

        <Table
          rowKey="name"
          columns={columns}
          dataSource={crbs}
          loading={loading}
          size="middle"
          pagination={{pageSize: 20}}
          locale={{emptyText: "No ClusterRoleBindings found"}}
          title={() => (
            <div>
              <span style={{fontWeight: 600}}>ClusterRoleBindings</span>
              &nbsp;&nbsp;&nbsp;&nbsp;
              <Button icon={<ReloadOutlined />} onClick={() => this.fetchCrbs()} loading={loading} size="small">
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
          title={modalMode === "add" ? "Add Cluster Role Binding" : "Edit Cluster Role Binding"}
          open={modalVisible}
          onOk={() => this.handleSubmit()}
          onCancel={() => this.closeModal()}
          confirmLoading={submitting}
          okText={modalMode === "add" ? "Create" : "Update"}
          width={620}
          destroyOnHidden
        >
          <Form ref={this.formRef} layout="vertical">
            <Form.Item label="Name" name="name" rules={[{required: true, message: "Required"}]}>
              <Input disabled={modalMode === "edit"} placeholder="my-binding" />
            </Form.Item>

            <Space.Compact style={{width: "100%", marginBottom: 0}}>
              <Form.Item
                label="Role Ref Kind"
                name="roleRefKind"
                style={{width: "40%", marginBottom: 24}}
              >
                <Select
                  disabled={modalMode === "edit"}
                  options={[
                    {label: "ClusterRole", value: "ClusterRole"},
                    {label: "Role", value: "Role"},
                  ]}
                />
              </Form.Item>
              <Form.Item
                label="Role Ref Name"
                name="roleRef"
                style={{width: "60%", marginBottom: 24}}
                rules={[{required: true, message: "Required"}]}
              >
                <Input disabled={modalMode === "edit"} placeholder="cluster-admin" />
              </Form.Item>
            </Space.Compact>

            {modalMode === "edit" && (
              <div style={{marginBottom: 8, color: "#888", fontSize: 12}}>
                Note: roleRef is immutable after creation. Only subjects can be updated.
              </div>
            )}

            {/* Subjects */}
            <Form.List name="subjects">
              {(fields, {add, remove}) => (
                <>
                  <div style={{marginBottom: 8, fontWeight: 500}}>Subjects</div>
                  {fields.map(({key, name, ...rest}) => (
                    <Space key={key} align="baseline" style={{display: "flex", marginBottom: 4, flexWrap: "wrap"}}>
                      <Form.Item
                        {...rest}
                        name={[name, "kind"]}
                        rules={[{required: true, message: "Kind required"}]}
                        style={{marginBottom: 0}}
                      >
                        <Select
                          options={SUBJECT_KINDS.map(k => ({label: k, value: k}))}
                          placeholder="Kind"
                          style={{width: 140}}
                        />
                      </Form.Item>
                      <Form.Item
                        {...rest}
                        name={[name, "name"]}
                        rules={[{required: true, message: "Name required"}]}
                        style={{marginBottom: 0}}
                      >
                        <Input placeholder="name" style={{width: 160}} />
                      </Form.Item>
                      <Form.Item
                        {...rest}
                        name={[name, "namespace"]}
                        style={{marginBottom: 0}}
                      >
                        <Input placeholder="namespace (SA only)" style={{width: 160}} />
                      </Form.Item>
                      <MinusCircleOutlined onClick={() => remove(name)} style={{color: "#ff4d4f", cursor: "pointer"}} />
                    </Space>
                  ))}
                  <Button
                    type="dashed"
                    onClick={() => add({kind: "ServiceAccount", name: "", namespace: ""})}
                    icon={<PlusOutlined />}
                    size="small"
                    style={{marginTop: 4}}
                  >
                    Add Subject
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

export default ClusterRoleBindingListPage;
