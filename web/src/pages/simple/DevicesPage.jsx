import React from "react";
import {useTranslation} from "react-i18next";
import {Laptop, Network} from "lucide-react";
import {TabPage} from "@/components/shared/tab-page";
import MachineListPage from "@/pages/MachineListPage";
import NodeListPage from "@/pages/NodeListPage";

function DevicesPage({account}) {
  const {t} = useTranslation();
  return (
    <TabPage
      title={t("simple:Devices")}
      description={t("simple:The computers that run your apps.")}
      tabs={[
        {
          value: "machines",
          label: t("simple:My computers"),
          icon: Laptop,
          hint: t("simple:Add a computer here first, then turn it into a worker so apps can run on it."),
          content: <MachineListPage account={account} />,
        },
        {
          value: "nodes",
          label: t("simple:Workers"),
          icon: Network,
          hint: t("simple:Computers that have joined the cluster and can run apps."),
          content: <NodeListPage />,
        },
      ]}
    />
  );
}

export default DevicesPage;
