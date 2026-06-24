import React from "react";
import Loading from "./common/Loading";
import {Button, Card, Col, Input, InputNumber, Row, Select, Space, Tag} from "antd";
import {EyeInvisibleOutlined, EyeTwoTone} from "@ant-design/icons";
import * as MachineBackend from "./backend/MachineBackend";
import * as Setting from "./Setting";
import i18next from "i18next";

const statusColor = {
  Online: "green",
  Offline: "red",
  Deploying: "processing",
  Deployed: "blue",
  Failed: "red",
  Unknown: "default",
};

class MachineEditPage extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      machineName: props.match.params.machineName,
      machine: null,
    };
  }

  UNSAFE_componentWillMount() {
    this.getMachine();
  }

  getMachine() {
    MachineBackend.getMachine("admin", this.state.machineName)
      .then((res) => {
        if (res.status === "ok") {
          this.setState({machine: res.data});
        } else {
          Setting.showMessage("error", `${i18next.t("general:Failed to get")}: ${res.msg}`);
        }
      });
  }

  updateMachineField(key, value) {
    const machine = Setting.deepCopy(this.state.machine);
    machine[key] = value;
    this.setState({machine});
  }

  submitMachineEdit() {
    MachineBackend.updateMachine(this.state.machine.owner, this.state.machineName, this.state.machine)
      .then((res) => {
        if (res.status === "ok") {
          Setting.showMessage("success", i18next.t("general:Successfully saved"));
          this.setState({machineName: this.state.machine.name});
          this.props.history.push(`/machines/${this.state.machine.name}`);
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

  renderMachine() {
    const machine = this.state.machine;
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
          <span style={{fontSize: "22px", fontWeight: 600}}>
            {i18next.t("machine:Edit Machine")}&nbsp;&nbsp;
            <Tag color={statusColor[machine.status] || "default"}>{machine.status || "Unknown"}</Tag>
          </span>
          <Space>
            <Button type="primary" onClick={() => this.submitMachineEdit()}>{i18next.t("general:Save")}</Button>
          </Space>
        </div>

        <Card size="small" title={renderCardTitle(i18next.t("general:General Settings"), i18next.t("general:General Settings desc"))} style={sectionCardStyle} headStyle={cardHeadStyle}>
          <Row gutter={rowGutter}>
            {this.renderField(
              Setting.getLabel(i18next.t("general:Name"), i18next.t("general:Name - Tooltip")),
              <Input value={machine.name} disabled onChange={e => this.updateMachineField("name", e.target.value)} />,
              8
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("general:Display name"), i18next.t("general:Display name - Tooltip")),
              <Input value={machine.displayName} onChange={e => this.updateMachineField("displayName", e.target.value)} />,
              8
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("policy:Role"), i18next.t("machine:Role - Tooltip")),
              <Select
                value={machine.role || undefined}
                allowClear
                placeholder={i18next.t("policy:Role")}
                style={{width: "100%"}}
                options={[
                  {label: "master", value: "master"},
                  {label: "worker", value: "worker"},
                ]}
                onChange={value => this.updateMachineField("role", value || "")}
              />,
              8
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("machine:Operating system"), i18next.t("machine:Operating system - Tooltip")),
              <Input value={machine.os} onChange={e => this.updateMachineField("os", e.target.value)} />,
              8
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("general:Description"), i18next.t("general:Description")),
              <Input value={machine.description} onChange={e => this.updateMachineField("description", e.target.value)} />,
              16
            )}
          </Row>
        </Card>

        <Card size="small" title={renderCardTitle(i18next.t("machine:SSH Credentials"), i18next.t("machine:SSH Credentials desc"))} style={sectionCardStyle} headStyle={cardHeadStyle}>
          <Row gutter={rowGutter}>
            {this.renderField(
              Setting.getLabel(i18next.t("machine:IP address"), i18next.t("machine:IP address - Tooltip")),
              <Input value={machine.ip} onChange={e => this.updateMachineField("ip", e.target.value)} />,
              6
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("machine:SSH port"), i18next.t("machine:SSH port - Tooltip")),
              <InputNumber style={{width: "100%"}} min={1} max={65535} value={machine.port} onChange={value => this.updateMachineField("port", value)} />,
              6
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("machine:Username"), i18next.t("machine:Username - Tooltip")),
              <Input value={machine.username} onChange={e => this.updateMachineField("username", e.target.value)} />,
              6
            )}
            {this.renderField(
              Setting.getLabel(i18next.t("machine:Auth type"), i18next.t("machine:Auth type - Tooltip")),
              <Select
                value={machine.authType || "password"}
                style={{width: "100%"}}
                options={[
                  {label: i18next.t("machine:Password"), value: "password"},
                  {label: i18next.t("machine:Private key"), value: "privateKey"},
                ]}
                onChange={value => this.updateMachineField("authType", value)}
              />,
              6
            )}
            {machine.authType === "privateKey" ? (
              this.renderField(
                Setting.getLabel(i18next.t("machine:Private key"), i18next.t("machine:Private key - Tooltip")),
                <Input.TextArea rows={4} value={machine.privateKey} onChange={e => this.updateMachineField("privateKey", e.target.value)} />,
                24
              )
            ) : (
              this.renderField(
                Setting.getLabel(i18next.t("machine:Password"), i18next.t("machine:Password - Tooltip")),
                <Input.Password
                  value={machine.password}
                  iconRender={visible => (visible ? <EyeTwoTone /> : <EyeInvisibleOutlined />)}
                  onChange={e => this.updateMachineField("password", e.target.value)}
                />,
                12
              )
            )}
          </Row>
        </Card>
      </div>
    );
  }

  render() {
    return (
      <div style={{background: "var(--ant-color-bg-layout)", padding: "16px 20px 32px", minHeight: "100vh"}}>
        {this.state.machine !== null ? this.renderMachine() : <Loading type="page" tip={i18next.t("general:Loading...")} />}
      </div>
    );
  }
}

export default MachineEditPage;
