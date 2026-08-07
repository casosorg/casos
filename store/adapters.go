package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
)

// VisibleAdapterOverride describes a value the adapter injects by default
// that the user may review and change in the install form.
type VisibleAdapterOverride struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Default     string `json:"default"`
	Description string `json:"description"`
}

// helmChartAdapter encodes app-aware install value patches keyed by chart name.
// It makes an app usable right after installation (e.g. a reachable service
// type) without requiring the user to know chart internals.
type helmChartAdapter struct {
	valuesPatches    map[string]interface{}
	valuesPatchFn    func() (map[string]interface{}, error)
	visibleOverrides []VisibleAdapterOverride
}

// helmChartAdapterRegistry maps canonical chart names to install adaptations.
// Explicit user values always win over adapter patches; array patches merge
// into user arrays by name instead of replacing them.
var helmChartAdapterRegistry = map[string]helmChartAdapter{
	"grafana":  {valuesPatches: nodePortServiceValuesPatch},
	"pgadmin4": {valuesPatches: nodePortServiceValuesPatch},
	"n8n": {
		valuesPatches: map[string]interface{}{
			"service": map[string]interface{}{"type": "NodePort"},
			"extraEnv": []interface{}{
				map[string]interface{}{"name": "N8N_SECURE_COOKIE", "value": "false"},
			},
		},
		visibleOverrides: []VisibleAdapterOverride{{
			Key:         "extraEnv.N8N_SECURE_COOKIE",
			Label:       "N8N_SECURE_COOKIE",
			Default:     "false",
			Description: "n8n refuses plain HTTP when secure cookies are enabled; the App Store access URL requires this off",
		}},
	},
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
	"nextcloud": {valuesPatches: nodePortServiceValuesPatch},
}

var nodePortServiceValuesPatch = map[string]interface{}{
	"service": map[string]interface{}{"type": "NodePort"},
}

// GetHelmChartAdapterVisibleOverrides returns the user-adjustable overrides
// an installed chart ships with, for the install form to expose.
func GetHelmChartAdapterVisibleOverrides(chartName string) []VisibleAdapterOverride {
	adapter, ok := helmChartAdapterRegistry[strings.ToLower(strings.TrimSpace(chartName))]
	if !ok {
		return nil
	}
	return adapter.visibleOverrides
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
// install values; user-set top-level keys are left untouched, except that
// array patches merge into user arrays by item name.
func applyHelmChartAdapter(ch *chart.Chart, values, explicitValues map[string]interface{}) error {
	if ch == nil {
		return nil
	}
	adapter, ok := helmChartAdapterRegistry[strings.ToLower(strings.TrimSpace(ch.Name()))]
	if !ok {
		return nil
	}
	for topKey, patch := range adapter.valuesPatches {
		if err := mergeAdapterValuesPatch(values, explicitValues, topKey, patch); err != nil {
			return err
		}
	}
	if adapter.valuesPatchFn != nil {
		fnPatches, err := adapter.valuesPatchFn()
		if err != nil {
			return err
		}
		for topKey, patch := range fnPatches {
			if err := mergeAdapterValuesPatch(values, explicitValues, topKey, patch); err != nil {
				return err
			}
		}
	}
	return nil
}

// mergeAdapterValuesPatch merges one adapter patch into the install values;
// user-set top-level keys are left untouched, except array patches merge
// into user arrays by item name.
func mergeAdapterValuesPatch(values, explicitValues map[string]interface{}, topKey string, patch interface{}) error {
	existing, explicitlySet := explicitValues[topKey]
	if explicitlySet {
		existingArray, userArray := existing.([]interface{})
		patchArray, patchIsArray := patch.([]interface{})
		if !userArray || !patchIsArray {
			return nil
		}
		values[topKey] = mergeNamedHelmValuesArrays(existingArray, patchArray)
		return nil
	}
	return mergeHelmValueOverrides(values, map[string]interface{}{topKey: patch}, nil)
}

// mergeNamedHelmValuesArrays appends patch items whose name is not already
// present in the existing array, so user-defined entries win.
func mergeNamedHelmValuesArrays(existing, additions []interface{}) []interface{} {
	known := map[string]bool{}
	for _, item := range existing {
		if itemMap, ok := item.(map[string]interface{}); ok {
			if name, _ := itemMap["name"].(string); name != "" {
				known[name] = true
			}
		}
	}
	merged := append([]interface{}{}, existing...)
	for _, item := range additions {
		if itemMap, ok := item.(map[string]interface{}); ok {
			if name, _ := itemMap["name"].(string); known[name] {
				continue
			}
		}
		merged = append(merged, item)
	}
	return merged
}
