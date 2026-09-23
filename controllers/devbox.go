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
	// code-server binds this port and reads PASSWORD from the environment; the
	// official image already serves on 0.0.0.0:8080, so no command is needed.
	devboxContainerPort = 8080
	devboxHomeMount     = "/home/coder"
	devboxDefaultDisk   = "5Gi"
	devboxHttpPortName  = "http"

	// A DevBox that is handed a public key also runs an SSH server beside
	// code-server, so a desktop VS Code can open the same files over Remote-SSH
	// — the same workspace, edited with the editor the user already has set up.
	// It is a sidecar rather than a second image because the two have to share
	// the home disk: what one writes, the other sees.
	devboxSshImage     = "lscr.io/linuxserver/openssh-server:latest"
	devboxSshPortName  = "ssh"
	devboxSshPort      = 2222
	devboxSshUser      = "coder"
	devboxSshHome      = "/config"
	devboxSshStatePath = "ssh-server"
	// The code-server image runs as uid 1000; the SSH user has to be the same
	// one, or files made over SSH would be unwritable in the browser.
	devboxUid = "1000"
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

	publicKey := strings.TrimSpace(req.SshPublicKey)
	if publicKey != "" && !looksLikeSshPublicKey(publicKey) {
		c.ResponseError("that does not look like an SSH public key — paste the contents of a .pub file, one line starting with ssh-ed25519 or ssh-rsa")
		return
	}

	ports := []appPortRequest{{
		Name:          devboxHttpPortName,
		ContainerPort: devboxContainerPort,
		Protocol:      "TCP",
	}}
	if publicKey != "" {
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

	opts := workloadOptions{labels: map[string]string{devboxLabel: "true"}}
	if publicKey != "" {
		opts.mutate = addDevboxSshSidecar(publicKey)
	}
	if _, err := deployAppWorkload(cfg, appReq, opts); err != nil {
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
		if host, port := devboxAddress(svc, nodeIP, devboxHttpPortName); host != "" {
			summary.Url = fmt.Sprintf("http://%s:%d", urlHost(host), port)
		}
		if host, port := devboxAddress(svc, nodeIP, devboxSshPortName); host != "" {
			summary.SshHost = host
			summary.SshPort = port
			summary.SshUser = devboxSshUser
			summary.SshPath = devboxHomeMount
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

// addDevboxSshSidecar puts an SSH server in the pod beside code-server, sharing
// the home disk so both editors see the same files.
func addDevboxSshSidecar(publicKey string) func(*appsv1.Deployment) error {
	return func(depl *appsv1.Deployment) error {
		spec := &depl.Spec.Template.Spec
		if len(spec.Containers) == 0 {
			return fmt.Errorf("the workspace has no container to attach SSH to")
		}
		editor := &spec.Containers[0]

		// SSH is answered by the sidecar, so the port is declared there; the
		// service's target port is a number and reaches it either way.
		kept := editor.Ports[:0]
		for _, port := range editor.Ports {
			if port.Name != devboxSshPortName {
				kept = append(kept, port)
			}
		}
		editor.Ports = kept

		sidecar := corev1.Container{
			Name:  "sshd",
			Image: devboxSshImage,
			Env: buildEnvVars([]envVarRequest{
				{Name: "PUBLIC_KEY", Value: publicKey},
				{Name: "USER_NAME", Value: devboxSshUser},
				{Name: "PUID", Value: devboxUid},
				{Name: "PGID", Value: devboxUid},
				{Name: "PASSWORD_ACCESS", Value: "false"},
				{Name: "SUDO_ACCESS", Value: "false"},
			}),
			Ports: []corev1.ContainerPort{{
				Name:          devboxSshPortName,
				ContainerPort: devboxSshPort,
				Protocol:      corev1.ProtocolTCP,
			}},
		}

		for _, mount := range editor.VolumeMounts {
			if mount.MountPath != devboxHomeMount {
				continue
			}
			// The home disk, twice: once at the path code-server uses, so both
			// editors open the same files, and once as the SSH user's own home in
			// a corner of it — that is where the host keys live, and where a
			// desktop VS Code installs its remote server, and both should survive
			// a restart rather than greet the user with a changed host key and a
			// fresh download. A stateless box has no disk and keeps neither.
			sidecar.VolumeMounts = []corev1.VolumeMount{
				mount,
				{Name: mount.Name, MountPath: devboxSshHome, SubPath: devboxSshStatePath},
			}
			break
		}

		spec.Containers = append(spec.Containers, sidecar)
		return nil
	}
}
