import React, {useEffect, useState} from "react";
import {Button, Form, Input, Result, message} from "antd";
import i18next from "i18next";
import {useTranslation} from "react-i18next";
import * as Setting from "./Setting";
import * as AccountBackend from "./backend/AccountBackend";
import LanguageSelect from "./LanguageSelect";
import {getLocalPasswordErrorMessage} from "./localAuth";
import "./SigninPage.less";

function SigninPage({options, error, onRetry, logo}) {
  useTranslation();
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (options?.authMode === "casdoor") {
      const signinUrl = Setting.getSigninUrl();
      if (signinUrl) {window.location.replace(signinUrl);}
    }
  }, [options]);

  const completeSignin = () => {
    const from = sessionStorage.getItem("from") || "/";
    sessionStorage.removeItem("from");
    window.location.href = from === "/signin" ? "/" : from;
  };

  const submit = (request) => {
    setSubmitting(true);
    request()
      .then((res) => {
        if (res.status === "ok") {
          completeSignin();
        } else {
          message.error(res.msg);
          if (setupRequired && res.msg === "local administrator is already initialized") {
            onRetry();
          }
        }
      })
      .catch((requestError) => message.error(requestError.message))
      .finally(() => setSubmitting(false));
  };

  if (!options) {
    return (
      <div className="auth-result">
        <Result
          status="warning"
          title={i18next.t("account:Unable to load sign-in options")}
          subTitle={error}
          extra={<Button onClick={onRetry}>{i18next.t("account:Retry")}</Button>}
        />
      </div>
    );
  }

  if (options.authMode === "casdoor") {
    return <div className="auth-result"><span>{i18next.t("account:Redirecting to Casdoor...")}</span></div>;
  }

  const setupRequired = options.setupRequired === true;
  const pageTitle = setupRequired ? i18next.t("account:Initialize CasOS") : i18next.t("account:Sign in to CasOS");
  const pageDescription = setupRequired
    ? i18next.t("account:Set the password for the built-in administrator. No external identity service is required.")
    : i18next.t("account:Use the local administrator password for this CasOS instance.");

  return (
    <main className="auth-shell">
      <div className="auth-toolbar"><LanguageSelect /></div>
      <div className="auth-card">
        <img className="auth-logo" src={logo} alt="CasOS" />
        <h2 className="auth-title">{pageTitle}</h2>
        <p className="auth-description">{pageDescription}</p>

        {setupRequired ? (
          <Form layout="vertical" requiredMark={false} onFinish={(values) => submit(() => AccountBackend.setup(values.setupToken || "", values.password))}>
            <Form.Item label={i18next.t("account:Administrator account")}>
              <Input value="admin" disabled />
            </Form.Item>
            {options.setupTokenRequired === true && (
              <Form.Item
                label={i18next.t("account:Setup token")}
                name="setupToken"
                rules={[{required: true, message: i18next.t("account:Please enter the setup token")}]}
              >
                <Input.Password autoComplete="one-time-code" />
              </Form.Item>
            )}
            <Form.Item
              label={i18next.t("account:Account password")}
              name="password"
              rules={[
                {required: true, message: i18next.t("account:Please enter a password")},
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
              <Input.Password autoComplete="new-password" autoFocus />
            </Form.Item>
            <Form.Item
              label={i18next.t("account:Confirm password")}
              name="confirmPassword"
              dependencies={["password"]}
              rules={[
                {required: true, message: i18next.t("account:Please confirm the password")},
                ({getFieldValue}) => ({
                  validator(_, value) {
                    return !value || getFieldValue("password") === value
                      ? Promise.resolve()
                      : Promise.reject(new Error(i18next.t("account:Passwords do not match")));
                  },
                }),
              ]}
            >
              <Input.Password autoComplete="new-password" />
            </Form.Item>
            <div className="auth-password-note">{i18next.t("account:Use at least 8 characters and no more than 72 bytes. The password is stored as a bcrypt hash.")}</div>
            <Button type="primary" htmlType="submit" loading={submitting} block className="auth-submit">
              {i18next.t("account:Set up and continue")}
            </Button>
          </Form>
        ) : (
          <Form layout="vertical" requiredMark={false} initialValues={{username: "admin"}} onFinish={(values) => submit(() => AccountBackend.signinWithPassword(values.username, values.password))}>
            <Form.Item label={i18next.t("account:Account username")} name="username">
              <Input disabled />
            </Form.Item>
            <Form.Item label={i18next.t("account:Account password")} name="password" rules={[{required: true, message: i18next.t("account:Please enter your password")}]}>
              <Input.Password autoComplete="current-password" autoFocus />
            </Form.Item>
            <Button type="primary" htmlType="submit" loading={submitting} block className="auth-submit">
              {i18next.t("account:Sign In")}
            </Button>
          </Form>
        )}

        <div className="auth-status"><i />{i18next.t("account:Local authentication")}</div>
      </div>
    </main>
  );
}

export default SigninPage;
