import * as React from "react";
import i18next from "i18next";
import {Globe, LogOut, Moon, PanelLeft, PanelLeftClose, SlidersHorizontal, Sparkles, Sun, User} from "lucide-react";
import * as Setting from "@/Setting";
import {Button} from "@/components/ui/button";
import {Avatar, AvatarFallback, AvatarImage} from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {BreadcrumbBar} from "@/components/shared/breadcrumb-bar";
import {useUiMode} from "@/hooks/use-ui-mode";

/**
 * The one control that decides how much of Kubernetes the rest of the UI shows.
 * It stays in the header rather than in the account menu because a reader who
 * lands in simple mode and needs a page that is not in the rail has to be able
 * to find the way out without hunting through a dropdown.
 */
function UiModeToggle() {
  const {advanced, toggleMode} = useUiMode();
  const label = advanced ? i18next.t("simple:Simple mode") : i18next.t("simple:Advanced mode");
  return (
    <SimpleTooltip title={advanced ? i18next.t("simple:Switch back to the simplified interface") : i18next.t("simple:Show every Kubernetes page")}>
      <Button variant="ghost" size="sm" onClick={toggleMode} className="gap-1.5">
        {advanced ? <Sparkles className="size-4" /> : <SlidersHorizontal className="size-4" />}
        <span className="hidden md:inline">{label}</span>
      </Button>
    </SimpleTooltip>
  );
}

function ThemeToggle({themeAlgorithm, onChange}) {
  const isDark = Setting.isDarkTheme(themeAlgorithm);
  return (
    <SimpleTooltip title={isDark ? i18next.t("general:Light") : i18next.t("general:Dark")}>
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => onChange(isDark ? ["default"] : ["dark"])}
        aria-label="Toggle theme"
      >
        {isDark ? <Sun className="size-4" /> : <Moon className="size-4" />}
      </Button>
    </SimpleTooltip>
  );
}

function LanguageSelect() {
  const [, forceRender] = React.useReducer((count) => count + 1, 0);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="Change language">
          <Globe className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {Setting.Countries.map((country) => (
          <DropdownMenuItem
            key={country.key}
            onClick={() => {
              Setting.setLanguage(country.key);
              // i18next.changeLanguage does not re-render class components that
              // read i18next.t directly, and the whole shell does. Forcing a
              // render here is what makes the switch take effect immediately.
              forceRender();
            }}
          >
            <img
              src={`https://flagcdn.com/w40/${country.country.toLowerCase()}.png`}
              alt={country.alt}
              width={20}
              className="rounded-xs"
            />
            {country.label}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function AccountMenu({account, onOpenAccount, onSignout}) {
  if (!account) {
    return null;
  }
  const name = account.displayName || account.name || "";
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button type="button" className="hover:bg-accent flex items-center gap-2 rounded-md px-1.5 py-1 transition-colors">
          <Avatar>
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
  );
}

export function AppHeader({
  collapsed,
  onToggleSidebar,
  uri,
  account,
  navbarHtml,
  themeAlgorithm,
  onThemeChange,
  onOpenAccount,
  onSignout,
}) {
  return (
    <header className="bg-background/95 supports-[backdrop-filter]:bg-background/80 sticky top-0 z-30 flex h-13 shrink-0 items-center justify-between gap-2 border-b px-2 backdrop-blur">
      <div className="flex min-w-0 items-center gap-1">
        <Button variant="ghost" size="icon-sm" onClick={onToggleSidebar} aria-label="Toggle navigation">
          {collapsed ? <PanelLeft className="size-4" /> : <PanelLeftClose className="size-4" />}
        </Button>
        <BreadcrumbBar uri={uri} />
      </div>

      <div className="flex items-center gap-0.5 pr-1">
        {navbarHtml ? <div className="flex items-center" dangerouslySetInnerHTML={{__html: navbarHtml}} /> : null}
        <UiModeToggle />
        <ThemeToggle themeAlgorithm={themeAlgorithm} onChange={onThemeChange} />
        <LanguageSelect />
        <AccountMenu account={account} onOpenAccount={onOpenAccount} onSignout={onSignout} />
      </div>
    </header>
  );
}
