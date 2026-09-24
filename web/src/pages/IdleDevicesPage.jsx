import React from "react";
import i18next from "i18next";
import {useTranslation} from "react-i18next";
import {Cpu, HardDrive, Server, Zap} from "lucide-react";
import * as DeviceBackend from "@/backend/DeviceBackend";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Progress} from "@/components/ui/progress";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {MessageAlert} from "@/components/ui/alert";
import {DataTable} from "@/components/shared/data-table";
import {PageContainer, PageHeader} from "@/components/shared/page-header";
import {StatCard} from "@/components/shared/stat-card";
import {useResource} from "@/hooks/use-resource";

const POLL_INTERVAL = 20000;

const STATE_VARIANTS = {
  free: "success",
  busy: "warning",
  reserved: "secondary",
  cordoned: "muted",
  offline: "danger",
};

const STATE_LABELS = {
  free: "device:Free",
  busy: "device:Busy",
  reserved: "device:Reserved",
  cordoned: "device:Cordoned",
  offline: "device:Offline",
};

const formatCores = (milli) => (milli >= 1000 ? `${(milli / 1000).toFixed(1)}` : `${(milli / 1000).toFixed(2)}`);
const formatMiB = (mib) => (mib >= 1024 ? `${(mib / 1024).toFixed(1)} GiB` : `${Math.round(mib)} MiB`);

function CapacityCell({percent, free, total, usage}) {
  return (
    <div className="grid gap-1">
      <div className="flex items-baseline justify-between gap-2 text-xs">
        <span className="font-medium tabular-nums">{free}</span>
        <span className="text-muted-foreground tabular-nums">/ {total}</span>
      </div>
      <Progress value={percent} className="h-1.5" />
      {usage ? <span className="text-muted-foreground text-[11px] tabular-nums">{usage}</span> : null}
    </div>
  );
}

function IdleDevicesPage() {
  useTranslation();
  const {data: report, loading, refresh} = useResource(
    () => DeviceBackend.getDevices(),
    [],
    {initialData: {devices: [], totals: {}, usageKnown: false}, pollInterval: POLL_INTERVAL}
  );

  const devices = report?.devices ?? [];
  const totals = report?.totals ?? {};
  const hasGpus = (totals.gpuTotal ?? 0) > 0;

  const columns = [
    {
      key: "name",
      title: i18next.t("device:Device"),
      dataIndex: "name",
      minWidth: 200,
      sortable: true,
      render: (value, record) => (
        <div className="flex items-center gap-2">
          <Server className="text-muted-foreground size-4 shrink-0" />
          <div className="min-w-0">
            <div className="truncate font-medium">{value}</div>
            <div className="text-muted-foreground truncate text-xs">{(record.roles ?? []).join(", ")}</div>
          </div>
        </div>
      ),
    },
    {
      key: "state",
      title: i18next.t("device:Availability"),
      dataIndex: "state",
      width: 140,
      sortable: true,
      render: (value, record) => {
        const badge = <Badge variant={STATE_VARIANTS[value] ?? "muted"}>{i18next.t(STATE_LABELS[value] ?? "device:Busy")}</Badge>;
        // Name the taint that reserves it.
        return record.taints?.length > 0
          ? <SimpleTooltip title={record.taints.join(" · ")}><span>{badge}</span></SimpleTooltip>
          : badge;
      },
    },
    {
      key: "cpu",
      title: i18next.t("device:CPU free"),
      minWidth: 150,
      render: (_value, record) => (
        <CapacityCell
          percent={record.cpuPercent ?? 0}
          free={i18next.t("device:{{cores}} cores", {cores: formatCores(record.cpuFreeM ?? 0)})}
          total={formatCores(record.cpuTotalM ?? 0)}
          usage={record.usageKnown
            ? i18next.t("device:in use now: {{amount}}", {amount: formatCores(record.cpuUsedM ?? 0)})
            : ""}
        />
      ),
    },
    {
      key: "memory",
      title: i18next.t("device:Memory free"),
      minWidth: 150,
      render: (_value, record) => (
        <CapacityCell
          percent={record.memPercent ?? 0}
          free={formatMiB(record.memFreeMi ?? 0)}
          total={formatMiB(record.memTotalMi ?? 0)}
          usage={record.usageKnown
            ? i18next.t("device:in use now: {{amount}}", {amount: formatMiB(record.memUsedMi ?? 0)})
            : ""}
        />
      ),
    },
    ...(hasGpus
      ? [{
        key: "gpu",
        title: i18next.t("device:GPUs free"),
        width: 170,
        sortable: true,
        dataIndex: "gpuFree",
        render: (_value, record) => {
          if (!record.gpuTotal) {
            return <span className="text-muted-foreground text-xs">—</span>;
          }
          const model = (record.gpus ?? []).map((gpu) => gpu.product || gpu.resource).join(", ");
          return (
            <div className="grid gap-0.5">
              <Badge variant={record.gpuFree > 0 ? "success" : "muted"} className="w-fit tabular-nums">
                {record.gpuFree} / {record.gpuTotal}
              </Badge>
              <span className="text-muted-foreground truncate text-[11px]">{model}</span>
            </div>
          );
        },
      }]
      : []),
    {
      key: "pods",
      title: i18next.t("general:Pods"),
      dataIndex: "pods",
      width: 90,
      sortable: true,
      render: (value) => <span className="tabular-nums">{value ?? 0}</span>,
    },
  ];

  return (
    <PageContainer>
      <PageHeader
        title={i18next.t("device:Idle Devices")}
        description={i18next.t("device:What is free right now, freest first — room for the next job, and which GPUs nobody has claimed.")}
        actions={
          <Button variant="outline" onClick={() => refresh()}>
            {i18next.t("general:Refresh")}
          </Button>
        }
      />

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <StatCard
          label={i18next.t("device:Devices free")}
          value={totals.free ?? 0}
          suffix={`/ ${totals.devices ?? 0}`}
          icon={Server}
          tone={(totals.free ?? 0) > 0 ? "success" : "warning"}
          hint={i18next.t("device:Ready, schedulable and mostly uncommitted.")}
        />
        <StatCard
          label={i18next.t("device:CPU free")}
          value={formatCores(totals.cpuFreeM ?? 0)}
          suffix={i18next.t("general:cores")}
          icon={Cpu}
          tone="info"
          hint={i18next.t("device:Counted on free devices only.")}
        />
        <StatCard
          label={i18next.t("device:Memory free")}
          value={formatMiB(totals.memFreeMi ?? 0)}
          icon={HardDrive}
          tone="info"
          hint={i18next.t("device:Counted on free devices only.")}
        />
        <StatCard
          label={i18next.t("device:GPUs free")}
          value={totals.gpuFree ?? 0}
          suffix={`/ ${totals.gpuTotal ?? 0}`}
          icon={Zap}
          tone={(totals.gpuFree ?? 0) > 0 ? "success" : "default"}
          hint={hasGpus
            ? i18next.t("device:Unclaimed accelerators across the cluster.")
            : i18next.t("device:No accelerators reported by any node.")}
        />
      </div>

      {!loading && !report?.usageKnown && devices.length > 0 ? (
        <MessageAlert
          variant="info"
          title={i18next.t("device:No kubelet answered for live usage, so only committed capacity is shown.")}
        />
      ) : null}

      <DataTable
        testId="device-table"
        columns={columns}
        dataSource={devices}
        rowKey={(record) => record.name}
        loading={loading}
        searchable
        emptyIcon={Server}
        emptyText={i18next.t("device:No devices found. Add a node and it shows up here.")}
      />
    </PageContainer>
  );
}

export default IdleDevicesPage;
