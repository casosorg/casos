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
	// code-server binds this port and reads PASSWORD from the environment.
	devboxContainerPort = 8080
	devboxHomeMount     = "/home/coder"
	devboxDefaultDisk   = "5Gi"
	devboxHttpPortName  = "http"

	// A DevBox that is handed a public key also answers SSH, so a desktop VS
	// Code can open the same workspace over Remote-SSH. The server runs inside
	// the workspace container (see devbox_env.go), so a terminal opened that way
	// is in the workspace's image, not in some other container beside it.
	devboxSshPortName = "ssh"
	devboxSshPort     = 2222
	devboxSshUser     = "coder"
)

type deployDevboxRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// Image is the environment to work in. It need not contain code-server —
	// the editor is brought in beside it — so python, pytorch or a lab's own
	// image all work; empty uses the code-server image on its own.
	Image string `json:"image"`
	// Repo is cloned into the home disk on first start and opened as the
	// workspace; Branch picks one other than the default.
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	// Setup is a shell script run once, in the checkout, before the editor
	// first starts — installing requirements, say.
	Setup string `json:"setup"`
	// Password guards the editor. Empty means "generate one", handed back once
	// so the user can copy it — it is never read back afterwards.
	Password string `json:"password"`
	// DiskSize is the home volume; empty uses the default, "0" keeps the box
	// stateless (no disk, so a restart is a clean slate).
	DiskSize string `json:"diskSize"`
	// SshPublicKey, when given, adds an SSH server to the box and authorizes
	// this key on it, which is what a desktop VS Code connects over. Empty
	// leaves the box browser-only.
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
	// Where a desktop VS Code connects, when the box was created with a key.
	// The client turns these into an ssh command, a config entry and a
	// vscode:// link — three spellings of the same address.
	SshHost string `json:"sshHost"`
	SshPort int32  `json:"sshPort"`
	SshUser string `json:"sshUser"`
	// SshPath is the folder to open on the far side.
	SshPath string `json:"sshPath"`
	// Folder is where the workspace opens and where its runs start.
	Folder string `json:"folder"`
	Repo   string `json:"repo"`
	// Where the checkout stands as of the last start, which is as much as the
	// cluster can tell without reaching into the disk.
	Branch     string `json:"branch"`
	Commit     string `json:"commit"`
	CloneError string `json:"cloneError"`
	// PrepareStep is the init step a starting workspace is on — editor, clone
	// or setup — and SetupFailed says the setup script did not finish; the
	// editor starts either way, so the user can see why and fix it.
	PrepareStep string `json:"prepareStep"`
	SetupFailed bool   `json:"setupFailed"`
	HasSetup    bool   `json:"hasSetup"`
	PodName     string `json:"podName"`
	// The run this workspace was last frozen for, if it has one: a frozen box
	// with nothing queued is just a stopped box, and the two should not look
	// the same in a list.
	RunName   string `json:"runName"`
	RunStatus string `json:"runStatus"`
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

// devboxAddress is where one of a box's named ports is reachable from outside
// the cluster. A DevBox carries two of them — the editor and, when it has one,
// SSH — so they are looked up by name rather than by position: a service's
// ports come back in whatever order the API server kept them.
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
