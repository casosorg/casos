import * as React from "react";
import {cva} from "class-variance-authority";
import {AlertCircleIcon, CheckCircle2Icon, InfoIcon, TriangleAlertIcon} from "lucide-react";
import {cn} from "@/lib/utils";

const alertVariants = cva(
  "relative grid w-full grid-cols-[0_1fr] items-start gap-y-0.5 rounded-lg border px-4 py-3 text-sm has-[>svg]:grid-cols-[calc(var(--spacing)*4)_1fr] has-[>svg]:gap-x-3 [&>svg]:size-4 [&>svg]:translate-y-0.5",
  {
    variants: {
      variant: {
        default: "bg-card text-card-foreground",
        destructive: "border-destructive/30 bg-destructive/8 text-destructive [&>svg]:text-destructive",
        warning: "border-warning/35 bg-warning/10 text-warning [&>svg]:text-warning",
        success: "border-success/30 bg-success/8 text-success [&>svg]:text-success",
        info: "border-info/30 bg-info/8 text-info [&>svg]:text-info",
      },
    },
    defaultVariants: {variant: "default"},
  }
);

// data-variant is what lets a test assert "an error is showing" without
// selecting on generated Tailwind classes.
function Alert({className, variant = "default", ...props}) {
  return (
    <div data-slot="alert" data-variant={variant} role="alert" className={cn(alertVariants({variant}), className)} {...props} />
  );
}

// Unlike the upstream shadcn title this one wraps: the title here is usually a
// whole backend error message, and clamping it to one line hides the part that
// says what actually went wrong.
function AlertTitle({className, ...props}) {
  return (
    <div
      data-slot="alert-title"
      className={cn("col-start-2 min-h-4 font-medium tracking-tight break-words whitespace-pre-wrap", className)}
      {...props}
    />
  );
}

function AlertDescription({className, ...props}) {
  return (
    <div
      data-slot="alert-description"
      className={cn("col-start-2 grid justify-items-start gap-1 text-sm opacity-90 [&_p]:leading-relaxed", className)}
      {...props}
    />
  );
}

const variantIcons = {
  destructive: AlertCircleIcon,
  warning: TriangleAlertIcon,
  success: CheckCircle2Icon,
  info: InfoIcon,
  default: InfoIcon,
};

// Nearly every page renders the same "the request failed, here is why" banner.
// MessageAlert is that shape in one component: an icon picked from the variant,
// a title, and an optional description or dismiss affordance.
function MessageAlert({variant = "destructive", title, description, className, showIcon = true, action, ...props}) {
  const Icon = variantIcons[variant] ?? InfoIcon;
  return (
    <Alert variant={variant} className={className} {...props}>
      {showIcon ? <Icon /> : null}
      {title ? <AlertTitle>{title}</AlertTitle> : null}
      {description ? <AlertDescription>{description}</AlertDescription> : null}
      {action ? <div className="col-start-2 mt-2">{action}</div> : null}
    </Alert>
  );
}

export {Alert, AlertTitle, AlertDescription, MessageAlert, alertVariants};
