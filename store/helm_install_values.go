package store

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
)

const (
	bitnamiChartRepoURL         = "https://charts.bitnami.com/bitnami"
	bitnamiOCIChartRepoPrefix   = "oci://registry-1.docker.io/bitnamicharts/"
	bitnamiDockerOCIChartPrefix = "oci://docker.io/bitnamicharts/"
	bitnamiImageRegistry        = "docker.io"
)

type helmInstallValueAdjustments struct {
	legacyImages         bool
	tomcatDefaultWebapps bool
}

func (adjustments helmInstallValueAdjustments) warnings() []string {
	warnings := make([]string, 0, 2)
	if adjustments.legacyImages {
		warnings = append(warnings, bitnamiLegacyImageWarning())
	}
	if adjustments.tomcatDefaultWebapps {
		warnings = append(warnings, tomcatDefaultWebappsWarning())
	}
	return warnings
}

func bitnamiLegacyImageWarning() string {
	return fmt.Sprintf("Bitnami legacy image fallback is enabled: images use %s/bitnamilegacy from the frozen snapshot and global.security.allowInsecureImages=true; review these values before installation", bitnamiImageRegistry)
}

func tomcatDefaultWebappsWarning() string {
	return "CasOS enables Tomcat default webapps unless tomcatInstallDefaultWebapps is explicitly set"
}

// GetHelmChartInstallValues returns chart defaults with only the compatibility
// overrides that the install path will also enforce.
func GetHelmChartInstallValues(chartName, repoURL, version string) (string, error) {
	ch, err := loadChart(chartName, repoURL, version)
	if err != nil {
		return "", err
	}
	return renderHelmChartInstallValues(ch, repoURL)
}

// GetHelmValueOverrides returns only values changed from the install dialog's
// initial YAML. An empty baseline preserves the existing API behavior.
func GetHelmValueOverrides(valuesYAML, baselineYAML string) (string, error) {
	overrides, _, err := getHelmValueOverrides(valuesYAML, baselineYAML)
	return overrides, err
}

func getHelmValueOverrides(valuesYAML, baselineYAML string) (string, bool, error) {
	if strings.TrimSpace(baselineYAML) == "" {
		return valuesYAML, false, nil
	}
	values, err := parseValues(valuesYAML)
	if err != nil {
		return "", false, err
	}
	baseline, err := parseValues(baselineYAML)
	if err != nil {
		return "", false, fmt.Errorf("parse initial values: %w", err)
	}
	overrides, err := yaml.Marshal(changedHelmValues(baseline, values))
	if err != nil {
		return "", false, fmt.Errorf("marshal Helm value overrides: %w", err)
	}
	return string(overrides), true, nil
}

func renderHelmChartInstallValues(ch *chart.Chart, repoURL string) (string, error) {
	values, adjustments, err := buildHelmChartInstallValues(ch, repoURL)
	if err != nil {
		return "", err
	}
	data, err := yaml.Marshal(values)
	if err != nil {
		return "", err
	}
	if warnings := adjustments.warnings(); len(warnings) != 0 {
		return "# WARNING: " + strings.Join(warnings, "\n# WARNING: ") + "\n" + string(data), nil
	}
	return string(data), nil
}

func buildHelmChartInstallValues(ch *chart.Chart, repoURL string) (map[string]interface{}, helmInstallValueAdjustments, error) {
	if ch == nil {
		return map[string]interface{}{}, helmInstallValueAdjustments{}, nil
	}
	overrides, adjustments, err := prepareHelmInstallValues(ch, repoURL, map[string]interface{}{})
	if err != nil {
		return nil, helmInstallValueAdjustments{}, err
	}
	values := cloneHelmValues(ch.Values)
	if err := mergeHelmValueOverrides(values, overrides, nil); err != nil {
		return nil, helmInstallValueAdjustments{}, err
	}
	return values, adjustments, nil
}

