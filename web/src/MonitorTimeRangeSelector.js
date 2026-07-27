import React from "react";
import {Button, DatePicker, Segmented, Space, Switch, Typography} from "antd";
import {ReloadOutlined} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import {MONITOR_TIME_RANGE_OPTIONS} from "./monitorMetrics";

const {RangePicker} = DatePicker;
const {Text} = Typography;

function MonitorTimeRangeSelector({
  timePreset,
  onTimePresetChange,
  onCustomRangeChange,
  autoRefresh,
  onAutoRefreshChange,
  onRefresh,
  loading,
  disabled,
}) {
  const {t} = useTranslation();
  const options = MONITOR_TIME_RANGE_OPTIONS.map(option => ({label: t(option.labelKey), value: option.value}));
  return (
    <Space wrap size={[12, 12]}>
      <Segmented options={options} value={timePreset} onChange={onTimePresetChange} />
      {timePreset === "custom" && (
        <RangePicker
          showTime={{format: "HH:mm"}}
          format="YYYY-MM-DD HH:mm"
          onChange={dates => {
            const validDates = dates?.length === 2 && dates.every(Boolean);
            onCustomRangeChange(validDates ? dates.map(date => date.valueOf()) : null);
          }}
        />
      )}
      <Space size={8}>
        <Text>{t("monitor:Auto Refresh")}</Text>
        <Switch checked={autoRefresh} onChange={onAutoRefreshChange} disabled={timePreset === "custom"} />
      </Space>
      <Button icon={<ReloadOutlined />} loading={loading} disabled={disabled} onClick={onRefresh}>
        {t("general:Refresh")}
      </Button>
    </Space>
  );
}

export default MonitorTimeRangeSelector;
