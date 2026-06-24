import React from "react";
import {Alert, Button, Descriptions, Drawer, Form, Input, Space, Table, Tag, Typography} from "antd";
import {CloudSyncOutlined, FileSearchOutlined, ReloadOutlined, ToolOutlined} from "@ant-design/icons";
import * as MachineNodeDeployBackend from "./backend/MachineNodeDeployBackend";
import * as Setting from "./Setting";
import i18next from "i18next";

const {Text} = Typography;

const taskStatusColor = {
  pending: "default",
  running: "processing",
  succeeded: "green",
  failed: "red",
};

class MachineNodeDeployPanel extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      tasks: [],
      logs: [],
      selectedTaskId: null,
      preflight: null,
      preflightError: null,
      loadingTasks: false,
      loadingLogs: false,
      preflighting: false,
      deploying: false,
      repairing: false,
    };
    this.formRef = React.createRef();
    this.refreshTimer = null;
  }

  componentDidMount() {
    if (this.props.open && this.props.machine) {
      this.resetForm();
      this.loadTasks();
      this.startRefresh();
    }
  }

  componentDidUpdate(prevProps) {
    if (!prevProps.open && this.props.open && this.props.machine) {
      this.resetForm();
      this.loadTasks();
      this.startRefresh();
    } else if (this.props.open && this.props.machine?.name !== prevProps.machine?.name) {
      this.resetForm();
      this.loadTasks();
    }
    if (prevProps.open && !this.props.open) {
      this.stopRefresh();
    }
  }

  componentWillUnmount() {
    this.stopRefresh();
  }

  resetForm() {
    const machine = this.props.machine;
    this.setState({preflight: null, preflightError: null, logs: [], selectedTaskId: null}, () => {
      this.formRef.current?.setFieldsValue({
        workerNodeName: machine?.name || "",
        apiserverUrl: "",
      });
    });
  }

  startRefresh() {
    this.stopRefresh();
    this.refreshTimer = setInterval(() => this.loadTasks(false), 5000);
  }

  stopRefresh() {
    if (this.refreshTimer) {
      clearInterval(this.refreshTimer);
      this.refreshTimer = null;
    }
  }

  buildRequest(values) {
    const machine = this.props.machine;
    const owner = machine?.owner || this.props.account?.name;
    if (!owner) {
      throw new Error(i18next.t("machine:Machine owner is required"));
    }
    return {
      owner,
      machineName: machine?.name,
      nodeName: values.workerNodeName || machine?.name,
      apiserverUrl: values.apiserverUrl || "",
    };
  }

  ensureDefaultNodeName() {
    const machineName = this.props.machine?.name;
    const currentValues = this.formRef.current?.getFieldsValue() || {};
    if (!currentValues.workerNodeName && machineName) {
      this.formRef.current?.setFieldsValue({workerNodeName: machineName});
    }
  }

  loadTasks(showLoading = true, preferLatest = false) {
    const machine = this.props.machine;
    if (!machine) {
      return;
    }
    const owner = machine.owner || this.props.account?.name;
    if (!owner) {
      Setting.showMessage("error", i18next.t("machine:Machine owner is required"));
      return;
    }
    if (showLoading) {
      this.setState({loadingTasks: true});
    }
    MachineNodeDeployBackend.getMachineNodeTasks(owner, machine.name)
      .then(res => {
        if (res.status === "ok") {
          const tasks = res.data || [];
          const latestTaskId = tasks[0]?.id || null;
          const selectedTaskExists = tasks.some(task => task.id === this.state.selectedTaskId);
          let selectedTaskId = latestTaskId;
          if (!preferLatest && selectedTaskExists) {
            selectedTaskId = this.state.selectedTaskId;
          }
          this.setState({tasks, selectedTaskId}, () => {
            if (selectedTaskId) {
              this.loadLogs(selectedTaskId, false);
            }
          });
        } else {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch(e => Setting.showMessage("error", `${i18next.t("machine:Failed to load node tasks")}: ${e.message}`))
      .finally(() => this.setState({loadingTasks: false}));
  }

  loadLogs(taskId, showLoading = true) {
    if (!taskId) {
      return;
    }
    if (showLoading) {
      this.setState({loadingLogs: true});
    }
    MachineNodeDeployBackend.getMachineNodeLogs(taskId)
      .then(res => {
        if (res.status === "ok") {
          this.setState({logs: res.data || [], selectedTaskId: taskId});
        } else {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch(e => Setting.showMessage("error", `${i18next.t("machine:Failed to load node logs")}: ${e.message}`))
      .finally(() => this.setState({loadingLogs: false}));
  }

  preflight() {
    this.ensureDefaultNodeName();
    this.formRef.current?.validateFields()
      .then(values => {
        let request;
        try {
          request = this.buildRequest(values);
        } catch (e) {
          Setting.showMessage("error", e.message);
          return;
        }
        this.setState({preflighting: true, preflight: null, preflightError: null});
        MachineNodeDeployBackend.preflightMachineNode(request)
          .then(res => {
            if (res.status === "ok") {
              this.setState({preflight: res.data});
              Setting.showMessage("success", i18next.t("machine:Node preflight passed"));
            } else {
              this.setState({preflightError: res.msg});
              Setting.showMessage("error", res.msg);
            }
          })
          .catch(e => {
            const message = `${i18next.t("machine:Preflight check failed")}: ${e.message}`;
            this.setState({preflightError: message});
            Setting.showMessage("error", message);
          })
          .finally(() => this.setState({preflighting: false}));
      })
      .catch(() => {});
  }

  deploy(repair = false) {
    this.ensureDefaultNodeName();
    this.formRef.current?.validateFields()
      .then(values => {
        const key = repair ? "repairing" : "deploying";
        let request;
        try {
          request = this.buildRequest(values);
        } catch (e) {
          Setting.showMessage("error", e.message);
          return;
        }
        this.setState({[key]: true});
        const action = repair ? MachineNodeDeployBackend.repairMachineNode : MachineNodeDeployBackend.deployMachineNode;
        action(request)
          .then(res => {
            if (res.status === "ok") {
              Setting.showMessage("success", repair ? i18next.t("machine:Node repair started") : i18next.t("machine:Node deployment started"));
              this.setState({selectedTaskId: res.data?.id || null, logs: []}, () => this.loadTasks(true, true));
            } else {
              Setting.showMessage("error", res.msg);
              this.loadTasks(false, true);
            }
          })
          .catch(e => {
            Setting.showMessage("error", `${repair ? i18next.t("machine:Repair failed") : i18next.t("machine:Deployment failed")}: ${e.message}`);
            this.loadTasks(false, true);
          })
          .finally(() => this.setState({[key]: false}));
      })
      .catch(() => {});
  }

  renderPreflight() {
    if (this.state.preflightError) {
      return (
        <Alert
          type="error"
          showIcon
          message={i18next.t("machine:Preflight failed")}
          description={this.state.preflightError}
          style={{marginTop: 12}}
        />
      );
    }
    const result = this.state.preflight;
    if (!result) {
      return null;
    }
    const data = result.preflight || {};
    return (
      <Descriptions size="small" bordered column={2} style={{marginTop: 12}}>
        <Descriptions.Item label="Node">{result.nodeName || "-"}</Descriptions.Item>
        <Descriptions.Item label="Apiserver">{result.apiserverUrl || "-"}</Descriptions.Item>
        <Descriptions.Item label="OS">{data.os}</Descriptions.Item>
        <Descriptions.Item label="Arch">{data.arch}</Descriptions.Item>
        <Descriptions.Item label="systemd">{data.systemd ? "yes" : "no"}</Descriptions.Item>
        <Descriptions.Item label="Package">{data.packageTool}</Descriptions.Item>
        <Descriptions.Item label="sudo">{data.canSudo ? "yes" : "no"}</Descriptions.Item>
        <Descriptions.Item label="WSL">{data.wsl ? "yes" : "no"}</Descriptions.Item>
      </Descriptions>
    );
  }

  renderLogs() {
    const logs = this.state.logs || [];
    if (logs.length === 0) {
      return <Text type="secondary">{i18next.t("machine:No node deployment logs")}</Text>;
    }
    return (
      <div style={{background: "var(--ant-color-bg-layout)", border: "1px solid var(--ant-color-border)", borderRadius: 6, padding: 12, maxHeight: 260, overflow: "auto"}}>
        {logs.map(log => (
          <div key={log.id} style={{fontFamily: "monospace", fontSize: 12, lineHeight: "22px", whiteSpace: "pre-wrap"}}>
            <Text type="secondary">{log.createdAt}</Text>&nbsp;
            <Tag color={log.level === "error" ? "red" : "blue"}>{log.level}</Tag>
            {log.message}
          </div>
        ))}
      </div>
    );
  }

  render() {
    const {machine, open, onClose} = this.props;
    const {tasks, loadingTasks, preflighting, deploying, repairing, selectedTaskId} = this.state;
    const selectedTask = tasks.find(task => task.id === selectedTaskId);
    const columns = [
      {
        title: "ID",
        dataIndex: "id",
        width: 70,
      },
      {
        title: i18next.t("machine:Node name"),
        dataIndex: "nodeName",
        width: 150,
      },
      {
        title: i18next.t("general:Status"),
        dataIndex: "status",
        width: 120,
        render: value => <Tag color={taskStatusColor[value] || "default"}>{value}</Tag>,
      },
      {
        title: i18next.t("general:Phase"),
        dataIndex: "phase",
      },
      {
        title: i18next.t("general:Updated"),
        dataIndex: "updatedAt",
        width: 190,
      },
    ];

    return (
      <Drawer
        title={machine ? `${i18next.t("machine:Worker Node")} - ${machine.name}` : i18next.t("machine:Worker Node")}
        open={open}
        onClose={onClose}
        width={760}
        destroyOnHidden
      >
        <Form ref={this.formRef} layout="vertical">
          <Form.Item label={i18next.t("machine:Node name")} name="workerNodeName" rules={[{required: true, message: i18next.t("machine:Node name is required")}]}>
            <Input placeholder={machine?.name || "worker-node"} />
          </Form.Item>
          <Form.Item label={i18next.t("machine:Apiserver URL")} name="apiserverUrl">
            <Input placeholder={i18next.t("machine:Apiserver URL placeholder")} />
          </Form.Item>
        </Form>

        <Space wrap style={{marginBottom: 12}}>
          <Button icon={<FileSearchOutlined />} onClick={() => this.preflight()} loading={preflighting}>
            {i18next.t("machine:Preflight")}
          </Button>
          <Button type="primary" icon={<CloudSyncOutlined />} onClick={() => this.deploy(false)} loading={deploying}>
            {i18next.t("machine:Deploy Node")}
          </Button>
          <Button icon={<ToolOutlined />} onClick={() => this.deploy(true)} loading={repairing}>
            {i18next.t("machine:Repair Node")}
          </Button>
          <Button icon={<ReloadOutlined />} onClick={() => this.loadTasks()} loading={loadingTasks}>
            {i18next.t("general:Refresh")}
          </Button>
        </Space>

        {this.renderPreflight()}

        <Table
          style={{marginTop: 16}}
          size="small"
          rowKey="id"
          columns={columns}
          dataSource={tasks}
          loading={loadingTasks}
          pagination={{pageSize: 5}}
          onRow={record => ({
            onClick: () => {
              if (this.state.selectedTaskId !== record.id) {
                this.loadLogs(record.id);
              }
            },
          })}
        />

        {selectedTask?.errorMsg && (
          <Alert
            type="error"
            showIcon
            message={i18next.t("machine:Node deployment failed")}
            description={selectedTask.errorMsg}
            style={{marginTop: 12}}
          />
        )}

        <div style={{fontWeight: 600, margin: "16px 0 8px"}}>{i18next.t("machine:Node deployment logs")}</div>
        {this.renderLogs()}
      </Drawer>
    );
  }
}

export default MachineNodeDeployPanel;
