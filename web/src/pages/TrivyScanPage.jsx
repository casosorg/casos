import React, {useState} from "react";
import i18next from "i18next";
import {RefreshCw, RotateCw, ScanLine, ShieldCheck, Trash2} from "lucide-react";
import * as TrivyBackend from "@/backend/TrivyBackend";
import * as Setting from "@/Setting";
import {runAction, useResource} from "@/hooks/use-resource";
import {Badge} from "@/components/ui/badge";
import {Button} from "@/components/ui/button";
import {Input} from "@/components/ui/input";
import {SimpleTooltip} from "@/components/ui/tooltip";
import {DataTable} from "@/components/shared/data-table";
import {ConfirmDialog} from "@/components/shared/confirm-dialog";
import {Field, FormDialog} from "@/components/shared/form-dialog";
import {PageContainer} from "@/components/shared/page-header";
import {CodeText} from "@/components/shared/misc";

const SEVERITY_VARIANTS = {
  CRITICAL: "danger",
  HIGH: "warning",
  MEDIUM: "warning",
  LOW: "info",
  UNKNOWN: "muted",
};

// Counts are the point of this table, so a zero is deliberately quiet and any
// non-zero count is coloured by how much it should worry the reader.
function CountCell({count, variant}) {
  if (!count) {
    return <span className="text-muted-foreground tabular-nums">0</span>;
  }
  return (
    <Badge variant={variant} className="tabular-nums">
      {count > 999 ? "999+" : count}
    </Badge>
  );
}

function VulnerabilityDetail({record}) {
  if (!record.vulnerabilities || record.vulnerabilities.length === 0) {
    if (record.errorMsg) {
      return <p className="text-destructive p-4 text-sm">{record.errorMsg}</p>;
    }
    return <p className="text-muted-foreground p-4 text-sm">{i18next.t("trivy:No vulnerabilities found")}</p>;
  }

  const columns = [
    {
      key: "cve",
      title: "CVE ID",
      dataIndex: "VulnerabilityID",
      width: 190,
      render: (value) => (
        <a
          href={`https://nvd.nist.gov/vuln/detail/${value}`}
          target="_blank"
          rel="noreferrer"
          className="text-info text-xs hover:underline"
        >
          {value}
        </a>
      ),
    },
    {key: "pkg", title: i18next.t("trivy:Package"), dataIndex: "PkgName", width: 170, sortable: true},
    {key: "installed", title: i18next.t("trivy:Installed version"), dataIndex: "InstalledVersion", width: 130},
    {
      key: "fixed",
      title: i18next.t("trivy:Fixed In"),
      dataIndex: "FixedVersion",
      width: 130,
      render: (value) => value || <span className="text-muted-foreground">—</span>,
    },
    {
      key: "severity",
      title: i18next.t("trivy:Severity"),
      dataIndex: "Severity",
      width: 120,
      sortable: true,
      render: (value) => <Badge variant={SEVERITY_VARIANTS[value] ?? "muted"}>{value}</Badge>,
    },
    {key: "title", title: i18next.t("trivy:Title"), dataIndex: "Title", ellipsis: true},
  ];

  return (
    <div className="p-3">
      <DataTable
        columns={columns}
        dataSource={record.vulnerabilities}
        rowKey="VulnerabilityID"
        pageSize={10}
        dense
        className="shadow-none"
      />
    </div>
  );
}

