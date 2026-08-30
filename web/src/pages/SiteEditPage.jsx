import React, {useEffect, useState} from "react";
import {useHistory, useParams} from "react-router-dom";
import i18next from "i18next";
import {Link2} from "lucide-react";
import * as SiteBackend from "@/backend/SiteBackend";
import * as Setting from "@/Setting";
import {Button} from "@/components/ui/button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "@/components/ui/card";
import {Input} from "@/components/ui/input";
import {Textarea} from "@/components/ui/textarea";
import {Field} from "@/components/shared/form-dialog";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {PasswordInput} from "@/components/shared/password-input";
import {Loading} from "@/components/shared/loading";
import {LabelWithTip} from "@/components/shared/misc";

const BUILT_IN_SITE = "site-built-in";

function Section({title, description, children}) {
  return (
    <Card className="gap-4 py-5">
      <CardHeader className="px-5">
        <CardTitle className="text-[15px]">{title}</CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      <CardContent className="px-5">
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">{children}</div>
      </CardContent>
    </Card>
  );
}

/** An input whose value is also shown as an image preview when it resolves. */
function UrlWithPreview({value, onChange, id}) {
  return (
    <div className="grid gap-2">
      <div className="relative">
        <Link2 className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
        <Input id={id} value={value ?? ""} onChange={(event) => onChange(event.target.value)} className="pl-9" />
      </div>
      {value ? (
        <img src={value} alt={value} className="bg-muted h-20 w-fit max-w-full rounded-md border object-contain p-1" />
      ) : null}
    </div>
  );
}

