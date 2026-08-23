package store

import (
	"context"
	"fmt"
	"sort"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/casosorg/casos/conf"
)

const helmClaimOperationTimeout = 30 * time.Second

// helmUninstallTimeout is how long an uninstall may wait for the release's
// resources to go away. It matches the install budget rather than the shorter
// helmOperationTimeout: an uninstall waits on the same workloads an install
// does, plus any pre-delete hook Job the chart ships — Longhorn's runs well past
// five minutes — and a release whose uninstall times out is left wedged in the
// "uninstalling" state with no way out from the App Store.
func helmUninstallTimeout() time.Duration {
	raw := conf.GetConfigString("helmUninstallTimeout")
	if timeout, err := parseHelmInstallTimeout(raw); err == nil {
		return timeout
	}
	timeout, err := configuredHelmInstallTimeout()
	if err != nil {
		return defaultHelmInstallTimeout
	}
	return timeout
}

// Include legacy labels still used by older charts.
var helmReleaseClaimLabels = []string{"app.kubernetes.io/instance", "release", "app"}

// helmReleaseClaimNames lists the PersistentVolumeClaims that belong to a
// release. Claims are matched by label rather than by reading the release
// manifest, because volumeClaimTemplate claims are minted by the StatefulSet
// controller and never appear in the manifest Helm stores.
func helmReleaseClaimNames(ctx context.Context, cfg *rest.Config, releaseName, namespace string) ([]string, error) {
	if cfg == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("list the claims of release %s: %w", releaseName, err)
	}
	ctx, cancel := context.WithTimeout(ctx, helmClaimOperationTimeout)
	defer cancel()

	seen := map[string]bool{}
	for _, label := range helmReleaseClaimLabels {
		claims, err := client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", label, releaseName),
		})
		if err != nil {
			return nil, fmt.Errorf("list the claims of release %s: %w", releaseName, err)
		}
		for _, claim := range claims.Items {
			seen[claim.Name] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// deleteHelmReleaseClaims removes claims that helmReleaseClaimNames collected
// before the uninstall. A claim that is already gone is not an error: the chart
// may own its own cleanup hook for it.
func deleteHelmReleaseClaims(ctx context.Context, cfg *rest.Config, namespace string, names []string) error {
	if len(names) == 0 || cfg == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("delete the claims of the uninstalled release: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, helmClaimOperationTimeout)
	defer cancel()

	var failed []string
	for _, name := range names {
		err := client.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			failed = append(failed, fmt.Sprintf("%s (%v)", name, err))
		}
	}
	if len(failed) > 0 {
		// The release itself is gone by now, so this is reported as a partial
		// success: saying nothing would leave stale volumes to break the next
		// install of the same app, which is the failure this option exists for.
		return fmt.Errorf("the app was uninstalled but these volumes could not be deleted: %v", failed)
	}
	return nil
}
