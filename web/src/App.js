import React, {Component} from "react";
import {Redirect, Route, Switch, withRouter} from "react-router-dom";
import {StyleProvider, legacyLogicalPropertiesTransformer} from "@ant-design/cssinjs";
import {ConfigProvider, FloatButton, Layout} from "antd";
import * as Setting from "./Setting";
import * as AccountBackend from "./backend/AccountBackend";
import * as ConfigBackend from "./backend/ConfigBackend";
import * as SiteBackend from "./backend/SiteBackend";
import * as Conf from "./Conf";
import {getShadcnThemeComponents, getShadcnThemeToken} from "./shadcnTheme";
import ManagementPage from "./ManagementPage";
import AuthCallback from "./AuthCallback";
import SigninPage from "./SigninPage";

class App extends Component {
  constructor(props) {
    super(props);

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
      configLoaded: false,
    };
  }

  componentDidMount() {
    this.initConfig();
  }

  initConfig() {
    Setting.initServerUrl();
    ConfigBackend.getApplicationConfig()
      .then((res) => {
        if (res.status === "ok") {
          Conf.setConfig(res.data);
        } else {
          Setting.showMessage("error", `Failed to load config: ${res.msg}`);
        }
      })
      .catch(error => {
        Setting.showMessage("error", `Failed to load config: ${error}`);
      })
      .then(() => {
        Setting.initCasdoorSdk(Conf.AuthConfig);
        this.setState({configLoaded: true});
        this.getAccount();
        this.loadSite();
      });
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
    AccountBackend.getAccount().then((res) => {
      if (res.status === "ok") {
        this.setState({account: res.data});
      } else {
        this.setState({account: null});
      }
    }).catch(() => {
      this.setState({account: null});
    });
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
    if (!this.state.configLoaded) {
      return null;
    }

    return (
      <Layout id="parent-area">
        <Switch>
          <Route exact path="/callback" component={AuthCallback} />
          <Route exact path="/signin" render={(props) => this.renderHomeIfSignedIn(<SigninPage {...props} />)} />
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
