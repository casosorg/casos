import React, {useEffect, useState} from "react";
import {useHistory, useParams} from "react-router-dom";
import i18next from "i18next";
import * as MachineBackend from "@/backend/MachineBackend";
import * as Setting from "@/Setting";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "@/components/ui/card";
import {Input} from "@/components/ui/input";
import {Textarea} from "@/components/ui/textarea";
import {Field} from "@/components/shared/form-dialog";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {NumberInput} from "@/components/shared/number-input";
import {PasswordInput} from "@/components/shared/password-input";
import {SimpleSelect} from "@/components/shared/simple-select";
import {Loading} from "@/components/shared/loading";
import {LabelWithTip} from "@/components/shared/misc";
import {MACHINE_STATUS_VARIANTS} from "@/pages/MachineListPage";

function Section({title, description, children}) {
  return (
    <Card className="gap-4 py-5">
      <CardHeader className="px-5">
        <CardTitle className="text-[15px]">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="px-5">
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">{children}</div>
      </CardContent>
    </Card>
  );
}

function MachineEditPage() {
  const {machineName} = useParams();
  const history = useHistory();
  const [machine, setMachine] = useState(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    MachineBackend.getMachine("admin", machineName).then((res) => {
      if (res.status === "ok") {
        setMachine(res.data);
      } else {
        Setting.showMessage("error", `${i18next.t("general:Failed to get")}: ${res.msg}`);
      }
    });
  }, [machineName]);

  function updateField(key, value) {
    setMachine((previous) => ({...previous, [key]: value}));
  }

  function handleSave() {
    setSaving(true);
    MachineBackend.updateMachine(machine.owner, machineName, machine)
      .then((res) => {
        if (res.status !== "ok") {
          Setting.showMessage("error", `${i18next.t("general:Failed to save")}: ${res.msg}`);
          return;
        }
        Setting.showMessage("success", i18next.t("general:Successfully saved"));
        history.push(`/machines/${machine.name}`);
      })
      .catch((error) => Setting.showMessage("error", `${i18next.t("general:Failed to save")}: ${error}`))
      .finally(() => setSaving(false));
  }

  if (machine === null) {
    return <Loading type="page" tip={i18next.t("general:Loading...")} />;
  }

  return (
    <PageContainer>
      <PageHeader
        title={
          <span className="flex items-center gap-3">
            {i18next.t("machine:Edit Machine")}
            <Badge variant={MACHINE_STATUS_VARIANTS[machine.status] ?? "muted"}>{machine.status || "Unknown"}</Badge>
          </span>
        }
        actions={
          <Button onClick={handleSave} loading={saving}>
            {i18next.t("general:Save")}
          </Button>
        }
      />

      <Section title={i18next.t("general:General Settings")} description={i18next.t("machine:General Settings desc")}>
        <Field label={<LabelWithTip text={i18next.t("general:Name")} tooltip={i18next.t("general:Name - Tooltip")} />} htmlFor="machine-name">
          {/* The name is the record's key on the server, so it is shown but not editable. */}
          <Input id="machine-name" value={machine.name ?? ""} disabled />
        </Field>

        <Field
          label={<LabelWithTip text={i18next.t("general:Display name")} tooltip={i18next.t("general:Display name - Tooltip")} />}
          htmlFor="machine-display-name"
        >
          <Input
            id="machine-display-name"
            value={machine.displayName ?? ""}
            onChange={(event) => updateField("displayName", event.target.value)}
          />
        </Field>

        <Field label={<LabelWithTip text={i18next.t("policy:Role")} tooltip={i18next.t("machine:Role - Tooltip")} />}>
          <SimpleSelect
            value={machine.role ?? ""}
            onChange={(next) => updateField("role", next || "")}
            options={[
              {label: "master", value: "master"},
              {label: "worker", value: "worker"},
            ]}
            placeholder={i18next.t("policy:Role")}
          />
        </Field>

        <Field
          label={<LabelWithTip text={i18next.t("machine:Operating system")} tooltip={i18next.t("machine:Operating system - Tooltip")} />}
          htmlFor="machine-os"
        >
          <Input id="machine-os" value={machine.os ?? ""} onChange={(event) => updateField("os", event.target.value)} />
        </Field>

        <Field label={i18next.t("general:Description")} htmlFor="machine-description" className="md:col-span-2 lg:col-span-4">
          <Input
            id="machine-description"
            value={machine.description ?? ""}
            onChange={(event) => updateField("description", event.target.value)}
          />
        </Field>
      </Section>

      <Section title={i18next.t("machine:SSH Credentials")} description={i18next.t("machine:SSH Credentials desc")}>
        <Field
          label={<LabelWithTip text={i18next.t("machine:IP address")} tooltip={i18next.t("machine:IP address - Tooltip")} />}
          htmlFor="machine-ip"
        >
          <Input
            id="machine-ip"
            value={machine.ip ?? ""}
            onChange={(event) => updateField("ip", event.target.value)}
            className="font-mono text-xs"
          />
        </Field>

        <Field label={<LabelWithTip text={i18next.t("machine:SSH port")} tooltip={i18next.t("machine:SSH port - Tooltip")} />}>
          <NumberInput value={machine.port} onChange={(next) => updateField("port", next)} min={1} max={65535} />
        </Field>

        <Field
          label={<LabelWithTip text={i18next.t("general:Username")} tooltip={i18next.t("machine:Username - Tooltip")} />}
          htmlFor="machine-username"
        >
          <Input
            id="machine-username"
            value={machine.username ?? ""}
            onChange={(event) => updateField("username", event.target.value)}
          />
        </Field>

        <Field label={<LabelWithTip text={i18next.t("machine:Auth type")} tooltip={i18next.t("machine:Auth type - Tooltip")} />}>
          <SimpleSelect
            value={machine.authType || "password"}
            onChange={(next) => updateField("authType", next)}
            options={[
              {label: i18next.t("general:Password"), value: "password"},
              {label: i18next.t("machine:Private key"), value: "privateKey"},
            ]}
          />
        </Field>

        {machine.authType === "privateKey" ? (
          <Field
            label={<LabelWithTip text={i18next.t("machine:Private key")} tooltip={i18next.t("machine:Private key - Tooltip")} />}
            htmlFor="machine-key"
            className="md:col-span-2 lg:col-span-4"
          >
            <Textarea
              id="machine-key"
              rows={5}
              value={machine.privateKey ?? ""}
              onChange={(event) => updateField("privateKey", event.target.value)}
              className="font-mono text-xs"
            />
          </Field>
        ) : (
          <Field
            label={<LabelWithTip text={i18next.t("general:Password")} tooltip={i18next.t("machine:Password - Tooltip")} />}
            htmlFor="machine-password"
            className="md:col-span-2"
          >
            <PasswordInput
              id="machine-password"
              value={machine.password ?? ""}
              onChange={(event) => updateField("password", event.target.value)}
            />
          </Field>
        )}
      </Section>
    </PageContainer>
  );
}

export default MachineEditPage;
