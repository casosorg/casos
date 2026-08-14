import React from "react";
import {Button, Card, Form, Input, Result, message} from "antd";
import {KeyOutlined, SafetyCertificateOutlined} from "@ant-design/icons";
import i18next from "i18next";
import * as AccountBackend from "./backend/AccountBackend";
import * as Setting from "./Setting";
import {getLocalPasswordErrorMessage} from "./localAuth";

function AccountPage({account}) {
  const [form] = Form.useForm();
  const [submitting, setSubmitting] = React.useState(false);

  if (!Setting.isLocalAccount(account)) {
    return <Result status="info" title={i18next.t("account:This account is managed by Casdoor")} />;
  }

  const updatePassword = (values) => {
    setSubmitting(true);
    AccountBackend.updateAccount(values.currentPassword, values.newPassword)
      .then((res) => {
        if (res.status === "ok") {
          form.resetFields();
          message.success(i18next.t("account:Password updated"));
        } else {
          message.error(res.msg);
        }
      })
      .catch((error) => message.error(error.message))
      .finally(() => setSubmitting(false));
  };

  return (
    <div style={{minHeight: "calc(100vh - 120px)", padding: "28px", background: "var(--ant-color-bg-layout)"}}>
      <div style={{maxWidth: 760, margin: "0 auto"}}>
        <div style={{display: "flex", alignItems: "center", gap: 14, marginBottom: 22}}>
          <div style={{display: "grid", placeItems: "center", width: 44, height: 44, border: "1px solid var(--ant-color-border)", borderRadius: 12}}><SafetyCertificateOutlined /></div>
          <div>
            <h1 style={{fontSize: 24, margin: 0}}>{i18next.t("account:Local account")}</h1>
            <div style={{color: "var(--ant-color-text-secondary)", marginTop: 3}}>{i18next.t("account:Administrator credentials for this CasOS instance")}</div>
          </div>
        </div>
        <Card title={<span><KeyOutlined style={{marginRight: 8}} />{i18next.t("account:Change password")}</span>}>
          <Form form={form} layout="vertical" requiredMark={false} onFinish={updatePassword} style={{maxWidth: 480}}>
            <Form.Item label={i18next.t("account:Account username")}>
              <Input value={account.name} disabled />
            </Form.Item>
            <Form.Item label={i18next.t("account:Current password")} name="currentPassword" rules={[{required: true, message: i18next.t("account:Please enter your current password")}]}>
              <Input.Password autoComplete="current-password" />
            </Form.Item>
            <Form.Item
              label={i18next.t("account:New password")}
              name="newPassword"
              rules={[
                {required: true, message: i18next.t("account:Please enter a new password")},
                () => ({
                  validator(_, value) {
                    const validationError = value ? getLocalPasswordErrorMessage(value) : "";
                    return validationError
                      ? Promise.reject(new Error(validationError))
                      : Promise.resolve();
                  },
                }),
              ]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
            <Form.Item
              label={i18next.t("account:Confirm new password")}
              name="confirmPassword"
              dependencies={["newPassword"]}
              rules={[
                {required: true, message: i18next.t("account:Please confirm the password")},
                ({getFieldValue}) => ({
                  validator(_, value) {
                    return !value || getFieldValue("newPassword") === value
                      ? Promise.resolve()
                      : Promise.reject(new Error(i18next.t("account:Passwords do not match")));
                  },
                }),
              ]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
            <Button type="primary" htmlType="submit" loading={submitting}>{i18next.t("account:Update password")}</Button>
          </Form>
        </Card>
      </div>
    </div>
  );
}

export default AccountPage;
