import React from "react";
import {Button, Result, Spin} from "antd";
import {withRouter} from "react-router-dom";
import * as Setting from "./Setting";
import i18next from "i18next";

class AuthCallback extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      classes: props,
      msg: null,
    };
  }

  componentDidMount() {
    this.login();
  }

  getFromLink() {
    const from = sessionStorage.getItem("from");
    if (from === null) {
      return "/";
    }
    return from;
  }

  login() {
    Setting.signin()
      .then((res) => {
        if (res.status === "ok") {
          Setting.showMessage("success", i18next.t("account:Logged in successfully"));

          const link = this.getFromLink();
          Setting.goToLink(link);
        } else {
          this.setState({msg: res.msg});
        }
      })
      .catch((error) => this.setState({msg: error.message}));
  }

  render() {
    return (
      <div style={{textAlign: "center"}}>
        {this.state.msg === null ? (
          <Spin
            size="large"
            tip={i18next.t("account:Signing in...")}
            style={{paddingTop: "10%"}}
          />
        ) : (
          <div style={{display: "inline"}}>
            <Result
              status="error"
              title={i18next.t("account:Login Error")}
              subTitle={this.state.msg}
              extra={[
                <Button type="primary" key="signin" onClick={() => Setting.goToLink("/signin")}>
                  {i18next.t("account:Sign in again")}
                </Button>,
              ]}
            />
          </div>
        )}
      </div>
    );
  }
}

export default withRouter(AuthCallback);
