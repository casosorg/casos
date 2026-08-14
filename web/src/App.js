import React, {Component} from "react";
import {Redirect, Route, Switch, withRouter} from "react-router-dom";
import {StyleProvider, legacyLogicalPropertiesTransformer} from "@ant-design/cssinjs";
import {Button, ConfigProvider, FloatButton, Layout, Result, Spin} from "antd";
import * as Setting from "./Setting";
import * as AccountBackend from "./backend/AccountBackend";
import * as SiteBackend from "./backend/SiteBackend";
import {getShadcnThemeComponents, getShadcnThemeToken} from "./shadcnTheme";
import ManagementPage from "./ManagementPage";
import AuthCallback from "./AuthCallback";
import SigninPage from "./SigninPage";
import i18next from "i18next";

class App extends Component {
  constructor(props) {
    super(props);
    Setting.initServerUrl();

    let storageThemeAlgorithm = ["default"];
    try {
      const raw = localStorage.getItem("themeAlgorithm");
      if (raw) {storageThemeAlgorithm = JSON.parse(raw);}
    } catch {
      storageThemeAlgorithm = ["default"];
    }
    document.documentElement.setAttribute("data-theme", storageThemeAlgorithm.includes("dark") ? "dark" : "light");

    this.state = {
      account: undefined,
      uri: null,
      themeAlgorithm: storageThemeAlgorithm,
      site: undefined,
      logo: null,
      signinOptions: undefined,
      signinOptionsError: "",
    };
  }

  componentDidMount() {
    this.loadSigninOptions();
    this.getAccount();
    this.loadSite();
  }

  loadSigninOptions() {
    AccountBackend.getSigninOptions()
      .then((res) => {
        if (res?.status !== "ok") {
          this.setState({signinOptions: null, signinOptionsError: res?.msg || "Unable to load sign-in options"});
          return;
        }
        if (res.data?.authMode === "casdoor") {
          Setting.initCasdoorSdk(res.data.authConfig, res.data.oauthState);
        }
        this.setState({signinOptions: res.data, signinOptionsError: ""});
      })
      .catch((error) => this.setState({signinOptions: null, signinOptionsError: error.message}));
  }

  componentDidUpdate() {
    // eslint-disable-next-line no-restricted-globals
    const uri = location.pathname;
    if (this.state.uri !== uri) {
      this.setState({uri});
    }
  }

  loadSite() {
    SiteBackend.getBuiltInSite()
      .then((res) => {
        if (res && res.status === "ok" && res.data) {
          const site = res.data;
          this.setState({site});
          if (site.htmlTitle) {document.title = site.htmlTitle;}
          if (site.themeColor) {Setting.setThemeColor(site.themeColor);}
          this.updateFavicon(Setting.getFaviconUrl(this.state.themeAlgorithm, site.faviconUrl));
        }
      })
      .catch(() => {});
  }

  updateFavicon(url) {
    let link = document.querySelector("link[rel=\"icon\"]");
    if (!link) {
      link = document.createElement("link");
      link.rel = "icon";
      document.head.appendChild(link);
    }
    link.href = url;
  }

  getAccount() {
    AccountBackend.getAccount()
      .then((res) => {
        const account = res?.status === "ok" ? res.data : null;
        this.setState({account});
      })
      .catch(() => this.setState({account: null}));
  }

  signout() {
    AccountBackend.signout().then((res) => {
      if (res.status === "ok") {
        this.setState({account: null});
        Setting.showMessage("success", "Successfully signed out");
        Setting.goToLink("/");
      } else {
        Setting.showMessage("error", `Signout failed: ${res.msg}`);
      }
    });
  }

  onUpdateSite = () => {
    this.loadSite();
  };

  setLogoAndThemeAlgorithm = (nextThemeAlgorithm) => {
    this.setState({
      themeAlgorithm: nextThemeAlgorithm,
      logo: Setting.getLogo(nextThemeAlgorithm, this.state.site?.logoUrl),
    });
    localStorage.setItem("themeAlgorithm", JSON.stringify(nextThemeAlgorithm));
    document.documentElement.setAttribute("data-theme", nextThemeAlgorithm.includes("dark") ? "dark" : "light");
    this.updateFavicon(Setting.getFaviconUrl(nextThemeAlgorithm, this.state.site?.faviconUrl));
  };

  renderHomeIfSignedIn(component) {
    if (this.state.account === undefined) {
      return null;
    }
    if (this.state.account !== null) {
      return <Redirect to="/" />;
    }
    return component;
  }

  renderSigninIfNotSignedIn(component) {
    if (this.state.account === null) {
      sessionStorage.setItem("from", window.location.pathname);
      return <Redirect to="/signin" />;
    } else if (this.state.account === undefined) {
      return null;
    }
    return component;
  }

  renderContent() {
    if (this.state.signinOptions === undefined) {
      return (
        <div style={{display: "flex", alignItems: "center", justifyContent: "center", minHeight: "100vh", width: "100%"}}>
          <Spin size="large" />
        </div>
      );
    }

    if (this.state.signinOptions === null) {
      return (
        <Result
          status="warning"
          title={i18next.t("account:Unable to load sign-in options")}
          subTitle={this.state.signinOptionsError}
          extra={<Button onClick={() => {
            this.setState({signinOptions: undefined, signinOptionsError: ""});
            this.loadSigninOptions();
          }}>{i18next.t("account:Retry")}</Button>}
        />
      );
    }

    return (
      <Layout id="parent-area">
        <Switch>
          <Route exact path="/callback" render={(props) => this.state.signinOptions?.authMode === "casdoor" ? <AuthCallback {...props} /> : <Redirect to="/signin" />} />
          <Route exact path="/signin" render={(props) => this.renderHomeIfSignedIn(
            <SigninPage
              options={this.state.signinOptions}
              error={this.state.signinOptionsError}
              logo={this.state.logo || Setting.getLogo(this.state.themeAlgorithm, this.state.site?.logoUrl)}
              onRetry={() => {
                this.setState({signinOptions: undefined, signinOptionsError: ""});
                this.loadSigninOptions();
              }}
              {...props}
            />
          )} />
          <Route path="/" render={(props) => this.renderSigninIfNotSignedIn(
            <ManagementPage
              account={this.state.account}
              uri={this.state.uri}
              history={this.props.history}
              site={this.state.site}
              themeAlgorithm={this.state.themeAlgorithm}
              logo={this.state.logo}
              onSignout={this.signout.bind(this)}
              onUpdateSite={this.onUpdateSite}
              setLogoAndThemeAlgorithm={this.setLogoAndThemeAlgorithm}
              {...props}
            />
          )} />
        </Switch>
      </Layout>
    );
  }

  render() {
    const isDark = this.state.themeAlgorithm.includes("dark");
    const themeColor = Setting.getThemeColor();
    return (
      <React.Fragment>
        <ConfigProvider
          theme={{
            token: {
              ...getShadcnThemeToken(isDark),
              colorPrimary: themeColor,
              colorInfo: themeColor,
            },
            components: getShadcnThemeComponents(isDark),
            algorithm: Setting.getAlgorithm(this.state.themeAlgorithm),
          }}>
          <StyleProvider hashPriority="high" transformers={[legacyLogicalPropertiesTransformer]}>
            <React.Fragment>
              <FloatButton.BackTop />
              {this.renderContent()}
            </React.Fragment>
          </StyleProvider>
        </ConfigProvider>
      </React.Fragment>
    );
  }
}

export default withRouter(App);
