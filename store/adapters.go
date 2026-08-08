package store

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
)

// helmChartAdapter encodes app-aware install value patches keyed by chart name.
// It makes an app usable right after installation (e.g. a reachable service
// type) without requiring the user to know chart internals.
type helmEndpointBinding struct {
	valuesPath  []string
	hostPath    []string
	sourcePaths [][]string
	defaultHost string
}

type helmChartAdapter struct {
	valuesPatches map[string]interface{}
	// generatedValuesPatchFn mints per-install values such as a random secret.
	// The values preview skips it so the install dialog stays reproducible.
	generatedValuesPatchFn func(explicitValues map[string]interface{}) (map[string]interface{}, error)
	endpointBindings []helmEndpointBinding
	// preservedValuePaths are carried over from the installed release on upgrade.
	preservedValuePaths [][]string
}

// helmChartAdapterRegistry maps canonical chart names to install adaptations.
// Explicit user values always win over adapter patches.
var helmChartAdapterRegistry = map[string]helmChartAdapter{
	"grafana":  {valuesPatches: nodePortServiceValuesPatch()},
	"pgadmin4": {valuesPatches: nodePortServiceValuesPatch()},
	"n8n":      {valuesPatches: nodePortServiceValuesPatch()},
	"superset": {
		valuesPatches:          supersetValuesPatches(),
		generatedValuesPatchFn: supersetSecretKeyPatch,
		preservedValuePaths:    supersetPreservedValuePaths,
	},
	"nextcloud": {
		valuesPatches: nodePortServiceValuesPatch(),
		endpointBindings: []helmEndpointBinding{{
			valuesPath:  []string{"nextcloud", "trustedDomains"},
			hostPath:    []string{"nextcloud", "host"},
			sourcePaths: [][]string{{"nextcloud", "trustedDomains"}, {"httpRoute", "hostnames"}},
			defaultHost: "nextcloud.kube.home",
		}},
	},
}

// nodePortServiceValuesPatch returns a fresh patch each call so the registry
// never shares mutable state.
func nodePortServiceValuesPatch() map[string]interface{} {
	return map[string]interface{}{
		"service": map[string]interface{}{"type": "NodePort"},
	}
}

// supersetValuesPatches also clears the chart's node port default: it ships
// `http: nil`, which YAML reads as the string "nil" and renders as an invalid
// `nodePort: nil`. A real null lets Kubernetes allocate a free port, where a
// fixed number would collide with the next install.
func supersetValuesPatches() map[string]interface{} {
	patches := nodePortServiceValuesPatch()
	service := patches["service"].(map[string]interface{})
	service["nodePort"] = map[string]interface{}{"http": nil}
	patches["bootstrapScript"] = supersetBootstrapScript
	return patches
}

const (
	supersetDriverTarget    = "/tmp/pgdrivers"
	supersetPsycopg2Version = "2.9.10"
	supersetSecretKeyEnvVar = "SUPERSET_SECRET_KEY"
)

// supersetBootstrapScript makes the PostgreSQL driver importable at pod start:
// the apache/superset image does not ship psycopg2 and the app venv has no pip,
// so the system python installs it into a writable target dir. The import probe
// keeps images that already carry it, and every restart on an offline cluster,
// from reaching for PyPI at all; the short timeout fails fast when it must.
// ${PYTHONPATH:+...} drops the separator when PYTHONPATH is unset, where a
// trailing colon would put the working directory on sys.path.
var supersetBootstrapScript = strings.Join([]string{
	"#!/bin/bash",
	"if ! python -c 'import psycopg2' >/dev/null 2>&1; then",
	fmt.Sprintf(
		"  /usr/local/bin/python3 -m pip install --no-cache-dir --disable-pip-version-check --timeout 15 --retries 1 --target %s 'psycopg2-binary==%s' || true",
		supersetDriverTarget, supersetPsycopg2Version,
	),
	"fi",
	fmt.Sprintf("export PYTHONPATH=%s${PYTHONPATH:+:${PYTHONPATH}}", supersetDriverTarget),
}, "\n")

// supersetPreservedValuePaths must survive an upgrade: Superset encrypts stored
// database credentials with SECRET_KEY, so a new key orphans them. The second
// path is where releases installed before the switch keep their key.
var supersetPreservedValuePaths = [][]string{
	{"extraSecretEnv", supersetSecretKeyEnvVar},
	{"configOverrides", "secret"},
}

// supersetSecretKeyPatch generates a random SECRET_KEY per install, because
// Superset refuses to start with the packaged default. extraSecretEnv reaches
// superset/config.py through the env Secret, avoiding configOverrides, whose
// merge order the chart documents as unspecified.
func supersetSecretKeyPatch(explicitValues map[string]interface{}) (map[string]interface{}, error) {
	if supersetSecretKeyAlreadySet(explicitValues) {
		return nil, nil
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate Superset SECRET_KEY: %w", err)
	}
	return map[string]interface{}{
		"extraSecretEnv": map[string]interface{}{supersetSecretKeyEnvVar: hex.EncodeToString(buf)},
	}, nil
}

// supersetSecretKeyAlreadySet reports whether configOverrides already defines a
// SECRET_KEY. superset_config.py is read after the environment, so it wins and
// a generated key would only look like a rotation on upgrade.
func supersetSecretKeyAlreadySet(values map[string]interface{}) bool {
	overrides, ok := values["configOverrides"].(map[string]interface{})
	if !ok {
		return false
	}
	secret, ok := overrides["secret"].(string)
	return ok && strings.TrimSpace(secret) != ""
}

