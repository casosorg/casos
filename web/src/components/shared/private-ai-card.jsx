import React, {useEffect, useState} from "react";
import {useHistory} from "react-router-dom";
import {useTranslation} from "react-i18next";
import {Braces, Cpu, ExternalLink, Loader2, Settings2, Sparkles, TriangleAlert, Zap} from "lucide-react";
import * as TemplateBackend from "@/backend/TemplateBackend";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Card, CardAction, CardContent, CardDescription, CardFooter, CardHeader, CardTitle} from "@/components/ui/card";
import {Progress} from "@/components/ui/progress";
import {SimpleSelect} from "@/components/shared/simple-select";
import {runAction, useResource} from "@/hooks/use-resource";
import {useUiMode} from "@/hooks/use-ui-mode";

export const PRIVATE_AI_TEMPLATE = "local-ai";

const STATUS_POLL_INTERVAL = 4000;
const FAILURE_REASONS = ["CrashLoopBackOff", "ErrImagePull", "ImagePullBackOff", "CreateContainerError", "RunContainerError"];

// progressOf reads the percentage out of the line ollama pull redraws.
function progressOf(line) {
  const match = /(\d{1,3})%/.exec(line ?? "");
  return match ? Math.min(100, Number(match[1])) : null;
}

/**
 * describeStatus turns the Pods of an installing instance into the one stage a
 * reader cares about: downloading the app, starting it, downloading the model,
 * or ready.
 */
function describeStatus(status, model, t) {
  if (!status) {
    return {stage: t("privateAi:Preparing"), percent: null};
  }
  if (status.ready) {
    return {stage: t("launchpad:Ready"), percent: 100, ready: true};
  }
  const services = status.pods.filter((pod) => !pod.job);
  const failing = services.find((pod) => FAILURE_REASONS.includes(pod.reason));
  if (failing) {
    return {stage: t("privateAi:Something went wrong: {{reason}}", {reason: failing.reason}), percent: null, failed: true};
  }
  if (services.length === 0 || services.some((pod) => pod.reason === "ContainerCreating" || pod.phase === "Pending")) {
    return {stage: t("privateAi:Downloading the app — a few GB, only the first time"), percent: null};
  }
  if (services.some((pod) => !pod.ready)) {
    return {stage: t("privateAi:Starting"), percent: null};
  }
  const job = status.pods.find((pod) => pod.job && !pod.ready);
  if (job) {
    return {
      stage: job.progress
        ? t("privateAi:Downloading the model {{model}}", {model})
        : t("privateAi:Preparing the model"),
      percent: progressOf(job.progress),
    };
  }
  return {stage: t("privateAi:Starting"), percent: null};
}

function GpuBadge({detail}) {
  const {t} = useTranslation();
  if (!detail) {
    return null;
  }
  if (!detail.gpu) {
    return (
      <Badge variant="muted">
        <Cpu />
        {t("privateAi:CPU only — slower")}
      </Badge>
    );
  }
  const memory = detail.gpuMemoryMiB > 0 ? ` · ${Math.round(detail.gpuMemoryMiB / 1024)} GB` : "";
  return (
    <Badge variant="success">
      <Zap />
      {detail.gpu.replace(/^NVIDIA /, "")}{memory}
    </Badge>
  );
}

/**
 * PrivateAiCard installs the built-in private AI template in one click, then
 * follows it until the chat is ready to open. Once installed it becomes the
 * way back in.
 */
