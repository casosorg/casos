import React from "react";
import {Segmented} from "antd";
import {useTranslation} from "react-i18next";

function SeriesModeControl({value, onChange, modes}) {
  const {t} = useTranslation();
  if (!modes || modes.length <= 1) {return null;}
  const options = modes.map(mode => ({
    value: mode.value,
    label: t(mode.labelKey || `monitor:${mode.label}`),
  }));
  return <Segmented options={options} value={value} onChange={onChange} />;
}

export default SeriesModeControl;