func applyHelmEndpointBindings(ch *chart.Chart, values map[string]interface{}, bindings []helmEndpointBinding, nodeIPs []string) {
	for _, binding := range bindings {
		domains := []string{"localhost"}
		if host, ok := helmValueAtPath(values, binding.hostPath).(string); ok && strings.TrimSpace(host) != "" {
			domains = append(domains, host)
		} else if host, ok := helmValueAtPath(ch.Values, binding.hostPath).(string); ok && strings.TrimSpace(host) != "" {
			domains = append(domains, host)
		} else if binding.defaultHost != "" {
			domains = append(domains, binding.defaultHost)
		}
		for _, path := range binding.sourcePaths {
			domains = append(domains, helmStringValuesAtPath(values, path)...)
			if ch != nil {
				domains = append(domains, helmStringValuesAtPath(ch.Values, path)...)
			}
		}
		domains = append(domains, nodeIPs...)
		setHelmValueAtPath(values, binding.valuesPath, uniqueNonEmptyStrings(domains))
	}
}

func helmValueAtPath(values map[string]interface{}, path []string) interface{} {
	for index, key := range path {
		value, ok := values[key]
		if !ok {
			return nil
		}
		if index == len(path)-1 {
			return value
		}
		values, ok = value.(map[string]interface{})
		if !ok {
			return nil
		}
	}
	return nil
}

func helmStringValuesAtPath(values map[string]interface{}, path []string) []string {
	value := helmValueAtPath(values, path)
	var result []string
	switch typed := value.(type) {
	case []interface{}:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
	case []string:
		result = append(result, typed...)
	}
	return result
}

func setHelmValueAtPath(values map[string]interface{}, path []string, value interface{}) {
	for _, key := range path[:len(path)-1] {
		next, ok := values[key].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			values[key] = next
		}
		values = next
	}
	values[path[len(path)-1]] = value
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

// applyHelmChartAdapter merges chart-specific compatibility values into the
// install values; user-set top-level keys are left untouched.
func applyHelmChartAdapter(ch *chart.Chart, values, explicitValues map[string]interface{}, includeDynamic bool, nodeIPs func() []string) error {
	if ch == nil {
		return nil
	}
	adapter, ok := helmChartAdapterRegistry[helmChartAdapterKey(ch)]
	if !ok {
		return nil
	}
	patches := adapter.valuesPatches
	if includeDynamic {
		var ips []string
		if nodeIPs != nil {
			ips = nodeIPs()
		}
		applyHelmEndpointBindings(ch, values, adapter.endpointBindings, ips)
		if adapter.generatedValuesPatchFn != nil {
			generated, err := adapter.generatedValuesPatchFn(explicitValues)
			if err != nil {
				return fmt.Errorf("apply Helm chart adapter for %s: %w", ch.Name(), err)
			}
			patches = mergedHelmAdapterPatches(patches, generated)
		}
	}
	for topKey, patch := range patches {
		if adapterPatchExplicitlyOverridden(explicitValues, topKey, patch) {
			continue
		}
		if err := mergeHelmValueOverrides(values, map[string]interface{}{topKey: patch}, nil); err != nil {
			return fmt.Errorf("apply Helm chart adapter for %s: %w", ch.Name(), err)
		}
	}
	return nil
}

// preserveHelmChartAdapterValues copies the installed release's adapter-owned
// secrets into the upgrade values, where the adapter then reads them as
// explicit input and leaves them alone. Caller-set values win.
func preserveHelmChartAdapterValues(actionConfig *action.Configuration, ch *chart.Chart, releaseName string, values map[string]interface{}) {
	if ch == nil || actionConfig == nil || actionConfig.Releases == nil {
		return
	}
	adapter, ok := helmChartAdapterRegistry[helmChartAdapterKey(ch)]
	if !ok || len(adapter.preservedValuePaths) == 0 {
		return
	}
	installedRelease, err := actionConfig.Releases.Last(releaseName)
	if err != nil || installedRelease == nil || installedRelease.Chart == nil {
		return
	}
	installed, err := chartutil.CoalesceValues(installedRelease.Chart, cloneHelmValues(installedRelease.Config))
	if err != nil {
		return
	}
	for _, path := range adapter.preservedValuePaths {
		copyHelmValueIfUnset(installed, values, path)
	}
}

// copyHelmValueIfUnset copies one non-empty string leaf between value trees,
// creating parent maps on the way and never replacing an existing leaf.
func copyHelmValueIfUnset(source, target map[string]interface{}, path []string) {
	if len(path) == 0 {
		return
	}
	parents, leaf := path[:len(path)-1], path[len(path)-1]
	for _, key := range parents {
		next, ok := source[key].(map[string]interface{})
		if !ok {
			return
		}
		source = next
	}
	value, ok := source[leaf].(string)
	if !ok || value == "" {
		return
	}
	for _, key := range parents {
		switch existing := target[key].(type) {
		case map[string]interface{}:
			target = existing
		case nil:
			next := map[string]interface{}{}
			target[key] = next
			target = next
		default:
			return
		}
	}
	if _, exists := target[leaf]; exists {
		return
	}
	target[leaf] = value
}

// mergedHelmAdapterPatches overlays generated patches on the static ones
// without mutating the registry entry.
func mergedHelmAdapterPatches(static, generated map[string]interface{}) map[string]interface{} {
	if len(generated) == 0 {
		return static
	}
	merged := make(map[string]interface{}, len(static)+len(generated))
	for topKey, patch := range static {
		merged[topKey] = patch
	}
	for topKey, patch := range generated {
		merged[topKey] = patch
	}
	return merged
}

func helmChartAdapterKey(ch *chart.Chart) string {
	return strings.ToLower(strings.TrimSpace(ch.Name()))
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
