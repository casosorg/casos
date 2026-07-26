import React from "react";
import {Alert, Button, Form, Input, Modal, Popconfirm, Select, Space, Spin, Table, Tag, Typography} from "antd";
import {DeleteOutlined, ReloadOutlined, UpCircleOutlined} from "@ant-design/icons";
import i18next from "i18next";
import * as AppBackend from "./backend/AppBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import * as Setting from "./Setting";

const {Text} = Typography;

function t(key, opts) {
  return i18next.t(key, opts);
}

function statusColor(status) {
  switch (status) {
  case "deployed":
    return "green";
  case "failed":
    return "red";
  case "pending-install":
  case "pending-upgrade":
    return "processing";
  default:
    return "default";
  }
}

class HelmReleaseListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      releases: [],
      namespaces: [],
      namespace: "",
      loading: true,
      error: null,
      upgradeTarget: null,
      submitting: false,
      task: null,
    };
    this.formRef = React.createRef();
    this.pollTimer = null;
  }

  componentDidMount() {
    this.fetchNamespaces();
    this.fetchReleases();
  }

  componentWillUnmount() {
    this.stopPolling();
  }

  stopPolling() {
    if (this.pollTimer) {
      clearInterval(this.pollTimer);
      this.pollTimer = null;
    }
  }

  fetchNamespaces() {
    NamespaceBackend.getNamespaces().then(res => {
      if (res.status === "ok") {
        this.setState({namespaces: res.data ?? []});
      }
    }).catch(() => {});
  }

  fetchReleases() {
    this.setState({loading: true, error: null});
    AppBackend.getHelmReleases(this.state.namespace).then(res => {
      if (res.status === "ok") {
        this.setState({releases: res.data ?? []});
      } else {
        this.setState({error: res.msg});
      }
    }).catch(e => this.setState({error: e.message}))
      .finally(() => this.setState({loading: false}));
  }

  pollTask(taskId, onDone) {
    this.stopPolling();
    this.pollTimer = setInterval(() => {
      AppBackend.getHelmTask(taskId).then(res => {
        if (res.status !== "ok") {
          this.stopPolling();
          this.setState({submitting: false});
          Setting.showMessage("error", res.msg);
          return;
        }
        const task = res.data;
        this.setState({task});
        if (task.status === "succeeded" || task.status === "failed") {
          this.stopPolling();
          this.setState({submitting: false});
          if (task.status === "succeeded") {
            Setting.showMessage("success", t("helm:Operation succeeded"));
            onDone?.();
          } else {
            Setting.showMessage("error", task.error || t("helm:Operation failed"));
          }
          this.fetchReleases();
        }
      }).catch(() => {});
    }, 2000);
  }

  handleUninstall(release) {
    this.setState({submitting: true, task: null});
    AppBackend.uninstallHelmRelease({
      namespace: release.namespace,
      release: release.name,
    }).then(res => {
      if (res.status === "ok") {
        this.pollTask(res.data?.taskId);
      } else {
        this.setState({submitting: false});
        Setting.showMessage("error", res.msg);
      }
    }).catch(e => {
      this.setState({submitting: false});
      Setting.showMessage("error", e.message);
    });
  }

  openUpgrade(release) {
    this.setState({upgradeTarget: release, task: null}, () => {
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({
          repoUrl: "",
          chart: release.chart ?? "",
          version: "",
          values: "",
        });
      }, 0);
    });
  }

  handleUpgrade() {
    this.formRef.current?.validateFields().then(values => {
      const release = this.state.upgradeTarget;
      this.setState({submitting: true, task: null});
      AppBackend.upgradeHelmRelease({
        namespace: release.namespace,
        release: release.name,
        repoUrl: values.repoUrl,
        chart: values.chart,
        version: values.version,
        values: values.values ?? "",
      }).then(res => {
        if (res.status === "ok") {
          this.pollTask(res.data?.taskId, () => this.setState({upgradeTarget: null}));
        } else {
          this.setState({submitting: false});
          Setting.showMessage("error", res.msg);
        }
      }).catch(e => {
        this.setState({submitting: false});
        Setting.showMessage("error", e.message);
      });
    });
  }

  renderTaskLogs() {
    const {task} = this.state;
    if (!task || !(task.logs ?? []).length) {return null;}
    return (
      <pre style={{
        marginTop: 12, maxHeight: 180, overflow: "auto", fontSize: 12,
        background: "#f6f6f6", padding: 8, borderRadius: 6,
      }}>
        {task.logs.slice(-40).join("\n")}
      </pre>
    );
  }

  render() {
    const {releases, namespaces, namespace, loading, error, upgradeTarget, submitting} = this.state;

    const columns = [
      {
        title: t("helm:Release"),
        dataIndex: "name",
        key: "name",
        render: text => <Text strong>{text}</Text>,
      },
      {title: t("general:Namespaces"), dataIndex: "namespace", key: "namespace"},
      {title: t("appStore:Chart"), dataIndex: "chart", key: "chart"},
      {title: t("appStore:Chart version"), dataIndex: "version", key: "version"},
      {title: t("helm:App version"), dataIndex: "appVersion", key: "appVersion"},
      {title: t("helm:Revision"), dataIndex: "revision", key: "revision", width: 90},
      {
        title: t("general:Status"),
        dataIndex: "status",
        key: "status",
        render: status => <Tag color={statusColor(status)}>{status}</Tag>,
      },
      {title: t("general:Updated"), dataIndex: "updated", key: "updated"},
      {
        title: t("general:Action"),
        key: "action",
        width: 200,
        render: (_, record) => (
          <Space>
            <Button
              size="small"
              icon={<UpCircleOutlined />}
              onClick={() => this.openUpgrade(record)}
            >
              {t("helm:Upgrade")}
            </Button>
            <Popconfirm
              title={t("helm:Uninstall confirm", {name: record.name})}
              okText={t("general:OK")}
              cancelText={t("general:Cancel")}
              onConfirm={() => this.handleUninstall(record)}
            >
              <Button size="small" danger icon={<DeleteOutlined />}>
                {t("helm:Uninstall")}
              </Button>
            </Popconfirm>
          </Space>
        ),
      },
    ];

    const nsOptions = [{label: t("helm:All namespaces"), value: ""}]
      .concat(namespaces.map(ns => ({label: ns.name, value: ns.name})));

    return (
      <div style={{padding: 24}}>
        <div style={{display: "flex", gap: 12, marginBottom: 16, alignItems: "center", flexWrap: "wrap"}}>
          <Select
            style={{width: 220}}
            value={namespace}
            options={nsOptions}
            onChange={value => this.setState({namespace: value}, () => this.fetchReleases())}
          />
          <Button icon={<ReloadOutlined />} onClick={() => this.fetchReleases()} loading={loading}>
            {t("general:Refresh")}
          </Button>
        </div>

        {error && <Alert type="error" message={error} showIcon style={{marginBottom: 16}} />}

        <Spin spinning={submitting}>
          <Table
            rowKey={record => `${record.namespace}/${record.name}`}
            columns={columns}
            dataSource={releases}
            loading={loading}
            size="middle"
            pagination={{pageSize: 20, showSizeChanger: false}}
          />
        </Spin>

        {this.renderTaskLogs()}

        <Modal
          title={t("helm:Upgrade release", {name: upgradeTarget?.name ?? ""})}
          open={!!upgradeTarget}
          onOk={() => this.handleUpgrade()}
          onCancel={() => this.setState({upgradeTarget: null})}
          confirmLoading={submitting}
          okText={t("helm:Upgrade")}
          cancelText={t("general:Cancel")}
          width={640}
          destroyOnHidden
        >
          <Alert
            type="info"
            showIcon
            style={{marginBottom: 16}}
            message={t("helm:Upgrade desc")}
          />
          <Form ref={this.formRef} layout="vertical">
            <Form.Item
              label={t("appStore:Repository URL")}
              name="repoUrl"
              rules={[{required: true, message: t("helm:Repository URL required")}]}
            >
              <Input placeholder="https://charts.example.com 或 oci://registry/charts" />
            </Form.Item>
            <Form.Item
              label={t("appStore:Chart")}
              name="chart"
              rules={[{required: true, message: t("helm:Chart required")}]}
            >
              <Input />
            </Form.Item>
            <Form.Item label={t("appStore:Chart version")} name="version" extra={t("helm:Version empty hint")}>
              <Input placeholder="latest" />
            </Form.Item>
            <Form.Item label={t("appStore:Helm values")} name="values" extra={t("appStore:Helm values desc")}>
              <Input.TextArea rows={10} spellCheck={false} style={{fontFamily: "monospace", fontSize: 12}} />
            </Form.Item>
          </Form>
          {this.renderTaskLogs()}
        </Modal>
      </div>
    );
  }
}

export default HelmReleaseListPage;
