import React, {useEffect, useRef} from "react";
import {Form, Input, InputNumber, Modal, Select} from "antd";
import * as ServiceBackend from "./backend/ServiceBackend";
import * as Setting from "./Setting";

function getDefaultExposePorts(deploy) {
  const containerPort = deploy?.ports?.find((port) => Number(port.port) > 0)?.port;
  const numericPort = Number(containerPort);
  const port = numericPort > 0 ? numericPort : 80;
  return {
    port,
    targetPort: port,
  };
}

function getDefaultExposeSelector(deploy) {
  const entries = Object.entries(deploy?.selector || {}).filter(([key, value]) => key && value);
  if (entries.length > 0) {
    return Object.fromEntries(entries);
  }
  return deploy?.name ? {"app": deploy.name} : {};
}

function DeploymentExposeModal({deploy, open, onClose}) {
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = React.useState(false);
  const prevOpen = useRef(false);

  useEffect(() => {
    if (open && !prevOpen.current && deploy) {
      const defaults = getDefaultExposePorts(deploy);
      form.setFieldsValue({
        name: deploy.name,
        port: defaults.port,
        targetPort: defaults.targetPort,
        type: "ClusterIP",
      });
    }
    prevOpen.current = open;
  }, [open, deploy, form]);

  function handleOk() {
    form.validateFields().then(values => {
      const payload = {
        namespace: deploy.namespace,
        name: values.name,
        type: values.type,
        selector: getDefaultExposeSelector(deploy),
        ports: [{
          name: "http",
          protocol: "TCP",
          port: values.port,
          targetPort: String(values.targetPort),
        }],
      };
      setSubmitting(true);
      ServiceBackend.addService(payload).then(res => {
        if (res.status === "ok") {
          Setting.showMessage("success", `Service "${values.name}" created`);
          onClose();
        } else {
          Setting.showMessage("error", res.msg);
        }
      }).catch(e => Setting.showMessage("error", e.message))
        .finally(() => setSubmitting(false));
    });
  }

  return (
    <Modal
      title={`Expose Deployment: ${deploy?.name ?? ""}`}
      open={open}
      onOk={handleOk}
      onCancel={onClose}
      confirmLoading={submitting}
      okText="Create Service"
      width={480}
      destroyOnHidden
    >
      <Form form={form} layout="vertical">
        <Form.Item
          label="Service Name"
          name="name"
          rules={[{required: true, message: "Service name is required"}]}
        >
          <Input placeholder="my-deployment" />
        </Form.Item>
        <Form.Item label="Type" name="type" rules={[{required: true}]}>
          <Select options={[
            {label: "ClusterIP — 集群内访问", value: "ClusterIP"},
            {label: "NodePort — 节点端口对外暴露", value: "NodePort"},
            {label: "LoadBalancer — 云负载均衡器", value: "LoadBalancer"},
          ]} />
        </Form.Item>
        <Form.Item label="Port (Service 端口)" name="port" rules={[{required: true}]}>
          <InputNumber min={1} max={65535} style={{width: "100%"}} />
        </Form.Item>
        <Form.Item label="Target Port (Pod 容器端口)" name="targetPort" rules={[{required: true}]}>
          <InputNumber min={1} max={65535} style={{width: "100%"}} />
        </Form.Item>
      </Form>
    </Modal>
  );
}

export default DeploymentExposeModal;
