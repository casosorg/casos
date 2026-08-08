package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
)

// helmChartAdapter encodes app-aware install value patches keyed by chart name.
// It makes an app usable right after installation (e.g. a reachable service
// type) without requiring the user to know chart internals.
type helmChartAdapter struct {
	valuesPatches map[string]interface{}
	valuesPatchFn func() (map[string]interface{}, error)
}

// helmChartAdapterRegistry maps canonical chart names to install adaptations.
// Explicit user values always win over adapter patches.
var helmChartAdapterRegistry = map[string]helmChartAdapter{
	"grafana":  {valuesPatches: nodePortServiceValuesPatch()},
	"pgadmin4": {valuesPatches: nodePortServiceValuesPatch()},
	"n8n":      {valuesPatches: nodePortServiceValuesPatch()},
	"superset": {
		valuesPatches: map[string]interface{}{
			"service": map[string]interface{}{
				"type":     "NodePort",
				"nodePort": map[string]interface{}{"http": 30088},
			},
			"bootstrapScript": supersetBootstrapScript,
		},
		valuesPatchFn: supersetConfigOverridesPatch,
	},
	"nextcloud": {valuesPatches: nodePortServiceValuesPatch()},
}

// nodePortServiceValuesPatch returns a fresh patch each call so the registry
// never shares mutable state.
func nodePortServiceValuesPatch() map[string]interface{} {
	return map[string]interface{}{
		"service": map[string]interface{}{"type": "NodePort"},
	}
}

// supersetBootstrapScript installs the psycopg2 driver into a writable target
// dir at pod start: the apache/superset image has no psycopg2, the app venv
// has no pip, and the system python (/usr/local/bin/python3) does. ${...}
// keeps the shell expansion out of Helm's tpl processing.
const supersetBootstrapScript = "/usr/local/bin/python3 -m pip install --no-cache-dir --target /tmp/pgdrivers psycopg2-binary && export PYTHONPATH=/tmp/pgdrivers:${PYTHONPATH}"

// supersetConfigOverridesPatch generates a random SECRET_KEY: the default
// value makes Superset refuse to start. The configOverrides template emits
// the raw value, so the patch is a complete Python assignment.
func supersetConfigOverridesPatch() (map[string]interface{}, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate Superset SECRET_KEY: %w", err)
	}
	return map[string]interface{}{
		"configOverrides": map[string]interface{}{"secret": fmt.Sprintf("SECRET_KEY = %q", hex.EncodeToString(buf))},
	}, nil
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
		if adapterPatchExplicitlyOverridden(explicitValues, topKey, patch) {
			continue
		}
		if err := mergeHelmValueOverrides(values, map[string]interface{}{topKey: patch}, nil); err != nil {
			return fmt.Errorf("apply Helm chart adapter for %s: %w", ch.Name(), err)
		}
	}
	if adapter.valuesPatchFn != nil {
		fnPatches, err := adapter.valuesPatchFn()
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
