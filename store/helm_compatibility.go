package store

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
)

func validateHelmChartCompatibility(ctx context.Context, actionConfig *action.Configuration, releaseName, namespace string, chartToInstall *chart.Chart, values map[string]interface{}) error {
	if chartToInstall == nil || chartToInstall.Metadata == nil {
		return fmt.Errorf("chart metadata is missing")
	}
	if chartToInstall.Metadata.Deprecated {
		return fmt.Errorf("chart %s is deprecated and cannot be installed as a supported application", chartToInstall.Name())
	}
	if !isInstallableHelmChartMetadata(chartToInstall.Metadata) {
		return fmt.Errorf("chart %s is a library chart and cannot be installed as an application", chartToInstall.Name())
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, helmCompatibilityTimeout)
	defer cancel()
	// Server-side dry-run is the source of truth for compatibility. It uses the
	// target cluster's discovery and admission stack instead of maintaining a
	// second, inevitably stale allowlist of Kubernetes resource kinds.
	dryRun := newHelmCompatibilityDryRun(actionConfig, releaseName, namespace)
	_, err := dryRun.RunWithContext(ctx, chartToInstall, values)
	if err != nil {
		return fmt.Errorf("render chart %s for compatibility check: %w", chartToInstall.Name(), err)
	}
	return nil
}

func newHelmCompatibilityDryRun(actionConfig *action.Configuration, releaseName, namespace string) *action.Install {
	dryRun := action.NewInstall(actionConfig)
	dryRun.ReleaseName = releaseName
	dryRun.Namespace = namespace
	dryRun.CreateNamespace = true
	dryRun.DryRun = true
	dryRun.DryRunOption = "server"
	dryRun.Timeout = helmCompatibilityTimeout
	return dryRun
}