// prepareHelmInstallValues computes compatibility changes from Helm's fully
// coalesced values, then merges only changed paths into the caller's values.
func prepareHelmInstallValues(ch *chart.Chart, repoURL string, input map[string]interface{}) (map[string]interface{}, helmInstallValueAdjustments, error) {
	return prepareHelmInstallValuesWithMode(ch, repoURL, input, false)
}

func prepareHelmInstallValuesWithMode(ch *chart.Chart, repoURL string, input map[string]interface{}, inputIsOverrides bool) (map[string]interface{}, helmInstallValueAdjustments, error) {
	values := cloneHelmValues(input)
	if ch == nil || !isBitnamiCommunityChartRepo(repoURL) {
		return values, helmInstallValueAdjustments{}, nil
	}

	coalesced, err := chartutil.CoalesceValues(ch, cloneHelmValues(input))
	if err != nil {
		return nil, helmInstallValueAdjustments{}, fmt.Errorf("coalesce Helm install values: %w", err)
	}
	before := map[string]interface{}(coalesced)
	patched := cloneHelmValues(before)
	coalescedDefaults, err := chartutil.CoalesceValues(ch, map[string]interface{}{})
	if err != nil {
		return nil, helmInstallValueAdjustments{}, fmt.Errorf("coalesce Helm chart defaults: %w", err)
	}
	defaults := map[string]interface{}(coalescedDefaults)
	legacyImages := false
	if !hasCustomGlobalImageRegistry(before) && !hasExplicitLegacyImageOptOut(input, defaults, inputIsOverrides) {
		legacyImages = rewriteBitnamiLegacyImageRepositories(patched, input, defaults, inputIsOverrides, false)
	}
	adjustments := helmInstallValueAdjustments{
		legacyImages:         legacyImages,
		tomcatDefaultWebapps: applyTomcatInstallDefaults(ch.Name(), patched, input, defaults, inputIsOverrides),
	}
	if !adjustments.legacyImages && !adjustments.tomcatDefaultWebapps {
		return values, adjustments, nil
	}

	if adjustments.legacyImages {
		security, err := ensureHelmValuesPath(patched, "global", "security")
		if err != nil {
			return nil, helmInstallValueAdjustments{}, err
		}
		security["allowInsecureImages"] = true
	}

	if err := mergeHelmValueOverrides(values, changedHelmValues(before, patched), nil); err != nil {
		return nil, helmInstallValueAdjustments{}, err
	}
	return values, adjustments, nil
}

func hasCustomGlobalImageRegistry(values map[string]interface{}) bool {
	global, ok := values["global"].(map[string]interface{})
	if !ok {
		return false
	}
	registry, ok := global["imageRegistry"].(string)
	if !ok {
		return false
	}
	registry = strings.ToLower(strings.TrimSpace(registry))
	return registry != "" && registry != "docker.io" && registry != "registry-1.docker.io"
}

func hasExplicitLegacyImageOptOut(values, defaults map[string]interface{}, inputIsOverrides bool) bool {
	global, ok := values["global"].(map[string]interface{})
	if !ok {
		return false
	}
	security, ok := global["security"].(map[string]interface{})
	if !ok {
		return false
	}
	allowInsecureImages, ok := security["allowInsecureImages"].(bool)
	if !ok || allowInsecureImages {
		return false
	}
	if inputIsOverrides {
		return true
	}
	defaultGlobal, _ := defaults["global"].(map[string]interface{})
	defaultSecurity, _ := defaultGlobal["security"].(map[string]interface{})
	defaultValue, defaultExists := defaultSecurity["allowInsecureImages"]
	return !defaultExists || !reflect.DeepEqual(allowInsecureImages, defaultValue)
}

