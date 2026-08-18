import {ArrowRight, KeyRound, Laptop, Network, Rocket} from "lucide-react";
import {useTranslation} from "react-i18next";
import {Button} from "@/components/ui/button";
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from "@/components/ui/card";
import {Checkbox} from "@/components/ui/checkbox";
import {getFirstRunChecklist} from "@/FirstRunChecklistState";

const STEP_ICONS = {password: KeyRound, machine: Laptop, node: Network, app: Rocket};

export function FirstRunChecklist({account, signinOptions, machines, stats, onAction}) {
  const {t} = useTranslation();
  const steps = getFirstRunChecklist({account, signinOptions, machines, stats});
  const labels = {
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
  const completed = steps.filter((step) => step.done).length;

  return (
    <Card>
      <CardHeader className="gap-1">
        <div className="flex items-center justify-between gap-3">
          <CardTitle className="text-base">{t("onboarding:First-run checklist")}</CardTitle>
          <span className="text-muted-foreground text-xs tabular-nums">
            {t("onboarding:{{completed}} of 4 complete", {completed})}
          </span>
        </div>
        <CardDescription>{t("onboarding:Complete these steps to get a working CasOS cluster.")}</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-0">
        {steps.map((step) => {
          const Icon = STEP_ICONS[step.key];
          const label = labels[step.key];
          return (
            <div key={step.key} className="flex items-start gap-3 border-b py-3 last:border-b-0">
              <Checkbox checked={step.done} disabled aria-label={label.title} className="mt-0.5" />
              <Icon className={step.done ? "text-success mt-0.5 size-4 shrink-0" : "text-info mt-0.5 size-4 shrink-0"} />
              <div className="min-w-0 flex-1">
                <p className={step.done ? "text-muted-foreground text-sm line-through" : "text-sm font-medium"}>{label.title}</p>
                <p className="text-muted-foreground mt-1 text-xs">{label.description}</p>
              </div>
              {!step.done && onAction ? (
                <Button variant="link" size="sm" className="h-auto min-w-0 shrink-0 whitespace-normal px-1 text-right" onClick={() => onAction(step.key)}>
                  {label.action}
                  <ArrowRight />
                </Button>
              ) : null}
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
}
