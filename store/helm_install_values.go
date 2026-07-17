package store

import (
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
)

const (
	bitnamiChartRepoURL       = "https://charts.bitnami.com/bitnami"
	bitnamiOCIChartRepoPrefix = "oci://registry-1.docker.io/bitnamicharts/"
)

// GetHelmChartInstallValuesWithContext returns the values shown in the App
// Store install dialog. Raw chart defaults remain available through
// GetHelmChartDefaultValues.
func GetHelmChartInstallValuesWithContext(ctx context.Context, chartName, repoURL, version string) (string, error) {
	ch, err := loadChartWithContext(ctx, chartName, repoURL, version)
	if err != nil {
		return "", err
	}

	values, err := buildHelmChartInstallValues(ch, chartName, repoURL)
	if err != nil {
		return "", err
	}

	data, err := yaml.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func buildHelmChartInstallValues(ch *chart.Chart, chartName, repoURL string) (map[string]interface{}, error) {
	values := cloneHelmValues(ch.Values)
	if isBitnamiCommunityChartRepo(repoURL) {
		coalesced, err := chartutil.CoalesceValues(ch, values)
		if err != nil {
			return nil, fmt.Errorf("coalesce chart %s install values: %w", chartName, err)
		}
		values = map[string]interface{}(coalesced)
		applyHelmDependencyDefaults(ch, values)
		if err := applyBitnamiLegacyImageFallback(values); err != nil {
			return nil, fmt.Errorf("apply Bitnami legacy image fallback: %w", err)
		}
		applyBitnamiAppStoreChartDefaults(chartName, values)
	}
	return values, nil
}

func applyHelmDependencyDefaults(ch *chart.Chart, values map[string]interface{}) {
	applyHelmDependencyDefaultsWithPath(ch, values, make(map[*chart.Chart]struct{}))
}

func applyHelmDependencyDefaultsWithPath(ch *chart.Chart, values map[string]interface{}, path map[*chart.Chart]struct{}) {
	if ch == nil || ch.Metadata == nil {
		return
	}
	if _, seen := path[ch]; seen {
		return
	}
	path[ch] = struct{}{}
	defer delete(path, ch)
	for _, requirement := range ch.Metadata.Dependencies {
		if requirement == nil {
			continue
		}
		dependency := findHelmDependencyChart(ch, requirement)
		if dependency == nil {
			continue
		}
		dependencyDefaults := cloneHelmValues(dependency.Values)
		applyHelmDependencyDefaultsWithPath(dependency, dependencyDefaults, path)

		key := requirement.Name
		if strings.TrimSpace(requirement.Alias) != "" {
			key = requirement.Alias
		}
		target, ok := ensureHelmValuesMap(values, key)
		if !ok {
			continue
		}
		mergeMissingHelmValues(target, dependencyDefaults)
	}
}

func findHelmDependencyChart(parent *chart.Chart, requirement *chart.Dependency) *chart.Chart {
	for _, dependency := range parent.Dependencies() {
		if dependency == nil || dependency.Metadata == nil || dependency.Name() != requirement.Name {
			continue
		}
		if requirement.Version == "" || chartutil.IsCompatibleRange(requirement.Version, dependency.Metadata.Version) {
			return dependency
		}
	}
	return nil
}

func mergeMissingHelmValues(target, defaults map[string]interface{}) {
	for key, defaultValue := range defaults {
		currentValue, exists := target[key]
		if !exists {
			target[key] = cloneHelmValue(defaultValue)
			continue
		}
		currentMap, currentIsMap := currentValue.(map[string]interface{})
		defaultMap, defaultIsMap := defaultValue.(map[string]interface{})
		if currentIsMap && defaultIsMap {
			mergeMissingHelmValues(currentMap, defaultMap)
		}
	}
}

func applyBitnamiAppStoreChartDefaults(chartName string, values map[string]interface{}) {
	switch strings.ToLower(strings.TrimSpace(chartName)) {
	case "tomcat":
		values["tomcatInstallDefaultWebapps"] = true
	case "concourse":
		web, ok := ensureHelmValuesMap(values, "web")
		if !ok {
			return
		}
		if externalURL, _ := web["externalUrl"].(string); strings.TrimSpace(externalURL) == "" {
			web["externalUrl"] = "concourse.local"
		}
	case "ghost":
		if ghostHost, _ := values["ghostHost"].(string); strings.TrimSpace(ghostHost) == "" {
			values["ghostHost"] = "ghost.local"
		}
	case "elasticsearch":
		for _, component := range []string{"master", "data", "coordinating"} {
			if componentValues, ok := ensureHelmValuesMap(values, component); ok {
				componentValues["replicaCount"] = 1
			}
		}
		if ingest, ok := ensureHelmValuesMap(values, "ingest"); ok {
			ingest["enabled"] = false
		}
	}
}

// isBitnamiCommunityChartRepo intentionally limits compatibility rewrites to
// Bitnami's public community endpoints. Mirrors and private registries retain
// their operator-provided image policy unchanged.
func isBitnamiCommunityChartRepo(repoURL string) bool {
	normalized := strings.ToLower(strings.TrimRight(strings.TrimSpace(repoURL), "/"))
	return normalized == bitnamiChartRepoURL || strings.HasPrefix(normalized+"/", bitnamiOCIChartRepoPrefix)
}

func applyBitnamiLegacyImageFallback(values map[string]interface{}) error {
	if !wouldRewriteBitnamiLegacyImageRepositories(values, false) {
		return nil
	}
	global, ok := ensureHelmValuesMap(values, "global")
	if !ok {
		return fmt.Errorf("global must be a map to enable allowInsecureImages")
	}
	security, ok := ensureHelmValuesMap(global, "security")
	if !ok {
		return fmt.Errorf("global.security must be a map to enable allowInsecureImages")
	}
	if !rewriteBitnamiLegacyImageRepositories(values, false) {
		return nil
	}
	security["allowInsecureImages"] = true
	return nil
}

func wouldRewriteBitnamiLegacyImageRepositories(value interface{}, imageValues bool) bool {
	switch typed := value.(type) {
	case map[string]interface{}:
		if imageValues {
			if _, ok := bitnamiLegacyImageRepository(typed); ok {
				return true
			}
		}
		for key, child := range typed {
			if wouldRewriteBitnamiLegacyImageRepositories(child, isHelmImageValuesKey(key)) {
				return true
			}
		}
	case []interface{}:
		for _, child := range typed {
			if wouldRewriteBitnamiLegacyImageRepositories(child, imageValues) {
				return true
			}
		}
	}
	return false
}

func rewriteBitnamiLegacyImageRepositories(value interface{}, imageValues bool) bool {
	changed := false
	switch typed := value.(type) {
	case map[string]interface{}:
		if imageValues && rewriteBitnamiLegacyImageRepository(typed) {
			changed = true
		}
		for key, child := range typed {
			if rewriteBitnamiLegacyImageRepositories(child, isHelmImageValuesKey(key)) {
				changed = true
			}
		}
	case []interface{}:
		for _, child := range typed {
			if rewriteBitnamiLegacyImageRepositories(child, imageValues) {
				changed = true
			}
		}
	}
	return changed
}

func isHelmImageValuesKey(key string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(key)), "image")
}

