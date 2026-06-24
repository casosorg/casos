package controllers

import (
	"encoding/json"

	"github.com/casosorg/casos/deploy"
	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/server"
	corev1 "k8s.io/api/core/v1"
)

type nodeSummary struct {
	Name            string            `json:"name"`
	Status          string            `json:"status"`
	Roles           []string          `json:"roles"`
	Labels          map[string]string `json:"labels"`
	Unschedulable   bool              `json:"unschedulable"`
	KubeletVersion  string            `json:"kubeletVersion"`
	OS              string            `json:"os"`
	Arch            string            `json:"arch"`
	InternalIP      string            `json:"internalIP"`
	ExternalIP      string            `json:"externalIP"`
	CreatedAt       string            `json:"createdAt"`
	ResourceVersion string            `json:"resourceVersion"`
}

func toNodeSummary(n corev1.Node) nodeSummary {
	status := "Unknown"
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			if c.Status == corev1.ConditionTrue {
				status = "Ready"
			} else {
				status = "NotReady"
			}
		}
	}
	roles := []string{}
	for k := range n.Labels {
		if k == "node-role.kubernetes.io/control-plane" {
			roles = append(roles, "control-plane")
		} else if k == "node-role.kubernetes.io/worker" {
			roles = append(roles, "worker")
		}
	}
	if len(roles) == 0 {
		roles = append(roles, "worker")
	}
	var internalIP, externalIP string
	for _, addr := range n.Status.Addresses {
		switch addr.Type {
		case corev1.NodeInternalIP:
			internalIP = addr.Address
		case corev1.NodeExternalIP:
			externalIP = addr.Address
		}
	}

	return nodeSummary{
		Name:            n.Name,
		Status:          status,
		Roles:           roles,
		Labels:          n.Labels,
		Unschedulable:   n.Spec.Unschedulable,
		KubeletVersion:  n.Status.NodeInfo.KubeletVersion,
		OS:              n.Status.NodeInfo.OperatingSystem,
		Arch:            n.Status.NodeInfo.Architecture,
		InternalIP:      internalIP,
		ExternalIP:      externalIP,
		CreatedAt:       n.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		ResourceVersion: n.ResourceVersion,
	}
}

// GetNodes lists all nodes.
// @router /api/get-nodes [get]
func (c *ApiController) GetNodes() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	nodes, err := object.GetNodes(cfg)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	result := make([]nodeSummary, 0, len(nodes))
	for _, n := range nodes {
		result = append(result, toNodeSummary(n))
	}
	c.ResponseOk(result)
}

// GetNode returns a single node.
// @router /api/get-node [get]
func (c *ApiController) GetNode() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	name := c.GetString("name")
	node, err := object.GetNode(cfg, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toNodeSummary(*node))
}

type nodeRequest struct {
	Name            string            `json:"name"`
	Labels          map[string]string `json:"labels"`
	Unschedulable   bool              `json:"unschedulable"`
	ResourceVersion string            `json:"resourceVersion"`
}

// UpdateNode updates a node's labels and schedulability.
// @router /api/update-node [post]
func (c *ApiController) UpdateNode() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req nodeRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	existing, err := object.GetNode(cfg, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	existing.Labels = req.Labels
	existing.Spec.Unschedulable = req.Unschedulable
	existing.ResourceVersion = req.ResourceVersion
	updated, err := object.UpdateNode(cfg, existing)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(toNodeSummary(*updated))
}

// DeleteNode removes a node from the cluster (does not stop kubelet).
// @router /api/delete-node [post]
func (c *ApiController) DeleteNode() {
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req nodeRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if err := object.DeleteNode(cfg, req.Name); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}

// GetWorkerKubeconfig generates a signed node client certificate and returns a
// ready-to-use kubeconfig for kubelet. This is an operational helper, not a
// Node CRUD operation.
// @router /api/get-worker-kubeconfig [get]
func (c *ApiController) GetWorkerKubeconfig() {
	nodeName := c.GetString("nodeName")
	if nodeName == "" {
		nodeName = "wsl2-worker"
	}
	cfg := getServerConfig()
	if cfg == nil {
		c.ResponseError("server config not ready")
		return
	}
	wk, err := server.GenerateWorkerKubeconfig(*cfg, nodeName)
	if err != nil {
		c.ResponseError("generate worker kubeconfig: " + err.Error())
		return
	}
	c.ResponseOk(map[string]string{
		"nodeName":         wk.NodeName,
		"kubeconfig":       wk.Kubeconfig,
		"containerdConfig": deploy.GenerateContainerdConfig(cfg.SandboxImage, cfg.Socks5Proxy),
	})
}
