import React from "react";
import {
  Alert, Badge, Button, Form, Input, Modal, Popconfirm, Select, Table
} from "antd";
import {DeleteOutlined, PlusOutlined, ReloadOutlined} from "@ant-design/icons";
import * as PvcBackend from "./backend/PvcBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import * as Setting from "./Setting";

const ACCESS_MODE_OPTIONS = [
  {label: "ReadWriteOnce (单节点读写)", value: "ReadWriteOnce"},
  {label: "ReadOnlyMany (多节点只读)", value: "ReadOnlyMany"},
  {label: "ReadWriteMany (多节点读写)", value: "ReadWriteMany"},
];

const STATUS_BADGE = {
  Bound: "success",
  Pending: "processing",
  Lost: "error",
};

class PvcListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      pvcs: [],
      namespaces: [],
      loading: true,
      error: null,
      modalVisible: false,
      submitting: false,
    };
    this.formRef = React.createRef();
  }

  componentDidMount() {
    this.fetchPvcs();
    this.fetchNamespaces();
  }

  fetchNamespaces() {
    NamespaceBackend.getNamespaces().then(res => {
      if (res.status === "ok") {
        this.setState({namespaces: res.data ?? []});
      }
    }).catch(() => {});
  }

  fetchPvcs() {
    this.setState({loading: true, error: null});
    PvcBackend.getPvcs().then(res => {
      if (res.status === "ok") {
        this.setState({pvcs: res.data ?? []});
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
    const defaultNs = this.state.namespaces.length > 0 ? this.state.namespaces[0].name : "default";
    this.setState({modalVisible: true}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({
          namespace: defaultNs,
          name: "",
          storageClassName: "",
          accessMode: "ReadWriteOnce",
          storage: "1Gi",
        });
      }, 0);
    });
  }

  closeModal() {
    this.setState({modalVisible: false});
  }

  handleSubmit() {
    this.formRef.current?.validateFields().then(values => {
      this.setState({submitting: true});
      PvcBackend.addPvc(values).then(res => {
        if (res.status === "ok") {
          Setting.showMessage("success", "PVC created");
          this.closeModal();
          this.fetchPvcs();
        } else {
          Setting.showMessage("error", res.msg);
        }
      }).catch(e => Setting.showMessage("error", e.message))
        .finally(() => this.setState({submitting: false}));
    });
  }

  handleDelete(pvc) {
    PvcBackend.deletePvc(pvc.namespace, pvc.name).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", "PVC deleted");
        this.fetchPvcs();
      } else {
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => Setting.showMessage("error", e.message));
  }

  render() {
    const {pvcs, namespaces, loading, error, modalVisible, submitting} = this.state;

    const nsOptions = namespaces.map(ns => ({label: ns.name, value: ns.name}));

    const columns = [
      {title: "Namespace", dataIndex: "namespace", key: "namespace", width: 160},
      {title: "Name", dataIndex: "name", key: "name"},
      {
        title: "Status",
        dataIndex: "status",
        key: "status",
        width: 110,
        render: v => <Badge status={STATUS_BADGE[v] ?? "default"} text={v} />,
      },
      {title: "Storage", dataIndex: "storage", key: "storage", width: 100},
      {title: "Access Mode", dataIndex: "accessMode", key: "accessMode", width: 160},
      {title: "Storage Class", dataIndex: "storageClassName", key: "storageClassName", width: 150},
      {title: "Volume", dataIndex: "volumeName", key: "volumeName", ellipsis: true},
      {title: "Created", dataIndex: "createdAt", key: "createdAt", width: 180},
      {
        title: "Actions",
        key: "actions",
        width: 100,
        render: (_, record) => (
          <Popconfirm
            title={`Delete PVC "${record.name}"?`}
            description="Deleting a PVC may cause data loss if still mounted."
            okText="Delete"
            okType="danger"
            cancelText="Cancel"
            onConfirm={() => this.handleDelete(record)}
          >
            <Button size="small" danger icon={<DeleteOutlined />}>Delete</Button>
          </Popconfirm>
        ),
      },
    ];

    return (
      <div style={{padding: "24px"}}>
        {error && (
          <Alert type="error" message="Failed to fetch PVCs" description={error} style={{marginBottom: 16}} showIcon />
        )}

        <Table
          rowKey={r => `${r.namespace}/${r.name}`}
          columns={columns}
          dataSource={pvcs}
          loading={loading}
          size="middle"
          pagination={{pageSize: 20}}
          locale={{emptyText: "No Persistent Volume Claims found"}}
          title={() => (
            <div>
              <span style={{fontWeight: 600}}>Persistent Volume Claims</span>
              &nbsp;&nbsp;&nbsp;&nbsp;
              <Button icon={<ReloadOutlined />} onClick={() => this.fetchPvcs()} loading={loading} size="small">
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
          title="Add Persistent Volume Claim"
          open={modalVisible}
          onOk={() => this.handleSubmit()}
          onCancel={() => this.closeModal()}
          confirmLoading={submitting}
          okText="Create"
          width={520}
          destroyOnHidden
        >
          <Form ref={this.formRef} layout="vertical">
            <Form.Item label="Namespace" name="namespace" rules={[{required: true, message: "Namespace is required"}]}>
              <Select options={nsOptions} placeholder="Select a namespace" showSearch />
            </Form.Item>
            <Form.Item label="Name" name="name" rules={[{required: true, message: "Name is required"}]}>
              <Input placeholder="my-pvc" />
            </Form.Item>
            <Form.Item label="Storage Size" name="storage" rules={[{required: true, message: "Storage size is required"}]}>
              <Input placeholder="1Gi" />
            </Form.Item>
            <Form.Item label="Access Mode" name="accessMode" rules={[{required: true}]}>
              <Select options={ACCESS_MODE_OPTIONS} />
            </Form.Item>
            <Form.Item label="Storage Class" name="storageClassName">
              <Input placeholder="Leave empty to use cluster default" />
            </Form.Item>
          </Form>
        </Modal>
      </div>
    );
  }
}

export default PvcListPage;
