package deploy

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/casosorg/casos/server"
)

const nodeDeployResolverPath = "/etc/casos-resolv.conf"

const (
	dockerHubHostsPath   = "/etc/containerd/certs.d/docker.io/hosts.toml"
	k8sRegistryHostsPath = "/etc/containerd/certs.d/registry.k8s.io/hosts.toml"
)

type registryMirrorFileRunner interface {
	RunRootContext(ctx context.Context, command string) (string, error)
	WriteFileContext(ctx context.Context, path, content, mode string) error
}

type registryMirrorSelection struct {
	dockerHub bool
	k8s       bool
}

func (d *NodeDeployer) installNodeBinaries(ctx context.Context, runner *NodeDeploySSHRunner, arch, k8sVersion string) error {
	version := k8sVersion
	cniVersion := defaultNodeDeployCNIVersion

	d.logStep(nodeDeployPhaseInstalling, "Installing node dependencies and containerd")
	if _, err := runner.RunRootContext(ctx, "dpkg -s ca-certificates curl iptables socat conntrack ebtables ethtool kmod containerd >/dev/null 2>&1 || (apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl iptables socat conntrack ebtables ethtool kmod containerd)"); err != nil {
		return fmt.Errorf("install packages: %w", err)
	}
	if _, err := runner.RunRootContext(ctx, `set -e
install -d /etc/modules-load.d /etc/sysctl.d
printf '%s\n' overlay br_netfilter vxlan > /etc/modules-load.d/casos-kubernetes.conf
modprobe overlay
modprobe br_netfilter
modprobe vxlan
cat > /etc/sysctl.d/99-casos-kubernetes.conf <<'EOF'
net.bridge.bridge-nf-call-iptables = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward = 1
EOF
sysctl --system >/dev/null
test -e /proc/sys/net/bridge/bridge-nf-call-iptables`); err != nil {
		return fmt.Errorf("configure Kubernetes kernel networking: %w", err)
	}
	if _, err := runner.RunRootContext(ctx, fmt.Sprintf(`set -e
if systemctl is-active --quiet systemd-resolved 2>/dev/null; then
  for i in $(seq 1 30); do
    [ -f /run/systemd/resolve/resolv.conf ] && break
    sleep 1
  done
  test -f /run/systemd/resolve/resolv.conf
  resolver=/run/systemd/resolve/resolv.conf
else
  resolver=/etc/resolv.conf
fi
ln -sfn "$resolver" %[1]s
test -f %[1]s`, nodeDeployResolverPath)); err != nil {
		return fmt.Errorf("configure node resolver: %w", err)
	}

	d.logStep(nodeDeployPhaseConfiguring, "Configuring containerd")
	mirrors, err := d.resolveRegistryMirrors(ctx, runner)
	if err != nil {
		return err
	}
	if err := runner.WriteFileContext(ctx, "/etc/containerd/config.toml", GenerateContainerdConfig(d.config.SandboxImage), "0644"); err != nil {
		return fmt.Errorf("write /etc/containerd/config.toml: %w", err)
	}
	if err := d.reconcileRegistryMirrorFiles(ctx, runner, mirrors); err != nil {
		return err
	}
	if _, err := runner.RunRootContext(ctx, "systemctl enable --now containerd && systemctl restart containerd"); err != nil {
		return fmt.Errorf("start containerd: %w", err)
	}

	d.logStep(nodeDeployPhaseInstalling, "Ensuring upstream kubelet, kube-proxy, and CNI plugins")
	installCmd := fmt.Sprintf(`set -e
download() {
  url="$3"
  curl -fsSL --connect-timeout 20 --max-time 600 --retry 2 --retry-delay 5 --retry-connrefused "$@" || { echo "download failed: $url" >&2; exit 22; }
}
needs_kube_binary() {
  path="$1"
  if [ ! -x "$path" ]; then
    return 0
  fi
  "$path" --version 2>/dev/null | grep -Fq "Kubernetes %s" && return 1
  return 0
}
if needs_kube_binary /usr/local/bin/kubelet; then
  download -o /tmp/kubelet https://dl.k8s.io/release/%s/bin/linux/%s/kubelet
  install -o root -g root -m 0755 /tmp/kubelet /usr/local/bin/kubelet
fi
if needs_kube_binary /usr/local/bin/kube-proxy; then
  download -o /tmp/kube-proxy https://dl.k8s.io/release/%s/bin/linux/%s/kube-proxy
  install -o root -g root -m 0755 /tmp/kube-proxy /usr/local/bin/kube-proxy
fi
mkdir -p /opt/cni/bin /etc/cni/net.d
if [ ! -x /opt/cni/bin/bridge ] || [ ! -x /opt/cni/bin/loopback ] || [ ! -x /opt/cni/bin/portmap ]; then
  download -o /tmp/cni-plugins.tgz https://github.com/containernetworking/plugins/releases/download/%s/cni-plugins-linux-%s-%s.tgz
  tar -xzf /tmp/cni-plugins.tgz -C /opt/cni/bin
fi`, version, version, arch, version, arch, cniVersion, arch, cniVersion)
	if _, err := runner.RunRootContext(ctx, installCmd); err != nil {
		return fmt.Errorf("install node binaries: %w", err)
	}
	return nil
}

