import React, {useEffect, useRef, useState} from "react";
import {Alert, Form, Input, Modal, Select, Switch, Tag, Typography} from "antd";
import {LinkOutlined, LockOutlined} from "@ant-design/icons";
import * as IngressBackend from "./backend/IngressBackend";
import * as Setting from "./Setting";

const {Text} = Typography;

function DeploymentDomainModal({deploy, services, open, onClose, onCreated}) {
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = useState(false);
  const [serviceOptions, setServiceOptions] = useState([]);
  const [tlsEnabled, setTlsEnabled] = useState(false);
  const prevOpen = useRef(false);

  useEffect(() => {
    if (!open || prevOpen.current) {prevOpen.current = open; return;}
    prevOpen.current = open;

    if (!deploy) {return;}

    const ns = deploy.namespace;
    const matchingServices = (services ?? []).filter(s => s.namespace === ns);
    const opts = matchingServices.flatMap(s =>
      (s.ports ?? []).map(p => ({
        label: `${s.name}:${p.port}`,
        value: `${s.name}|${p.port}`,
        serviceName: s.name,
        port: p.port,
      }))
    );
    setServiceOptions(opts);
    setTlsEnabled(false);

    const defaultSvc = opts.find(o => o.serviceName === deploy.name) ?? opts[0];
    form.setFieldsValue({
      host: "",
      path: "/",
      service: defaultSvc?.value ?? undefined,
      tlsEnabled: false,
      clusterIssuer: "letsencrypt-prod",
    });
  }, [open, deploy, services, form]);

  function handleOk() {
    form.validateFields().then(values => {
      const [serviceName, servicePort] = values.service.split("|");
      const ingressName = `${deploy.name}-domain`;
      const payload = {
        namespace: deploy.namespace,
        name: ingressName,
        ingressClass: "",
        rules: [{
          host: values.host,
          path: values.path ?? "/",
          pathType: "Prefix",
          serviceName,
          servicePort: Number(servicePort),
        }],
        tlsEnabled: values.tlsEnabled ?? false,
        clusterIssuer: values.tlsEnabled ? (values.clusterIssuer || "letsencrypt-prod") : "",
      };
      setSubmitting(true);
      IngressBackend.addIngress(payload)
        .then(res => {
          if (res.status === "ok") {
            const scheme = values.tlsEnabled ? "https" : "http";
            Setting.showMessage("success", `Domain "${scheme}://${values.host}" bound to ${deploy.name}`);
            onCreated?.();
            onClose();
          } else {
            Setting.showMessage("error", res.msg);
          }
        })
        .catch(e => Setting.showMessage("error", e.message))
        .finally(() => setSubmitting(false));
    });
  }

  const hasServices = serviceOptions.length > 0;

  return (
    <Modal
      title={
        <span>
          <LinkOutlined style={{marginRight: 8, color: "#1677ff"}} />
          Bind Domain — {deploy?.name ?? ""}
        </span>
      }
      open={open}
      onOk={handleOk}
      onCancel={onClose}
      confirmLoading={submitting}
      okText="Bind Domain"
      okButtonProps={{disabled: !hasServices}}
      width={480}
      destroyOnHidden
    >
      {!hasServices && (
        <Alert
          type="warning"
          showIcon
          message="No Service found for this deployment"
          description={
            <span>
              Please use the <Text strong>Expose</Text> button first to create a Service, then bind a domain.
            </span>
          }
          style={{marginBottom: 16}}
        />
      )}

      <Form form={form} layout="vertical">
        <Form.Item
          label="Domain"
          name="host"
          rules={[
            {required: true, message: "Domain is required"},
            {
              pattern: /^[a-zA-Z0-9*]([a-zA-Z0-9\-.*]*[a-zA-Z0-9])?$/,
              message: "Enter a valid domain, e.g. erp.company.internal",
            },
          ]}
          extra="The domain you want to use to access this application, e.g. erp.company.internal"
        >
          <Input
            placeholder="erp.company.internal"
            prefix={tlsEnabled ? "https://" : "http://"}
            disabled={!hasServices}
          />
        </Form.Item>

        <Form.Item
          label="Service"
          name="service"
          rules={[{required: true, message: "Please select a service"}]}
          extra="The service and port that traffic will be forwarded to"
        >
          <Select
            placeholder="Select service:port"
            options={serviceOptions}
            disabled={!hasServices}
            optionRender={opt => (
              <span>
                {opt.data.serviceName}
                <Tag color="blue" style={{marginLeft: 8}}>{opt.data.port}</Tag>
              </span>
            )}
          />
        </Form.Item>

        <Form.Item
          label="Path"
          name="path"
          extra="URL path prefix to match (leave / to route all traffic)"
        >
          <Input placeholder="/" disabled={!hasServices} />
        </Form.Item>

        <Form.Item
          label={
            <span>
              <LockOutlined style={{marginRight: 6, color: tlsEnabled ? "#52c41a" : undefined}} />
              Enable HTTPS (cert-manager)
            </span>
          }
          name="tlsEnabled"
          valuePropName="checked"
          extra={tlsEnabled ? "cert-manager will automatically issue and renew a TLS certificate for this domain" : "Enable to get a free TLS certificate via cert-manager + Let's Encrypt"}
        >
          <Switch
            disabled={!hasServices}
            onChange={v => {
              setTlsEnabled(v);
              form.setFieldValue("tlsEnabled", v);
            }}
          />
        </Form.Item>

        {tlsEnabled && (
          <Form.Item
            label="Cluster Issuer"
            name="clusterIssuer"
            rules={[{required: true, message: "Cluster issuer is required when HTTPS is enabled"}]}
            extra="The cert-manager ClusterIssuer to use. Must be pre-installed in your cluster."
          >
            <Input placeholder="letsencrypt-prod" disabled={!hasServices} />
          </Form.Item>
        )}
      </Form>
    </Modal>
  );
}

export default DeploymentDomainModal;
