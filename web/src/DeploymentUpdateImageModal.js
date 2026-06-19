import React from "react";
import {Modal, Select, Tag} from "antd";
import * as PodBackend from "./backend/PodBackend";
import * as DeploymentBackend from "./backend/DeploymentBackend";
import * as Setting from "./Setting";

class DeploymentUpdateImageModal extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      tags: [],
      tagsLoading: false,
      selectedTag: null,
      submitting: false,
    };
  }

  componentDidUpdate(prevProps) {
    if (!prevProps.open && this.props.open && this.props.deploy) {
      const imageRepo = (this.props.deploy.image ?? "").split(":")[0];
      this.setState({tags: [], tagsLoading: true, selectedTag: null});
      PodBackend.getDockerHubImageTags(imageRepo)
        .then(res => {
          if (res.status === "ok") {
            this.setState({tags: res.data ?? []});
          } else {
            Setting.showMessage("error", res.msg);
          }
        })
        .catch(e => Setting.showMessage("error", e.message))
        .finally(() => this.setState({tagsLoading: false}));
    }
  }

  handleOk() {
    const {deploy, onClose, onUpdated} = this.props;
    const {selectedTag} = this.state;
    if (!selectedTag) {
      Setting.showMessage("error", "Please select a version");
      return;
    }
    const imageRepo = deploy.image.split(":")[0];
    const newImage = `${imageRepo}:${selectedTag}`;
    this.setState({submitting: true});
    DeploymentBackend.updateDeployment({...deploy, image: newImage})
      .then(res => {
        if (res.status === "ok") {
          Setting.showMessage("success", `Updated to ${newImage}`);
          onClose();
          onUpdated?.();
        } else {
          Setting.showMessage("error", res.msg);
        }
      })
      .catch(e => Setting.showMessage("error", e.message))
      .finally(() => this.setState({submitting: false}));
  }

  render() {
    const {deploy, open, onClose} = this.props;
    const {tags, tagsLoading, selectedTag, submitting} = this.state;

    const colonIdx = (deploy?.image ?? "").lastIndexOf(":");
    const repo = colonIdx > 0 ? deploy.image.slice(0, colonIdx) : (deploy?.image ?? "");
    const currentTag = colonIdx > 0 ? deploy.image.slice(colonIdx + 1) : "latest";

    return (
      <Modal
        title={deploy ? `Update Image — ${deploy.name}` : "Update Image"}
        open={open}
        onOk={() => this.handleOk()}
        onCancel={onClose}
        confirmLoading={submitting}
        okText="Update"
        width={480}
        destroyOnHidden
      >
        {deploy && (
          <div>
            <div style={{marginBottom: 12, fontSize: 13, color: "#595959"}}>
              Image: <b>{repo}</b> &nbsp; Current version: <Tag color="blue">{currentTag}</Tag>
            </div>
            <Select
              style={{width: "100%"}}
              placeholder="Select a version to update to"
              loading={tagsLoading}
              value={selectedTag}
              onChange={v => this.setState({selectedTag: v})}
              showSearch
              options={tags.map(t => ({
                label: (
                  <span>
                    {t}
                    {t === currentTag && <Tag color="blue" style={{marginLeft: 8}}>current</Tag>}
                  </span>
                ),
                value: t,
              }))}
              notFoundContent={tagsLoading ? "Loading…" : "No tags found"}
            />
          </div>
        )}
      </Modal>
    );
  }
}

export default DeploymentUpdateImageModal;
