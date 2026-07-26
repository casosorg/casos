import React from "react";
import {Alert, Collapse, Divider, Form, Input, InputNumber, Modal, Select, Space, Spin, Tag, Tooltip, Typography} from "antd";
import {InfoCircleOutlined, LockOutlined} from "@ant-design/icons";
import i18next from "i18next";
import * as AppBackend from "./backend/AppBackend";
import * as NamespaceBackend from "./backend/NamespaceBackend";
import * as NodeBackend from "./backend/NodeBackend";
import EnvVarEditor from "./EnvVarEditor";

const {Text, Paragraph} = Typography;

function t(key, opts) {
  return i18next.t(key, opts);
}

// Keys whose values are likely sensitive (API keys, passwords, tokens, secrets)
const SENSITIVE_RE = /key|secret|password|token|credential|auth/i;

function isSensitive(name) {
  return SENSITIVE_RE.test(name);
}

function isHelmTemplate(template) {
  return template?.packageType === "helm";
}

function getSourceLabel(template) {
  if (template?.source === "artifacthub") {return "ArtifactHub";}
  if (template?.source === "sealos") {return "Sealos";}
  if (template?.source === "repository") {return t("appStore:Custom repository");}
  return template?.source || "";
}

function editorRowsToPayload(rows = []) {
  return rows.filter(e => e.name).map(e => ({name: e.name, value: e.value ?? ""}));
}

function InputField({input}) {
  const sensitive = isSensitive(input.name);
  const Field = sensitive ? Input.Password : Input;
  const suffix = sensitive ? (
    <Tooltip title={t("appStore:Sensitive value hint")}>
      <LockOutlined style={{color: "rgba(0,0,0,0.35)"}} />
    </Tooltip>
  ) : undefined;

  return (
    <Form.Item
      key={input.name}
      label={
        <span>
          {input.name}
          {input.description && (
            <Tooltip title={input.description}>
              <InfoCircleOutlined style={{marginLeft: 6, color: "rgba(0,0,0,0.35)"}} />
            </Tooltip>
          )}
        </span>
      }
      name={["inputs", input.name]}
      initialValue={input.default ?? ""}
      rules={input.required ? [{required: true, message: `${input.name} is required`}] : []}
    >
      <Field placeholder={input.default || undefined} suffix={!sensitive ? suffix : undefined} />
    </Form.Item>
  );
}

