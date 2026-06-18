import React from "react";
import {Typography} from "antd";
import CasbinRuleListPage from "./CasbinRuleListPage";

const {Text} = Typography;

const description = (
  <>
    <strong>Authorization Webhook</strong> — intercepts every API request (get / list / watch / create …) after Node and RBAC authorizers, before the request reaches the handler.
    {" "}<strong>p</strong> = policy (subject, namespace, resource, verb, effect) · <strong>g</strong> = role assignment · <Text code style={{fontSize: 11}}>*</Text> = wildcard.
    {" "}A default <Text code style={{fontSize: 11}}>allow *</Text> rule is seeded on first run. system:* users bypass this webhook entirely.
  </>
);

export default function AuthorizationPolicyPage(props) {
  return (
    <CasbinRuleListPage
      {...props}
      scope="authorization"
      title="Authorization Policy"
      description={description}
    />
  );
}
