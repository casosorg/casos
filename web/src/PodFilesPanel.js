import React, {useCallback, useEffect, useState} from "react";
import {Alert, Breadcrumb, Button, Select, Space, Spin, Table, Upload} from "antd";
import {ArrowUpOutlined, DownloadOutlined, FileOutlined, FolderOutlined, LinkOutlined, ReloadOutlined, UploadOutlined} from "@ant-design/icons";
import * as PodBackend from "./backend/PodBackend";
import * as Setting from "./Setting";

function joinPath(...parts) {
  return ("/" + parts.join("/")).replace(/\/+/g, "/");
}

function parentPath(path) {
  const parts = path.replace(/\/+$/, "").split("/").filter(Boolean);
  parts.pop();
  return "/" + parts.join("/");
}

function formatSize(bytes, type) {
  if (type === "dir") {return "-";}
  if (bytes < 1024) {return `${bytes} B`;}
  if (bytes < 1024 * 1024) {return `${(bytes / 1024).toFixed(1)} KiB`;}
  return `${(bytes / 1024 / 1024).toFixed(1)} MiB`;
}

function PodFilesPanel({pod, active}) {
  const [container, setContainer] = useState("");
  const [currentPath, setCurrentPath] = useState("/");
  const [entries, setEntries] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [uploading, setUploading] = useState(false);

  const fetchDir = useCallback((ctr = container, dirPath = currentPath) => {
    if (!pod || !ctr) {return;}
    setLoading(true);
    setError(null);
    PodBackend.listPodFiles(pod.namespace, pod.name, ctr, dirPath).then(res => {
      if (res.status === "ok") {
        const sorted = (res.data || []).slice().sort((a, b) => {
          if (a.type === "dir" && b.type !== "dir") {return -1;}
          if (a.type !== "dir" && b.type === "dir") {return 1;}
          return a.name.localeCompare(b.name);
        });
        setEntries(sorted);
      } else {
        setError(res.msg);
      }
    }).catch(err => setError(err.message))
      .finally(() => setLoading(false));
  }, [container, currentPath, pod]);

  useEffect(() => {
    if (!active || !pod) {return;}
    const defaultContainer = pod.containers?.[0] || "";
    setContainer(defaultContainer);
    setCurrentPath("/");
    fetchDir(defaultContainer, "/");
  }, [active, pod]);

  function navigate(path) {
    setCurrentPath(path);
    fetchDir(container, path);
  }

  async function download(entry) {
    const filePath = joinPath(currentPath, entry.name);
    try {
      const res = await PodBackend.downloadPodFile(pod.namespace, pod.name, container, filePath);
      if (!res.ok) {
        Setting.showMessage("error", "Download failed");
        return;
      }
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = entry.name;
      link.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      Setting.showMessage("error", err.message);
    }
  }

  async function upload(file) {
    setUploading(true);
    try {
      const res = await PodBackend.uploadPodFile(pod.namespace, pod.name, container, currentPath, file);
      if (res.status === "ok") {
        Setting.showMessage("success", `Uploaded: ${res.data}`);
        fetchDir(container, currentPath);
      } else {
        Setting.showMessage("error", res.msg);
      }
    } catch (err) {
      Setting.showMessage("error", err.message);
    } finally {
      setUploading(false);
    }
    return false;
  }

  const pathParts = currentPath.replace(/\/+$/, "").split("/").filter(Boolean);
  const breadcrumbItems = [
    {title: <span style={{cursor: "pointer"}} onClick={() => navigate("/")}>/</span>},
    ...pathParts.map((part, index) => {
      const target = "/" + pathParts.slice(0, index + 1).join("/");
      return {
        title: index === pathParts.length - 1 ? part : <span style={{cursor: "pointer"}} onClick={() => navigate(target)}>{part}</span>,
      };
    }),
  ];
  const columns = [
    {
      title: "Name",
      dataIndex: "name",
      key: "name",
      ellipsis: true,
      render: (name, record) => {
        const icon = record.type === "dir" ? <FolderOutlined style={{color: "#faad14", marginRight: 8}} /> : record.type === "link" ? <LinkOutlined style={{color: "#52c41a", marginRight: 8}} /> : <FileOutlined style={{color: "#8c8c8c", marginRight: 8}} />;
        if (record.type === "dir") {
          return <span style={{cursor: "pointer", color: "#1677ff"}} onClick={() => navigate(joinPath(currentPath, name))}>{icon}{name}</span>;
        }
        return <span>{icon}{name}</span>;
      },
    },
    {title: "Size", dataIndex: "size", key: "size", width: 110, render: (value, record) => formatSize(value, record.type)},
    {title: "Modified", dataIndex: "modTime", key: "modTime", width: 180},
    {title: "", key: "actions", width: 110, render: (_, record) => record.type !== "dir" ? <Button size="small" icon={<DownloadOutlined />} onClick={() => download(record)}>Download</Button> : null},
  ];
  const containerOptions = (pod?.containers || []).map(item => ({label: item, value: item}));

  return (
    <Space direction="vertical" style={{width: "100%"}} size={12}>
      <Space wrap>
        {containerOptions.length > 1 && (
          <Select
            value={container}
            options={containerOptions}
            style={{width: 180}}
            onChange={value => {
              setContainer(value);
              setCurrentPath("/");
              fetchDir(value, "/");
            }}
          />
        )}
        <Button icon={<ArrowUpOutlined />} disabled={currentPath === "/"} onClick={() => navigate(parentPath(currentPath))} />
        <Breadcrumb items={breadcrumbItems} />
        <Upload beforeUpload={upload} showUploadList={false} disabled={uploading}>
          <Button icon={<UploadOutlined />} loading={uploading}>Upload here</Button>
        </Upload>
        <Button icon={<ReloadOutlined />} loading={loading} onClick={() => fetchDir()} />
      </Space>
      {error && <Alert type="error" showIcon message={error} />}
      <Spin spinning={loading}>
        <Table
          rowKey="name"
          columns={columns}
          dataSource={entries}
          size="middle"
          pagination={false}
          scroll={{y: 460}}
        />
      </Spin>
    </Space>
  );
}

export default PodFilesPanel;