function TrivyScanPage() {
  const {data: results, loading, refresh} = useResource(() => TrivyBackend.getTrivyScanResults(), [], {initialData: []});

  const [dialogOpen, setDialogOpen] = useState(false);
  const [image, setImage] = useState("");
  const [imageError, setImageError] = useState("");
  const [scanning, setScanning] = useState(false);

  async function handleScan() {
    if (!image.trim()) {
      setImageError(i18next.t("trivy:Image required"));
      return;
    }
    setImageError("");
    setScanning(true);
    const ok = await runAction(TrivyBackend.triggerTrivyScan(image.trim()), {
      successMessage: i18next.t("trivy:Scan complete"),
    });
    setScanning(false);
    if (ok) {
      setDialogOpen(false);
      setImage("");
      refresh();
    }
  }

  async function handleDelete(id) {
    const ok = await runAction(TrivyBackend.deleteTrivyScanResult(id), {
      successMessage: i18next.t("general:Successfully deleted"),
    });
    if (ok) {
      refresh();
    }
  }

  // A rescan is a delete followed by a fresh scan: the backend keys results by
  // image, so leaving the failed row in place would just be overwritten anyway.
  async function handleRescan(record) {
    try {
      await TrivyBackend.deleteTrivyScanResult(record.id);
      const res = await TrivyBackend.triggerTrivyScan(record.image);
      if (res.status === "ok") {
        Setting.showMessage("success", i18next.t("trivy:Scan complete"));
      } else {
        Setting.showMessage("error", res.msg);
      }
    } catch (error) {
      Setting.showMessage("error", error.message);
    } finally {
      refresh();
    }
  }

  const columns = [
    {
      key: "image",
      title: i18next.t("general:Image"),
      dataIndex: "image",
      sortable: true,
      render: (value) => <CodeText>{value}</CodeText>,
    },
    {
      key: "status",
      title: i18next.t("general:Status"),
      dataIndex: "status",
      width: 110,
      sortable: true,
      render: (value) => (
        <Badge variant={value === "done" ? "success" : value === "failed" ? "danger" : "info"}>{value ?? "pending"}</Badge>
      ),
    },
    {
      key: "critical",
      title: "CRITICAL",
      dataIndex: "critical",
      width: 110,
      align: "right",
      sortable: true,
      render: (value) => <CountCell count={value} variant="danger" />,
    },
    {
      key: "high",
      title: "HIGH",
      dataIndex: "high",
      width: 90,
      align: "right",
      sortable: true,
      render: (value) => <CountCell count={value} variant="warning" />,
    },
    {
      key: "medium",
      title: "MEDIUM",
      dataIndex: "medium",
      width: 100,
      align: "right",
      sortable: true,
      render: (value) => <CountCell count={value} variant="warning" />,
    },
    {
      key: "low",
      title: "LOW",
      dataIndex: "low",
      width: 90,
      align: "right",
      sortable: true,
      render: (value) => <CountCell count={value} variant="info" />,
    },
    {
      key: "scannedAt",
      title: i18next.t("trivy:Scanned At"),
      dataIndex: "scannedAt",
      width: 190,
      sortable: true,
      render: (value) => (value ? Setting.getFormattedDate(value) : "—"),
    },
    {
      key: "action",
      title: i18next.t("general:Action"),
      width: 120,
      align: "right",
      render: (_, record) => (
        <div className="flex justify-end gap-1">
          {record.status === "failed" ? (
            <SimpleTooltip title={i18next.t("trivy:Rescan")}>
              <Button variant="ghost" size="icon-sm" onClick={() => handleRescan(record)}>
                <RotateCw className="size-4" />
              </Button>
            </SimpleTooltip>
          ) : null}
          <ConfirmDialog
            title={i18next.t("trivy:Delete this result?")}
            confirmText={i18next.t("general:OK")}
            cancelText={i18next.t("general:Cancel")}
            onConfirm={() => handleDelete(record.id)}
          >
            <Button variant="ghost" size="icon-sm" className="text-muted-foreground hover:text-destructive">
              <Trash2 className="size-4" />
            </Button>
          </ConfirmDialog>
        </div>
      ),
    },
  ];

  return (
    <PageContainer>
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <p className="text-muted-foreground flex items-center gap-2 text-sm">
          <ShieldCheck className="size-4" />
          {i18next.t("trivy:page desc")}
        </p>
        <div className="flex items-center gap-2">
          <SimpleTooltip title={i18next.t("trivy:Refresh tooltip")}>
            <Button variant="outline" size="sm" onClick={() => refresh()} loading={loading}>
              <RefreshCw />
              {i18next.t("general:Refresh")}
            </Button>
          </SimpleTooltip>
          <Button
            size="sm"
            onClick={() => {
              setImage("");
              setImageError("");
              setDialogOpen(true);
            }}
          >
            <ScanLine />
            {i18next.t("trivy:Scan Image")}
          </Button>
        </div>
      </div>

      <DataTable
        columns={columns}
        dataSource={results}
        rowKey="id"
        loading={loading}
        searchable
        emptyText="No scans yet"
        expandable={{
          rowExpandable: (record) => record.status === "done" || Boolean(record.errorMsg),
          expandedRowRender: (record) => <VulnerabilityDetail record={record} />,
        }}
      />

      <FormDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        title={i18next.t("trivy:Scan Image")}
        submitText={i18next.t("trivy:Start Scan")}
        cancelText={i18next.t("general:Cancel")}
        submitting={scanning}
        onSubmit={handleScan}
      >
        <Field
          label={i18next.t("trivy:Image name")}
          htmlFor="trivy-image"
          required
          error={imageError}
          hint={i18next.t("trivy:scan may take minutes")}
        >
          <Input
            id="trivy-image"
            value={image}
            onChange={(event) => setImage(event.target.value)}
            placeholder="e.g. nginx:1.25 or docker.io/library/nginx:latest"
            autoFocus
          />
        </Field>
      </FormDialog>
    </PageContainer>
  );
}

export default TrivyScanPage;
