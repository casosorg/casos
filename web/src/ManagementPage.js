import React, {useEffect, useRef, useState} from "react";
import {useTranslation} from "react-i18next";
import {Link, Redirect, Route, Switch, withRouter} from "react-router-dom";
import {Avatar, Button, Card, Dropdown, Layout, Menu, Result} from "antd";
import {
  AppstoreOutlined,
  ClusterOutlined,
  DashboardOutlined,
  DownOutlined,
  LayoutOutlined,
  LockOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  NodeIndexOutlined,
  SettingOutlined,
  UserOutlined
} from "@ant-design/icons";
import "./App.less";
import * as Setting from "./Setting";
import LanguageSelect from "./LanguageSelect";
import ThemeSelect from "./ThemeSelect";
import BreadcrumbBar from "./common/BreadcrumbBar";
import PodListPage from "./PodListPage";
import DeploymentListPage from "./DeploymentListPage";
import ConfigMapListPage from "./ConfigMapListPage";
import SecretListPage from "./SecretListPage";
import NamespaceListPage from "./NamespaceListPage";
import NodeListPage from "./NodeListPage";
import ServiceAccountListPage from "./ServiceAccountListPage";
import ServiceListPage from "./ServiceListPage";
import ClusterRoleBindingListPage from "./ClusterRoleBindingListPage";
import DashboardPage from "./DashboardPage";
import SiteListPage from "./SiteListPage";
import SiteEditPage from "./SiteEditPage";
import i18next from "i18next";

const {Header, Footer, Content, Sider} = Layout;

function getMenuParentKey(uri) {
  if (!uri) {return null;}
  if (uri === "/dashboard") {return null;}
  if (uri.includes("/pods") || uri.includes("/deployments")) {return "/workloads";}
  if (uri.includes("/nodes") || uri.includes("/namespaces") || uri.includes("/serviceaccounts")) {return "/cluster";}
  if (uri.includes("/configmaps") || uri.includes("/secrets")) {return "/configuration";}
  if (uri.includes("/services")) {return "/networking";}
  if (uri.includes("/clusterrolebindings")) {return "/accesscontrol";}
  if (uri.includes("/sites")) {return "/admin";}
  return null;
}

const siderMenuOpenKeysLsKey = "siderMenuOpenKeys";
const defaultMenuOpenKeys = ["/workloads", "/cluster", "/configuration", "/networking", "/accesscontrol", "/admin"];

function readSavedMenuOpenKeys() {
  try {
    const raw = localStorage.getItem(siderMenuOpenKeysLsKey);
    if (!raw) {return defaultMenuOpenKeys;}
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((k) => typeof k === "string") : defaultMenuOpenKeys;
  } catch {
    return defaultMenuOpenKeys;
  }
}

function persistMenuOpenKeys(keys) {
  try {
    localStorage.setItem(siderMenuOpenKeysLsKey, JSON.stringify(keys));
  } catch {
    // ignore
  }
}