class DeployAppModal extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      namespaces: [],
      nodeIP: null,
      envVars: [],
      submitting: false,
      result: null,
      error: null,
      // Helm-specific state
      chartInfoLoading: false,
      chartInfo: null,
      availableVersions: [],
      selectedVersion: "",
      helmTask: null,
      taskId: null,
    };
    this.formRef = React.createRef();
    this.pollTimer = null;
  }

  componentDidUpdate(prevProps) {
    if (this.props.open && !prevProps.open && this.props.template) {
      this.setState({
        result: null, error: null, envVars: [],
        chartInfo: null, availableVersions: [], selectedVersion: this.props.template?.version ?? "",
        helmTask: null, taskId: null,
      });
      this.fetchNamespaces();
      this.fetchNodeIP();
      if (isHelmTemplate(this.props.template)) {
        this.fetchChartInfo(this.props.template.version);
      }
    }
    if (!this.props.open && prevProps.open) {
      this.stopPolling();
    }
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
        const namespaces = res.data ?? [];
        this.setState({namespaces});
        const defaultNs = namespaces.length > 0 ? namespaces[0].name : "default";
        const tpl = this.props.template;
        setTimeout(() => {
          this.formRef.current?.setFieldsValue({
            namespace: defaultNs,
            name: tpl?.name ?? "",
            image: tpl?.image ?? "",
            replicas: 1,
            serviceType: "NodePort",
          });
        }, 0);
      }
    }).catch(() => {});
  }

  fetchNodeIP() {
    NodeBackend.getNodes().then(res => {
      if (res.status === "ok") {
        const nodes = res.data ?? [];
        for (const node of nodes) {
          if (node.externalIP) {
            this.setState({nodeIP: node.externalIP});
            return;
          }
        }
        for (const node of nodes) {
          if (node.internalIP) {
            this.setState({nodeIP: node.internalIP});
            return;
          }
        }
      }
    }).catch(() => {});
  }

  fetchChartInfo(version) {
    const tpl = this.props.template;
    if (!tpl) {return;}
    this.setState({chartInfoLoading: true, error: null});
    AppBackend.getHelmChartInfo({
      repoUrl: tpl.repoUrl,
      chart: tpl.chartName,
      version: version ?? "",
      source: tpl.source,
      repoName: tpl.repoName,
    }).then(res => {
      if (res.status === "ok") {
        const info = res.data?.info ?? null;
        const versions = res.data?.availableVersions ?? [];
        this.setState({
          chartInfo: info,
          availableVersions: versions,
          selectedVersion: version || info?.version || "",
        });
        setTimeout(() => {
          this.formRef.current?.setFieldsValue({helmValues: info?.defaultValues ?? ""});
        }, 0);
      } else {
        this.setState({error: res.msg});
      }
    }).catch(e => this.setState({error: e.message}))
      .finally(() => this.setState({chartInfoLoading: false}));
  }

  handleVersionChange(version) {
    this.setState({selectedVersion: version});
    this.fetchChartInfo(version);
  }

  startPolling(taskId) {
    this.stopPolling();
    this.pollTimer = setInterval(() => {
      AppBackend.getHelmTask(taskId).then(res => {
        if (res.status !== "ok") {
          this.stopPolling();
          this.setState({error: res.msg, submitting: false});
          return;
        }
        const task = res.data;
        this.setState({helmTask: task});
        if (task.status === "succeeded" || task.status === "failed") {
          this.stopPolling();
          this.setState({
            submitting: false,
            result: task.status === "succeeded" ? {helm: task.result ?? {}} : null,
            error: task.status === "failed" ? task.error : null,
          });
        }
      }).catch(() => {});
    }, 2000);
  }

  handleSubmit() {
    this.formRef.current?.validateFields().then(values => {
      const tpl = this.props.template;

      if (isHelmTemplate(tpl)) {
        const payload = {
          namespace: values.namespace,
          release: values.name,
          repoUrl: tpl?.repoUrl,
          chart: tpl?.chartName,
          version: this.state.selectedVersion || tpl?.version || "",
          values: values.helmValues ?? "",
        };
        this.setState({submitting: true, error: null, helmTask: null});
        AppBackend.installHelmChart(payload)
          .then(res => {
            if (res.status === "ok") {
              const taskId = res.data?.taskId;
              this.setState({taskId});
              this.startPolling(taskId);
            } else {
              this.setState({error: res.msg, submitting: false});
            }
          })
          .catch(e => this.setState({error: e.message, submitting: false}));
        return;
      }

      // Merge template inputs into env vars (inputs take precedence, empties are skipped)
      const inputEnvVars = Object.entries(values.inputs ?? {})
        .filter(([, v]) => v !== "" && v !== null && v !== undefined)
        .map(([k, v]) => ({name: k, value: String(v)}));

      const payload = {
        namespace: values.namespace,
        name: values.name,
        image: values.image,
        replicas: values.replicas ?? 1,
        ports: (tpl?.ports ?? []).map((p, i) => ({name: `port-${i}`, containerPort: p, protocol: "TCP"})),
        envVars: [...inputEnvVars, ...editorRowsToPayload(this.state.envVars)],
        serviceType: values.serviceType,
      };

      this.setState({submitting: true, error: null});
      AppBackend.deployApp(payload)
        .then(res => {
          if (res.status === "ok") {
            this.setState({result: res.data});
          } else {
            this.setState({error: res.msg});
          }
        })
        .catch(e => this.setState({error: e.message}))
        .finally(() => {
          if (!isHelmTemplate(tpl)) {
            this.setState({submitting: false});
          }
        });
    });
  }

  handleClose() {
    this.stopPolling();
    this.setState({result: null, error: null, envVars: [], helmTask: null, taskId: null});
    this.props.onClose?.();
  }

  renderHelmInfo() {
    const {template} = this.props;
    if (!isHelmTemplate(template)) {return null;}
    const sourceLabel = getSourceLabel(template);
    return (
      <Alert
        type="info"
        showIcon
        style={{marginBottom: 16}}
        message={t("appStore:Helm chart")}
        description={(
          <div style={{display: "grid", gap: 4}}>
            {sourceLabel && <div>{t("appStore:Source")}: <Text code>{sourceLabel}</Text></div>}
            {template.repoName && <div>{t("appStore:Repository")}: <Text code>{template.repoName}</Text></div>}
            {template.repoUrl && <div>{t("appStore:Repository URL")}: <Text copyable>{template.repoUrl}</Text></div>}
            {template.chartName && <div>{t("appStore:Chart")}: <Text code>{template.chartName}</Text></div>}
          </div>
        )}
      />
    );
  }

  renderHelmProgress() {
    const {helmTask, submitting} = this.state;
    if (!submitting || !helmTask) {return null;}
    const logs = helmTask.logs ?? [];
    return (
      <div style={{marginTop: 16}}>
        <Space>
          <Spin size="small" />
          <Text>{t("appStore:Installing chart")}</Text>
          <Tag color="processing">{helmTask.status}</Tag>
        </Space>
        {logs.length > 0 && (
          <pre style={{
            marginTop: 8, maxHeight: 200, overflow: "auto", fontSize: 12,
            background: "#f6f6f6", padding: 8, borderRadius: 6,
          }}>
            {logs.slice(-40).join("\n")}
          </pre>
        )}
      </div>
    );
  }

  renderInputs() {
    const {template} = this.props;
    const inputs = template?.inputs ?? [];
    if (inputs.length === 0) {return null;}

    const required = inputs.filter(i => i.required);
    const optional = inputs.filter(i => !i.required);

    return (
      <>
        <Divider orientation="left" orientationMargin={0} style={{marginTop: 4, marginBottom: 12}}>
          <Text style={{fontSize: 13}}>{t("appStore:App config")}</Text>
        </Divider>

        {required.length > 0 && (
          <>
            <div style={{marginBottom: 8, fontSize: 12, color: "rgba(0,0,0,0.45)"}}>
              {t("appStore:App config desc")}
            </div>
            {required.map(inp => <InputField key={inp.name} input={inp} />)}
          </>
        )}

        {optional.length > 0 && (
          <Collapse
            ghost
            size="small"
            style={{marginBottom: 8}}
            items={[{
              key: "optional",
              label: <Text style={{fontSize: 13}}>{t("appStore:Optional settings")}</Text>,
              children: optional.map(inp => <InputField key={inp.name} input={inp} />),
            }]}
          />
        )}
      </>
    );
  }

  renderResult() {
    const {result, nodeIP} = this.state;
    if (!result) {return null;}
    if (result.helm) {
      const helm = result.helm;
      return (
        <Alert
          type="success"
          showIcon
          message={t("appStore:Deploy success")}
          description={(
            <div style={{display: "grid", gap: 4}}>
              <div>{t("appStore:Helm release")} <Text code>{helm.name}</Text> {t("appStore:Helm release installed")}</div>
              <div>{t("general:Namespaces")}: <Text code>{helm.namespace}</Text></div>
              {helm.chart && <div>{t("appStore:Chart")}: <Text code>{helm.chart}</Text></div>}
              {helm.version && <div>{t("appStore:Chart version")}: <Text code>{helm.version}</Text></div>}
              {helm.status && <div>{t("general:Status")}: <Tag color="green">{helm.status}</Tag></div>}
              {helm.notes && (
                <Collapse
                  ghost
                  size="small"
                  items={[{
                    key: "notes",
                    label: <Text style={{fontSize: 13}}>{t("appStore:Release notes")}</Text>,
                    children: <Paragraph style={{whiteSpace: "pre-wrap", fontSize: 12}}>{helm.notes}</Paragraph>,
                  }]}
                />
              )}
            </div>
          )}
          style={{marginTop: 16}}
        />
      );
    }

    const svc = result.service;
    const accessUrls = svc && nodeIP
      ? (svc.ports ?? []).filter(p => p.nodePort).map(p => `http://${nodeIP}:${p.nodePort}`)
      : [];
    return (
      <Alert
        type="success"
        showIcon
        message={t("appStore:Deploy success")}
        description={
          <div>
            <div>
              Deployment <Text code>{result.deployment?.name}</Text> {t("appStore:Deployment started")}
            </div>
            {accessUrls.length > 0 && (
              <div style={{marginTop: 6}}>
                Access URL:
                {accessUrls.map((url, i) => (
                  <a key={i} href={url} target="_blank" rel="noopener noreferrer" style={{marginLeft: 8}}>
                    {url}
                  </a>
                ))}
              </div>
            )}
            {svc && accessUrls.length === 0 && (
              <div style={{marginTop: 6}}>
                {t("appStore:Service info prefix")} <Text code>{svc.name}</Text> {t("appStore:Service info suffix")}
                {svc.ports?.map(p => (
                  <Tag key={p.name} style={{marginLeft: 4}}>{svc.name}:{p.port}</Tag>
                ))}
              </div>
            )}
          </div>
        }
        style={{marginTop: 16}}
      />
    );
  }

  render() {
    const {open, template} = this.props;
    const {namespaces, envVars, submitting, result, error, chartInfoLoading, availableVersions, selectedVersion} = this.state;
    if (!template) {return null;}

    const nsOptions = namespaces.map(ns => ({label: ns.name, value: ns.name}));
    const isDone = !!result;
    const helmTemplate = isHelmTemplate(template);
    const versionOptions = availableVersions.map(v => ({label: v, value: v}));

    return (
      <Modal
        title={
          <Space>
            {template.icon && (
              <img src={template.icon} alt="" style={{width: 24, height: 24, objectFit: "contain"}}
                onError={e => {e.target.style.display = "none";}} />
            )}
            <span>{t("appStore:Deploy app title", {title: template.title})}</span>
          </Space>
        }
        open={open}
        onOk={isDone ? () => this.handleClose() : () => this.handleSubmit()}
        onCancel={() => this.handleClose()}
        okText={isDone ? t("appStore:Done") : (helmTemplate ? t("appStore:Deploy Helm chart") : t("appStore:Deploy"))}
        okButtonProps={submitting ? {disabled: true} : {}}
        cancelButtonProps={isDone ? {style: {display: "none"}} : {}}
        confirmLoading={submitting}
        width={680}
        destroyOnHidden
      >
        {error && (
          <Alert type="error" message={error} showIcon style={{marginBottom: 16}} />
        )}

        {!isDone && (
          <Form ref={this.formRef} layout="vertical">
            {this.renderHelmInfo()}
            <Form.Item
              label={t("general:Namespaces")}
              name="namespace"
              rules={[{required: true}]}
            >
              <Select options={nsOptions} showSearch placeholder={t("general:Namespaces")} />
            </Form.Item>
            <Form.Item
              label={helmTemplate ? t("appStore:Release name") : t("appStore:App name")}
              name="name"
              rules={[
                {required: true, message: t("appStore:App name required")},
                {pattern: /^[a-z0-9][a-z0-9-]*$/, message: t("appStore:App name pattern")},
              ]}
            >
              <Input placeholder={helmTemplate ? t("appStore:Release name placeholder") : t("appStore:App name placeholder")} />
            </Form.Item>

            {helmTemplate ? (
              <Spin spinning={chartInfoLoading}>
                {versionOptions.length > 0 && (
                  <Form.Item label={t("appStore:Chart version")}>
                    <Select
                      showSearch
                      value={selectedVersion || undefined}
                      options={versionOptions}
                      onChange={v => this.handleVersionChange(v)}
                      placeholder={t("appStore:Latest version")}
                    />
                  </Form.Item>
                )}
                <Form.Item
                  label={t("appStore:Helm values")}
                  name="helmValues"
                  extra={t("appStore:Helm values desc")}
                >
                  <Input.TextArea
                    rows={12}
                    spellCheck={false}
                    style={{fontFamily: "monospace", fontSize: 12}}
                  />
                </Form.Item>
              </Spin>
            ) : (
              <>
                <Form.Item
                  label={t("general:Image")}
                  name="image"
                  rules={[{required: true, message: t("appStore:Image required")}]}
                >
                  <Input />
                </Form.Item>
                <Form.Item label={t("appStore:Replicas")} name="replicas" rules={[{required: true}]}>
                  <InputNumber min={1} max={20} style={{width: "100%"}} />
                </Form.Item>
                <Form.Item label={t("appStore:Service type")} name="serviceType">
                  <Select options={[
                    {label: t("appStore:ClusterIP desc"), value: "ClusterIP"},
                    {label: t("appStore:NodePort desc"), value: "NodePort"},
                  ]} />
                </Form.Item>

                {template.ports?.length > 0 && (
                  <Form.Item label={t("appStore:Ports")}>
                    <Space wrap>
                      {template.ports.map(p => <Tag key={p}>:{p}/TCP</Tag>)}
                    </Space>
                  </Form.Item>
                )}

                {this.renderInputs()}

                <Divider orientation="left" orientationMargin={0} style={{marginTop: 4, marginBottom: 12}}>
                  <Text style={{fontSize: 13}}>{t("appStore:Env vars")}</Text>
                </Divider>
                <EnvVarEditor
                  value={envVars}
                  onChange={rows => this.setState({envVars: rows})}
                  configMaps={[]}
                  secrets={[]}
                />
              </>
            )}
          </Form>
        )}

        {this.renderHelmProgress()}
        {this.renderResult()}
      </Modal>
    );
  }
}

export default DeployAppModal;
