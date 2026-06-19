import React from "react";
import {Button, Form, Input, Modal, Popconfirm, Select, Space, Table, Tag, Tooltip, Typography} from "antd";
import {DeleteOutlined, PlusOutlined, ReloadOutlined, SafetyCertificateOutlined} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import i18next from "i18next";
import * as CasbinRuleBackend from "./backend/CasbinRuleBackend";
import * as Setting from "./Setting";

const {Text} = Typography;
const {Option} = Select;

const RESOURCES = ["*", "pods", "deployments", "statefulsets", "services", "ingresses",
  "configmaps", "secrets", "persistentvolumeclaims", "nodes", "namespaces",
  "serviceaccounts", "clusterrolebindings"];

const ADMISSION_ACTIONS = ["*", "CREATE", "UPDATE", "DELETE", "CONNECT"];
const AUTHORIZATION_VERBS = ["*", "get", "list", "watch", "create", "update", "patch", "delete", "deletecollection"];

function ruleTag(pType) {
  return pType === "g"
    ? <Tag color="blue">{i18next.t("policy:g role")}</Tag>
    : <Tag color="green">{i18next.t("policy:p policy")}</Tag>;
}

function eftTag(v4) {
  return v4 === "deny"
    ? <Tag color="red">{i18next.t("policy:deny")}</Tag>
    : <Tag color="green">{i18next.t("policy:allow")}</Tag>;
}

class CasbinRuleListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      rules: [],
      loading: true,
      modalVisible: false,
      submitting: false,
    };
    this.formRef = React.createRef();
  }

  componentDidMount() {
    this.loadRules();
  }

  componentDidUpdate(prevProps) {
    if (prevProps.scope !== this.props.scope) {
      this.loadRules();
    }
  }

  loadRules() {
    const {scope} = this.props;
    this.setState({loading: true});
    CasbinRuleBackend.getCasbinRules(scope)
      .then(res => {
        if (res.status === "ok") {
          this.setState({rules: res.data || [], loading: false});
        } else {
          Setting.showMessage("error", res.msg);
          this.setState({loading: false});
        }
      })
      .catch(err => {
        Setting.showMessage("error", err.message);
        this.setState({loading: false});
      });
  }

  handleAdd(values) {
    const {scope} = this.props;
    this.setState({submitting: true});
    const rule = {
      scope,
      pType: values.pType,
      v0: values.v0,
      v1: values.v1 || "",
      v2: values.v2 || "",
      v3: values.v3 || "",
      v4: values.pType === "p" ? (values.v4 || "allow") : "",
    };
    CasbinRuleBackend.addCasbinRule(rule)
      .then(res => {
        if (res.status === "ok") {
          Setting.showMessage("success", i18next.t("policy:Rule added"));
          this.setState({modalVisible: false, submitting: false});
          this.formRef.current?.resetFields();
          this.loadRules();
        } else {
          Setting.showMessage("error", res.msg);
          this.setState({submitting: false});
        }
      })
      .catch(err => {
        Setting.showMessage("error", err.message);
        this.setState({submitting: false});
      });
  }

  handleDelete(id) {
    const {scope} = this.props;
    CasbinRuleBackend.deleteCasbinRule(id, scope)
      .then(res => {
        if (res.status === "ok") {
          Setting.showMessage("success", i18next.t("policy:Rule deleted"));
          this.loadRules();
        } else {
          Setting.showMessage("error", res.msg);
        }
      });
  }

  handleReload() {
    const {scope} = this.props;
    CasbinRuleBackend.reloadCasbinEnforcer(scope)
      .then(res => {
        if (res.status === "ok") {
          Setting.showMessage("success", i18next.t("policy:Enforcer reloaded"));
        } else {
          Setting.showMessage("error", res.msg);
        }
      });
  }

  renderAddModal() {
    const {modalVisible, submitting} = this.state;
    const {scope} = this.props;
    const actionOptions = scope === "authorization" ? AUTHORIZATION_VERBS : ADMISSION_ACTIONS;
    const actionLabel = scope === "authorization" ? i18next.t("policy:Verb") : i18next.t("general:Action");

    return (
      <Modal
        title={i18next.t("policy:Add Rule")}
        open={modalVisible}
        onCancel={() => {
          this.setState({modalVisible: false});
          this.formRef.current?.resetFields();
        }}
        onOk={() => this.formRef.current?.submit()}
        confirmLoading={submitting}
        destroyOnClose
      >
        <Form ref={this.formRef} layout="vertical" onFinish={(v) => this.handleAdd(v)}
          initialValues={{pType: "p", v1: "*", v2: "*", v3: "*", v4: "allow"}}>
          <Form.Item name="pType" label={i18next.t("policy:Type")} rules={[{required: true}]}>
            <Select>
              <Option value="p">{i18next.t("policy:p policy")}</Option>
              <Option value="g">{i18next.t("policy:g role")}</Option>
            </Select>
          </Form.Item>
          <Form.Item noStyle shouldUpdate={(prev, cur) => prev.pType !== cur.pType}>
            {({getFieldValue}) => {
              const isPolicy = getFieldValue("pType") === "p";
              return (
                <>
                  <Form.Item
                    name="v0"
                    label={isPolicy ? i18next.t("policy:Subject") : i18next.t("policy:User Group")}
                    rules={[{required: true, message: i18next.t("policy:required")}]}
                  >
                    <Input placeholder={isPolicy ? i18next.t("policy:subject placeholder") : i18next.t("policy:user placeholder")} />
                  </Form.Item>
                  {isPolicy ? (
                    <>
                      <Form.Item name="v1" label={i18next.t("policy:Namespace")}>
                        <Input placeholder={i18next.t("policy:namespace placeholder")} />
                      </Form.Item>
                      <Form.Item name="v2" label={i18next.t("policy:Resource")}>
                        <Select showSearch allowClear placeholder={i18next.t("policy:resource placeholder")}>
                          {RESOURCES.map(r => <Option key={r} value={r}>{r}</Option>)}
                        </Select>
                      </Form.Item>
                      <Form.Item name="v3" label={actionLabel}>
                        <Select placeholder={i18next.t("policy:action placeholder")}>
                          {actionOptions.map(a => <Option key={a} value={a}>{a}</Option>)}
                        </Select>
                      </Form.Item>
                      <Form.Item name="v4" label={i18next.t("policy:Effect")} rules={[{required: true}]}>
                        <Select>
                          <Option value="allow">{i18next.t("policy:allow")}</Option>
                          <Option value="deny">{i18next.t("policy:deny")}</Option>
                        </Select>
                      </Form.Item>
                    </>
                  ) : (
                    <Form.Item name="v1" label={i18next.t("policy:Role")} rules={[{required: true, message: i18next.t("policy:required")}]}>
                      <Input placeholder={i18next.t("policy:role placeholder")} />
                    </Form.Item>
                  )}
                </>
              );
            }}
          </Form.Item>
        </Form>
      </Modal>
    );
  }

  render() {
    const {rules, loading} = this.state;
    const {title, description, scope} = this.props;
    const actionLabel = scope === "authorization" ? i18next.t("policy:Verb") : i18next.t("general:Action");

    const columns = [
      {title: i18next.t("policy:Type"), dataIndex: "pType", width: 90, render: (v) => ruleTag(v)},
      {title: i18next.t("policy:Subject column"), dataIndex: "v0", render: (v) => <Text code>{v}</Text>},
      {title: i18next.t("policy:Namespace column"), dataIndex: "v1", render: (v) => v ? <Text code>{v}</Text> : <Text type="secondary">—</Text>},
      {title: i18next.t("policy:Resource"), dataIndex: "v2", render: (v) => v ? <Text code>{v}</Text> : <Text type="secondary">—</Text>},
      {title: actionLabel, dataIndex: "v3", render: (v) => v ? <Tag>{v}</Tag> : <Text type="secondary">—</Text>},
      {title: i18next.t("policy:Effect"), dataIndex: "v4", width: 80, render: (v, record) => record.pType === "p" ? eftTag(v) : <Text type="secondary">—</Text>},
      {
        title: i18next.t("policy:Op column"),
        key: "actions",
        width: 60,
        align: "center",
        render: (_, record) => (
          <Popconfirm title={i18next.t("policy:Delete this rule?")} onConfirm={() => this.handleDelete(record.id)}>
            <Button type="text" danger icon={<DeleteOutlined />} size="small" />
          </Popconfirm>
        ),
      },
    ];

    return (
      <div style={{padding: "24px"}}>
        <div style={{display: "flex", alignItems: "center", marginBottom: 16, gap: 8}}>
          <SafetyCertificateOutlined style={{fontSize: 20, color: "#1677ff"}} />
          <span style={{fontSize: 16, fontWeight: 600}}>{title}</span>
          <div style={{flex: 1}} />
          <Space>
            <Tooltip title={i18next.t("policy:Reload Enforcer tooltip")}>
              <Button icon={<ReloadOutlined />} onClick={() => this.handleReload()}>
                {i18next.t("policy:Reload Enforcer")}
              </Button>
            </Tooltip>
            <Button type="primary" icon={<PlusOutlined />} onClick={() => this.setState({modalVisible: true})}>
              {i18next.t("policy:Add Rule")}
            </Button>
          </Space>
        </div>

        {description && (
          <div style={{marginBottom: 12, padding: "8px 12px", background: "rgba(22,119,255,0.05)", borderRadius: 6, border: "1px solid rgba(22,119,255,0.15)"}}>
            <Text type="secondary" style={{fontSize: 12}}>{description}</Text>
          </div>
        )}

        <Table
          columns={columns}
          dataSource={rules}
          rowKey="id"
          loading={loading}
          pagination={{pageSize: 20}}
          size="middle"
        />

        {this.renderAddModal()}
      </div>
    );
  }
}

// Wrapper so the class component re-renders on language change
export default function CasbinRuleListPageWrapper(props) {
  useTranslation();
  return <CasbinRuleListPage {...props} />;
}