func (d *NodeDeployer) resolveRegistryMirrors(ctx context.Context, runner registryMirrorFileRunner) (registryMirrorSelection, error) {
	switch d.config.RegistryMirrorMode {
	case server.RegistryMirrorModeAlways:
		d.logStep(nodeDeployPhaseConfiguring, "Registry mirror mode always: enabling Docker Hub and registry.k8s.io mirrors")
		return registryMirrorSelection{dockerHub: true, k8s: true}, nil
	case server.RegistryMirrorModeNever:
		d.logStep(nodeDeployPhaseConfiguring, "Registry mirror mode never: disabling Docker Hub and registry.k8s.io mirrors")
		return registryMirrorSelection{}, nil
	case server.RegistryMirrorModeAuto:
		d.logStep(nodeDeployPhaseConfiguring, "Registry mirror mode auto: probing canonical registries from the target worker")
	default:
		return registryMirrorSelection{}, fmt.Errorf("unsupported registry mirror mode %q", d.config.RegistryMirrorMode)
	}

	dockerHubReachable, dockerHubDetail, err := probeCanonicalRegistry(ctx, runner, "https://registry-1.docker.io/v2/")
	if err != nil {
		return registryMirrorSelection{}, fmt.Errorf("probe Docker Hub from worker: %w", err)
	}
	d.logRegistryProbe("Docker Hub", dockerHubReachable, dockerHubDetail)

	k8sReachable, k8sDetail, err := probeCanonicalRegistry(ctx, runner, "https://registry.k8s.io/v2/")
	if err != nil {
		return registryMirrorSelection{}, fmt.Errorf("probe registry.k8s.io from worker: %w", err)
	}
	d.logRegistryProbe("registry.k8s.io", k8sReachable, k8sDetail)

	return registryMirrorSelection{dockerHub: !dockerHubReachable, k8s: !k8sReachable}, nil
}

func probeCanonicalRegistry(ctx context.Context, runner registryMirrorFileRunner, url string) (bool, string, error) {
	command := fmt.Sprintf(`if curl -sS --location --noproxy '*' --output /dev/null --connect-timeout 4 --max-time 8 %s 2>/dev/null; then
  printf reachable
else
  rc=$?
  printf 'unreachable:%%s' "$rc"
fi`, shellSingleQuote(url))
	output, err := runner.RunRootContext(ctx, command)
	if err != nil {
		return false, "", err
	}
	result := strings.TrimSpace(output)
	if result == "reachable" {
		return true, "HTTP response received", nil
	}
	if strings.HasPrefix(result, "unreachable:") {
		return false, "curl exit " + strings.TrimPrefix(result, "unreachable:"), nil
	}
	return false, "", fmt.Errorf("unexpected probe result %q", result)
}

func (d *NodeDeployer) logRegistryProbe(name string, reachable bool, detail string) {
	if reachable {
		d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("%s is reachable from the target worker (%s); mirror disabled", name, detail))
		return
	}
	d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("%s is unreachable from the target worker (%s); mirror enabled", name, detail))
}

func (d *NodeDeployer) reconcileRegistryMirrorFiles(ctx context.Context, runner registryMirrorFileRunner, selection registryMirrorSelection) error {
	targets := []struct {
		name          string
		path          string
		content       string
		legacyContent string
		enabled       bool
	}{
		{name: "Docker Hub", path: dockerHubHostsPath, content: GenerateDockerHubHostsToml(), legacyContent: legacyDockerHubHostsToml(), enabled: selection.dockerHub},
		{name: "registry.k8s.io", path: k8sRegistryHostsPath, content: GenerateK8sRegistryHostsToml(), legacyContent: legacyK8sRegistryHostsToml(), enabled: selection.k8s},
	}
	for _, target := range targets {
		action, err := reconcileRegistryMirrorFile(ctx, runner, target.path, target.content, target.legacyContent, target.enabled)
		if err != nil {
			return fmt.Errorf("reconcile %s registry mirror: %w", target.name, err)
		}
		switch action {
		case "written":
			d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("Wrote CasOS managed %s mirror configuration", target.name))
		case "removed-managed":
			d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("Removed CasOS managed %s mirror configuration", target.name))
		case "removed-legacy":
			d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("Removed legacy CasOS %s mirror configuration", target.name))
		case "preserved":
			d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("Preserved unmanaged %s registry configuration", target.name))
		case "absent":
			d.logStep(nodeDeployPhaseConfiguring, fmt.Sprintf("No CasOS managed %s mirror configuration to remove", target.name))
		default:
			return fmt.Errorf("reconcile %s registry mirror: unexpected action %q", target.name, action)
		}
	}
	return nil
}

