import React, {useMemo} from "react";
import {Alert, Select, Space, Typography} from "antd";
import {useTranslation} from "react-i18next";

const {Text} = Typography;

function SeriesObjectPicker({objects, selectedObjects, onChange, limit, hardLimit, disabled}) {
  const {t} = useTranslation();
  const options = useMemo(() => (objects || []).map(item => ({
    label: item.namespace ? `${item.namespace}/${item.name}` : item.name,
    value: item.name,
  })), [objects]);
  const count = options.length;
  return (
    <Space direction="vertical" style={{width: "100%"}} size={8}>
      <Space wrap>
        <Text>{t("monitor:Pods")}</Text>
        <Select
          mode="multiple"
          allowClear
          maxTagCount="responsive"
          placeholder={t("monitor:Select Pods")}
          value={selectedObjects}
          onChange={values => onChange((values || []).slice(0, hardLimit || limit || 20))}
          options={options}
          disabled={disabled}
          style={{minWidth: 280}}
        />
      </Space>
      {count > limit && (
        <Alert
          type="info"
          showIcon
          message={t("monitor:Pod series are limited")}
          description={t("monitor:Select Pods or use the default Top N limit to avoid drawing too many series.")}
        />
      )}
    </Space>
  );
}

export default SeriesObjectPicker;
