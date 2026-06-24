import React from "react";
import {Badge, Button, Form, Input, Modal, Popconfirm, Space, Table, Tag, Tooltip, Typography} from "antd";
import {DeleteOutlined, RedoOutlined, ReloadOutlined, SafetyCertificateOutlined, ScanOutlined} from "@ant-design/icons";
import i18next from "i18next";
import * as TrivyBackend from "./backend/TrivyBackend";
import * as Setting from "./Setting";

const {Text, Paragraph} = Typography;

function severityTag(level) {
  const colors = {CRITICAL: "red", HIGH: "orange", MEDIUM: "gold", LOW: "blue", UNKNOWN: "default"};
  return <Tag color={colors[level] || "default"}>{level}</Tag>;
}

function statusTag(status) {
  if (status === "done") {return <Tag color="green">done</Tag>;}
  if (status === "failed") {return <Tag color="red">failed</Tag>;}
  return <Tag color="processing">pending</Tag>;
}

function scoreCell(count, color) {
  if (!count) {return <Text type="secondary">0</Text>;}
  return <Badge count={count} color={color} overflowCount={999} />;
}

class TrivyScanPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      results: [],
      loading: false,
      scanning: false,
      modalVisible: false,
    };
    this.formRef = React.createRef();
  }

  componentDidMount() {
    this.loadResults();
  }

  loadResults() {
    this.setState({loading: true});
    TrivyBackend.getTrivyScanResults().then(res => {
      if (res.status === "ok") {
        this.setState({results: res.data || [], loading: false});
      } else {
        Setting.showMessage("error", res.msg);
        this.setState({loading: false});
      }
    });
  }

  handleScan(values) {
    this.setState({scanning: true});
    TrivyBackend.triggerTrivyScan(values.image.trim()).then(res => {
      this.setState({scanning: false, modalVisible: false});
      if (res.status === "ok") {
        Setting.showMessage("success", i18next.t("trivy:Scan complete"));
        this.loadResults();
      } else {
        Setting.showMessage("error", res.msg);
      }
    });
  }

  handleDelete(id) {
    TrivyBackend.deleteTrivyScanResult(id).then(res => {
      if (res.status === "ok") {
        Setting.showMessage("success", i18next.t("general:Successfully deleted"));
        this.loadResults();
      } else {
        Setting.showMessage("error", res.msg);
      }
    });
  }

  getColumns() {
    return [
      {
        title: i18next.t("general:Image"),
        dataIndex: "image",
        key: "image",
        render: (v) => <Text code style={{wordBreak: "break-all"}}>{v}</Text>,
      },
      {
        title: i18next.t("general:Status"),
        dataIndex: "status",
        key: "status",
        width: 100,
        render: (v) => statusTag(v),
      },
      {
        title: "CRITICAL",
        dataIndex: "critical",
        key: "critical",
        width: 100,
        render: (v) => scoreCell(v, "red"),
      },
      {
        title: "HIGH",
        dataIndex: "high",
        key: "high",
        width: 80,
        render: (v) => scoreCell(v, "orange"),
      },
      {
        title: "MEDIUM",
        dataIndex: "medium",
        key: "medium",
        width: 80,
        render: (v) => scoreCell(v, "gold"),
      },
      {
        title: "LOW",
        dataIndex: "low",
        key: "low",
        width: 70,
        render: (v) => scoreCell(v, "blue"),
      },
      {
        title: i18next.t("trivy:Scanned At"),
        dataIndex: "scannedAt",
        key: "scannedAt",
        width: 180,
        render: (v) => v ? Setting.getFormattedDate(v) : "-",
      },
      {
        title: i18next.t("general:Action"),
        key: "action",
        width: 110,
        render: (_, record) => (
          <Space size={4}>
            {record.status === "failed" && (
              <Tooltip title={i18next.t("trivy:Rescan")}>
                <Button
                  type="text"
                  icon={<RedoOutlined />}
                  size="small"
                  onClick={() => {
                    TrivyBackend.deleteTrivyScanResult(record.id).then(() => {
                      TrivyBackend.triggerTrivyScan(record.image).then(res => {
                        if (res.status === "ok") {
                          Setting.showMessage("success", i18next.t("trivy:Scan complete"));
                        } else {
                          Setting.showMessage("error", res.msg);
                        }
                        this.loadResults();
                      });
                    });
                  }}
                />
              </Tooltip>
            )}
            <Popconfirm
              title={i18next.t("trivy:Delete this result?")}
              onConfirm={() => this.handleDelete(record.id)}
              okText={i18next.t("general:OK")}
              cancelText={i18next.t("general:Cancel")}
            >
              <Tooltip title={i18next.t("general:Delete")}>
                <Button type="text" danger icon={<DeleteOutlined />} size="small" />
              </Tooltip>
            </Popconfirm>
          </Space>
        ),
      },
    ];
  }

  renderVulnDetail(record) {
    if (!record.vulnerabilities || record.vulnerabilities.length === 0) {
      if (record.errorMsg) {
        return <Paragraph type="danger">{record.errorMsg}</Paragraph>;
      }
      return <Text type="secondary">{i18next.t("trivy:No vulnerabilities found")}</Text>;
    }

    const vulnColumns = [
      {title: "CVE ID", dataIndex: "VulnerabilityID", key: "cve", width: 180,
        render: (v) => <a href={`https://nvd.nist.gov/vuln/detail/${v}`} target="_blank" rel="noreferrer">{v}</a>},
      {title: i18next.t("trivy:Package"), dataIndex: "PkgName", key: "pkg", width: 160},
      {title: i18next.t("trivy:Installed"), dataIndex: "InstalledVersion", key: "installed", width: 120},
      {title: i18next.t("trivy:Fixed In"), dataIndex: "FixedVersion", key: "fixed", width: 120,
        render: (v) => v || <Text type="secondary">-</Text>},
      {title: i18next.t("trivy:Severity"), dataIndex: "Severity", key: "severity", width: 110,
        render: (v) => severityTag(v)},
      {title: i18next.t("trivy:Title"), dataIndex: "Title", key: "title",
        render: (v) => <Text style={{fontSize: 12}}>{v}</Text>},
    ];

    return (
      <Table
        columns={vulnColumns}
        dataSource={record.vulnerabilities}
        rowKey="VulnerabilityID"
        size="small"
        pagination={{pageSize: 10, showSizeChanger: false}}
      />
    );
  }

  render() {
    const {results, loading, scanning, modalVisible} = this.state;

    return (
      <div style={{padding: "16px 24px"}}>
        <Space style={{marginBottom: 16}}>
          <Button
            type="primary"
            icon={<ScanOutlined />}
            onClick={() => {this.setState({modalVisible: true}); this.formRef.current?.resetFields();}}
          >
            {i18next.t("trivy:Scan Image")}
          </Button>
          <Tooltip title={i18next.t("trivy:Refresh tooltip")}>
            <Button icon={<ReloadOutlined />} onClick={() => this.loadResults()}>
              {i18next.t("general:Refresh")}
            </Button>
          </Tooltip>
        </Space>

        <p style={{color: "var(--ant-color-text-secondary)", marginBottom: 12, fontSize: 13}}>
          <SafetyCertificateOutlined style={{marginRight: 6}} />
          {i18next.t("trivy:page desc")}
        </p>

        <Table
          columns={this.getColumns()}
          dataSource={results}
          rowKey="id"
          loading={loading}
          size="middle"
          expandable={{
            expandedRowRender: (record) => this.renderVulnDetail(record),
            rowExpandable: (record) => record.status === "done" || !!record.errorMsg,
          }}
          pagination={{pageSize: 20, showSizeChanger: false}}
        />

        <Modal
          title={i18next.t("trivy:Scan Image")}
          open={modalVisible}
          onCancel={() => this.setState({modalVisible: false})}
          footer={null}
          destroyOnClose
        >
          <Form ref={this.formRef} layout="vertical" onFinish={(v) => this.handleScan(v)}>
            <Form.Item
              name="image"
              label={i18next.t("trivy:Image name")}
              rules={[{required: true, message: i18next.t("appStore:Image required")}]}
            >
              <Input placeholder="e.g. nginx:1.25 or docker.io/library/nginx:latest" autoFocus />
            </Form.Item>
            <p style={{color: "var(--ant-color-text-secondary)", fontSize: 12, marginTop: -8, marginBottom: 12}}>
              {i18next.t("trivy:scan may take minutes")}
            </p>
            <Form.Item style={{marginBottom: 0}}>
              <Space>
                <Button type="primary" htmlType="submit" loading={scanning} icon={<ScanOutlined />}>
                  {i18next.t("trivy:Start Scan")}
                </Button>
                <Button onClick={() => this.setState({modalVisible: false})}>
                  {i18next.t("general:Cancel")}
                </Button>
              </Space>
            </Form.Item>
          </Form>
        </Modal>
      </div>
    );
  }
}

export default TrivyScanPage;
