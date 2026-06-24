package deploy

import (
	"context"
	"fmt"
)

func (d *NodeDeployer) startKubeProxy(ctx context.Context, runner *NodeDeploySSHRunner) error {
	d.logStep(nodeDeployPhaseStarting, "Starting kube-proxy")
	if _, err := runner.RunRootContext(ctx, "systemctl daemon-reload && systemctl enable kube-proxy && systemctl restart kube-proxy"); err != nil {
		return fmt.Errorf("start kube-proxy: %w", err)
	}
	return nil
}

func kubeProxyConfig() string {
	return fmt.Sprintf(`apiVersion: kubeproxy.config.k8s.io/v1alpha1
kind: KubeProxyConfiguration
clientConnection:
  kubeconfig: /etc/kubernetes/worker.kubeconfig
mode: iptables
clusterCIDR: %s
`, nodeDeployClusterCIDR)
}

func kubeProxyService() string {
	return `[Unit]
Description=Kubernetes Kube-Proxy
After=network.target

[Service]
ExecStart=/usr/local/bin/kube-proxy \
  --config=/var/lib/kube-proxy/config.yaml \
  --v=2
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`
}
