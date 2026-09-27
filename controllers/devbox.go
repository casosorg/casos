package controllers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
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

	devboxSshPortName = "ssh"
	devboxSshPort     = 2222
	devboxSshUser     = "coder"
	devboxSshKeyEnv   = "CASOS_SSH_PUBLIC_KEY"
	devboxSshVolume   = "casos-ssh"
	// Pinned: the copy script names files inside this image. The scripts
	// below also spell out /opt/casos-ssh and /home/coder literally.
	devboxSshImage = "lscr.io/linuxserver/openssh-server:10.3_p1-r1-ls237"
	devboxSshTools = "/opt/casos-ssh"

	// A first pull of a CUDA or data-science image easily takes longer than
	// the default ten minutes, which would mark a box that is still starting
	// as failed.
	devboxProgressDeadline = int32(60 * 60)
)

// sshd runs inside the workspace container, so a Remote-SSH session gets the
// image's own shell and tools. It is Alpine's build, carried in with its musl
// loader and libraries, so it runs whatever libc the workspace image uses.
const devboxSshCopyScript = `set -e
d=/opt/casos-ssh
mkdir -p "$d/lib"
cp /lib/ld-musl-*.so.1 "$d/ld-musl"
cp /usr/sbin/sshd.pam "$d/sshd.bin"
cp /usr/lib/ssh/sshd-session.pam "$d/sshd-session.bin"
cp /usr/lib/ssh/sshd-auth.pam "$d/sshd-auth.bin"
cp /usr/lib/ssh/sftp-server "$d/sftp-server.bin"
cp /usr/bin/ssh-keygen "$d/ssh-keygen.bin"
for bin in "$d"/*.bin; do
  ldd "$bin" | awk '$2 == "=>" && $3 !~ /ld-musl/ {print $3}'
done | sort -u | while read -r lib; do cp -L "$lib" "$d/lib/"; done
for bin in "$d"/*.bin; do
  printf '#!/bin/sh\nexec %s/ld-musl --library-path %s/lib %s "$@"\n' "$d" "$d" "$bin" > "${bin%.bin}"
  chmod 755 "${bin%.bin}"
done
`

// Runs as the workspace user from a postStart hook, which leaves the image's
// own entrypoint alone. It always exits 0: a broken sshd must not take the
// editor down with it. The host key lives on the home disk, so a restart does
// not trip a changed-host-key warning.
const devboxSshStartScript = `d=/opt/casos-ssh
s=/home/coder/.casos/ssh
mkdir -p "$s" && chmod 700 "$s" || exit 0
exec >"$s/start.log" 2>&1
[ -f "$s/host_ed25519" ] || "$d/ssh-keygen" -q -t ed25519 -N "" -f "$s/host_ed25519"
printf '%s\n' "$CASOS_SSH_PUBLIC_KEY" > "$s/authorized_keys"
export PATH="/home/coder/.local/bin:$PATH"
# sshd hands sessions a bare environment, and login shells reset PATH from
# /etc/profile, so the image's own (conda, CUDA) is replayed for both.
(unset PASSWORD CASOS_SSH_PUBLIC_KEY HOSTNAME PWD OLDPWD SHLVL; export -p) > "$s/env.sh"
p=/home/coder/.profile
if ! grep -qs '.casos/ssh/env.sh' "$p"; then
  { echo '[ -f "$HOME/.casos/ssh/env.sh" ] && . "$HOME/.casos/ssh/env.sh"'; cat "$p" 2>/dev/null; } > "$s/profile" && cat "$s/profile" > "$p"
fi
cat > "$s/sshd_config" <<CFG
Port 2222
HostKey $s/host_ed25519
AuthorizedKeysFile $s/authorized_keys
PidFile none
PasswordAuthentication no
KbdInteractiveAuthentication no
UsePAM no
StrictModes no
X11Forwarding no
SshdSessionPath $d/sshd-session
SshdAuthPath $d/sshd-auth
Subsystem sftp $d/sftp-server
CFG
# sshd honours only the first SetEnv line.
command -v awk >/dev/null && awk 'BEGIN {
  line = "SetEnv"
  for (name in ENVIRON) {
    value = ENVIRON[name]
    if (name !~ /^[A-Za-z_][A-Za-z0-9_]*$/ || name ~ /^(PASSWORD|CASOS_SSH_PUBLIC_KEY|HOSTNAME|PWD|OLDPWD|SHLVL|_)$/ || value ~ /[\n\\"]/) continue
    line = line " \"" name "=" value "\""
  }
  print line
}' >> "$s/sshd_config"
"$d/sshd" -f "$s/sshd_config" -E "$s/sshd.log"
exit 0
`

type deployDevboxRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// Image is the environment: any glibc image, as the editor is brought in.
	// Empty uses the code-server image itself.
	Image string `json:"image"`
	// Source is where the project comes from: "git" clones Repo, "local" copies
	// the Git repository at LocalPath on the machine running casos, "empty"
	// starts a new repository named after the box. Empty with no Repo opens the
	// bare home folder.
	Source string `json:"source"`
	// Repo is cloned into the home disk on first start, and opened as the workspace.
	Repo      string `json:"repo"`
	LocalPath string `json:"localPath"`
	Branch    string `json:"branch"`
	// Setup runs once in the checkout, in the image, before the editor starts.
	Setup string `json:"setup"`
	// Password guards the editor. Empty means "generate one", handed back once
	// so the user can copy it — it is never read back afterwards.
	Password string `json:"password"`
	// DiskSize is the home volume; empty uses the default, "0" keeps the box
	// stateless (no disk, so a restart is a clean slate).
	DiskSize string `json:"diskSize"`
	// SshPublicKeys, one per line, also open the box to Remote-SSH.
	SshPublicKeys string  `json:"sshPublicKeys"`
	CpuLimit      *string `json:"cpuLimit"`
	MemoryLimit   *string `json:"memoryLimit"`

	// Set only for a sandbox an AI agent asks for over MCP.
	expiresAt time.Time
	agent     string
}

type devboxSummary struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Image      string `json:"image"`
	Status     string `json:"status"`
	Replicas   int32  `json:"replicas"`
	Ready      int32  `json:"ready"`
	Url        string `json:"url"`
	CreatedAt  string `json:"createdAt"`
	SshHost    string `json:"sshHost"`
	SshPort    int32  `json:"sshPort"`
	SshUser    string `json:"sshUser"`
	SshPath    string `json:"sshPath"`
	ActiveRuns int    `json:"activeRuns"`
	Folder     string `json:"folder"`
	Source     string `json:"source"`
	Repo       string `json:"repo"`
	LocalRepo  string `json:"localRepo"`
	// Read back from the clone step of the current pod.
	Branch      string `json:"branch"`
	Commit      string `json:"commit"`
	CloneError  string `json:"cloneError"`
	SetupFailed bool   `json:"setupFailed"`
	// The init container a starting box is on.
	PrepareStep   string   `json:"prepareStep"`
	PodName       string   `json:"podName"`
	LogContainers []string `json:"logContainers"`
	// The access token of the agent that created the box, and when it is reclaimed.
	Agent     string `json:"agent"`
	ExpiresAt string `json:"expiresAt"`
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
	result, err := deployDevbox(cfg, req)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(result)
}

