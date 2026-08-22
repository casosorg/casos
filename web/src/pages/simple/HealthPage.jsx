import React from "react";
import {useTranslation} from "react-i18next";
import {Activity, ScrollText} from "lucide-react";
import {TabPage} from "@/components/shared/tab-page";
import LogSearchPage from "@/pages/LogSearchPage";
import MonitorPage from "@/pages/MonitorPage";

function HealthPage() {
  const {t} = useTranslation();
  return (
    <TabPage
      title={t("simple:Health")}
      description={t("simple:Check how your apps are doing and find out why one is not working.")}
      tabs={[
        {
          value: "monitor",
          label: t("simple:Usage"),
          icon: Activity,
          hint: t("simple:How much processing power and memory each app is using."),
          content: <MonitorPage />,
        },
        {
          value: "logs",
          label: t("simple:Messages"),
          icon: ScrollText,
          hint: t("simple:Everything your apps have written down. Search here when something is broken."),
          content: <LogSearchPage />,
        },
      ]}
    />
  );
}

export default HealthPage;
