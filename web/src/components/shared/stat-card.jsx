import {cn} from "@/lib/utils";
import {Card, CardAction, CardDescription, CardFooter, CardHeader, CardTitle} from "@/components/ui/card";

// `tone` tints the icon chip rather than the number. A wall of these has to
// read as one surface, and a grid of differently coloured figures does not.
const TONE_CLASSES = {
  default: "bg-muted text-muted-foreground border-transparent",
  success: "border-success/25 bg-success/10 text-success",
  warning: "border-warning/30 bg-warning/12 text-warning",
  danger: "border-destructive/25 bg-destructive/10 text-destructive",
  info: "border-info/25 bg-info/10 text-info",
};

/**
 * A single number with its label, used across the dashboard and the monitor
 * page. Built on the card's own header slots so the label, the figure and the
 * icon line up with every other card on the page.
 */
export function StatCard({label, value, suffix, icon: Icon, tone = "default", hint, className, onClick}) {
  // Most values are a figure or two; a timestamp is not, and at the headline
  // size it would wrap and drag the whole row of cards taller.
  const wordy = String(value ?? "").length > 12;

  return (
    <Card
      onClick={onClick}
      className={cn(
        "@container/card from-primary/5 to-card dark:bg-card gap-0 bg-gradient-to-t",
        onClick && "hover:border-ring/60 cursor-pointer transition-colors",
        className
      )}
    >
      <CardHeader>
        <CardDescription className="truncate">{label}</CardDescription>
        <CardTitle
          className={cn(
            "font-semibold tracking-tight tabular-nums",
            wordy ? "text-xl" : "text-2xl @[16rem]/card:text-3xl"
          )}
        >
          {value}
          {suffix ? (
            <span className="text-muted-foreground ml-1.5 text-sm font-normal tracking-normal">{suffix}</span>
          ) : null}
        </CardTitle>
        {Icon ? (
          <CardAction>
            <span className={cn("flex size-8 items-center justify-center rounded-lg border", TONE_CLASSES[tone] ?? TONE_CLASSES.default)}>
              <Icon className="size-4" />
            </span>
          </CardAction>
        ) : null}
      </CardHeader>
      {hint ? (
        <CardFooter className="text-muted-foreground pt-4 text-sm">{hint}</CardFooter>
      ) : null}
    </Card>
  );
}
