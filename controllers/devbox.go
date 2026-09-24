package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"

	"github.com/casosorg/casos/object"
)

const (
	devboxLabel         = "casos.io/devbox"
	devboxDefaultImage  = "codercom/code-server:latest"
	devboxContainerPort = 8080
	devboxHomeMount     = "/home/coder"
	devboxDefaultDisk   = "5Gi"
	devboxHttpPortName  = "http"

	devboxSshPortName = "ssh"
	devboxSshPort     = 2222
	devboxSshUser     = "coder"
)

type deployDevboxRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// Any glibc image; the editor is injected. Empty means code-server's own.
	Image  string `json:"image"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	// Run once in the checkout before the editor first starts.
	Setup    string `json:"setup"`
	Password string `json:"password"`
	// "0" means no disk: a stateless box.
	DiskSize     string  `json:"diskSize"`
	SshPublicKey string  `json:"sshPublicKey"`
	CpuLimit     *string `json:"cpuLimit"`
	MemoryLimit  *string `json:"memoryLimit"`
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
	SshHost   string `json:"sshHost"`
	SshPort   int32  `json:"sshPort"`
	SshUser   string `json:"sshUser"`
	SshPath   string `json:"sshPath"`
	// Where the workspace opens and its runs start.
	Folder string `json:"folder"`
	Repo   string `json:"repo"`
	// As of the last start, read from the clone step's termination message.
	Branch      string `json:"branch"`
	Commit      string `json:"commit"`
	CloneError  string `json:"cloneError"`
	PrepareStep string `json:"prepareStep"`
	SetupFailed bool   `json:"setupFailed"`
	HasSetup    bool   `json:"hasSetup"`
	PodName     string `json:"podName"`
	// The latest run frozen from this box, so a frozen box doesn't look merely stopped.
	RunName   string `json:"runName"`
	RunStatus string `json:"runStatus"`
}

type deployDevboxResult struct {
	devboxSummary
	Password string `json:"password"`
}

func randomPassword() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "changeme"
	}
	return hex.EncodeToString(b)
}

// DeployDevbox
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

	env := devboxEnvironment{
		image:  image,
		repo:   strings.TrimSpace(req.Repo),
		branch: strings.TrimSpace(req.Branch),
		setup:  strings.TrimSpace(req.Setup),
		folder: devboxHomeMount,
	}
	if env.repo != "" {
		name, err := devboxRepoFolder(env.repo)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}
		env.folder = devboxHomeMount + "/" + name
	}

	env.sshPublicKey = strings.TrimSpace(req.SshPublicKey)
	if env.sshPublicKey != "" && !looksLikeSshPublicKey(env.sshPublicKey) {
		c.ResponseError("that does not look like an SSH public key — paste the contents of a .pub file, one line starting with ssh-ed25519 or ssh-rsa")
		return
	}

	ports := []appPortRequest{{
		Name:          devboxHttpPortName,
		ContainerPort: devboxContainerPort,
		Protocol:      "TCP",
	}}
	if env.sshPublicKey != "" {
		ports = append(ports, appPortRequest{
			Name:          devboxSshPortName,
			ContainerPort: devboxSshPort,
			Protocol:      "TCP",
		})
	}

	appReq := deployAppRequest{
		Namespace:   req.Namespace,
		Name:        req.Name,
		Image:       image,
		EnvVars:     []envVarRequest{{Name: "PASSWORD", Value: password}},
		Ports:       ports,
		Volumes:     volumes,
		ServiceType: "NodePort",
		resourceRequest: resourceRequest{
			CpuLimit:    req.CpuLimit,
			MemoryLimit: req.MemoryLimit,
		},
	}

	opts := workloadOptions{
		labels: map[string]string{devboxLabel: "true"},
		mutate: applyDevboxEnvironment(env),
	}
	if _, err := deployAppWorkload(cfg, appReq, opts); err != nil {
		c.ResponseError(err.Error())
		return
	}

	summary := devboxSummary{Name: req.Name, Namespace: req.Namespace, Image: image, Status: "pending"}
	if depl, err := object.GetDeployment(cfg, req.Namespace, req.Name); err == nil {
		summary = devboxSummaryOf(cfg, *depl, clusterNodeIP(cfg), nil)
	}

	c.ResponseOk(deployDevboxResult{devboxSummary: summary, Password: password})
}

// GetDevboxes
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

	latestRuns := latestDevboxRuns(cfg, namespace)
	podsByBox := map[string][]corev1.Pod{}
	if pods, err := object.GetPods(cfg, namespace); err == nil {
		for _, pod := range pods {
			if pod.Labels[devboxLabel] == "true" {
				key := pod.Namespace + "/" + pod.Labels[appInstanceLabel]
				podsByBox[key] = append(podsByBox[key], pod)
			}
		}
	}
	nodeIP := clusterNodeIP(cfg)
	result := []devboxSummary{}
	for _, d := range deployments {
		if d.Labels[devboxLabel] != "true" {
			continue
		}
		summary := devboxSummaryOf(cfg, d, nodeIP, podsByBox[d.Namespace+"/"+d.Name])
		if run, ok := latestRuns[d.Name]; ok {
			summary.RunName = run.Name
			summary.RunStatus = run.Status
		}
		result = append(result, summary)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt > result[j].CreatedAt })
	c.ResponseOk(result)
}

func devboxSummaryOf(cfg *rest.Config, d appsv1.Deployment, nodeIP string, pods []corev1.Pod) devboxSummary {
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
		Folder:    devboxFolder(d),
		Repo:      d.Annotations[devboxRepoAnnotation],
	}
	for _, init := range d.Spec.Template.Spec.InitContainers {
		if init.Name == devboxSetupInit {
			summary.HasSetup = true
		}
	}
	if status != "stopped" {
		state := devboxPodStateOf(pods)
		summary.PodName = state.podName
		summary.Branch = state.branch
		summary.Commit = state.commit
		summary.CloneError = state.cloneError
		summary.SetupFailed = state.setupFailed
		if status == "pending" {
			summary.PrepareStep = state.prepareStep
		}
	}
	if svc, err := object.GetService(cfg, d.Namespace, d.Name); err == nil {
		if host, port := devboxAddress(svc, nodeIP, devboxHttpPortName); host != "" {
			summary.Url = fmt.Sprintf("http://%s:%d", urlHost(host), port)
		}
		if host, port := devboxAddress(svc, nodeIP, devboxSshPortName); host != "" {
			summary.SshHost = host
			summary.SshPort = port
			summary.SshUser = devboxSshUser
			summary.SshPath = summary.Folder
		}
	}
	return summary
}

// Ports are matched by name: the API server does not keep their order.
func devboxAddress(svc *corev1.Service, nodeIP, portName string) (string, int32) {
	if svc == nil {
		return "", 0
	}
	for _, port := range svc.Spec.Ports {
		if port.Name != portName {
			continue
		}
		switch svc.Spec.Type {
		case corev1.ServiceTypeNodePort:
			if nodeIP != "" && port.NodePort != 0 {
				return nodeIP, port.NodePort
			}
		case corev1.ServiceTypeLoadBalancer:
			for _, ingress := range svc.Status.LoadBalancer.Ingress {
				host := ingress.IP
				if host == "" {
					host = ingress.Hostname
				}
				if host != "" {
					return host, port.Port
				}
			}
		default:
			if svc.Spec.ClusterIP != "" {
				return svc.Spec.ClusterIP, port.Port
			}
		}
	}
	return "", 0
}

func urlHost(host string) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]"
	}
	return host
}

func looksLikeSshPublicKey(key string) bool {
	if strings.ContainsAny(key, "\r\n") {
		return false
	}
	fields := strings.Fields(key)
	return len(fields) >= 2 && (strings.HasPrefix(fields[0], "ssh-") || strings.HasPrefix(fields[0], "ecdsa-") || strings.HasPrefix(fields[0], "sk-"))
}
