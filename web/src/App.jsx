import React, {Component, Suspense, lazy} from "react";
import {Redirect, Route, Switch, withRouter} from "react-router-dom";
import * as Setting from "@/Setting";
import * as AccountBackend from "@/backend/AccountBackend";
import * as SiteBackend from "@/backend/SiteBackend";
import {Toaster} from "@/components/ui/sonner";
import {TooltipProvider} from "@/components/ui/tooltip";
import {UiModeProvider} from "@/hooks/use-ui-mode";
import ManagementPage from "@/pages/ManagementPage";
import {Loading} from "@/components/shared/loading";
import AuthCallback from "@/pages/AuthCallback";
import SigninPage from "@/pages/SigninPage";

// The desktop pulls in the window manager and every app icon; a reader who
// stays in the sidebar UI should never download it.
const DesktopPage = lazy(() => import("@/pages/DesktopPage"));

class App extends Component {
  constructor(props) {
    super(props);
    Setting.initServerUrl();

    const themeAlgorithm = Setting.readThemeAlgorithm();
    // Applied before the first paint so a dark-mode reload never flashes the
    // light palette.
    Setting.applyThemeAlgorithm(themeAlgorithm);
    Setting.setThemeColor(Setting.getThemeColor());

    this.state = {
      account: undefined,
      uri: null,
      themeAlgorithm,
      site: undefined,
      logo: Setting.getLogo(themeAlgorithm, null),
    };
  }

  componentDidMount() {
    this.getAccount();
    this.loadSite();
  }

  componentDidUpdate() {
    const uri = window.location.pathname;
    if (this.state.uri !== uri) {
      this.setState({uri});
    }
  }

  loadSite() {
    SiteBackend.getBuiltInSite()
      .then((res) => {
        if (res && res.status === "ok" && res.data) {
          const site = res.data;
          this.setState({site, logo: Setting.getLogo(this.state.themeAlgorithm, site.logoUrl)});
          if (site.htmlTitle) {
            document.title = site.htmlTitle;
          }
          if (site.themeColor) {
            Setting.setThemeColor(site.themeColor);
          }
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
      this.setState({account: res.data});
    });
  }

  onUpdateAccount = () => {
    this.getAccount();
  };

  onUpdateSite = () => {
    this.loadSite();
  };

  signout = () => {
    AccountBackend.signout().then((res) => {
      if (res.status === "ok") {
        this.setState({account: null});
        Setting.showMessage("success", "Successfully signed out");
        Setting.goToLink("/");
      } else {
        Setting.showMessage("error", `Signout failed: ${res.msg}`);
      }
    });
  };

  setLogoAndThemeAlgorithm = (nextThemeAlgorithm) => {
    this.setState({
      themeAlgorithm: nextThemeAlgorithm,
      logo: Setting.getLogo(nextThemeAlgorithm, this.state.site?.logoUrl),
    });
    localStorage.setItem("themeAlgorithm", JSON.stringify(nextThemeAlgorithm));
    Setting.applyThemeAlgorithm(nextThemeAlgorithm);
    this.updateFavicon(Setting.getFaviconUrl(nextThemeAlgorithm, this.state.site?.faviconUrl));
  };

  renderHomeIfSignedIn(component) {
    if (this.state.account !== null && this.state.account !== undefined) {
      return <Redirect to="/" />;
    }
    return component;
  }

  renderSigninIfNotSignedIn(component) {
    if (this.state.account === null) {
      sessionStorage.setItem("from", window.location.pathname);
      return <Redirect to="/signin" />;
    }
    if (this.state.account === undefined) {
      // The account request is still in flight; rendering nothing avoids a
      // one-frame redirect to the sign-in page on every reload.
      return null;
    }
    return component;
  }

  render() {
    return (
      <TooltipProvider>
        <UiModeProvider>
          <Toaster />
          <Switch>
            <Route exact path="/callback" component={AuthCallback} />
            <Route
              exact
              path="/signin"
              render={(props) =>
                this.renderHomeIfSignedIn(
                  <SigninPage logo={this.state.logo} themeAlgorithm={this.state.themeAlgorithm} site={this.state.site} {...props} />
                )
              }
            />
            <Route
              path="/desktop"
              render={(props) =>
                this.renderSigninIfNotSignedIn(
                  <Suspense fallback={<Loading type="page" />}>
                    <DesktopPage
                      account={this.state.account}
                      site={this.state.site}
                      themeAlgorithm={this.state.themeAlgorithm}
                      logo={this.state.logo}
                      onSignout={this.signout}
                      onUpdateSite={this.onUpdateSite}
                      onUpdateAccount={this.onUpdateAccount}
                      setLogoAndThemeAlgorithm={this.setLogoAndThemeAlgorithm}
                      {...props}
                    />
                  </Suspense>
                )
              }
            />
            <Route
              path="/"
              render={(props) =>
                this.renderSigninIfNotSignedIn(
                  <ManagementPage
                    account={this.state.account}
                    uri={this.state.uri}
                    site={this.state.site}
                    themeAlgorithm={this.state.themeAlgorithm}
                    logo={this.state.logo}
                    onSignout={this.signout}
                    onUpdateSite={this.onUpdateSite}
                    onUpdateAccount={this.onUpdateAccount}
                    setLogoAndThemeAlgorithm={this.setLogoAndThemeAlgorithm}
                    {...props}
                  />
                )
              }
            />
          </Switch>
        </UiModeProvider>
      </TooltipProvider>
    );
  }
}

export default withRouter(App);