func rewriteBitnamiLegacyImageRepository(values map[string]interface{}) bool {
	repository, ok := bitnamiLegacyImageRepository(values)
	if !ok {
		return false
	}
	values["repository"] = repository
	return true
}

func bitnamiLegacyImageRepository(values map[string]interface{}) (string, bool) {
	repository, ok := values["repository"].(string)
	if !ok || !usesVersionedBitnamiImage(values) {
		return "", false
	}
	registry, _ := values["registry"].(string)
	registry = strings.ToLower(strings.TrimSpace(registry))
	if registry != "" && registry != "docker.io" && registry != "registry-1.docker.io" {
		return "", false
	}
	switch {
	case strings.HasPrefix(repository, "bitnami/"):
		return "bitnamilegacy/" + strings.TrimPrefix(repository, "bitnami/"), true
	case strings.HasPrefix(repository, "docker.io/bitnami/"):
		return "docker.io/bitnamilegacy/" + strings.TrimPrefix(repository, "docker.io/bitnami/"), true
	case strings.HasPrefix(repository, "registry-1.docker.io/bitnami/"):
		return "registry-1.docker.io/bitnamilegacy/" + strings.TrimPrefix(repository, "registry-1.docker.io/bitnami/"), true
	default:
		return "", false
	}
}

func usesVersionedBitnamiImage(values map[string]interface{}) bool {
	if digest, ok := values["digest"].(string); ok && strings.TrimSpace(digest) != "" {
		return true
	}

	tag, ok := values["tag"].(string)
	if !ok {
		return false
	}
	tag = strings.TrimSpace(tag)
	return tag != "" && !strings.EqualFold(tag, "latest")
}

func ensureHelmValuesMap(parent map[string]interface{}, key string) (map[string]interface{}, bool) {
	if current, ok := parent[key].(map[string]interface{}); ok {
		return current, true
	}
	if current, exists := parent[key]; exists && current != nil {
		return nil, false
	}
	current := map[string]interface{}{}
	parent[key] = current
	return current, true
}

func cloneHelmValues(values map[string]interface{}) map[string]interface{} {
	if values == nil {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(values))
	for key, value := range values {
		cloned[key] = cloneHelmValue(value)
	}
	return cloned
}

func cloneHelmValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return cloneHelmValues(typed)
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for i, child := range typed {
			cloned[i] = cloneHelmValue(child)
		}
		return cloned
	default:
		return typed
	}
}
