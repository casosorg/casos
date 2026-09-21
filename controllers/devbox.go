package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/client-go/rest"

	"github.com/casosorg/casos/object"
)

// A DevBox is the launchpad's deploy pipeline with one preset: a container that
// serves VS Code (code-server) in the browser, reachable over a NodePort, its
// home directory kept on a disk that survives a restart. It is the thinnest
// shape of the "connect an editor to a reproducible environment" workflow —
// nothing here is new machinery, only defaults chosen so a user who does not
// know Kubernetes gets a working editor from one form.
//
// Because a DevBox is an ordinary image app underneath, its list is the only
// endpoint it needs of its own: starting, stopping and deleting one reuse the
// image-app endpoints, and the launchpad shows it alongside everything else.

const (
	devboxLabel        = "casos.io/devbox"
	devboxDefaultImage = "codercom/code-server:latest"
	// code-server binds this port and reads PASSWORD from the environment; the
	// official image already serves on 0.0.0.0:8080, so no command is needed.
	devboxContainerPort = 8080
	devboxHomeMount     = "/home/coder"
	devboxDefaultDisk   = "5Gi"
)

type deployDevboxRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// Image overrides the code-server image, for a lab that keeps its own or a
	// pinned digest; empty uses the default.
	Image string `json:"image"`
	// Password guards the editor. Empty means "generate one", handed back once
	// so the user can copy it — it is never read back afterwards.
	Password string `json:"password"`
	// DiskSize is the home volume; empty uses the default, "0" keeps the box
	// stateless (no disk, so a restart is a clean slate).
	DiskSize    string  `json:"diskSize"`
	CpuLimit    *string `json:"cpuLimit"`
	MemoryLimit *string `json:"memoryLimit"`
}

type devboxSummary struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Image     string `json:"image"`
	Status    string `json:"status"`
	Replicas  int32  `json:"replicas"`
	Ready     int32  `json:"ready"`
	Url       string `json:"url"`
	CreatedAt string `json:"createdAt"`
}

type deployDevboxResult struct {
	devboxSummary
	// Password is returned only on create, so the UI can show it once.
	Password string `json:"password"`
}

func randomPassword() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "changeme"
	}
	return hex.EncodeToString(b)
}

// DeployDevbox stands up a code-server workspace and hands back where to reach
// it and the password to get in.
// @router /api/deploy-devbox [post]
func (c *ApiController) DeployDevbox() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req deployDevboxRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		c.ResponseError("a name is required")
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}

	image := strings.TrimSpace(req.Image)
	if image == "" {
		image = devboxDefaultImage
	}
	password := strings.TrimSpace(req.Password)
	if password == "" {
		password = randomPassword()
	}

	var volumes []volumeRequest
	if size := strings.TrimSpace(req.DiskSize); size != "0" {
		if size == "" {
			size = devboxDefaultDisk
		}
		volumes = []volumeRequest{{MountPath: devboxHomeMount, Size: size}}
	}

	appReq := deployAppRequest{
		Namespace: req.Namespace,
		Name:      req.Name,
		Image:     image,
		EnvVars:   []envVarRequest{{Name: "PASSWORD", Value: password}},
		Ports: []appPortRequest{{
			Name:          "http",
			ContainerPort: devboxContainerPort,
			Protocol:      "TCP",
		}},
		Volumes:     volumes,
		ServiceType: "NodePort",
		resourceRequest: resourceRequest{
			CpuLimit:    req.CpuLimit,
			MemoryLimit: req.MemoryLimit,
		},
	}

	if _, err := deployAppWorkload(cfg, appReq, map[string]string{devboxLabel: "true"}); err != nil {
		c.ResponseError(err.Error())
		return
	}

	summary := devboxSummary{Name: req.Name, Namespace: req.Namespace, Image: image, Status: "pending"}
	if depl, err := object.GetDeployment(cfg, req.Namespace, req.Name); err == nil {
		summary = devboxSummaryOf(cfg, *depl, clusterNodeIP(cfg))
	}

	c.ResponseOk(deployDevboxResult{devboxSummary: summary, Password: password})
}

// GetDevboxes lists the code-server workspaces, newest-looking first, with the
// address each answers on so the UI can offer an "Open" straight from the row.
// @router /api/get-devboxes [get]
func (c *ApiController) GetDevboxes() {
	if c.RequireSignedIn() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")

	deployments, err := object.GetDeployments(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	nodeIP := clusterNodeIP(cfg)
	result := []devboxSummary{}
	for _, d := range deployments {
		if d.Labels[devboxLabel] != "true" {
			continue
		}
		result = append(result, devboxSummaryOf(cfg, d, nodeIP))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt > result[j].CreatedAt })
	c.ResponseOk(result)
}

func devboxSummaryOf(cfg *rest.Config, d appsv1.Deployment, nodeIP string) devboxSummary {
	status, _ := deploymentAppStatus(d)
	replicas := int32(0)
	if d.Spec.Replicas != nil {
		replicas = *d.Spec.Replicas
	}
	summary := devboxSummary{
		Name:      d.Name,
		Namespace: d.Namespace,
		Image:     imageOf(d),
		Status:    status,
		Replicas:  replicas,
		Ready:     d.Status.ReadyReplicas,
		CreatedAt: d.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
	}
	if svc, err := object.GetService(cfg, d.Namespace, d.Name); err == nil {
		summary.Url = firstUrl(appUrls(nil, svc, nodeIP))
	}
	return summary
}

func firstUrl(urls []string) string {
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}