func deployDevbox(cfg *rest.Config, req deployDevboxRequest) (*deployDevboxResult, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("a name is required")
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

	sshKeys, err := normalizeSshPublicKeys(req.SshPublicKeys)
	if err != nil {
		return nil, err
	}

	env := devboxEnvironment{
		image:  image,
		source: strings.TrimSpace(req.Source),
		branch: strings.TrimSpace(req.Branch),
		setup:  strings.TrimSpace(req.Setup),
		folder: devboxHomeMount,
	}
	if env.source == "" && strings.TrimSpace(req.Repo) != "" {
		env.source = devboxSourceGit
	}
	switch env.source {
	case "":
	case devboxSourceGit:
		env.repo = strings.TrimSpace(req.Repo)
		if env.repo == "" {
			return nil, fmt.Errorf("a repository address is required")
		}
		name, err := devboxRepoFolder(env.repo)
		if err != nil {
			return nil, err
		}
		env.folder = devboxHomeMount + "/" + name
	case devboxSourceLocal:
		local, err := inspectLocalRepo(req.LocalPath)
		if err != nil {
			return nil, err
		}
		env.local = local
		env.folder = devboxHomeMount + "/" + local.folder
		if env.branch == "" {
			env.branch = local.branch
		}
	case devboxSourceEmpty:
		env.branch = ""
		env.folder = devboxHomeMount + "/" + req.Name
	default:
		return nil, fmt.Errorf("unknown project source %q", env.source)
	}

	ports := []appPortRequest{{Name: devboxHttpPortName, ContainerPort: devboxContainerPort, Protocol: "TCP"}}
	if sshKeys != "" {
		ports = append(ports, appPortRequest{Name: devboxSshPortName, ContainerPort: devboxSshPort, Protocol: "TCP"})
	}

	appReq := deployAppRequest{
		Namespace:   req.Namespace,
		Name:        req.Name,
		Image:       image,
		EnvVars:     devboxEnvVars(env, password, sshKeys),
		Ports:       ports,
		Volumes:     volumes,
		ServiceType: "NodePort",
		resourceRequest: resourceRequest{
			CpuLimit:    req.CpuLimit,
			MemoryLimit: req.MemoryLimit,
		},
		Command: []string{"/bin/sh", "-c", devboxEditorScript},
	}
	setUp := applyDevboxEnvironment(env, sshKeys != "")
	opts := workloadOptions{
		labels: map[string]string{devboxLabel: "true"},
		mutate: func(depl *appsv1.Deployment) error {
			if err := setUp(depl); err != nil {
				return err
			}
			deadline := devboxProgressDeadline
			depl.Spec.ProgressDeadlineSeconds = &deadline
			applyDevboxLease(&depl.ObjectMeta, req.expiresAt, req.agent)
			return nil
		},
	}

	if _, err := deployAppWorkload(cfg, appReq, opts); err != nil {
		return nil, err
	}

	summary := devboxSummary{Name: req.Name, Namespace: req.Namespace, Image: image, Status: "pending", Folder: env.folder, Source: env.source, Repo: env.repo, LocalRepo: env.local.root}
	if depl, err := object.GetDeployment(cfg, req.Namespace, req.Name); err == nil {
		summary = devboxSummaryOf(cfg, *depl, clusterNodeIP(cfg), nil)
	}
	return &deployDevboxResult{devboxSummary: summary, Password: password}, nil
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
	result, err := listDevboxes(cfg, c.GetString("namespace"))
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(result)
}

