import React from "react";
import {Typography} from "antd";
import CasbinRuleListPage from "./CasbinRuleListPage";

const {Text} = Typography;

const description = (
  <>
    <strong>ValidatingAdmissionWebhook</strong> — intercepts write operations (CREATE / UPDATE / DELETE / CONNECT) before resources are persisted to etcd.
    {" "}<strong>p</strong> = policy (subject, namespace, resource, action, effect) · <strong>g</strong> = role assignment · <Text code style={{fontSize: 11}}>*</Text> = wildcard.
    {" "}A default <Text code style={{fontSize: 11}}>allow *</Text> rule is seeded on first run. system:* users are never affected.
  </>
);

export default function AdmissionPolicyPage(props) {
  return (
    <CasbinRuleListPage
      {...props}
      scope="admission"
      title="Admission Policy"
      description={description}
    />
  );
}
