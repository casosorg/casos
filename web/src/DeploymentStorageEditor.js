import React from "react";
import {Button, Input, Space, Tag, Tooltip} from "antd";
import {HddOutlined, MinusCircleOutlined, PlusOutlined} from "@ant-design/icons";

// Add mode:  value = [{mountPath, size}],  onChange receives updated array
// Edit mode: value = [{claimName, mountPath}], read-only display
function DeploymentStorageEditor({mode, value = [], onChange}) {
  if (mode !== "add") {
    return (
      <div>
        {value.length === 0 ? (
          <span style={{color: "#8c8c8c", fontSize: 12}}>No persistent storage attached.</span>
        ) : (
          <Space wrap>
            {value.map((v, i) => (
              <Tooltip key={i} title={`PVC: ${v.claimName}`}>
                <Tag icon={<HddOutlined />} color="geekblue">{v.mountPath}</Tag>
              </Tooltip>
            ))}
          </Space>
        )}
        <div style={{color: "#8c8c8c", fontSize: 12, marginTop: 4}}>
          Storage mounts cannot be changed after creation.
        </div>
      </div>
    );
  }

  const update = (i, field, val) => {
    const next = [...value];
    next[i] = {...next[i], [field]: val};
    onChange(next);
  };

  return (
    <div>
      {value.map((v, i) => (
        <div key={i} style={{display: "flex", gap: 8, marginBottom: 8, alignItems: "center"}}>
          <Input
            placeholder="Mount path, e.g. /data"
            value={v.mountPath}
            style={{flex: 2}}
            onChange={e => update(i, "mountPath", e.target.value)}
          />
          <Input
            placeholder="Size, e.g. 1Gi"
            value={v.size}
            style={{flex: 1}}
            onChange={e => update(i, "size", e.target.value)}
          />
          <MinusCircleOutlined
            style={{color: "#ff4d4f", fontSize: 16, cursor: "pointer", flexShrink: 0}}
            onClick={() => onChange(value.filter((_, j) => j !== i))}
          />
        </div>
      ))}
      <Button size="small" icon={<PlusOutlined />} onClick={() => onChange([...value, {mountPath: "", size: "1Gi"}])}>
        Add Volume
      </Button>
      {value.length === 0 && (
        <div style={{color: "#8c8c8c", fontSize: 12, marginTop: 4}}>
          No persistent storage. Data will be lost when the container restarts.
        </div>
      )}
    </div>
  );
}

export default DeploymentStorageEditor;
