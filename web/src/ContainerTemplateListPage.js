import React from "react";
import {withRouter} from "react-router-dom";
import {Alert, Button, Card, Form, Modal, Select, Space, Tag} from "antd";
import {AppstoreOutlined, ReloadOutlined, RocketOutlined} from "@ant-design/icons";
import * as ContainerTemplateBackend from "./backend/ContainerTemplateBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import * as Setting from "./Setting";

class ContainerTemplateListPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      templates: [],
      namespaces: [],
      loading: false,
      error: null,
      deployModalVisible: false,
      deployingTemplate: null,
      submitting: false,
    };
    this.formRef = React.createRef();
  }

  componentDidMount() {
    this.fetchTemplates();
    this.fetchNamespaces();
  }

  fetchTemplates() {
    this.setState({loading: true, error: null});
    ContainerTemplateBackend.getContainerTemplates().then(res => {
      if (res.status === "ok") {
        this.setState({templates: res.data ?? []});
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

  fetchNamespaces() {
    NamespaceBackend.getNamespaces().then(res => {
      if (res.status === "ok") {
        this.setState({namespaces: res.data ?? []});
      }
    }).catch(() => {});
  }

  openDeployModal(template) {
    const nsList = this.state.namespaces.map(n => n.name);
    const defaultNs = nsList.includes("default") ? "default" : (nsList[0] ?? "default");
    this.setState({
      deployModalVisible: true,
      deployingTemplate: template,
    }, () => {
      // Set initial form values after the modal is rendered.
      setTimeout(() => {
        this.formRef.current?.setFieldsValue({namespace: defaultNs});
      }, 0);
    });
  }

  closeDeployModal() {
    this.setState({deployModalVisible: false, deployingTemplate: null});
  }

  handleDeploy() {
    this.formRef.current?.validateFields().then(values => {
      const {deployingTemplate} = this.state;
      this.setState({submitting: true});
      ContainerTemplateBackend.deployContainerTemplate(
        deployingTemplate.name,
        values.namespace
      ).then(res => {
        if (res.status === "ok") {
          Setting.showMessage("success", `Deployed ${deployingTemplate.displayName}`);
          this.closeDeployModal();
          // Navigate to Pod list after a short delay.
          setTimeout(() => this.props.history.push("/pods"), 1500);
        } else {
          Setting.showMessage("error", res.msg);
        }
      }).catch(e => Setting.showMessage("error", e.message))
        .finally(() => this.setState({submitting: false}));
    });
  }

  renderTemplateCard(template) {
    return (
      <Card
        key={template.name}
        hoverable
        style={{width: 280}}
        styles={{body: {padding: 16}}}
      >
        <Space direction="vertical" size={8} style={{width: "100%"}}>
          <div style={{display: "flex", alignItems: "center", gap: 8}}>
            <span style={{fontSize: 28}}>{template.icon}</span>
            <span style={{fontWeight: 600, fontSize: 16}}>{template.displayName}</span>
          </div>
          <div style={{fontSize: 12, color: "#8c8c8c", minHeight: 36}}>
            {template.description}
          </div>
          <div style={{fontSize: 11, color: "#bfbfbf"}}>
            <code style={{background: "#f5f5f5", padding: "2px 6px", borderRadius: 3}}>
              {template.image}
            </code>
          </div>
          <Tag color="blue">Port: {template.defaultPort}</Tag>
          <Button
            type="primary"
            icon={<RocketOutlined />}
            onClick={() => this.openDeployModal(template)}
            block
          >
            Deploy
          </Button>
        </Space>
      </Card>
    );
  }

  render() {
    const {templates, loading, error, deployModalVisible, deployingTemplate, submitting} = this.state;
    const nsOptions = this.state.namespaces.map(ns => ({label: ns.name, value: ns.name}));

    return (
      <div style={{padding: 24}}>
        {error && (
          <Alert type="error" message={error} style={{marginBottom: 16}} showIcon />
        )}
        <div style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          marginBottom: 16,
        }}>
          <Space>
            <AppstoreOutlined style={{fontSize: 18, color: "#1677ff"}} />
            <span style={{fontWeight: 600, fontSize: 16}}>App Store</span>
          </Space>
          <Button
            icon={<ReloadOutlined />}
            onClick={() => this.fetchTemplates()}
            loading={loading}
            size="small"
          >
            Refresh
          </Button>
        </div>
        <div style={{
          display: "flex",
          flexWrap: "wrap",
          gap: 16,
        }}>
          {templates.map(t => this.renderTemplateCard(t))}
        </div>
        {templates.length === 0 && !loading && (
          <div style={{textAlign: "center", color: "#bfbfbf", padding: 48}}>
            No templates available.
          </div>
        )}

        <Modal
          title={deployingTemplate ? `Deploy ${deployingTemplate.displayName}` : "Deploy"}
          open={deployModalVisible}
          onOk={() => this.handleDeploy()}
          onCancel={() => this.closeDeployModal()}
          confirmLoading={submitting}
          okText="Deploy"
          destroyOnHidden
        >
          {deployingTemplate && (
            <Form ref={this.formRef} layout="vertical">
              <div style={{marginBottom: 16, padding: 12, background: "#f5f5f5", borderRadius: 6}}>
                <div><b>{deployingTemplate.icon} {deployingTemplate.displayName}</b></div>
                <div style={{fontSize: 12, color: "#8c8c8c", marginTop: 4}}>
                  {deployingTemplate.description}
                </div>
              </div>
              <Form.Item
                label="Namespace"
                name="namespace"
                rules={[{required: true, message: "Namespace is required"}]}
              >
                <Select options={nsOptions} placeholder="Select a namespace" showSearch />
              </Form.Item>
              <Form.Item label="Image">
                <code>{deployingTemplate.image}</code>
              </Form.Item>
              <Form.Item label="Service Port">
                <Tag color="blue">{deployingTemplate.defaultPort} (ClusterIP)</Tag>
              </Form.Item>
            </Form>
          )}
        </Modal>
      </div>
    );
  }
}

export default withRouter(ContainerTemplateListPage);