export function PrivateAiCard() {
  const history = useHistory();
  const {t} = useTranslation();
  const {resolvePath} = useUiMode();
  const [model, setModel] = useState("");
  const [installing, setInstalling] = useState(false);

  const {data: detail} = useResource(() => TemplateBackend.getTemplate(PRIVATE_AI_TEMPLATE), [], {toastOnError: false});
  const {data: instances, refresh: refreshInstances} = useResource(() => TemplateBackend.getTemplateInstances(), [], {
    initialData: null,
    toastOnError: false,
  });
  const instance = (instances ?? []).find((item) => item.template === PRIVATE_AI_TEMPLATE) ?? null;

  const [status, setStatus] = useState(null);
  const ready = Boolean(status?.ready);
  const instanceNamespace = instance?.namespace;
  const instanceName = instance?.name;
  useEffect(() => {
    if (!instanceName || ready) {
      return undefined;
    }
    let cancelled = false;
    const load = () =>
      TemplateBackend.getTemplateInstanceStatus(instanceNamespace, instanceName)
        .then((res) => {
          if (!cancelled && res?.status === "ok") {
            setStatus(res.data);
          }
        })
        .catch(() => {});
    load();
    const timer = setInterval(load, STATUS_POLL_INTERVAL);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [instanceNamespace, instanceName, ready]);

  const modelInput = detail?.inputs?.find((input) => input.key === "model");
  const chosenModel = model || modelInput?.default || "";

  async function install() {
    setInstalling(true);
    try {
      await runAction(
        TemplateBackend.deployTemplate({
          name: PRIVATE_AI_TEMPLATE,
          namespace: "default",
          inputs: chosenModel ? {model: chosenModel} : {},
        }),
        {onSuccess: () => refreshInstances({silent: true})}
      );
    } finally {
      setInstalling(false);
    }
  }

  if (instances === null) {
    return null;
  }

  const appUrl = instance?.apps?.find((app) => app.url)?.url;
  const instanceModel = instance?.inputs?.model;
  const progress = instance ? describeStatus(status, instanceModel, t) : null;
  const manage = instance
    ? () => history.push(resolvePath(`/templates/instances/${instance.namespace}/${instance.name}`))
    : null;

  return (
    <Card data-testid="private-ai-card" className="from-primary/8 to-card bg-gradient-to-br">
      <CardHeader>
        <span className="bg-primary/12 text-primary mb-1 flex size-11 items-center justify-center rounded-xl">
          <Sparkles className="size-5.5" />
        </span>
        <CardTitle className="text-xl">{t("privateAi:Private AI")}</CardTitle>
        <CardDescription className="text-base">
          {instance
            ? t("privateAi:Your own ChatGPT, running on this machine. Chats and files never leave it.")
            : t("privateAi:Your own ChatGPT on this machine in one click: Ollama and Open WebUI, set up and connected, on your GPU when you have one.")}
        </CardDescription>
        <CardAction>
          <GpuBadge detail={detail} />
        </CardAction>
      </CardHeader>

      {instance && !progress.ready ? (
        <CardContent className="grid gap-2">
          <div className="flex items-center gap-2 text-sm font-medium">
            {progress.failed ? (
              <TriangleAlert className="text-destructive size-4" />
            ) : (
              <Loader2 className="text-muted-foreground size-4 animate-spin" />
            )}
            <span>{progress.stage}</span>
            {progress.percent !== null ? (
              <span className="text-muted-foreground ml-auto tabular-nums">{progress.percent}%</span>
            ) : null}
          </div>
          {progress.percent !== null ? <Progress value={progress.percent} className="h-2" /> : null}
        </CardContent>
      ) : null}

      {!instance && modelInput ? (
        <CardContent>
          <label className="grid max-w-sm gap-1.5 text-sm">
            <span className="text-muted-foreground">{t("privateAi:Model")}</span>
            <SimpleSelect
              value={chosenModel}
              onChange={setModel}
              options={(modelInput.options ?? []).map((option) => ({label: option, value: option}))}
              aria-label={t("privateAi:Model")}
            />
          </label>
        </CardContent>
      ) : null}

      <CardFooter className="flex-wrap gap-2">
        {instance ? (
          <>
            <Button disabled={!progress.ready || !appUrl} asChild={progress.ready && Boolean(appUrl)}>
              {progress.ready && appUrl ? (
                <a href={appUrl} target="_blank" rel="noreferrer">
                  {t("privateAi:Start chatting")}
                  <ExternalLink />
                </a>
              ) : (
                <span>{t("privateAi:Start chatting")}</span>
              )}
            </Button>
            <Button variant="outline" onClick={manage}>
              <Settings2 />
              {t("privateAi:Manage")}
            </Button>
            {progress.ready ? (
              <Button variant="ghost" onClick={() => history.push(resolvePath("/agent-access"))}>
                <Braces />
                {t("privateAi:Use from other apps")}
              </Button>
            ) : null}
          </>
        ) : (
          <>
            <Button onClick={install} disabled={installing || !detail}>
              {installing ? <Loader2 className="animate-spin" /> : <Sparkles />}
              {t("privateAi:Install Private AI")}
            </Button>
            <Button variant="ghost" onClick={() => history.push(resolvePath(`/templates/${PRIVATE_AI_TEMPLATE}`))}>
              {t("general:More options")}
            </Button>
          </>
        )}
      </CardFooter>
    </Card>
  );
}
