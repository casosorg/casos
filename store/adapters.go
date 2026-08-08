package store

import (
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
)

// helmChartAdapter encodes app-aware install value patches keyed by chart name.
// It makes an app usable right after installation (e.g. a reachable service
// type) without requiring the user to know chart internals.
type helmChartAdapter struct {
	valuesPatches map[string]interface{}
	valuesPatchFn func(nodeIPs []string) (map[string]interface{}, error)
}

// helmChartAdapterRegistry maps canonical chart names to install adaptations.
// Explicit user values always win over adapter patches.
var helmChartAdapterRegistry = map[string]helmChartAdapter{
	"grafana":  {valuesPatches: nodePortServiceValuesPatch()},
	"pgadmin4": {valuesPatches: nodePortServiceValuesPatch()},
	"n8n":      {valuesPatches: nodePortServiceValuesPatch()},
	"superset": {valuesPatches: nodePortServiceValuesPatch()},
	"nextcloud": {
		valuesPatches: map[string]interface{}{
			"service": map[string]interface{}{"type": "NodePort"},
		},
		valuesPatchFn: nextcloudTrustedDomainsPatch,
	},
}

// nodePortServiceValuesPatch returns a fresh patch each call so the registry
// never shares mutable state.
func nodePortServiceValuesPatch() map[string]interface{} {
	return map[string]interface{}{
		"service": map[string]interface{}{"type": "NodePort"},
	}
}

// nextcloudTrustedDomainsPatch configures trusted_domains via the chart's
// config.php fragment mechanism (nextcloud.configs): the probe host (the chart
// probes /status.php with Host nextcloud.kube.home, so omitting it makes
// liveness fail and the pod restart-loop) plus the cluster node IPs the user
// reaches the app through. The filename must match Nextcloud's *.config.php
// loading pattern.
func nextcloudTrustedDomainsPatch(nodeIPs []string) (map[string]interface{}, error) {
	var builder strings.Builder
	builder.WriteString("<?php\n$CONFIG['trusted_domains'] = array(\n")
	builder.WriteString("  0 => 'nextcloud.kube.home',\n")
	for index, ip := range nodeIPs {
		builder.WriteString(fmt.Sprintf("  %d => '%s',\n", index+1, ip))
	}
	builder.WriteString(");\n")
	return map[string]interface{}{
		"nextcloud": map[string]interface{}{
			"configs": map[string]interface{}{
				"trusted_domains.config.php": builder.String(),
			},
		},
	}, nil
}

// applyHelmChartAdapter merges chart-specific compatibility values into the
// install values; user-set top-level keys are left untouched.
func applyHelmChartAdapter(ch *chart.Chart, values, explicitValues map[string]interface{}, nodeIPs []string) error {
	if ch == nil {
		return nil
	}
	adapter, ok := helmChartAdapterRegistry[strings.ToLower(strings.TrimSpace(ch.Name()))]
	if !ok {
		return nil
	}
	for topKey, patch := range adapter.valuesPatches {
		if adapterPatchExplicitlyOverridden(explicitValues, topKey, patch) {
			continue
		}
		if err := mergeHelmValueOverrides(values, map[string]interface{}{topKey: patch}, nil); err != nil {
			return fmt.Errorf("apply Helm chart adapter for %s: %w", ch.Name(), err)
		}
	}
	if adapter.valuesPatchFn != nil {
		fnPatches, err := adapter.valuesPatchFn(nodeIPs)
		if err != nil {
			return err
		}
		for topKey, patch := range fnPatches {
			if adapterPatchExplicitlyOverridden(explicitValues, topKey, patch) {
				continue
			}
			if err := mergeHelmValueOverrides(values, map[string]interface{}{topKey: patch}, nil); err != nil {
				return fmt.Errorf("apply Helm chart adapter for %s: %w", ch.Name(), err)
			}
		}
	}
	return nil
}

// adapterPatchExplicitlyOverridden reports whether the user explicitly set
// any leaf of the patch within explicitValues. Leaf-level checking (not the
// top-level key) keeps the adapter active when the user only touched a
// sibling key, e.g. service.port while the patch targets service.type.
func adapterPatchExplicitlyOverridden(explicitValues map[string]interface{}, topKey string, patch interface{}) bool {
	explicit, exists := explicitValues[topKey]
	if !exists {
		return false
	}
	patchMap, patchIsMap := patch.(map[string]interface{})
	explicitMap, explicitIsMap := explicit.(map[string]interface{})
	if !patchIsMap || !explicitIsMap {
		return true
	}
	for key, subPatch := range patchMap {
		if adapterPatchExplicitlyOverridden(explicitMap, key, subPatch) {
			return true
		}
	}
	return false
}