func reconcileRegistryMirrorFile(ctx context.Context, runner registryMirrorFileRunner, path, content, legacyContent string, enabled bool) (string, error) {
	legacyDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(legacyContent)))
	if enabled {
		state, err := registryMirrorFileState(ctx, runner, path, legacyDigest)
		if err != nil {
			return "", err
		}
		if state == "unmanaged" {
			return "preserved", nil
		}
		if err := runner.WriteFileContext(ctx, path, content, "0644"); err != nil {
			return "", fmt.Errorf("write %s: %w", path, err)
		}
		return "written", nil
	}

	cleanupCommand := fmt.Sprintf(`set -e
path=%s
if [ -L "$path" ] || { [ -e "$path" ] && [ ! -f "$path" ]; }; then
  printf preserved
  exit 0
fi
if [ ! -e "$path" ]; then
  printf absent
  exit 0
fi
if [ "$(sed -n '1p' "$path")" = %s ]; then
  rm -f -- "$path"
  printf removed-managed
  exit 0
fi
actual_digest=$(sha256sum "$path")
actual_digest=${actual_digest%%%% *}
if [ "$actual_digest" = %s ]; then
  rm -f -- "$path"
  printf removed-legacy
  exit 0
fi
printf preserved`, shellSingleQuote(path), shellSingleQuote(generatedRegistryHostsMarker), shellSingleQuote(legacyDigest))
	action, err := runner.RunRootContext(ctx, cleanupCommand)
	if err != nil {
		return "", fmt.Errorf("clean up %s: %w", path, err)
	}
	return strings.TrimSpace(action), nil
}

func registryMirrorFileState(ctx context.Context, runner registryMirrorFileRunner, path, legacyDigest string) (string, error) {
	command := fmt.Sprintf(`set -e
path=%s
if [ -L "$path" ] || { [ -e "$path" ] && [ ! -f "$path" ]; }; then
  printf unmanaged
  exit 0
fi
if [ ! -e "$path" ]; then
  printf absent
  exit 0
fi
if [ "$(sed -n '1p' "$path")" = %s ]; then
  printf managed
  exit 0
fi
actual_digest=$(sha256sum "$path")
actual_digest=${actual_digest%%%% *}
if [ "$actual_digest" = %s ]; then
  printf legacy
  exit 0
fi
printf unmanaged`, shellSingleQuote(path), shellSingleQuote(generatedRegistryHostsMarker), shellSingleQuote(legacyDigest))
	state, err := runner.RunRootContext(ctx, command)
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", path, err)
	}
	state = strings.TrimSpace(state)
	switch state {
	case "absent", "managed", "legacy", "unmanaged":
		return state, nil
	default:
		return "", fmt.Errorf("inspect %s: unexpected state %q", path, state)
	}
}

func (d *NodeDeployer) writeNodeFiles(ctx context.Context, runner *NodeDeploySSHRunner, nodeName, kubeconfig, kubeletServerCert, kubeletServerKey string) error {
	ca, err := extractCertificateAuthority(kubeconfig)
	if err != nil {
		return err
	}
	if err = runner.WriteFileContext(ctx, "/etc/kubernetes/worker.kubeconfig", kubeconfig, "0600"); err != nil {
		return fmt.Errorf("write /etc/kubernetes/worker.kubeconfig: %w", err)
	}
	if err = runner.WriteFileContext(ctx, "/etc/kubernetes/ca.crt", ca, "0644"); err != nil {
		return fmt.Errorf("write /etc/kubernetes/ca.crt: %w", err)
	}
	if kubeletServerCert != "" && kubeletServerKey != "" {
		if err = runner.WriteFileContext(ctx, "/var/lib/kubelet/pki/kubelet-server.crt", kubeletServerCert, "0600"); err != nil {
			return fmt.Errorf("write kubelet server cert: %w", err)
		}
		if err = runner.WriteFileContext(ctx, "/var/lib/kubelet/pki/kubelet-server.key", kubeletServerKey, "0600"); err != nil {
			return fmt.Errorf("write kubelet server key: %w", err)
		}
	}
	if err = runner.WriteFileContext(ctx, "/var/lib/kubelet/config.yaml", kubeletConfig(), "0644"); err != nil {
		return fmt.Errorf("write /var/lib/kubelet/config.yaml: %w", err)
	}
	if err = runner.WriteFileContext(ctx, "/etc/systemd/system/kubelet.service", kubeletService(nodeName), "0644"); err != nil {
		return fmt.Errorf("write /etc/systemd/system/kubelet.service: %w", err)
	}
	if err = runner.WriteFileContext(ctx, "/var/lib/kube-proxy/config.yaml", kubeProxyConfig(), "0644"); err != nil {
		return fmt.Errorf("write /var/lib/kube-proxy/config.yaml: %w", err)
	}
	if err = runner.WriteFileContext(ctx, "/etc/systemd/system/kube-proxy.service", kubeProxyService(), "0644"); err != nil {
		return fmt.Errorf("write /etc/systemd/system/kube-proxy.service: %w", err)
	}
	return nil
}
