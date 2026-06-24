package deploy

import (
	"context"
	"fmt"
)

func (d *NodeDeployer) startKubelet(ctx context.Context, runner *NodeDeploySSHRunner) error {
	d.logStep(nodeDeployPhaseStarting, "Starting kubelet")
	if _, err := runner.RunRootContext(ctx, "systemctl daemon-reload && systemctl enable kubelet && systemctl restart kubelet"); err != nil {
		return fmt.Errorf("start kubelet: %w", err)
	}
	return nil
}

func kubeletConfig() string {
	return fmt.Sprintf(`apiVersion: kubelet.config.k8s.io/v1beta1
kind: KubeletConfiguration
cgroupDriver: systemd
failSwapOn: false
containerRuntimeEndpoint: unix:///run/containerd/containerd.sock
clusterDNS:
  - %s
clusterDomain: cluster.local
`, nodeDeployClusterDNS)
}

func kubeletService(nodeName string) string {
	return fmt.Sprintf(`[Unit]
Description=Kubernetes Kubelet
After=containerd.service
Requires=containerd.service

[Service]
ExecStart=/usr/local/bin/kubelet \
  --kubeconfig=/etc/kubernetes/worker.kubeconfig \
  --config=/var/lib/kubelet/config.yaml \
  --client-ca-file=/etc/kubernetes/ca.crt \
  --register-node=true \
  --hostname-override=%s \
  --v=2
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, nodeName)
}