func listDevboxes(cfg *rest.Config, namespace string) ([]devboxSummary, error) {
	deployments, err := object.GetDeployments(cfg, namespace)
	if err != nil {
		return nil, err
	}

	nodeIP := clusterNodeIP(cfg)
	activeRuns := activeDevboxRuns(cfg, namespace)
	pods, _ := object.GetPods(cfg, namespace)
	result := []devboxSummary{}
	for _, d := range deployments {
		if d.Labels[devboxLabel] != "true" {
			continue
		}
		summary := devboxSummaryOf(cfg, d, nodeIP, pods)
		summary.ActiveRuns = activeRuns[d.Namespace+"/"+d.Name]
		result = append(result, summary)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt > result[j].CreatedAt })
	return result, nil
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
		Source:    d.Annotations[devboxSourceAnnotation],
		Repo:      d.Annotations[devboxRepoAnnotation],
		LocalRepo: d.Annotations[devboxLocalRepoAnnotation],
		Agent:     d.Annotations[devboxAgentAnnotation],
	}
	if summary.Source == "" && summary.Repo != "" {
		summary.Source = devboxSourceGit
	}
	if expiresAt, ok := devboxExpiry(d.ObjectMeta); ok {
		summary.ExpiresAt = expiresAt.Format(time.RFC3339)
	}
	if status != "stopped" {
		state := devboxPodStateOf(d, pods)
		summary.PodName = state.podName
		summary.Branch = state.branch
		summary.Commit = state.commit
		summary.CloneError = state.cloneError
		summary.SetupFailed = state.setupFailed
		summary.LogContainers = state.logContainers
		if status == "pending" {
			summary.PrepareStep = state.prepareStep
		}
	}
	if svc, err := object.GetService(cfg, d.Namespace, d.Name); err == nil {
		if host, port := servicePortAddress(svc, nodeIP, devboxHttpPortName); host != "" {
			summary.Url = fmt.Sprintf("http://%s:%d", urlHost(host), port)
		}
		if host, port := servicePortAddress(svc, nodeIP, devboxSshPortName); host != "" {
			summary.SshHost = host
			summary.SshPort = port
			summary.SshUser = devboxSshUser
			summary.SshPath = summary.Folder
		}
	}
	return summary
}

// servicePortAddress looks a port up by name, as the API server does not keep
// the order ports were declared in.
func servicePortAddress(svc *corev1.Service, nodeIP, name string) (string, int32) {
	for _, port := range svc.Spec.Ports {
		if port.Name != name {
			continue
		}
		switch svc.Spec.Type {
		case corev1.ServiceTypeNodePort:
			if nodeIP != "" && port.NodePort != 0 {
				return nodeIP, port.NodePort
			}
		case corev1.ServiceTypeLoadBalancer:
			for _, ingress := range svc.Status.LoadBalancer.Ingress {
				if ingress.IP != "" {
					return ingress.IP, port.Port
				}
				if ingress.Hostname != "" {
					return ingress.Hostname, port.Port
				}
			}
		default:
			if svc.Spec.ClusterIP != "" && svc.Spec.ClusterIP != corev1.ClusterIPNone {
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

// Each key is re-marshalled, so options such as command="..." and anything
// that is not a public key never reach authorized_keys.
func normalizeSshPublicKeys(text string) (string, error) {
	var keys []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			return "", fmt.Errorf("that is not an SSH public key: paste the contents of a .pub file, such as ~/.ssh/id_ed25519.pub, never the private key")
		}
		entry := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
		if comment = strings.Join(strings.Fields(comment), " "); comment != "" {
			entry += " " + comment
		}
		keys = append(keys, entry)
	}
	return strings.Join(keys, "\n"), nil
}

func addDevboxSshServer(depl *appsv1.Deployment) error {
	spec := &depl.Spec.Template.Spec
	if len(spec.Containers) == 0 {
		return fmt.Errorf("the workspace has no container to run SSH in")
	}
	editor := &spec.Containers[0]
	tools := corev1.VolumeMount{Name: devboxSshVolume, MountPath: devboxSshTools}

	spec.Volumes = append(spec.Volumes, corev1.Volume{
		Name:         devboxSshVolume,
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	})
	spec.InitContainers = append(spec.InitContainers, corev1.Container{
		Name:         "ssh-tools",
		Image:        devboxSshImage,
		Command:      []string{"/bin/sh", "-c", devboxSshCopyScript},
		VolumeMounts: []corev1.VolumeMount{tools},
		// Same as the editor, so a quota that requires limits still admits the pod.
		Resources: editor.Resources,
	})
	tools.ReadOnly = true
	editor.VolumeMounts = append(editor.VolumeMounts, tools)
	editor.Lifecycle = &corev1.Lifecycle{
		PostStart: &corev1.LifecycleHandler{
			Exec: &corev1.ExecAction{Command: []string{"/bin/sh", "-c", devboxSshStartScript}},
		},
	}
	return nil
}
