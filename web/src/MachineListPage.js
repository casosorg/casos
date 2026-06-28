import React from "react";
import {Link} from "react-router-dom";
import {Button, Form, Input, InputNumber, Modal, Popconfirm, Select, Table, Tag, Tooltip} from "antd";
import {CloudSyncOutlined, DeleteOutlined, EditOutlined, PlusOutlined} from "@ant-design/icons";
import * as Setting from "./Setting";
import * as MachineBackend from "./backend/MachineBackend";
import MachineNodeDeployPanel from "./MachineNodeDeployPanel";
import i18next from "i18next";

const statusColor = {
  Online: "green",
  Offline: "red",
  Deploying: "processing",
  Deployed: "blue",
  Failed: "red",
  Unknown: "default",
};

class MachineListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      machines: [],
      loading: false,
      modalVisible: false,
      submitting: false,
      deployPanelVisible: false,
      deployingMachine: null,
    };
    this.formRef = React.createRef();
  }

  openDeployPanel(record) {
    this.setState({deployPanelVisible: true, deployingMachine: record});
  }

  closeDeployPanel() {
    this.setState({deployPanelVisible: false, deployingMachine: null}, () => this.getMachines());
  }

  UNSAFE_componentWillMount() {
    this.getMachines();
  }

  getMachines() {
    this.setState({loading: true});
    MachineBackend.getGlobalMachines()
      .then((res) => {
        this.setState({loading: false});
        if (res.status === "ok") {
          this.setState({machines: res.data || []});
        } else {
          Setting.showMessage("error", res.msg);
        }
      });
  }

  deleteMachine(record) {
    MachineBackend.deleteMachine(record)
      .then((res) => {
        if (res.status === "ok") {
          Setting.showMessage("success", i18next.t("general:Successfully deleted"));
          this.setState({machines: this.state.machines.filter(m => m.name !== record.name)});
        } else {
          Setting.showMessage("error", `${i18next.t("general:Failed to delete")}: ${res.msg}`);
        }
      });
  }

  openAddModal() {
    this.setState({modalVisible: true}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({
          name: "",
          displayName: "",
          ip: "",
          port: 22,
          username: "root",
          authType: "password",
          password: "",
          privateKey: "",
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
      const machine = {
        owner: "admin",
        name: values.name,
        displayName: values.displayName || values.name,
        ip: values.ip,
        port: values.port,
        username: values.username,
        authType: values.authType,
        password: values.authType === "password" ? values.password : "",
        privateKey: values.authType === "privateKey" ? values.privateKey : "",
        status: "Unknown",
      };
      MachineBackend.addMachine(machine)
        .then(res => {
          if (res.status === "ok") {
            Setting.showMessage("success", i18next.t("general:Successfully added"));
            this.closeModal();
            this.getMachines();
          } else {
            Setting.showMessage("error", `${i18next.t("general:Failed to add")}: ${res.msg}`);
          }
        })
        .catch(e => Setting.showMessage("error", e.message))
        .finally(() => this.setState({submitting: false}));
    });
  }

  render() {
    const deployWorkerNodeLabel = i18next.t("machine:Deploy worker node", {defaultValue: "Deploy worker node"});
    const columns = [
      {
        title: i18next.t("general:Name"),
        dataIndex: "name",
        key: "name",
        width: "160px",
        render: (text, record) => <Link to={`/machines/${record.name}`}>{text}</Link>,
      },
      {
        title: i18next.t("general:Display name"),
        dataIndex: "displayName",
        key: "displayName",
        width: "160px",
      },
      {
        title: i18next.t("machine:IP address"),
        dataIndex: "ip",
        key: "ip",
        width: "140px",
      },
      {
        title: i18next.t("machine:SSH port"),
        dataIndex: "port",
        key: "port",
        width: "100px",
      },
      {
        title: i18next.t("machine:Username"),
        dataIndex: "username",
        key: "username",
        width: "120px",
      },
      {
        title: i18next.t("policy:Role"),
        dataIndex: "role",
        key: "role",
        width: "120px",
      },
      {
        title: i18next.t("general:Status"),
        dataIndex: "status",
        key: "status",
        width: "120px",
        render: (text) => <Tag color={statusColor[text] || "default"}>{text || "Unknown"}</Tag>,
      },
      {
        title: i18next.t("general:Action"),
        dataIndex: "action",
        key: "action",
        width: "170px",
        fixed: "right",
        render: (text, record) => (
          <div style={{display: "flex", alignItems: "center", gap: "2px"}}>
            <Tooltip title={deployWorkerNodeLabel}>
              <Button aria-label={deployWorkerNodeLabel} type="text" size="small" icon={<CloudSyncOutlined />} style={{width: "28px", height: "28px", padding: 0, borderRadius: "6px"}} onClick={() => this.openDeployPanel(record)} />
            </Tooltip>
            <Tooltip title={i18next.t("general:Edit")}>
              <Button type="text" size="small" icon={<EditOutlined />} style={{width: "28px", height: "28px", padding: 0, borderRadius: "6px"}} onClick={() => this.props.history.push(`/machines/${record.name}`)} />
            </Tooltip>
            <Popconfirm
              title={`${i18next.t("general:Sure to delete")}: ${record.name} ?`}
              onConfirm={() => this.deleteMachine(record)}
              okText={i18next.t("general:OK")}
              cancelText={i18next.t("general:Cancel")}
            >
              <Tooltip title={i18next.t("general:Delete")}>
                <Button type="text" size="small" danger icon={<DeleteOutlined />} style={{width: "28px", height: "28px", padding: 0, borderRadius: "6px"}} />
              </Tooltip>
            </Popconfirm>
          </div>
        ),
      },
    ];

    return (
      <div style={{padding: "16px"}}>
        <Table
          scroll={{x: "max-content"}}
          columns={columns}
          dataSource={this.state.machines}
          rowKey="name"
          size="middle"
          bordered
          loading={this.state.loading}
          title={() => (
            <div>
              {i18next.t("general:Machines")}&nbsp;&nbsp;&nbsp;&nbsp;
              <Button type="primary" size="small" icon={<PlusOutlined />} onClick={() => this.openAddModal()}>{i18next.t("general:Add")}</Button>
            </div>
          )}
        />

        <Modal
          title={i18next.t("machine:Add Machine")}
          open={this.state.modalVisible}
          onOk={() => this.handleSubmit()}
          onCancel={() => this.closeModal()}
          confirmLoading={this.state.submitting}
          okText={i18next.t("general:Add")}
          width={560}
          destroyOnHidden
        >
          <Form ref={this.formRef} layout="vertical">
            <Form.Item label={i18next.t("general:Name")} name="name" rules={[{required: true, message: i18next.t("policy:required")}, {pattern: /^[a-z0-9-]+$/, message: "lowercase letters, digits and dashes only"}]}>
              <Input placeholder="my-machine" />
            </Form.Item>
            <Form.Item label={i18next.t("general:Display name")} name="displayName">
              <Input placeholder="My Machine" />
            </Form.Item>
            <Form.Item label={i18next.t("machine:IP address")} name="ip" rules={[{required: true, message: i18next.t("policy:required")}]}>
              <Input placeholder="192.168.1.10" />
            </Form.Item>
            <Form.Item label={i18next.t("machine:SSH port")} name="port" rules={[{required: true, message: i18next.t("policy:required")}]}>
              <InputNumber style={{width: "100%"}} min={1} max={65535} />
            </Form.Item>
            <Form.Item label={i18next.t("machine:Username")} name="username" rules={[{required: true, message: i18next.t("policy:required")}]}>
              <Input placeholder="root" />
            </Form.Item>
            <Form.Item label={i18next.t("machine:Auth type")} name="authType">
              <Select options={[
                {label: i18next.t("machine:Password"), value: "password"},
                {label: i18next.t("machine:Private key"), value: "privateKey"},
              ]} />
            </Form.Item>
            <Form.Item noStyle shouldUpdate={(prev, cur) => prev.authType !== cur.authType}>
              {({getFieldValue}) => getFieldValue("authType") === "privateKey" ? (
                <Form.Item label={i18next.t("machine:Private key")} name="privateKey" rules={[{required: true, message: i18next.t("policy:required")}]}>
                  <Input.TextArea rows={4} placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" />
                </Form.Item>
              ) : (
                <Form.Item label={i18next.t("machine:Password")} name="password" rules={[{required: true, message: i18next.t("policy:required")}]}>
                  <Input.Password />
                </Form.Item>
              )}
            </Form.Item>
          </Form>
        </Modal>
        <MachineNodeDeployPanel
          open={this.state.deployPanelVisible}
          machine={this.state.deployingMachine}
          account={this.props.account}
          onClose={() => this.closeDeployPanel()}
        />
      </div>
    );
  }
}

export default MachineListPage;
