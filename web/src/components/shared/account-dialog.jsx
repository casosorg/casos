import React, {useEffect, useState} from "react";
import i18next from "i18next";
import * as AccountBackend from "@/backend/AccountBackend";
import * as Setting from "@/Setting";
import {Avatar, AvatarFallback, AvatarImage} from "@/components/ui/avatar";
import {Input} from "@/components/ui/input";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PasswordInput} from "@/components/shared/password-input";

const emptyForm = {displayName: "", avatar: "", currentPassword: "", newPassword: "", confirmPassword: ""};

/**
 * Edits the built-in account: its display name always, and its password when
 * both new-password fields are filled. Only reachable while CasOS runs without
 * Casdoor, since a Casdoor account is edited in Casdoor.
 */
export function AccountDialog({account, open, onOpenChange, onUpdateAccount}) {
  const [values, setValues] = useState(emptyForm);
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (open) {
      setValues({...emptyForm, displayName: account?.displayName || "", avatar: account?.avatar || ""});
      setErrors({});
    }
  }, [open, account]);

  function setField(field, value) {
    setValues((prev) => ({...prev, [field]: value}));
  }

  function validate() {
    const nextErrors = {};
    if (values.newPassword && !values.currentPassword) {
      nextErrors.currentPassword = i18next.t("account:Please input your password");
    }
    if (values.newPassword !== (values.confirmPassword || "")) {
      nextErrors.confirmPassword = i18next.t("account:Passwords do not match");
    }
    setErrors(nextErrors);
    return Object.keys(nextErrors).length === 0;
  }

  async function handleSubmit() {
    if (!validate()) {
      return;
    }
    setSubmitting(true);
    try {
      const res = await AccountBackend.updateAccount({
        displayName: values.displayName,
        avatar: values.avatar,
        currentPassword: values.currentPassword || "",
        newPassword: values.newPassword || "",
      });
      if (res.status !== "ok") {
        Setting.showMessage("error", res.msg);
        return;
      }
      Setting.showMessage("success", i18next.t("general:Successfully saved"));
      onUpdateAccount?.();
      onOpenChange(false);
    } catch (error) {
      Setting.showMessage("error", error.message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={i18next.t("account:My Account")}
      submitText={i18next.t("general:Save")}
      cancelText={i18next.t("general:Cancel")}
      submitting={submitting}
      onSubmit={handleSubmit}
    >
      <Field label={i18next.t("general:Display name")} htmlFor="account-display-name">
        <Input
          id="account-display-name"
          value={values.displayName}
          onChange={(event) => setField("displayName", event.target.value)}
          autoComplete="off"
        />
      </Field>

      <Field label={i18next.t("account:Avatar")} htmlFor="account-avatar">
        <div className="flex items-center gap-3">
          <Avatar className="size-10 shrink-0">
            {values.avatar ? <AvatarImage src={values.avatar} alt="" /> : null}
            <AvatarFallback style={{backgroundColor: Setting.getAvatarColor(account?.name), color: "#fff"}}>
              {Setting.getShortName(account?.name)}
            </AvatarFallback>
          </Avatar>
          <Input
            id="account-avatar"
            value={values.avatar}
            onChange={(event) => setField("avatar", event.target.value)}
            placeholder="https://example.com/me.png"
            autoComplete="off"
          />
        </div>
      </Field>

      <Field label={i18next.t("account:Old Password")} htmlFor="account-current-password" error={errors.currentPassword}>
        <PasswordInput
          id="account-current-password"
          value={values.currentPassword}
          onChange={(event) => setField("currentPassword", event.target.value)}
          placeholder={i18next.t("account:Enter current password")}
          autoComplete="current-password"
        />
      </Field>

      <Field label={i18next.t("account:New password")} htmlFor="account-new-password">
        <PasswordInput
          id="account-new-password"
          value={values.newPassword}
          onChange={(event) => setField("newPassword", event.target.value)}
          placeholder={i18next.t("account:Enter new password")}
          autoComplete="new-password"
        />
      </Field>

      <Field label={i18next.t("account:Confirm password")} htmlFor="account-confirm-password" error={errors.confirmPassword}>
        <PasswordInput
          id="account-confirm-password"
          value={values.confirmPassword}
          onChange={(event) => setField("confirmPassword", event.target.value)}
          placeholder={i18next.t("account:Enter new password")}
          autoComplete="new-password"
        />
      </Field>
    </FormDialog>
  );
}
