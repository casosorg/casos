package server

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/sirupsen/logrus"
	cmapp "k8s.io/kubernetes/cmd/kube-controller-manager/app"
)

// StartControllerManager launches kube-controller-manager in-process. Must be
// called after the apiserver is ready.
func StartControllerManager(ctx context.Context, cfg Config) error {
	certDir := filepath.Join(cfg.DataDir, "tls")
	kubeconfigPath, err := ensureComponentKubeconfig(
		certDir,
		fmt.Sprintf("https://127.0.0.1:%d", cfg.ApiserverPort),
		"controller-manager",
	)
	if err != nil {
		return fmt.Errorf("controller-manager kubeconfig: %w", err)
	}

	caKey := filepath.Join(certDir, "ca.key")
	caCrt := filepath.Join(certDir, "ca.crt")
	saKey := filepath.Join(certDir, "sa.key")

	go func() {
		cmd := cmapp.NewControllerManagerCommand()
		cmd.SetArgs([]string{
			"--kubeconfig=" + kubeconfigPath,
			"--leader-elect=false",
			"--bind-address=127.0.0.1",
			"--secure-port=10257",
			"--cluster-signing-cert-file=" + caCrt,
			"--cluster-signing-key-file=" + caKey,
			"--root-ca-file=" + caCrt,
			"--service-account-private-key-file=" + saKey,
			"--allocate-node-cidrs=true",
			"--cluster-cidr=10.244.0.0/16",
		})
		if err := cmd.ExecuteContext(ctx); err != nil && ctx.Err() == nil {
			logrus.Errorf("controller-manager exited: %v", err)
		}
	}()

	logrus.Info("controller-manager started in-process")
	return nil
}
