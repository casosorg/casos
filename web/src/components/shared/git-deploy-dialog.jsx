import React, {useEffect, useState} from "react";
import i18next from "i18next";
import * as GitBuildBackend from "@/backend/GitBuildBackend";
import {Input} from "@/components/ui/input";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {SimpleSelect} from "@/components/shared/simple-select";
import {runAction} from "@/hooks/use-resource";
import {nameFromRepo} from "@/lib/git";

const emptyForm = (namespace) => ({repo: "", token: "", branch: "", path: "", name: "", nameTouched: false, port: "", namespace});

/**
 * Asks for a Git repository and starts its first build; casos deploys
 * the app once the build has pushed its image.
 */
export function GitDeployDialog({open, onOpenChange, namespaces, defaultNamespace = "default", onStarted}) {
  const [form, setForm] = useState(() => emptyForm(defaultNamespace));
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (open) {
      setForm(emptyForm(defaultNamespace));
    }
  }, [open, defaultNamespace]);

  function setField(key, value) {
    setForm((prev) => ({...prev, [key]: value}));
  }

  function setRepo(repo) {
    setForm((prev) => ({...prev, repo, name: prev.nameTouched ? prev.name : nameFromRepo(repo)}));
  }

  async function submit() {
    setSubmitting(true);
    try {
      await runAction(
        GitBuildBackend.deployGitApp({
          namespace: form.namespace,
          name: form.name.trim(),
          repo: form.repo.trim(),
          token: form.token.trim(),
          branch: form.branch.trim(),
          path: form.path.trim(),
          port: Number(form.port) || 0,
        }),
        {
          successMessage: i18next.t("launchpad:Build started"),
          onSuccess: (res) => {
            onOpenChange(false);
            onStarted?.({namespace: res.data.namespace, name: res.data.app});
          },
        }
      );
    } finally {
      setSubmitting(false);
    }
  }

  const namespaceOptions = (namespaces?.length ? namespaces.map((item) => item.name) : [form.namespace])
    .map((name) => ({label: name, value: name}));

  return (
    <FormDialog
      open={open}
      onOpenChange={onOpenChange}
      title={i18next.t("launchpad:Deploy from Git")}
      description={i18next.t("launchpad:casos clones the repository, builds it and deploys it. A Dockerfile is used when there is one; otherwise Node.js, Python, Go and static sites are recognised.")}
      onSubmit={submit}
      submitText={i18next.t("launchpad:Build and deploy")}
      cancelText={i18next.t("general:Cancel")}
      submitting={submitting}
      submitDisabled={!form.repo.trim() || !form.name.trim()}
    >
      <Field label={i18next.t("launchpad:Repository")} htmlFor="git-repo" required>
        <Input
          id="git-repo"
          value={form.repo}
          onChange={(e) => setRepo(e.target.value)}
          placeholder="https://github.com/owner/project.git"
          autoFocus
          data-testid="git-repo"
        />
      </Field>
      <Field label={i18next.t("launchpad:Access token")} htmlFor="git-token" hint={i18next.t("launchpad:A GitHub, GitLab or Gitea token that can read a private repository. Rebuilds reuse it.")}>
        <Input
          id="git-token"
          type="password"
          autoComplete="new-password"
          value={form.token}
          onChange={(e) => setField("token", e.target.value)}
          placeholder={i18next.t("launchpad:Not needed for a public repository")}
          data-testid="git-token"
        />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label={i18next.t("launchpad:Branch")} htmlFor="git-branch">
          <Input id="git-branch" value={form.branch} onChange={(e) => setField("branch", e.target.value)} placeholder={i18next.t("devbox:Default branch")} />
        </Field>
        <Field label={i18next.t("launchpad:Folder")} htmlFor="git-path" hint={i18next.t("launchpad:For a monorepo.")}>
          <Input id="git-path" value={form.path} onChange={(e) => setField("path", e.target.value)} placeholder="apps/web" />
        </Field>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label={i18next.t("general:Name")} htmlFor="git-name" required>
          <Input
            id="git-name"
            value={form.name}
            onChange={(e) => setForm((prev) => ({...prev, name: e.target.value, nameTouched: true}))}
            data-testid="git-name"
          />
        </Field>
        <Field label={i18next.t("general:Namespace")} htmlFor="git-namespace">
          <SimpleSelect id="git-namespace" value={form.namespace} onChange={(value) => setField("namespace", value)} options={namespaceOptions} />
        </Field>
      </div>
      <Field label={i18next.t("launchpad:Port")} htmlFor="git-port" hint={i18next.t("launchpad:Leave blank to use the Dockerfile's EXPOSE, or the usual port for the stack.")}>
        <Input id="git-port" type="number" min={1} max={65535} value={form.port} onChange={(e) => setField("port", e.target.value)} placeholder="3000" />
      </Field>
    </FormDialog>
  );
}

export default GitDeployDialog;
