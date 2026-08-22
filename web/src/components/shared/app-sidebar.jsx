import * as React from "react";
import {Link} from "react-router-dom";
import i18next from "i18next";
import {ChevronDown} from "lucide-react";
import {cn} from "@/lib/utils";
import {getNavGroups, navGroups} from "@/nav";
import {useUiMode} from "@/hooks/use-ui-mode";
import {SimpleTooltip} from "@/components/ui/tooltip";

const OPEN_KEYS_STORAGE = "siderMenuOpenKeys";
const DEFAULT_OPEN_KEYS = navGroups.filter((group) => group.children).map((group) => group.key);

export function readSavedOpenKeys() {
  try {
    const raw = localStorage.getItem(OPEN_KEYS_STORAGE);
    if (!raw) {
      return DEFAULT_OPEN_KEYS;
    }
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((key) => typeof key === "string") : DEFAULT_OPEN_KEYS;
  } catch {
    return DEFAULT_OPEN_KEYS;
  }
}

export function persistOpenKeys(keys) {
  try {
    localStorage.setItem(OPEN_KEYS_STORAGE, JSON.stringify(keys));
  } catch {
    // Private-mode storage failures must not take the navigation down.
  }
}

function NavLink({to, active, collapsed, icon: Icon, label, nested = false}) {
  const content = (
    <Link
      to={to}
      className={cn(
        "flex items-center gap-2.5 rounded-md px-2.5 py-2 text-sm transition-colors",
        collapsed && "justify-center px-0",
        nested && !collapsed && "pl-9 text-[13px]",
        active
          ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium"
          : "text-sidebar-foreground/70 hover:bg-sidebar-accent/60 hover:text-sidebar-foreground"
      )}
    >
      {Icon ? <Icon className="size-4 shrink-0" /> : null}
      {!collapsed ? <span className="truncate">{label}</span> : null}
    </Link>
  );

  return collapsed ? <SimpleTooltip title={label} side="right">{content}</SimpleTooltip> : content;
}

/**
 * Fixed navigation rail. When collapsed it drops to icons only and the groups
 * stop expanding in place — an accordion inside a 64px column is unreadable, so
 * the icon links straight to the group's first page instead.
 */
export function AppSidebar({collapsed, selectedKey, openKeys, onOpenKeysChange, logo}) {
  const {mode} = useUiMode();
  const groups = getNavGroups(mode);

  function toggleGroup(key) {
    onOpenKeysChange(openKeys.includes(key) ? openKeys.filter((item) => item !== key) : [...openKeys, key]);
  }

  return (
    <aside
      className={cn(
        "bg-sidebar fixed inset-y-0 left-0 z-40 flex flex-col border-r transition-[width] duration-200",
        collapsed ? "w-16" : "w-64"
      )}
    >
      <div className={cn("flex h-13 shrink-0 items-center border-b", collapsed ? "justify-center px-0" : "px-5")}>
        <Link to="/" className="flex items-center overflow-hidden">
          <img
            src={logo}
            alt="CasOS"
            className={cn("object-contain transition-all", collapsed ? "size-7 rounded-md" : "h-8 max-w-[150px]")}
          />
        </Link>
      </div>

      <nav className="scrollbar-thin flex-1 space-y-0.5 overflow-y-auto p-2">
        {groups.map((group) => {
          if (!group.children) {
            return (
              <NavLink
                key={group.key}
                to={group.path}
                active={selectedKey === group.key}
                collapsed={collapsed}
                icon={group.icon}
                label={i18next.t(group.label)}
              />
            );
          }

          const groupLabel = i18next.t(group.label);
          const isOpen = openKeys.includes(group.key);
          const hasActiveChild = group.children.some((child) => child.key === selectedKey);

          if (collapsed) {
            return (
              <NavLink
                key={group.key}
                to={group.children[0].path}
                active={hasActiveChild}
                collapsed
                icon={group.icon}
                label={groupLabel}
              />
            );
          }

          return (
            <div key={group.key}>
              <button
                type="button"
                onClick={() => toggleGroup(group.key)}
                className={cn(
                  "flex w-full items-center gap-2.5 rounded-md px-2.5 py-2 text-sm transition-colors",
                  hasActiveChild ? "text-sidebar-foreground font-medium" : "text-sidebar-foreground/70",
                  "hover:bg-sidebar-accent/60 hover:text-sidebar-foreground"
                )}
                aria-expanded={isOpen}
              >
                {group.icon ? <group.icon className="size-4 shrink-0" /> : null}
                <span className="flex-1 truncate text-left">{groupLabel}</span>
                <ChevronDown className={cn("size-3.5 shrink-0 transition-transform", isOpen && "rotate-180")} />
              </button>
              {isOpen ? (
                <div className="mt-0.5 space-y-0.5">
                  {group.children.map((child) => (
                    <NavLink
                      key={child.key}
                      to={child.path}
                      active={selectedKey === child.key}
                      collapsed={false}
                      label={i18next.t(child.label)}
                      nested
                    />
                  ))}
                </div>
              ) : null}
            </div>
          );
        })}
      </nav>
    </aside>
  );
}
