import React from "react";
import Loading from "./common/Loading";
import {Button, Card, Col, Image, Input, Row, Space} from "antd";
import {EyeInvisibleOutlined, EyeTwoTone, LinkOutlined} from "@ant-design/icons";
import * as SiteBackend from "./backend/SiteBackend";
import * as Setting from "./Setting";
import i18next from "i18next";

class SiteEditPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      siteName: props.match.params.siteName,
      site: null,
    };
  }

  UNSAFE_componentWillMount() {
    this.getSite();
  }

  getSite() {
    SiteBackend.getSite("admin", this.state.siteName)
      .then((res) => {
        if (res.status === "ok") {
          this.setState({site: res.data});
        } else {
          Setting.showMessage("error", `${i18next.t("general:Failed to get")}: ${res.msg}`);
        }
      });
  }

  updateSiteField(key, value) {
    const site = Setting.deepCopy(this.state.site);
    site[key] = value;
    this.setState({site});
  }

  submitSiteEdit() {
    SiteBackend.updateSite(this.state.site.owner, this.state.siteName, this.state.site)
      .then((res) => {
        if (res.status === "ok") {
          Setting.showMessage("success", i18next.t("general:Successfully saved"));
          Setting.setThemeColor(this.state.site.themeColor || Setting.getThemeColor());
          this.setState({siteName: this.state.site.name});
          if (this.props.onUpdateSite) {
            this.props.onUpdateSite();
          }
          this.props.history.push(`/sites/${this.state.site.name}`);
        } else {
          Setting.showMessage("error", `${i18next.t("general:Failed to save")}: ${res.msg}`);
        }
      })
      .catch(error => {
        Setting.showMessage("error", `${i18next.t("general:Failed to save")}: ${error}`);
      });
  }

  renderField(label, control, span = 8) {
    return (
      <Col style={{marginTop: "12px"}} span={Setting.isMobile() ? 22 : span}>
        <div style={{marginBottom: "6px", color: "var(--ant-color-text-secondary)", fontWeight: 500, lineHeight: "22px", fontSize: "13px"}}>{label}</div>
        {control}
      </Col>
    );
  }

  renderSite() {
    const site = this.state.site;
    const rowGutter = [16, 8];
    const cardHeadStyle = {background: "transparent", borderBottom: "none", fontWeight: 600, fontSize: "15px"};
    const sectionCardStyle = {marginBottom: "16px", borderRadius: "14px", boxShadow: "0 1px 3px rgba(0,0,0,0.06)", padding: "18px"};

    const renderCardTitle = (title, desc) => (
      <div>
        <div style={{fontWeight: 600, fontSize: "15px"}}>{title}</div>
        <div style={{fontSize: "13px", color: "var(--ant-color-text-tertiary)", fontWeight: 400, marginTop: "2px"}}>{desc}</div>
      </div>
    );

    return (
      <div>
        <div style={{marginBottom: "16px", display: "flex", justifyContent: "space-between", alignItems: "center"}}>
          <span style={{fontSize: "22px", fontWeight: 600}}>{i18next.t("site:Edit Site")}</span>
          <Space>
            <Button onClick={() => this.submitSiteEdit()}>{i18next.t("general:Save")}</Button>
            <Button type="primary" onClick={() => this.submitSiteEdit()}>{i18next.t("general:Save")}</Button>
          </Space>
        </div>

        <Card size="small" title={renderCardTitle(i18next.t("general:General Settings"), i18next.t("general:General Settings desc"))} style={sectionCardStyle} headStyle={cardHeadStyle}>
          <Row gutter={rowGutter}>
            {this.renderField(
              Setting.getLabel(i18next.t("general:Name"), i18next.t("general:Name - Tooltip")),
              <Input value={site.name} disabled={site.name === "site-built-in"} onChange={e => this.updateSiteField("name", e.target.value)} />,
              8
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("general:Display name"), i18next.t("general:Display name - Tooltip")),
              <Input value={site.displayName} onChange={e => this.updateSiteField("displayName", e.target.value)} />,
              8
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("general:HTML title"), i18next.t("general:HTML title - Tooltip")),
              <Input value={site.htmlTitle} onChange={e => this.updateSiteField("htmlTitle", e.target.value)} />,
              8
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("site:Theme color"), i18next.t("site:Theme color - Tooltip")),
              <input type="color" value={site.themeColor || "#404040"} style={{height: "32px", width: "64px", cursor: "pointer", border: "1px solid #d9d9d9", borderRadius: "6px", padding: "2px"}} onChange={e => this.updateSiteField("themeColor", e.target.value)} />,
              8
            )}
          </Row>
        </Card>

        <Card size="small" title={renderCardTitle(i18next.t("general:Branding"), i18next.t("general:Branding desc"))} style={sectionCardStyle} headStyle={cardHeadStyle}>
          <Row gutter={rowGutter}>
            {this.renderField(
              Setting.getLabel(i18next.t("general:Favicon URL"), i18next.t("general:Favicon URL - Tooltip")),
              <Space direction="vertical" style={{width: "100%"}}>
                <Input prefix={<LinkOutlined />} value={site.faviconUrl} onChange={e => this.updateSiteField("faviconUrl", e.target.value)} />
                {site.faviconUrl ? (
                  <Image src={site.faviconUrl} alt={site.faviconUrl} height={90} preview={{mask: "Preview"}} />
                ) : null}
              </Space>,
              12
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("general:Logo URL"), i18next.t("general:Logo URL - Tooltip")),
              <Space direction="vertical" style={{width: "100%"}}>
                <Input prefix={<LinkOutlined />} value={site.logoUrl} onChange={e => this.updateSiteField("logoUrl", e.target.value)} />
                {site.logoUrl ? (
                  <Image src={site.logoUrl} alt={site.logoUrl} height={90} preview={{mask: "Preview"}} />
                ) : null}
              </Space>,
              12
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("general:Static base URL"), i18next.t("general:Static base URL - Tooltip")),
              <Input prefix={<LinkOutlined />} value={site.staticBaseUrl} onChange={e => this.updateSiteField("staticBaseUrl", e.target.value)} />,
              12
            )}
          </Row>
        </Card>

        <Card size="small" title={renderCardTitle(i18next.t("general:Content"), i18next.t("general:Content desc"))} style={sectionCardStyle} headStyle={cardHeadStyle}>
          <Row gutter={rowGutter}>
            {this.renderField(
              Setting.getLabel(i18next.t("general:Navbar HTML"), i18next.t("general:Navbar HTML - Tooltip")),
              <Input.TextArea rows={3} value={site.navbarHtml} onChange={e => this.updateSiteField("navbarHtml", e.target.value)} />,
              12
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("general:Footer HTML"), i18next.t("general:Footer HTML - Tooltip")),
              <Input.TextArea rows={3} value={site.footerHtml} onChange={e => this.updateSiteField("footerHtml", e.target.value)} />,
              12
            )}
          </Row>
        </Card>

        <Card size="small" title={renderCardTitle(i18next.t("site:Authentication"), i18next.t("site:Authentication desc"))} style={sectionCardStyle} headStyle={cardHeadStyle}>
          <Row gutter={rowGutter}>
            {this.renderField(
              Setting.getLabel(i18next.t("site:OIDC issuer"), i18next.t("site:OIDC issuer - Tooltip")),
              <Input prefix={<LinkOutlined />} value={site.issuer} onChange={e => this.updateSiteField("issuer", e.target.value)} />,
              12
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("provider:Client ID"), i18next.t("provider:Client ID - Tooltip")),
              <Input value={site.clientId} onChange={e => this.updateSiteField("clientId", e.target.value)} />,
              6
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("provider:Client secret"), i18next.t("provider:Client secret - Tooltip")),
              <Input.Password
                value={site.clientSecret}
                iconRender={visible => (visible ? <EyeTwoTone /> : <EyeInvisibleOutlined />)}
                onChange={e => this.updateSiteField("clientSecret", e.target.value)}
              />,
              6
            )}
          </Row>
        </Card>

        <Card size="small" title={renderCardTitle(i18next.t("site:Advanced"), i18next.t("site:Advanced desc"))} style={sectionCardStyle} headStyle={cardHeadStyle}>
          <Row gutter={rowGutter}>
            {this.renderField(
              Setting.getLabel(i18next.t("site:Socks5 proxy"), i18next.t("site:Socks5 proxy - Tooltip")),
              <Input value={site.socks5Proxy} onChange={e => this.updateSiteField("socks5Proxy", e.target.value)} />,
              8
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("site:Log config"), i18next.t("site:Log config - Tooltip")),
              <Input value={site.logConfig} onChange={e => this.updateSiteField("logConfig", e.target.value)} />,
              24
            )}
          </Row>
        </Card>
      </div>
    );
  }

  render() {
    return (
      <div style={{background: "var(--ant-color-bg-layout)", padding: "16px 20px 32px", minHeight: "100vh"}}>
        {this.state.site !== null ? this.renderSite() : <Loading type="page" tip={i18next.t("general:Loading...")} />}
      </div>
    );
  }
}

export default SiteEditPage;
