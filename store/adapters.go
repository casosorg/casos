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
}

// helmChartAdapterRegistry maps canonical chart names to install adaptations.
// Explicit user values always win over adapter patches.
var helmChartAdapterRegistry = map[string]helmChartAdapter{
	"grafana":   {valuesPatches: nodePortServiceValuesPatch},
	"pgadmin4":  {valuesPatches: nodePortServiceValuesPatch},
	"n8n":       {valuesPatches: nodePortServiceValuesPatch},
	"superset":  {valuesPatches: nodePortServiceValuesPatch},
	"nextcloud": {valuesPatches: nodePortServiceValuesPatch},
}

var nodePortServiceValuesPatch = map[string]interface{}{
	"service": map[string]interface{}{"type": "NodePort"},
}

// applyHelmChartAdapter merges chart-specific compatibility values into the
// install values; user-set top-level keys are left untouched.
func applyHelmChartAdapter(ch *chart.Chart, values, explicitValues map[string]interface{}) error {
	if ch == nil {
		return nil
	}
	adapter, ok := helmChartAdapterRegistry[strings.ToLower(strings.TrimSpace(ch.Name()))]
	if !ok {
		return nil
	}
	for topKey, patch := range adapter.valuesPatches {
		if _, explicitlySet := explicitValues[topKey]; explicitlySet {
			continue
		}
		if err := mergeHelmValueOverrides(values, map[string]interface{}{topKey: patch}, nil); err != nil {
			return fmt.Errorf("apply Helm chart adapter for %s: %w", ch.Name(), err)
		}
	}
	return nil
}