func applyTomcatInstallDefaults(chartName string, values, explicitValues, defaults map[string]interface{}, inputIsOverrides bool) bool {
	if !strings.EqualFold(strings.TrimSpace(chartName), "tomcat") {
		return false
	}
	if explicitValue, explicitlySet := explicitValues["tomcatInstallDefaultWebapps"]; explicitlySet {
		defaultValue, defaultExists := defaults["tomcatInstallDefaultWebapps"]
		if inputIsOverrides || !defaultExists || !reflect.DeepEqual(explicitValue, defaultValue) {
			return false
		}
	}
	current, exists := values["tomcatInstallDefaultWebapps"]
	currentBool, isBool := current.(bool)
	if !exists || !isBool || currentBool {
		return false
	}
	values["tomcatInstallDefaultWebapps"] = true
	return true
}

func isBitnamiCommunityChartRepo(repoURL string) bool {
	normalized := strings.ToLower(strings.TrimRight(strings.TrimSpace(repoURL), "/"))
	return normalized == bitnamiChartRepoURL ||
		strings.HasPrefix(normalized+"/", bitnamiOCIChartRepoPrefix) ||
		strings.HasPrefix(normalized+"/", bitnamiDockerOCIChartPrefix)
}

func rewriteBitnamiLegacyImageRepositories(value, explicitValue, defaultValue interface{}, inputIsOverrides, imageValues bool) bool {
	changed := false
	switch typed := value.(type) {
	case map[string]interface{}:
		if imageValues && !hasExplicitImageLocation(explicitValue, defaultValue, inputIsOverrides) && rewriteBitnamiLegacyImageRepository(typed) {
			changed = true
		}
		for key, child := range typed {
			var explicitChild interface{}
			var defaultChild interface{}
			if explicitMap, ok := explicitValue.(map[string]interface{}); ok {
				explicitChild = explicitMap[key]
			}
			if defaultMap, ok := defaultValue.(map[string]interface{}); ok {
				defaultChild = defaultMap[key]
			}
			if rewriteBitnamiLegacyImageRepositories(child, explicitChild, defaultChild, inputIsOverrides, isHelmImageValuesKey(key)) {
				changed = true
			}
		}
	case []interface{}:
		var explicitItems []interface{}
		var defaultItems []interface{}
		if items, ok := explicitValue.([]interface{}); ok {
			explicitItems = items
		}
		if items, ok := defaultValue.([]interface{}); ok {
			defaultItems = items
		}
		for index, child := range typed {
			var explicitChild interface{}
			if index < len(explicitItems) {
				explicitChild = explicitItems[index]
			}
			var defaultChild interface{}
			if index < len(defaultItems) {
				defaultChild = defaultItems[index]
			}
			if rewriteBitnamiLegacyImageRepositories(child, explicitChild, defaultChild, inputIsOverrides, imageValues) {
				changed = true
			}
		}
	}
	return changed
}

func hasExplicitImageLocation(value, defaultValue interface{}, inputIsOverrides bool) bool {
	explicitValues, ok := value.(map[string]interface{})
	if !ok {
		return false
	}
	if inputIsOverrides {
		_, registryExplicit := explicitValues["registry"]
		_, repositoryExplicit := explicitValues["repository"]
		return registryExplicit || repositoryExplicit
	}
	defaultValues, _ := defaultValue.(map[string]interface{})
	for _, key := range []string{"registry", "repository", "tag", "digest"} {
		explicit, exists := explicitValues[key]
		if !exists {
			continue
		}
		if defaultValue, defaultExists := defaultValues[key]; !defaultExists || !reflect.DeepEqual(explicit, defaultValue) {
			return true
		}
	}
	return false
}

func isHelmImageValuesKey(key string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(key)), "image")
}

