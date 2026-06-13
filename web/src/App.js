import React, {Component} from "react";
import {ConfigProvider, Layout} from "antd";
import * as Setting from "./Setting";
import ManagementPage from "./ManagementPage";

const {Footer} = Layout;

class App extends Component {
  constructor(props) {
    super(props);
    Setting.initServerUrl();
  }

  renderFooter() {
    return (
      <Footer style={{textAlign: "center"}}>
        Casos — Control Plane ©{new Date().getFullYear()}
      </Footer>
    );
  }

  render() {
    return (
      <ConfigProvider>
        <Layout id="parent-area">
          <ManagementPage />
          {this.renderFooter()}
        </Layout>
      </ConfigProvider>
    );
  }
}

export default App;
