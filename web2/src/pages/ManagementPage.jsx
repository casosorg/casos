import React, {useEffect, useRef, useState} from "react";
import {useTranslation} from "react-i18next";
import {withRouter} from "react-router-dom";
import * as Setting from "@/Setting";
import {findGroupOf} from "@/nav";
import {AppSidebar, persistOpenKeys, readSavedOpenKeys} from "@/components/shared/app-sidebar";
import {AppHeader} from "@/components/shared/app-header";
import {AccountDialog} from "@/components/shared/account-dialog";
import {AppRoutes} from "@/routes";
import {cn} from "@/lib/utils";

function ManagementPage(props) {
  // Subscribing to the i18n context is what re-renders the shell (and the nav
  // labels underneath it) when the language changes.
  useTranslation();

  const {account, site, themeAlgorithm, logo, uri, onSignout, onUpdateSite, onUpdateAccount, setLogoAndThemeAlgorithm} = props;
  const [accountUpdatedAt, setAccountUpdatedAt] = useState(0);

  const [collapsed, setCollapsed] = useState(() => localStorage.getItem("siderCollapsed") === "true");
  const [accountDialogOpen, setAccountDialogOpen] = useState(false);
  const wasCollapsedRef = useRef(false);

  const currentUri = uri || window.location.pathname;
  const firstSegment = currentUri.split("/").filter(Boolean)[0] || "dashboard";
  const selectedKey = `/${firstSegment}`;

  const [openKeys, setOpenKeys] = useState(() => {
    if (localStorage.getItem("siderCollapsed") === "true") {
      return [];
    }
    const saved = new Set(readSavedOpenKeys());
    const group = findGroupOf(selectedKey);
    if (group) {
      saved.add(group.key);
    }
    return [...saved];
  });

  // Navigating into a collapsed group opens it, and expanding the rail restores
  // whatever the reader last had open rather than the defaults.
  useEffect(() => {
    if (collapsed) {
      wasCollapsedRef.current = true;
      setOpenKeys([]);
      return;
    }
    const justExpanded = wasCollapsedRef.current;
    wasCollapsedRef.current = false;
    const group = findGroupOf(selectedKey);

    setOpenKeys((previous) => {
      if (justExpanded) {
        const restored = new Set(readSavedOpenKeys());
        if (group) {
          restored.add(group.key);
        }
        return [...restored];
      }
      if (group && !previous.includes(group.key)) {
        return [...previous, group.key];
      }
      return previous;
    });
  }, [selectedKey, collapsed]);

  useEffect(() => {
    if (!collapsed) {
      persistOpenKeys(openKeys);
    }
  }, [openKeys, collapsed]);

  function toggleSidebar() {
    const next = !collapsed;
    setCollapsed(next);
    localStorage.setItem("siderCollapsed", String(next));
  }

  function handleOpenAccount() {
    // A built-in account is edited in place; a Casdoor account is edited in
    // Casdoor.
    if (Setting.isBasicLoginMode(account)) {
      setAccountDialogOpen(true);
      return;
    }
    const profileUrl = Setting.getMyProfileUrl(account);
    if (profileUrl !== "") {
      window.open(profileUrl, "_blank", "noreferrer");
    }
  }

  function handleAccountUpdated() {
    onUpdateAccount?.();
    setAccountUpdatedAt(Date.now());
  }

  const sidebarLogo = logo || Setting.getLogo(themeAlgorithm || [], site?.logoUrl);
  const navbarHtml = Setting.getNavbarHtml(themeAlgorithm || [], site?.navbarHtml);
  const footerHtml = Setting.getFooterHtml(themeAlgorithm || [], site?.footerHtml, site);

  return (
    <div data-testid="management-layout" className="bg-muted/30 min-h-screen">
      <AppSidebar
        collapsed={collapsed}
        selectedKey={selectedKey}
        openKeys={openKeys}
        onOpenKeysChange={setOpenKeys}
        logo={sidebarLogo}
      />

      <div className={cn("flex min-h-screen flex-col transition-[margin] duration-200", collapsed ? "ml-16" : "ml-64")}>
        <AppHeader
          collapsed={collapsed}
          onToggleSidebar={toggleSidebar}
          uri={currentUri}
          account={account}
          navbarHtml={navbarHtml}
          themeAlgorithm={themeAlgorithm}
          onThemeChange={setLogoAndThemeAlgorithm}
          onOpenAccount={handleOpenAccount}
          onSignout={onSignout}
        />

        <main className="flex min-w-0 flex-1 flex-col">
          <AppRoutes account={account} accountUpdatedAt={accountUpdatedAt} onOpenAccount={handleOpenAccount} onUpdateSite={onUpdateSite} />
        </main>

        <footer className="text-muted-foreground flex items-center justify-center border-t py-5 text-sm">
          <div dangerouslySetInnerHTML={{__html: footerHtml}} />
        </footer>
      </div>

      <AccountDialog
        account={account}
        open={accountDialogOpen}
        onOpenChange={setAccountDialogOpen}
        onUpdateAccount={handleAccountUpdated}
      />
    </div>
  );
}

export default withRouter(ManagementPage);