func rewriteBitnamiLegacyImageRepository(values map[string]interface{}) bool {
	repository, ok := values["repository"].(string)
	if !ok || !hasBitnamiImageVersion(values) {
		return false
	}
	registry, _ := values["registry"].(string)
	registry = strings.ToLower(strings.TrimSpace(registry))
	preferredRegistry := bitnamiImageRegistry
	if registry != "" && registry != "docker.io" && registry != "registry-1.docker.io" {
		return false
	}

	var imageName string
	switch {
	case strings.HasPrefix(repository, "bitnami/"):
		imageName = strings.TrimPrefix(repository, "bitnami/")
	case strings.HasPrefix(repository, "docker.io/bitnami/"):
		imageName = strings.TrimPrefix(repository, "docker.io/bitnami/")
	case strings.HasPrefix(repository, "registry-1.docker.io/bitnami/"):
		imageName = strings.TrimPrefix(repository, "registry-1.docker.io/bitnami/")
	case strings.HasPrefix(repository, "bitnamilegacy/"):
		imageName = strings.TrimPrefix(repository, "bitnamilegacy/")
	case strings.HasPrefix(repository, "docker.io/bitnamilegacy/"):
		imageName = strings.TrimPrefix(repository, "docker.io/bitnamilegacy/")
	case strings.HasPrefix(repository, "registry-1.docker.io/bitnamilegacy/"):
		imageName = strings.TrimPrefix(repository, "registry-1.docker.io/bitnamilegacy/")
	default:
		return false
	}
	wantedRepository := "bitnamilegacy/" + imageName
	if registry == preferredRegistry && repository == wantedRepository {
		return false
	}
	values["registry"] = preferredRegistry
	values["repository"] = wantedRepository
	return true
}

func hasBitnamiImageVersion(values map[string]interface{}) bool {
	if digest, ok := values["digest"].(string); ok && strings.TrimSpace(digest) != "" {
		return true
	}
	tag, ok := values["tag"].(string)
	if !ok {
		return false
	}
	return strings.TrimSpace(tag) != ""
}

func ensureHelmValuesPath(root map[string]interface{}, path ...string) (map[string]interface{}, error) {
	current := root
	for index, key := range path {
		existing, exists := current[key]
		if !exists || existing == nil {
			next := map[string]interface{}{}
			current[key] = next
			current = next
			continue
		}
		next, ok := existing.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s must be a map to apply Bitnami image compatibility", strings.Join(path[:index+1], "."))
		}
		current = next
	}
	return current, nil
}

func changedHelmValues(before, after map[string]interface{}) map[string]interface{} {
	changes := map[string]interface{}{}
	for key, afterValue := range after {
		beforeValue, exists := before[key]
		if !exists {
			changes[key] = cloneHelmValue(afterValue)
			continue
		}
		beforeMap, beforeIsMap := beforeValue.(map[string]interface{})
		afterMap, afterIsMap := afterValue.(map[string]interface{})
		if beforeIsMap && afterIsMap {
			if nested := changedHelmValues(beforeMap, afterMap); len(nested) != 0 {
				changes[key] = nested
			}
			continue
		}
		if !reflect.DeepEqual(beforeValue, afterValue) {
			changes[key] = cloneHelmValue(afterValue)
		}
	}
	return changes
}

func mergeHelmValueOverrides(target, overrides map[string]interface{}, path []string) error {
	for key, override := range overrides {
		currentPath := append(path, key)
		overrideMap, isMap := override.(map[string]interface{})
		if !isMap {
			target[key] = cloneHelmValue(override)
			continue
		}
		existing, exists := target[key]
		if !exists || existing == nil {
			existing = map[string]interface{}{}
			target[key] = existing
		}
		targetMap, ok := existing.(map[string]interface{})
		if !ok {
			return fmt.Errorf("%s must be a map to apply Bitnami image compatibility", strings.Join(currentPath, "."))
		}
		if err := mergeHelmValueOverrides(targetMap, overrideMap, currentPath); err != nil {
			return err
		}
	}
	return nil
}

func cloneHelmValues(values map[string]interface{}) map[string]interface{} {
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
		for index, child := range typed {
			cloned[index] = cloneHelmValue(child)
		}
		return cloned
	default:
		return typed
	}
}
