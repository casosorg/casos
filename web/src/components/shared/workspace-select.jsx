import React from "react";
import i18next from "i18next";
import {Boxes, Check, ChevronsUpDown} from "lucide-react";
import {DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger} from "@/components/ui/dropdown-menu";
import {cn} from "@/lib/utils";
import {ALL_WORKSPACES, useWorkspace} from "@/hooks/use-workspace";

/**
 * The workspace switcher. It reads as a place rather than a filter, because
 * that is what it is: the namespace new things are created in, and the one the
 * lists are about.
 */
export function WorkspaceSelect({className, onDark = false}) {
  const {workspace, setWorkspace, namespaces, refresh} = useWorkspace();
  const label = workspace || i18next.t("general:All namespaces");

  // Namespaces come and go while the desktop stays open, so the list is read
  // again each time it is asked for rather than once at startup.
  return (
    <DropdownMenu onOpenChange={(open) => open && refresh()}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          data-testid="workspace-select"
          className={cn(
            "flex h-8 max-w-52 items-center gap-1.5 rounded-lg px-2 text-sm font-medium",
            onDark ? "text-white/90 hover:bg-white/15" : "hover:bg-accent",
            className
          )}
        >
          <Boxes className="size-4 shrink-0 opacity-70" />
          <span className="truncate">{label}</span>
          <ChevronsUpDown className="size-3.5 shrink-0 opacity-50" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-80 w-56 overflow-auto">
        <DropdownMenuItem onClick={() => setWorkspace(ALL_WORKSPACES)}>
          <Check className={cn("size-4", workspace ? "opacity-0" : "opacity-100")} />
          {i18next.t("general:All namespaces")}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        {namespaces.map((namespace) => (
          <DropdownMenuItem key={namespace} onClick={() => setWorkspace(namespace)}>
            <Check className={cn("size-4", namespace === workspace ? "opacity-100" : "opacity-0")} />
            <span className="truncate">{namespace}</span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

export default WorkspaceSelect;
