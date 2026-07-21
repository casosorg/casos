package deploy

import (
	"context"
	"fmt"
)

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

	d.logStep(nodeDeployPhaseConfiguring, "Configuring containerd")
	if err := runner.WriteFileContext(ctx, "/etc/containerd/config.toml", GenerateContainerdConfig(d.config.SandboxImage, d.config.Socks5Proxy), "0644"); err != nil {
		return fmt.Errorf("write /etc/containerd/config.toml: %w", err)
	}
	if d.config.Socks5Proxy != "" {
		if err := runner.WriteFileContext(ctx, "/etc/containerd/certs.d/docker.io/hosts.toml", GenerateDockerHubHostsToml(), "0644"); err != nil {
			return fmt.Errorf("write /etc/containerd/certs.d/docker.io/hosts.toml: %w", err)
		}
		if err := runner.WriteFileContext(ctx, "/etc/containerd/certs.d/registry.k8s.io/hosts.toml", GenerateK8sRegistryHostsToml(), "0644"); err != nil {
			return fmt.Errorf("write /etc/containerd/certs.d/registry.k8s.io/hosts.toml: %w", err)
		}
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

func (d *NodeDeployer) writeNodeFiles(ctx context.Context, runner *NodeDeploySSHRunner, nodeName, kubeconfig string) error {
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
