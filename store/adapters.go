package store

import (
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
)

// VisibleAdapterOverride describes a value the adapter injects by default
// that the user may review and change in the install form.
type VisibleAdapterOverride struct {
	Key         string                 `json:"key"`
	Label       string                 `json:"label"`
	Default     string                 `json:"default"`
	Description string                 `json:"description"`
	ValuesPatch map[string]interface{} `json:"valuesPatch"`
}

// helmChartAdapter encodes app-aware install value patches keyed by chart name.
// It makes an app usable right after installation (e.g. a reachable service
// type) without requiring the user to know chart internals.
type helmChartAdapter struct {
	valuesPatches    map[string]interface{}
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
			"env": []interface{}{
				map[string]interface{}{"name": "N8N_SECURE_COOKIE", "value": "false"},
			},
		},
		visibleOverrides: []VisibleAdapterOverride{{
			Key:         "env.N8N_SECURE_COOKIE",
			Label:       "N8N_SECURE_COOKIE",
			Default:     "false",
			Description: "n8n refuses plain HTTP when secure cookies are enabled; the App Store access URL requires this off",
			ValuesPatch: map[string]interface{}{
				"env": []interface{}{
					map[string]interface{}{"name": "N8N_SECURE_COOKIE", "value": "false"},
				},
			},
		}},
	},
	"superset":  {valuesPatches: nodePortServiceValuesPatch},
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

// ApplyHelmChartAdapterOverrides applies user-chosen values for visible
// adapter overrides onto the install values. Unknown keys are ignored.
func ApplyHelmChartAdapterOverrides(chartName string, values map[string]interface{}, overrides map[string]string) {
	if len(overrides) == 0 {
		return
	}
	adapter, ok := helmChartAdapterRegistry[strings.ToLower(strings.TrimSpace(chartName))]
	if !ok {
		return
	}
	for _, override := range adapter.visibleOverrides {
		value, chosen := overrides[override.Key]
		if !chosen {
			continue
		}
		patch := cloneHelmValues(override.ValuesPatch)
		replaceAdapterOverrideValue(patch, value)
		for topKey, subPatch := range patch {
			_ = mergeHelmValueOverrides(values, map[string]interface{}{topKey: subPatch}, nil)
		}
	}
}

// replaceAdapterOverrideValue replaces the first "value" field in a
// visible-override patch with the user's choice.
func replaceAdapterOverrideValue(patch map[string]interface{}, value string) {
	for key, item := range patch {
		switch typed := item.(type) {
		case string:
			if key == "value" {
				patch[key] = value
				return
			}
		case map[string]interface{}:
			replaceAdapterOverrideValue(typed, value)
		case []interface{}:
			for _, entry := range typed {
				if entryMap, ok := entry.(map[string]interface{}); ok {
					replaceAdapterOverrideValue(entryMap, value)
				}
			}
		}
	}
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
		existing, explicitlySet := explicitValues[topKey]
		if explicitlySet {
			existingArray, userArray := existing.([]interface{})
			patchArray, patchIsArray := patch.([]interface{})
			if !userArray || !patchIsArray {
				continue
			}
			values[topKey] = mergeNamedHelmValuesArrays(existingArray, patchArray)
			continue
		}
		if err := mergeHelmValueOverrides(values, map[string]interface{}{topKey: patch}, nil); err != nil {
			return fmt.Errorf("apply Helm chart adapter for %s: %w", ch.Name(), err)
		}
	}
	return nil
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
