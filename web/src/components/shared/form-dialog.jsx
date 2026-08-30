import * as React from "react";
import {cn} from "@/lib/utils";
import {Button} from "@/components/ui/button";
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from "@/components/ui/dialog";
import {Label} from "@/components/ui/label";

/**
 * The "open a modal, fill a short form, POST it" shape that most create/edit
 * flows in this app reduce to. It owns the chrome — title, footer, submit
 * state, Enter-to-submit — so a page only writes its fields.
 */
export function FormDialog({
  open,
  onOpenChange,
  title,
  description,
  children,
  onSubmit,
  submitText = "Save",
  cancelText = "Cancel",
  submitting = false,
  submitDisabled = false,
  submitVariant = "default",
  size = "default",
  footer,
}) {
  const sizeClass = {
    default: "sm:max-w-lg",
    lg: "sm:max-w-2xl",
    xl: "sm:max-w-4xl",
  }[size];

  function handleSubmit(event) {
    event.preventDefault();
    onSubmit?.(event);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className={cn("max-h-[90vh] overflow-hidden", sizeClass)}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description ? <DialogDescription>{description}</DialogDescription> : null}
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex min-h-0 flex-col gap-4">
          <div className="scrollbar-thin -mx-1 max-h-[60vh] overflow-y-auto px-1 py-0.5">
            <div className="grid gap-4">{children}</div>
          </div>
          <DialogFooter>
            {footer ?? (
              <>
                <Button type="button" variant="outline" onClick={() => onOpenChange?.(false)}>
                  {cancelText}
                </Button>
                <Button type="submit" variant={submitVariant} loading={submitting} disabled={submitDisabled}>
                  {submitText}
                </Button>
              </>
            )}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * A labelled control with optional hint and error text. Deliberately
 * uncontrolled about the input itself so it wraps anything — Input, Textarea,
 * SimpleSelect, a custom editor.
 */
export function Field({label, htmlFor, required = false, hint, error, children, className}) {
  return (
    // content-start: in a grid row made tall by a neighbouring field, stretched
    // rows push the label down and drag absolutely-positioned input adornments
    // below the control.
    <div className={cn("grid content-start gap-2", className)}>
      {label ? (
        <Label htmlFor={htmlFor}>
          {label}
          {required ? <span className="text-destructive">*</span> : null}
        </Label>
      ) : null}
      {children}
      {hint && !error ? <p className="text-muted-foreground text-xs">{hint}</p> : null}
      {error ? <p className="text-destructive text-xs">{error}</p> : null}
    </div>
  );
}
