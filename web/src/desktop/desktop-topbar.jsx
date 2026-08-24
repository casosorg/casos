import React, {useMemo, useState} from "react";
import i18next from "i18next";
import {Cpu, Globe, LayoutPanelLeft, LogOut, MemoryStick, Moon, Search, Sun, User} from "lucide-react";
import * as Setting from "@/Setting";
import {Avatar, AvatarFallback, AvatarImage} from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {cn} from "@/lib/utils";
import {NotificationCenter} from "@/desktop/notification-center";
import {tintClass} from "@/desktop/registry";

function percent(used, total) {
  if (!total) {
    return 0;
  }
  return Math.min(100, Math.round((used / total) * 100));
}

/** The cluster's own vital signs, kept where an OS would keep them. */
function ClusterMonitor({stats}) {
  const cpu = percent(stats?.clusterCPUUsedM, stats?.clusterCPUTotalM);
  const memory = percent(stats?.clusterMemUsedMi, stats?.clusterMemTotalMi);

  return (
    <div className="hidden items-center gap-3 rounded-full border border-white/20 bg-white/15 px-3 py-1 text-xs font-medium text-white lg:flex">
      <span className="flex items-center gap-1.5" title={i18next.t("dashboard:label CPU")}>
        <Cpu className="size-3.5" />
        {cpu}%
      </span>
      <span className="flex items-center gap-1.5" title={i18next.t("dashboard:label Memory")}>
        <MemoryStick className="size-3.5" />
        {memory}%
      </span>
      <span className="text-white/80">
        {stats?.nodesReady ?? 0}/{stats?.nodesTotal ?? 0} {i18next.t("general:Nodes")}
      </span>
    </div>
  );
}

/** Search every app by name and launch it, the way the sealos search box does. */
function AppSearch({apps, onOpenApp}) {
  const [term, setTerm] = useState("");

  const matches = useMemo(() => {
    const needle = term.trim().toLowerCase();
    if (!needle) {
      return [];
    }
    return apps
      .filter((app) => {
        const label = app.labelKey ? i18next.t(app.labelKey) : (app.name ?? "");
        return label.toLowerCase().includes(needle);
      })
      .slice(0, 8);
  }, [apps, term]);

  return (
    <div className="relative w-full max-w-sm">
      <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-white/70" />
      <input
        data-testid="desktop-search"
        value={term}
        onChange={(event) => setTerm(event.target.value)}
        placeholder={i18next.t("desktop:Search apps")}
        className="h-8 w-full rounded-lg border border-white/20 bg-white/15 pr-3 pl-9 text-sm text-white placeholder:text-white/60 focus:outline-none"
      />
      {matches.length > 0 && (
        <div className="absolute top-10 left-0 z-10 w-full overflow-hidden rounded-xl border border-white/15 bg-black/60 p-1.5 backdrop-blur-2xl">
          {matches.map((app) => {
            const Icon = app.icon;
            return (
              <button
                key={app.key}
                type="button"
                className="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm text-white/90 hover:bg-white/10"
                onClick={() => {
                  onOpenApp(app);
                  setTerm("");
                }}
              >
                <span className={cn("flex size-6 items-center justify-center rounded-md text-white", app.iconUrl ? "bg-white" : tintClass(app.tint))}>
                  {app.iconUrl ? <img src={app.iconUrl} alt="" className="size-4 object-contain" /> : Icon ? <Icon className="size-3.5" /> : null}
                </span>
                {app.labelKey ? i18next.t(app.labelKey) : app.name}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

/**
 * The desktop's status bar: search, cluster vitals, notifications, and the
 * account controls that the sidebar UI keeps in its header.
 */
export function DesktopTopBar({apps, stats, account, logo, themeAlgorithm, onOpenApp, onThemeChange, onOpenAccount, onSignout, onExitDesktop}) {
  const isDark = Setting.isDarkTheme(themeAlgorithm);
  const [, forceRender] = React.useReducer((count) => count + 1, 0);
  const name = account?.displayName || account?.name || "";

  return (
    <header
      data-testid="desktop-topbar"
      className="relative z-[800] flex h-13 shrink-0 items-center gap-3 border-b border-white/10 bg-black/20 px-4 backdrop-blur-xl"
    >
      <div className="flex shrink-0 items-center gap-2">
        {logo ? <img src={logo} alt="" className="h-6 max-w-28 object-contain" /> : null}
      </div>

      <div className="flex flex-1 justify-center">
        <AppSearch apps={apps} onOpenApp={onOpenApp} />
      </div>

      <div className="flex shrink-0 items-center gap-1.5">
        <ClusterMonitor stats={stats} />
        <NotificationCenter stats={stats} />

        <button
          type="button"
          aria-label={i18next.t("desktop:Management view")}
          title={i18next.t("desktop:Management view")}
          onClick={onExitDesktop}
          className="flex size-8 items-center justify-center rounded-lg text-white/90 hover:bg-white/15"
        >
          <LayoutPanelLeft className="size-4" />
        </button>

        <button
          type="button"
          aria-label={isDark ? i18next.t("general:Light") : i18next.t("general:Dark")}
          onClick={() => onThemeChange(isDark ? ["default"] : ["dark"])}
          className="flex size-8 items-center justify-center rounded-lg text-white/90 hover:bg-white/15"
        >
          {isDark ? <Sun className="size-4" /> : <Moon className="size-4" />}
        </button>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              aria-label={i18next.t("desktop:Change language")}
              className="flex size-8 items-center justify-center rounded-lg text-white/90 hover:bg-white/15"
            >
              <Globe className="size-4" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {Setting.Countries.map((country) => (
              <DropdownMenuItem
                key={country.key}
                onClick={() => {
                  Setting.setLanguage(country.key);
                  forceRender();
                }}
              >
                <img src={`https://flagcdn.com/w40/${country.country.toLowerCase()}.png`} alt={country.alt} width={20} className="rounded-xs" />
                {country.label}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>

        {account ? (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button type="button" className="flex items-center gap-2 rounded-lg px-1.5 py-1 text-white hover:bg-white/15">
                <Avatar className="size-7">
                  {account.avatar ? <AvatarImage src={account.avatar} alt={name} /> : null}
                  <AvatarFallback style={{backgroundColor: Setting.getAvatarColor(account.name), color: "#fff"}}>
                    {Setting.getShortName(account.name)}
                  </AvatarFallback>
                </Avatar>
                <span className="hidden max-w-[120px] truncate text-sm font-medium sm:inline">{name}</span>
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-48">
              <DropdownMenuItem onClick={onOpenAccount}>
                <User />
                {i18next.t("account:My Account")}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem variant="destructive" onClick={onSignout}>
                <LogOut />
                {i18next.t("account:Sign Out")}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null}
      </div>
    </header>
  );
}

export default DesktopTopBar;
