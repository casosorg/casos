package server

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/sirupsen/logrus"
	schedapp "k8s.io/kubernetes/cmd/kube-scheduler/app"
)

// StartScheduler launches kube-scheduler in-process. Must be called after the
// apiserver is ready (i.e. after the readyCh from Start is closed).
func StartScheduler(ctx context.Context, cfg Config) error {
	certDir := filepath.Join(cfg.DataDir, "tls")
	kubeconfigPath, err := ensureComponentKubeconfig(
		certDir,
		fmt.Sprintf("https://127.0.0.1:%d", cfg.ApiserverPort),
		"scheduler",
	)
	if err != nil {
		return fmt.Errorf("scheduler kubeconfig: %w", err)
	}

	go func() {
		cmd := schedapp.NewSchedulerCommand(ctx.Done())
		cmd.SetArgs([]string{
			"--kubeconfig=" + kubeconfigPath,
			"--leader-elect=false",
			"--bind-address=127.0.0.1",
			"--secure-port=10259",
		})
		if err := cmd.ExecuteContext(ctx); err != nil && ctx.Err() == nil {
			logrus.Errorf("scheduler exited: %v", err)
		}
	}()

	logrus.Info("scheduler started in-process")
	return nil
}
