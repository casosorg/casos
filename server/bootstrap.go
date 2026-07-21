package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	admissionregv1 "k8s.io/api/admissionregistration/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Bootstrap creates cluster-wide resources required for worker-node components
// to function correctly. It is idempotent — safe to call on every startup.
func Bootstrap(ctx context.Context, cfg *rest.Config, srvCfg Config) error {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("bootstrap client: %w", err)
	}
	// The webhook config carries the CA bundle the apiserver uses to trust the
	// admission server. It must be refreshed first: if it lags behind the CA on
	// disk every admission call fails TLS verification, so it cannot be gated
	// behind the workload steps below. Those steps run independently — a failure
	// in one must not skip the others.
	errs := []error{ensureCasbinWebhook(ctx, client, srvCfg)}
	errs = append(errs, ensureNodeProxierBinding(ctx, client))
	errs = append(errs, ensureFlannel(ctx, client, srvCfg))
	errs = append(errs, ensureClusterDNS(ctx, client, srvCfg))
	if srvCfg.StorageProvisionerEnabled {
		errs = append(errs, ensureDefaultStorageProvisioner(ctx, client, srvCfg))
	}
	return errors.Join(errs...)
}

// ensureCasbinWebhook registers the ValidatingWebhookConfiguration that routes
// admission requests to the casos Casbin enforcement server.
func ensureCasbinWebhook(ctx context.Context, client kubernetes.Interface, cfg Config) error {
	certDir := filepath.Join(cfg.DataDir, "tls")
	caData, err := os.ReadFile(filepath.Join(certDir, "ca.crt"))
	if err != nil {
		return fmt.Errorf("read CA for webhook: %w", err)
	}

	url := fmt.Sprintf("https://127.0.0.1:%d/admission/validate", cfg.WebhookPort)
	sideEffects := admissionregv1.SideEffectClassNone
	failurePolicy := admissionregv1.Ignore
	all := admissionregv1.AllScopes
	whConfig := &admissionregv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "casbin-admission"},
		Webhooks: []admissionregv1.ValidatingWebhook{
			{
				Name: "admission.casbin.io",
				ClientConfig: admissionregv1.WebhookClientConfig{
					URL:      &url,
					CABundle: caData,
				},
				Rules: []admissionregv1.RuleWithOperations{
					{
						Operations: []admissionregv1.OperationType{
							admissionregv1.OperationAll,
						},
						Rule: admissionregv1.Rule{
							APIGroups:   []string{"*"},
							APIVersions: []string{"*"},
							Resources:   []string{"*"},
							Scope:       &all,
						},
					},
				},
				NamespaceSelector:       &metav1.LabelSelector{},
				SideEffects:             &sideEffects,
				FailurePolicy:           &failurePolicy,
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	ar := client.AdmissionregistrationV1().ValidatingWebhookConfigurations()

	// Remove the old name left by a previous release.
	if err := ar.Delete(ctx, "casbin-gatekeeper", metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		logrus.Warnf("delete legacy casbin-gatekeeper webhook: %v", err)
	}

	existing, err := ar.Get(ctx, "casbin-admission", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := ar.Create(ctx, whConfig, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create casbin-admission webhook: %w", err)
		}
		logrus.Info("created ValidatingWebhookConfiguration casbin-admission")
		return nil
	}
	if err != nil {
		return fmt.Errorf("get casbin-admission webhook: %w", err)
	}
	whConfig.ResourceVersion = existing.ResourceVersion
	if _, err := ar.Update(ctx, whConfig, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update casbin-admission webhook: %w", err)
	}
	logrus.Info("updated ValidatingWebhookConfiguration casbin-admission")
	return nil
}

// ensureNodeProxierBinding grants system:node-proxier to the system:nodes group
// so that kube-proxy can reuse the node kubeconfig (same file kubelet uses) to
// watch EndpointSlices and program iptables rules for NodePort services.
func ensureNodeProxierBinding(ctx context.Context, client kubernetes.Interface) error {
	const name = "casos:node-proxier"
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "system:node-proxier",
		},
		Subjects: []rbacv1.Subject{
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Group",
				Name:     "system:nodes",
			},
		},
	}
	_, err := client.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create %s ClusterRoleBinding: %w", name, err)
	}
	logrus.Infof("created ClusterRoleBinding %s", name)
	return nil
}