function ManagementPage(props) {
  useTranslation();
  const [siderCollapsed, setSiderCollapsed] = useState(() => localStorage.getItem("siderCollapsed") === "true");
  const siderWasCollapsedRef = useRef(false);
  const [menuOpenKeys, setMenuOpenKeys] = useState(() => {
    if (localStorage.getItem("siderCollapsed") === "true") {return [];}
    const saved = readSavedMenuOpenKeys();
    // eslint-disable-next-line no-restricted-globals
    const parentKey = getMenuParentKey(props.uri || location.pathname);
    const next = new Set(saved);
    if (parentKey) {next.add(parentKey);}
    return [...next];
  });

  useEffect(() => {
    if (siderCollapsed) {
      siderWasCollapsedRef.current = true;
      setMenuOpenKeys([]);
      return;
    }
    const justExpanded = siderWasCollapsedRef.current;
    siderWasCollapsedRef.current = false;
    const parentKey = getMenuParentKey(props.uri);
    setMenuOpenKeys(prev => {
      if (justExpanded) {
        const saved = readSavedMenuOpenKeys();
        const next = new Set(saved);
        if (parentKey) {next.add(parentKey);}
        return [...next];
      }
      if (parentKey && !prev.includes(parentKey)) {return [...prev, parentKey];}
      return prev;
    });
  }, [props.uri, siderCollapsed]);

  useEffect(() => {
    if (!siderCollapsed) {persistMenuOpenKeys(menuOpenKeys);}
  }, [menuOpenKeys, siderCollapsed]);

  const {account, site, themeAlgorithm, logo, uri, onSignout, onUpdateSite, setLogoAndThemeAlgorithm} = props;
  const isDark = Array.isArray(themeAlgorithm) && themeAlgorithm.includes("dark");
  // eslint-disable-next-line no-restricted-globals
  const currentUri = uri || location.pathname;
  const firstSeg = currentUri.split("/").filter(Boolean)[0] || "dashboard";
  const selectedLeafKey = "/" + firstSeg;
  const siderLogo = logo || Setting.getLogo(themeAlgorithm || [], site?.logoUrl);
  const navbarHtml = Setting.getNavbarHtml(themeAlgorithm || [], site?.navbarHtml);

  const toggleSider = () => {
    const next = !siderCollapsed;
    setSiderCollapsed(next);
    localStorage.setItem("siderCollapsed", String(next));
  };

  function renderAvatar() {
    if (!account) {return null;}
    const avatarStyle = {verticalAlign: "middle", flexShrink: 0};
    if (account.avatar) {
      return <Avatar src={account.avatar} style={avatarStyle} size={32}>{Setting.getShortName(account.name)}</Avatar>;
    }
    return (
      <Avatar style={{...avatarStyle, backgroundColor: Setting.getAvatarColor(account.name)}} size={32}>
        {Setting.getShortName(account.name)}
      </Avatar>
    );
  }

  function renderUserInfo() {
    return (
      <div style={{display: "flex", alignItems: "center", gap: "8px", cursor: "pointer"}}>
        {renderAvatar()}
        {!Setting.isMobile() && (
          <span style={{fontSize: "14px", fontWeight: 500, maxWidth: "120px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", cursor: "pointer"}}>
            {account?.displayName || account?.name || ""}
          </span>
        )}
        <DownOutlined style={{fontSize: "10px", opacity: 0.45}} />
      </div>
    );
  }

  function renderAccountDropdown() {
    if (!account) {return null;}
    const items = [
      {
        key: "account",
        icon: <UserOutlined />,
        label: i18next.t("account:My Account"),
        onClick: () => window.open(Setting.getMyProfileUrl(account), "_blank"),
      },
      {
        key: "signout",
        icon: <LogoutOutlined />,
        label: i18next.t("account:Sign Out"),
        onClick: onSignout,
      },
    ];
    return (
      <Dropdown key="/rightDropDown" menu={{items}} placement="bottomRight">
        <div className="rightDropDown" style={{cursor: "pointer", userSelect: "none"}}>
          {renderUserInfo()}
        </div>
      </Dropdown>
    );
  }

  function getMenuItems() {
    const allItems = [
      Setting.getItem(<Link to="/dashboard">{i18next.t("general:Dashboard")}</Link>, "/dashboard", <DashboardOutlined />),
      Setting.getItem(<Link to="/pods">{i18next.t("general:Workloads")}</Link>, "/workloads", <AppstoreOutlined />, [
        Setting.getItem(<Link to="/deployments">{i18next.t("general:Deployments")}</Link>, "/deployments"),
        Setting.getItem(<Link to="/pods">{i18next.t("general:Pods")}</Link>, "/pods"),
      ]),
      Setting.getItem(<Link to="/nodes">{i18next.t("general:Cluster")}</Link>, "/cluster", <ClusterOutlined />, [
        Setting.getItem(<Link to="/nodes">{i18next.t("general:Nodes")}</Link>, "/nodes"),
        Setting.getItem(<Link to="/namespaces">{i18next.t("general:Namespaces")}</Link>, "/namespaces"),
        Setting.getItem(<Link to="/serviceaccounts">{i18next.t("general:ServiceAccounts")}</Link>, "/serviceaccounts"),
      ]),
      Setting.getItem(<Link to="/configmaps">{i18next.t("general:Configuration")}</Link>, "/configuration", <SettingOutlined />, [
        Setting.getItem(<Link to="/configmaps">{i18next.t("general:ConfigMaps")}</Link>, "/configmaps"),
        Setting.getItem(<Link to="/secrets">{i18next.t("general:Secrets")}</Link>, "/secrets"),
      ]),
      Setting.getItem(<Link to="/services">{i18next.t("general:Networking")}</Link>, "/networking", <NodeIndexOutlined />, [
        Setting.getItem(<Link to="/services">{i18next.t("general:Services")}</Link>, "/services"),
      ]),
      Setting.getItem(<Link to="/clusterrolebindings">{i18next.t("general:Access Control")}</Link>, "/accesscontrol", <LockOutlined />, [
        Setting.getItem(<Link to="/clusterrolebindings">{i18next.t("general:ClusterRoleBindings")}</Link>, "/clusterrolebindings"),
      ]),
      Setting.getItem(<Link to="/sites/site-built-in">{i18next.t("general:Admin")}</Link>, "/admin", <LayoutOutlined />, [
        Setting.getItem(<Link to="/sites/site-built-in">{i18next.t("general:Sites")}</Link>, "/sites"),
      ]),
    ];
    return allItems;
  }

  function renderRouter() {
    return (
      <Switch>
        <Redirect exact from="/" to="/dashboard" />
        <Route exact path="/dashboard" render={(props) => <DashboardPage {...props} />} />
        <Route exact path="/deployments" render={(props) => <DeploymentListPage {...props} />} />
        <Route exact path="/pods" render={(props) => <PodListPage {...props} />} />
        <Route exact path="/nodes" render={(props) => <NodeListPage {...props} />} />
        <Route exact path="/namespaces" render={(props) => <NamespaceListPage {...props} />} />
        <Route exact path="/serviceaccounts" render={(props) => <ServiceAccountListPage {...props} />} />
        <Route exact path="/configmaps" render={(props) => <ConfigMapListPage {...props} />} />
        <Route exact path="/secrets" render={(props) => <SecretListPage {...props} />} />
        <Route exact path="/services" render={(props) => <ServiceListPage {...props} />} />
        <Route exact path="/clusterrolebindings" render={(props) => <ClusterRoleBindingListPage {...props} />} />
        <Route exact path="/sites" render={(props) => <SiteListPage account={account} {...props} />} />
        <Route exact path="/sites/:siteName" render={(props) => <SiteEditPage account={account} onUpdateSite={onUpdateSite} {...props} />} />
        <Route path="" render={() => <Result status="404" title="404 NOT FOUND" subTitle="Sorry, the page you visited does not exist." extra={<a href="/"><Button type="primary">Back Home</Button></a>} />} />
      </Switch>
    );
  }

  const siderWidth = 256;
  const siderCollapsedWidth = 80;

  return (
    <React.Fragment>
      <Sider
        collapsed={siderCollapsed}
        collapsedWidth={siderCollapsedWidth}
        width={siderWidth}
        trigger={null}
        theme={isDark ? "dark" : "light"}
        style={{
          height: "100vh",
          position: "fixed",
          left: 0,
          top: 0,
          bottom: 0,
          zIndex: 100,
          boxShadow: "none",
          borderRight: isDark ? "1px solid rgba(255,255,255,0.07)" : "1px solid #eaedf3",
          background: isDark ? "#141414" : "#fafbfc",
          display: "flex",
          flexDirection: "column",
        }}
      >
        <div style={{
          height: 52,
          flexShrink: 0,
          display: "flex",
          alignItems: "center",
          justifyContent: siderCollapsed ? "center" : "flex-start",
          padding: siderCollapsed ? "0" : "0 16px 0 24px",
          overflow: "hidden",
          borderBottom: isDark ? "1px solid rgba(255,255,255,0.07)" : "1px solid #eaedf3",
        }}>
          <Link to="/">
            <img
              src={siderLogo}
              alt="logo"
              style={{
                height: siderCollapsed ? 28 : 38,
                width: siderCollapsed ? 28 : undefined,
                maxWidth: siderCollapsed ? 28 : 160,
                objectFit: "contain",
                borderRadius: siderCollapsed ? 6 : 0,
                transition: "max-width 0.2s, height 0.2s, width 0.2s",
              }}
            />
          </Link>
        </div>
        <div className="sider-menu-container" style={{flex: 1, overflow: "auto", paddingTop: "6px"}}>
          <Menu
            mode="inline"
            items={getMenuItems()}
            selectedKeys={[selectedLeafKey]}
            openKeys={menuOpenKeys}
            onOpenChange={setMenuOpenKeys}
            theme={isDark ? "dark" : "light"}
            style={{borderRight: 0, background: isDark ? "#141414" : "#fafbfc"}}
          />
        </div>
      </Sider>

      <div style={{marginLeft: siderCollapsed ? siderCollapsedWidth : siderWidth, flex: 1, minWidth: 0, transition: "margin-left 0.2s", display: "flex", flexDirection: "column", minHeight: "100vh"}}>
        <Header style={{display: "flex", justifyContent: "space-between", alignItems: "center", padding: "0 8px 0 0", marginBottom: "0", backgroundColor: isDark ? "#141414" : "#ffffff", position: "sticky", top: 0, zIndex: 99, borderBottom: isDark ? "1px solid rgba(255,255,255,0.07)" : "1px solid #f0f0f0", boxShadow: "none", height: "52px", lineHeight: "52px"}}>
          <div style={{display: "flex", alignItems: "center"}}>
            <Button
              icon={siderCollapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
              onClick={toggleSider}
              type="text"
              style={{fontSize: 16, width: 40, height: 40}}
            />
            <BreadcrumbBar uri={currentUri} />
          </div>
          <div style={{display: "flex", alignItems: "center", gap: "2px", paddingRight: "8px"}}>
            {navbarHtml && (
              <div style={{display: "flex", alignItems: "center"}} dangerouslySetInnerHTML={{__html: navbarHtml}} />
            )}
            <ThemeSelect themeAlgorithm={themeAlgorithm || []} onChange={setLogoAndThemeAlgorithm} />
            <LanguageSelect className="select-box" />
            {renderAccountDropdown()}
          </div>
        </Header>

        <Content style={{display: "flex", flexDirection: "column"}}>
          <Card className="content-warp-card" styles={{body: {padding: 0, margin: 0}}}>
            {renderRouter()}
          </Card>
        </Content>

        <Footer id="footer" style={{textAlign: "center", height: "67px"}}>
          <div dangerouslySetInnerHTML={{__html: Setting.getFooterHtml(themeAlgorithm || [], site?.footerHtml, site)}} />
        </Footer>
      </div>
    </React.Fragment>
  );
}

export default withRouter(ManagementPage);
