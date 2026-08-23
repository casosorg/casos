import {ArrowRight, Check, KeyRound, Laptop, Network, Rocket} from "lucide-react";
import {useTranslation} from "react-i18next";
import {Button} from "@/components/ui/button";
import {Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle} from "@/components/ui/card";
import {Badge} from "@/components/ui/badge";
import {Progress} from "@/components/ui/progress";
import {cn} from "@/lib/utils";
import {isFirstRunComplete} from "@/lib/firstRunChecklist";
import {useUiMode} from "@/hooks/use-ui-mode";

const STEP_ICONS = {password: KeyRound, machine: Laptop, node: Network, app: Rocket};

// steps comes from getFirstRunChecklist: the caller owns the requests behind it
// and stops making them once setup is done, so this renders what it is given.
export function FirstRunChecklist({steps, onAction}) {
  const {t} = useTranslation();
  const {advanced} = useUiMode();
  const simpleLabels = {
    password: {
      title: t("simple:Set your own password"),
      description: t("simple:Nobody else should be able to sign in with the password CasOS shipped with."),
      action: t("simple:Change password"),
    },
    machine: {
      title: t("simple:Add a computer"),
      description: t("simple:CasOS needs at least one computer it can reach over the network."),
      action: t("simple:Open Devices"),
    },
    node: {
      title: t("simple:Let that computer run apps"),
      description: t("simple:Until it joins the cluster there is nowhere for an app to run."),
      action: t("simple:Open Devices"),
    },
    app: {
      title: t("simple:Install your first app"),
      description: t("simple:Pick anything from the App Store — CasOS handles the setup."),
      action: t("simple:Open App Store"),
    },
  };
  const advancedLabels = {
    password: {
      title: t("onboarding:Change the default password"),
      description: t("onboarding:Use a unique password for the built-in admin account."),
      action: t("onboarding:Open account settings"),
    },
    machine: {
      title: t("onboarding:Add a machine"),
      description: t("onboarding:Add an SSH-accessible machine before deploying it as a node."),
      action: t("onboarding:Open Machines"),
    },
    node: {
      title: t("onboarding:Deploy a machine as a node"),
      description: t("onboarding:A node is required before CasOS can schedule workloads."),
      action: t("onboarding:Open Machines"),
    },
    app: {
      title: t("onboarding:Install your first app"),
      description: t("onboarding:Install an app from the App Store after a node is ready."),
      action: t("onboarding:Open App Store"),
    },
  };
  const labels = advanced ? advancedLabels : simpleLabels;

  // A finished checklist is not a dashboard widget — it stops being rendered.
  if (!Array.isArray(steps) || steps.length === 0 || isFirstRunComplete(steps)) {
    return null;
  }

  const completed = steps.filter((step) => step.done).length;

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("onboarding:First-run checklist")}</CardTitle>
        <CardDescription>{t("onboarding:Complete these steps to get a working CasOS cluster.")}</CardDescription>
        <CardAction>
          <Badge variant="outline" className="tabular-nums">
            {t("onboarding:{{completed}} of {{total}} complete", {completed, total: steps.length})}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="grid gap-4">
        <Progress value={(completed / steps.length) * 100} className="h-1.5" />
        <div className="grid">
          {steps.map((step) => {
            const Icon = step.done ? Check : STEP_ICONS[step.key];
            const label = labels[step.key];
            return (
              <div key={step.key} className="flex items-start gap-3 border-b py-3 first:pt-0 last:border-b-0 last:pb-0">
                <span
                  className={cn(
                    "flex size-7 shrink-0 items-center justify-center rounded-full border",
                    step.done ? "bg-primary text-primary-foreground border-transparent" : "bg-muted text-muted-foreground"
                  )}
                >
                  <Icon className="size-3.5" />
                </span>
                <div className="min-w-0 flex-1 space-y-1">
                  <p className={cn("text-sm leading-none font-medium", step.done && "text-muted-foreground line-through")}>
                    {label.title}
                  </p>
                  <p className="text-muted-foreground text-sm">{label.description}</p>
                </div>
                {!step.done && onAction ? (
                  <Button variant="outline" size="sm" className="shrink-0" onClick={() => onAction(step.key)}>
                    {label.action}
                    <ArrowRight />
                  </Button>
                ) : null}
              </div>
            );
          })}
        </div>
      </CardContent>
    </Card>
  );
}
