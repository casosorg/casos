package store

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const helmInstallPreflightTimeout = 5 * time.Second

// validateHelmInstallNodes checks the scheduler has at least one node on which
// a newly installed workload can be placed. Taints are intentionally left to
// the rendered Pod tolerations; this check only rejects an empty or fully
// cordoned cluster.
func validateHelmInstallNodes(ctx context.Context, client kubernetes.Interface) error {
	if client == nil {
		return fmt.Errorf("Helm install preflight Kubernetes client is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list Kubernetes nodes for Helm install preflight: %w", err)
	}
	if len(nodes.Items) == 0 {
		return fmt.Errorf("Helm install preflight failed: no Kubernetes nodes are registered; add at least one worker node before installing")
	}
	for _, node := range nodes.Items {
		if !node.Spec.Unschedulable {
			return nil
		}
	}
	return fmt.Errorf("Helm install preflight failed: all Kubernetes nodes are cordoned (unschedulable); uncordon at least one worker node before installing")
}

func validateHelmInstallPreflight(parent context.Context, cfg *rest.Config) error {
	if cfg == nil {
		return fmt.Errorf("Helm install preflight REST config is nil")
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, helmInstallPreflightTimeout)
	defer cancel()
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("create Kubernetes client for Helm install preflight: %w", err)
	}
	return validateHelmInstallNodes(ctx, client)
}
