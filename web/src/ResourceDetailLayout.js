import React from "react";
import {Alert, Button, Space, Spin, Tabs, Typography} from "antd";
import {ArrowLeftOutlined, ReloadOutlined} from "@ant-design/icons";
import {useHistory} from "react-router-dom";

const {Text} = Typography;

function ResourceDetailLayout({title, subtitle, loading, error, onRefresh, activeTab, onTabChange, tabs}) {
  const history = useHistory();
  return (
    <div style={{padding: 24}}>
      <Space direction="vertical" size={16} style={{width: "100%"}}>
        <div style={{display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 16}}>
          <Space align="start">
            <Button icon={<ArrowLeftOutlined />} onClick={() => history.goBack()} />
            <div>
              <Typography.Title level={4} style={{margin: 0}}>{title}</Typography.Title>
              {subtitle && <Text type="secondary">{subtitle}</Text>}
            </div>
          </Space>
          {onRefresh && (
            <Button icon={<ReloadOutlined />} loading={loading} onClick={onRefresh}>
              Refresh
            </Button>
          )}
        </div>
        {error && <Alert type="error" showIcon message={error} />}
        <Spin spinning={loading}>
          <Tabs activeKey={activeTab} onChange={onTabChange} items={tabs} />
        </Spin>
      </Space>
    </div>
  );
}

export default ResourceDetailLayout;