function SiteEditPage({onUpdateSite}) {
  const {siteName} = useParams();
  const history = useHistory();
  const [site, setSite] = useState(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    SiteBackend.getSite("admin", siteName).then((res) => {
      if (res.status === "ok") {
        setSite(res.data);
      } else {
        Setting.showMessage("error", `${i18next.t("general:Failed to get")}: ${res.msg}`);
      }
    });
  }, [siteName]);

  function updateField(key, value) {
    setSite((previous) => ({...previous, [key]: value}));
  }

  function handleSave() {
    setSaving(true);
    SiteBackend.updateSite(site.owner, siteName, site)
      .then((res) => {
        if (res.status !== "ok") {
          Setting.showMessage("error", `${i18next.t("general:Failed to save")}: ${res.msg}`);
          return;
        }
        Setting.showMessage("success", i18next.t("general:Successfully saved"));
        Setting.setThemeColor(site.themeColor);
        onUpdateSite?.();
        // The name is editable, so the URL has to follow it or a reload 404s.
        history.push(`/sites/${site.name}`);
      })
      .catch((error) => Setting.showMessage("error", `${i18next.t("general:Failed to save")}: ${error}`))
      .finally(() => setSaving(false));
  }

  if (site === null) {
    return <Loading type="page" tip={i18next.t("general:Loading...")} />;
  }

  return (
    <PageContainer>
      <PageHeader
        title={i18next.t("site:Edit Site")}
        actions={
          <Button onClick={handleSave} loading={saving}>
            {i18next.t("general:Save")}
          </Button>
        }
      />

      <Section title={i18next.t("general:General Settings")} description={i18next.t("general:General Settings desc")}>
        <Field label={<LabelWithTip text={i18next.t("general:Name")} tooltip={i18next.t("general:Name - Tooltip")} />} htmlFor="site-name">
          <Input
            id="site-name"
            value={site.name ?? ""}
            disabled={site.name === BUILT_IN_SITE}
            onChange={(event) => updateField("name", event.target.value)}
          />
        </Field>

        <Field
          label={<LabelWithTip text={i18next.t("general:Display name")} tooltip={i18next.t("general:Display name - Tooltip")} />}
          htmlFor="site-display-name"
        >
          <Input
            id="site-display-name"
            value={site.displayName ?? ""}
            onChange={(event) => updateField("displayName", event.target.value)}
          />
        </Field>

        <Field
          label={<LabelWithTip text={i18next.t("general:HTML title")} tooltip={i18next.t("general:HTML title - Tooltip")} />}
          htmlFor="site-html-title"
        >
          <Input
            id="site-html-title"
            value={site.htmlTitle ?? ""}
            onChange={(event) => updateField("htmlTitle", event.target.value)}
          />
        </Field>

        <Field label={<LabelWithTip text={i18next.t("site:Theme color")} tooltip={i18next.t("site:Theme color - Tooltip")} />}>
          <div className="flex items-center gap-2">
            <input
              type="color"
              value={site.themeColor || "#404040"}
              onChange={(event) => updateField("themeColor", event.target.value)}
              className="h-9 w-16 cursor-pointer rounded-md border p-1"
            />
            <Input
              value={site.themeColor ?? ""}
              onChange={(event) => updateField("themeColor", event.target.value)}
              className="w-32 font-mono text-xs"
            />
          </div>
        </Field>
      </Section>

      <Section title={i18next.t("general:Branding")} description={i18next.t("general:Branding desc")}>
        <Field label={<LabelWithTip text={i18next.t("general:Favicon URL")} tooltip={i18next.t("general:Favicon URL - Tooltip")} />}>
          <UrlWithPreview id="site-favicon" value={site.faviconUrl} onChange={(value) => updateField("faviconUrl", value)} />
        </Field>

        <Field label={<LabelWithTip text={i18next.t("general:Logo URL")} tooltip={i18next.t("general:Logo URL - Tooltip")} />}>
          <UrlWithPreview id="site-logo" value={site.logoUrl} onChange={(value) => updateField("logoUrl", value)} />
        </Field>

        <Field
          label={<LabelWithTip text={i18next.t("general:Static base URL")} tooltip={i18next.t("general:Static base URL - Tooltip")} />}
          htmlFor="site-static-base"
        >
          <div className="relative">
            <Link2 className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
            <Input
              id="site-static-base"
              value={site.staticBaseUrl ?? ""}
              onChange={(event) => updateField("staticBaseUrl", event.target.value)}
              className="pl-9"
            />
          </div>
        </Field>
      </Section>

      <Section title={i18next.t("general:Content")} description={i18next.t("general:Content desc")}>
        <Field
          label={<LabelWithTip text={i18next.t("general:Navbar HTML")} tooltip={i18next.t("general:Navbar HTML - Tooltip")} />}
          htmlFor="site-navbar"
          className="lg:col-span-2"
        >
          <Textarea
            id="site-navbar"
            rows={3}
            value={site.navbarHtml ?? ""}
            onChange={(event) => updateField("navbarHtml", event.target.value)}
            className="font-mono text-xs"
          />
        </Field>

        <Field
          label={<LabelWithTip text={i18next.t("general:Footer HTML")} tooltip={i18next.t("general:Footer HTML - Tooltip")} />}
          htmlFor="site-footer"
          className="lg:col-span-2"
        >
          <Textarea
            id="site-footer"
            rows={3}
            value={site.footerHtml ?? ""}
            onChange={(event) => updateField("footerHtml", event.target.value)}
            className="font-mono text-xs"
          />
        </Field>
      </Section>

      <Section title={i18next.t("site:Authentication")} description={i18next.t("site:Authentication desc")}>
        <Field
          label={<LabelWithTip text={i18next.t("site:OIDC issuer")} tooltip={i18next.t("site:OIDC issuer - Tooltip")} />}
          htmlFor="site-issuer"
          className="lg:col-span-2"
        >
          <div className="relative">
            <Link2 className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2" />
            <Input
              id="site-issuer"
              value={site.issuer ?? ""}
              onChange={(event) => updateField("issuer", event.target.value)}
              className="pl-9"
            />
          </div>
        </Field>

        <Field
          label={<LabelWithTip text={i18next.t("provider:Client ID")} tooltip={i18next.t("provider:Client ID - Tooltip")} />}
          htmlFor="site-client-id"
        >
          <Input
            id="site-client-id"
            value={site.clientId ?? ""}
            onChange={(event) => updateField("clientId", event.target.value)}
          />
        </Field>

        <Field
          label={<LabelWithTip text={i18next.t("provider:Client secret")} tooltip={i18next.t("provider:Client secret - Tooltip")} />}
          htmlFor="site-client-secret"
        >
          <PasswordInput
            id="site-client-secret"
            value={site.clientSecret ?? ""}
            onChange={(event) => updateField("clientSecret", event.target.value)}
          />
        </Field>
      </Section>

      <Section title={i18next.t("site:Advanced")} description={i18next.t("site:Advanced desc")}>
        <Field
          label={<LabelWithTip text={i18next.t("site:Socks5 proxy")} tooltip={i18next.t("site:Socks5 proxy - Tooltip")} />}
          htmlFor="site-socks5"
        >
          <Input
            id="site-socks5"
            value={site.socks5Proxy ?? ""}
            onChange={(event) => updateField("socks5Proxy", event.target.value)}
          />
        </Field>

        <Field
          label={<LabelWithTip text={i18next.t("site:Log config")} tooltip={i18next.t("site:Log config - Tooltip")} />}
          htmlFor="site-log-config"
          className="lg:col-span-3"
        >
          <Input
            id="site-log-config"
            value={site.logConfig ?? ""}
            onChange={(event) => updateField("logConfig", event.target.value)}
            className="font-mono text-xs"
          />
        </Field>
      </Section>
    </PageContainer>
  );
}

export default SiteEditPage;
