import React, {useEffect, useMemo, useState} from "react";
import i18next from "i18next";
import {useTranslation} from "react-i18next";
import {ArrowLeft, BookOpen, Code2, ExternalLink, Rocket} from "lucide-react";
import * as NamespaceBackend from "@/backend/NamespaceBackend";
import * as TemplateBackend from "@/backend/TemplateBackend";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "@/components/ui/card";
import {Input} from "@/components/ui/input";
import {Label} from "@/components/ui/label";
import {MessageAlert} from "@/components/ui/alert";
import {Loading} from "@/components/shared/loading";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {PasswordInput} from "@/components/shared/password-input";
import {SimpleSelect} from "@/components/shared/simple-select";
import {AppIcon} from "@/components/shared/app-icon";
import {cn} from "@/lib/utils";
import {runAction, useResource} from "@/hooks/use-resource";
import {useWorkspace} from "@/hooks/use-workspace";

function isSecretField(key) {
  return /pass|secret|token|key/i.test(key);
}

/**
 * The deploy form for one template.
 *
 * The fields are the template's own: it declares what it needs, and casos
 * renders that declaration. The YAML beside them is what the cluster will
 * actually be asked for, rendered by the backend from the same values — so the
 * preview cannot drift from what is deployed.
 */
function TemplateDeployPage(props) {
  useTranslation();
  const {history, match} = props;
  const templateName = match.params.name;

  const {workspace} = useWorkspace();
  const [namespace, setNamespace] = useState(workspace || "default");
  const [values, setValues] = useState({});
  const [domain, setDomain] = useState("");
  const [detail, setDetail] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [submitting, setSubmitting] = useState(false);
  const [showYaml, setShowYaml] = useState(false);
  const [preview, setPreview] = useState(null);

  const {data: namespaces} = useResource(() => NamespaceBackend.getNamespaces(), [], {initialData: [], toastOnError: false});

  useEffect(() => {
    setLoading(true);
    TemplateBackend.getTemplate(templateName, namespace)
      .then((res) => {
        if (res.status === "ok") {
          setDetail(res.data);
          setValues((current) => {
            const next = {...current};
            (res.data.inputs ?? []).forEach((input) => {
              if (next[input.key] === undefined) {
                next[input.key] = input.default ?? "";
              }
            });
            return next;
          });
          setError(null);
        } else {
          setError(res.msg);
        }
      })
      .catch((requestError) => setError(requestError.message))
      .finally(() => setLoading(false));
    // The namespace only decides which cluster the names are rendered for, so
    // re-reading on a change keeps the preview honest.
  }, [templateName, namespace]);

  const missing = useMemo(() => {
    return (detail?.inputs ?? [])
      .filter((input) => input.required && !String(values[input.key] ?? "").trim())
      .map((input) => input.key);
  }, [detail, values]);

  function loadPreview() {
    TemplateBackend.previewTemplate({name: templateName, namespace, domain, inputs: values})
      .then((res) => {
        if (res.status === "ok") {
          setPreview(res.data);
        }
      })
      .catch(() => {});
  }

  function toggleYaml() {
    const next = !showYaml;
    setShowYaml(next);
    if (next) {
      loadPreview();
    }
  }

  function deploy() {
    setSubmitting(true);
    runAction(
      TemplateBackend.deployTemplate({name: templateName, namespace, domain, inputs: values}),
      {
        successMessage: i18next.t("template:App deployed"),
        onSuccess: (res) => history.push(`/templates/instances/${res.data.namespace}/${res.data.name}`),
      }
    ).finally(() => setSubmitting(false));
  }

  if (loading) {
    return <Loading type="page" />;
  }

  if (!detail) {
    return (
      <PageContainer>
        <MessageAlert title={error ?? i18next.t("template:Template not found")} />
        <div>
          <Button variant="outline" onClick={() => history.push("/app-store/templates")}>
            <ArrowLeft />
            {i18next.t("launchpad:Back")}
          </Button>
        </div>
      </PageContainer>
    );
  }

  return (
    <PageContainer>
      <PageHeader
        title={
          <span className="flex items-center gap-2">
            <AppIcon src={detail.icon} name={detail.title} chartName={detail.name} size="md" />
            {detail.title}
          </span>
        }
        description={detail.description}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" onClick={() => history.push("/app-store/templates")}>
              <ArrowLeft />
              {i18next.t("launchpad:Back")}
            </Button>
            {detail.readme ? (
              <Button variant="outline" asChild>
                <a href={detail.readme} target="_blank" rel="noreferrer">
                  <BookOpen />
                  {i18next.t("template:Readme")}
                </a>
              </Button>
            ) : null}
            <Button variant={showYaml ? "default" : "outline"} onClick={toggleYaml}>
              <Code2 />
              {i18next.t("launchpad:YAML")}
            </Button>
            <Button onClick={deploy} disabled={submitting || missing.length > 0} data-testid="template-deploy">
              <Rocket />
              {i18next.t("template:Deploy")}
            </Button>
          </div>
        }
      />

      {error ? <MessageAlert title={error} /> : null}

      <div className={cn("grid gap-4", showYaml && "xl:grid-cols-[minmax(0,1fr)_minmax(0,28rem)]")}>
        <div className="grid gap-4">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">{i18next.t("template:Where it goes")}</CardTitle>
              <CardDescription>{i18next.t("template:The namespace it is deployed into, and the domain its addresses are built from.")}</CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4 sm:grid-cols-2">
              <div className="grid gap-1.5">
                <Label className="text-sm">{i18next.t("general:Namespace")}</Label>
                <SimpleSelect
                  value={namespace}
                  onChange={setNamespace}
                  options={namespaces.map((item) => ({label: item.name, value: item.name}))}
                  placeholder="default"
                />
              </div>
              <div className="grid gap-1.5">
                <Label className="text-sm">{i18next.t("template:Domain")}</Label>
                <Input
                  value={domain}
                  onChange={(event) => setDomain(event.target.value)}
                  placeholder={i18next.t("template:This cluster's address")}
                  className="font-mono text-sm"
                />
                <p className="text-muted-foreground text-xs">
                  {i18next.t("template:Templates publish themselves at a subdomain of this. It needs an ingress controller and DNS pointing here.")}
                </p>
              </div>
            </CardContent>
          </Card>

          {(detail.inputs ?? []).length > 0 ? (
            <Card>
              <CardHeader>
                <CardTitle className="text-base">{i18next.t("template:Settings")}</CardTitle>
                <CardDescription>{i18next.t("template:What this app asks for.")}</CardDescription>
              </CardHeader>
              <CardContent className="grid gap-4">
                {detail.inputs.map((input) => (
                  <div key={input.key} className="grid gap-1.5">
                    <Label className="flex items-center gap-1.5 text-sm">
                      {input.key}
                      {input.required ? <Badge variant="secondary">{i18next.t("template:required")}</Badge> : null}
                    </Label>
                    {input.type === "boolean" || (input.options ?? []).length > 0 ? (
                      <SimpleSelect
                        value={String(values[input.key] ?? input.default ?? "")}
                        onChange={(next) => setValues((current) => ({...current, [input.key]: next}))}
                        options={(input.options ?? ["true", "false"]).map((option) => ({label: option, value: option}))}
                        className="sm:max-w-60"
                      />
                    ) : isSecretField(input.key) ? (
                      <PasswordInput
                        value={values[input.key] ?? ""}
                        onChange={(event) => setValues((current) => ({...current, [input.key]: event.target.value}))}
                      />
                    ) : (
                      <Input
                        value={values[input.key] ?? ""}
                        onChange={(event) => setValues((current) => ({...current, [input.key]: event.target.value}))}
                        data-testid={`template-input-${input.key}`}
                      />
                    )}
                    {input.description ? <p className="text-muted-foreground text-xs">{input.description}</p> : null}
                  </div>
                ))}
              </CardContent>
            </Card>
          ) : null}

          <Card>
            <CardHeader>
              <CardTitle className="text-base">{i18next.t("template:Names it will use")}</CardTitle>
              <CardDescription>{i18next.t("template:Generated so two installs of the same app never collide.")}</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-wrap gap-1.5">
              {Object.entries(detail.defaults ?? {}).map(([key, value]) => (
                <Badge key={key} variant="secondary" className="font-mono">{key}={value}</Badge>
              ))}
              {detail.url ? (
                <Button size="sm" variant="ghost" asChild>
                  <a href={detail.url} target="_blank" rel="noreferrer">
                    <ExternalLink />
                    {i18next.t("template:Project site")}
                  </a>
                </Button>
              ) : null}
            </CardContent>
          </Card>
        </div>

        {showYaml ? (
          <Card className="xl:sticky xl:top-4">
            <CardHeader>
              <CardTitle className="text-base">{i18next.t("launchpad:Manifests")}</CardTitle>
              <CardDescription>{i18next.t("template:Rendered by the backend from the values above.")}</CardDescription>
              <div className="mt-2 flex items-center gap-2">
                <Button size="sm" variant="outline" onClick={loadPreview}>{i18next.t("general:Refresh")}</Button>
                {preview?.instance ? <Badge variant="secondary">{preview.instance}</Badge> : null}
              </div>
            </CardHeader>
            <CardContent>
              <pre className="bg-muted/50 max-h-[65vh] overflow-auto rounded-lg p-3 font-mono text-xs" data-testid="template-yaml">
                {preview?.yaml ?? i18next.t("template:Rendering…")}
              </pre>
            </CardContent>
          </Card>
        ) : null}
      </div>
    </PageContainer>
  );
}

export default TemplateDeployPage;
