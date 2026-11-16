import React, {useState} from "react";
import {useTranslation} from "react-i18next";
import {Pencil, Plus, RefreshCw, Trash2} from "lucide-react";
import * as AiModelBackend from "@/backend/AiModelBackend";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {Textarea} from "@/components/ui/textarea";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {DataTable} from "@/components/shared/data-table";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {CodeText} from "@/components/shared/misc";
import {runAction, useResource} from "@/hooks/use-resource";

export function AiProvidersCard({onChange}) {
  const {t} = useTranslation();
  const {data: providers, loading, refresh} = useResource(() => AiModelBackend.getAiProviders(), [], {initialData: []});
  const [form, setForm] = useState(null);
  const [submitting, setSubmitting] = useState(false);

  function openForm(provider = {}) {
    setForm({...provider, name: provider.name ?? "", baseUrl: provider.baseUrl ?? "", apiKey: "", modelText: (provider.models ?? []).join("\n")});
  }

  function update(field, value) {
    setForm((previous) => ({...previous, [field]: value}));
  }

  function changed() {
    refresh();
    onChange();
  }

  async function save() {
    setSubmitting(true);
    const models = form.modelText.split("\n").map((model) => model.trim()).filter(Boolean);
    const ok = await runAction(AiModelBackend.saveAiProvider({
      name: form.name.trim(), baseUrl: form.baseUrl.trim(), apiKey: form.apiKey,
      models, resourceVersion: form.resourceVersion,
    }));
    setSubmitting(false);
    if (ok) {
      setForm(null);
      changed();
    }
  }

  async function remove(provider) {
    if (await runAction(AiModelBackend.deleteAiProvider(provider.name))) {
      changed();
    }
  }

  const columns = [
    {key: "name", title: t("general:Name"), dataIndex: "name"},
    {key: "baseUrl", title: t("agent:Base URL"), dataIndex: "baseUrl"},
    {
      key: "models", title: t("agent:Models"),
      render: (_, provider) => <div className="flex flex-col gap-1">{provider.models.map((model) => (
        <CodeText key={model} copyable>{`${provider.name}/${model}`}</CodeText>
      ))}</div>,
    },
    {
      key: "actions", title: t("general:Action"), align: "right",
      render: (_, provider) => (
        <div className="flex justify-end gap-2">
          <Button variant="outline" size="sm" onClick={() => openForm(provider)} aria-label={t("general:Edit")}><Pencil /></Button>
          <ConfirmDialog title={t("agent:Remove this AI provider?")} onConfirm={() => remove(provider)} confirmText={t("general:Delete")}>
            <Button variant="outline" size="sm" aria-label={t("general:Delete")}><Trash2 /></Button>
          </ConfirmDialog>
        </div>
      ),
    },
  ];

  return (
    <>
      <DataTable
        title={t("agent:AI providers")}
        description={t("agent:Connect an OpenAI-compatible service such as Sub2API. Your access tokens use the models listed here.")}
        columns={columns}
        dataSource={providers}
        rowKey={(provider) => provider.name}
        loading={loading}
        emptyText={t("agent:No AI providers yet")}
        toolbar={
          <>
            <Button variant="outline" size="sm" onClick={() => refresh()} loading={loading}><RefreshCw />{t("general:Refresh")}</Button>
            <Button size="sm" onClick={() => openForm()}><Plus />{t("agent:Add AI provider")}</Button>
          </>
        }
      />
      <FormDialog
        open={Boolean(form)}
        onOpenChange={(open) => !open && setForm(null)}
        title={t(form?.resourceVersion ? "agent:Edit AI provider" : "agent:Add AI provider")}
        onSubmit={save}
        submitting={submitting}
        submitText={t("general:Save")}
        cancelText={t("general:Cancel")}
        submitDisabled={!form?.name.trim() || !form?.baseUrl.trim() || !form?.modelText.trim()}
      >
        {form ? (
          <>
            <Field label={t("general:Name")} htmlFor="ai-provider-name" required hint={t("agent:Lowercase letters, numbers and hyphens. Used as the model prefix.")}>
              <Input id="ai-provider-name" value={form.name} disabled={Boolean(form.resourceVersion)} onChange={(event) => update("name", event.target.value)} placeholder="sub2api" autoComplete="off" />
            </Field>
            <Field label={t("agent:Base URL")} htmlFor="ai-provider-url" required hint={t("agent:Include the API path, for example https://gateway.example.com/v1.")}>
              <Input id="ai-provider-url" value={form.baseUrl} onChange={(event) => update("baseUrl", event.target.value)} placeholder="https://gateway.example.com/v1" autoComplete="off" />
            </Field>
            <Field label={t("agent:Upstream API key")} htmlFor="ai-provider-key" hint={t(form.hasApiKey ? "agent:Leave blank to keep the saved key." : "agent:Optional for services that do not require a key.")}>
              <Input id="ai-provider-key" type="password" value={form.apiKey} onChange={(event) => update("apiKey", event.target.value)} autoComplete="new-password" />
            </Field>
            <Field label={t("agent:Models")} htmlFor="ai-provider-models" required hint={t("agent:Enter upstream model IDs, one per line. Clients use provider-name/model-id.")}>
              <Textarea id="ai-provider-models" value={form.modelText} onChange={(event) => update("modelText", event.target.value)} rows={4} />
            </Field>
          </>
        ) : null}
      </FormDialog>
    </>
  );
}
