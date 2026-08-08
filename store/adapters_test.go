package store

import (
	"reflect"
	"strings"
	"testing"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
)

func testChart(name string) *chart.Chart {
	return &chart.Chart{
		Metadata: &chart.Metadata{Name: name},
		Values:   map[string]interface{}{},
	}
}

func TestHelmChartAdapterInjectsNodePort(t *testing.T) {
	for _, app := range []string{"grafana", "pgadmin4", "n8n", "superset", "nextcloud"} {
		values, adjustments, err := prepareHelmInstallValues(testChart(app), "https://example.com/charts", map[string]interface{}{})
		if err != nil {
			t.Fatalf("%s: %v", app, err)
		}
		service, ok := values["service"].(map[string]interface{})
		if !ok || service["type"] != "NodePort" {
			t.Errorf("%s: expected service.type NodePort, got %#v", app, values["service"])
		}
		if adjustments.legacyImages || adjustments.tomcatDefaultWebapps {
			t.Errorf("%s: non-Bitnami chart should not apply Bitnami adjustments", app)
		}
	}
}

func TestHelmChartAdapterRespectsExplicitServiceType(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("n8n"), "https://example.com/charts", map[string]interface{}{
		"service": map[string]interface{}{"type": "LoadBalancer"},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	service, ok := values["service"].(map[string]interface{})
	if !ok || service["type"] != "LoadBalancer" {
		t.Errorf("expected user service.type LoadBalancer preserved, got %#v", values["service"])
	}
}

func TestHelmChartAdapterSkipsUnregisteredChart(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("some-other-app"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if _, exists := values["service"]; exists {
		t.Errorf("unregistered chart should not be patched, got %#v", values["service"])
	}
}

func TestHelmChartAdapterKeepsBitnamiAdjustments(t *testing.T) {
	ch := &chart.Chart{
		Metadata: &chart.Metadata{Name: "grafana"},
		Values: map[string]interface{}{
			"image": map[string]interface{}{
				"repository": "bitnami/grafana",
				"tag":        "11.6.1-debian-12-r0",
			},
		},
	}
	values, adjustments, err := prepareHelmInstallValues(ch, bitnamiChartRepoURL, map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if !adjustments.legacyImages {
		t.Error("expected Bitnami legacy image adjustment to be preserved")
	}
	image, ok := values["image"].(map[string]interface{})
	if !ok || image["repository"] != "bitnamilegacy/grafana" {
		t.Errorf("expected rewritten legacy image repository, got %#v", values["image"])
	}
	service, ok := values["service"].(map[string]interface{})
	if !ok || service["type"] != "NodePort" {
		t.Errorf("grafana should get NodePort even on Bitnami repo, got %#v", values["service"])
	}
}

func TestHelmChartAdapterOverridesModeInjectNodePortWhenSiblingChanged(t *testing.T) {
	values, _, err := prepareHelmInstallValuesWithMode(testChart("grafana"), "https://example.com/charts", map[string]interface{}{
		"service": map[string]interface{}{"port": 3001},
	}, true)
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	service, _ := values["service"].(map[string]interface{})
	if service["type"] != "NodePort" {
		t.Errorf("adapter must inject NodePort when only a sibling key changed; got %#v", values["service"])
	}
	if service["port"] != 3001 {
		t.Errorf("user port must be preserved, got %#v", values["service"])
	}
}

func TestHelmChartAdapterOverridesModeRespectsExplicitType(t *testing.T) {
	values, _, err := prepareHelmInstallValuesWithMode(testChart("grafana"), "https://example.com/charts", map[string]interface{}{
		"service": map[string]interface{}{"type": "LoadBalancer", "port": 3001},
	}, true)
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	service, _ := values["service"].(map[string]interface{})
	if service["type"] != "LoadBalancer" {
		t.Errorf("user service.type must win; got %#v", values["service"])
	}
}

func supersetSecretKey(t *testing.T, values map[string]interface{}) string {
	t.Helper()
	env, ok := values["extraSecretEnv"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected extraSecretEnv, got %#v", values["extraSecretEnv"])
	}
	key, ok := env["SUPERSET_SECRET_KEY"].(string)
	if !ok {
		t.Fatalf("expected SUPERSET_SECRET_KEY string, got %#v", env["SUPERSET_SECRET_KEY"])
	}
	return key
}

func TestSupersetAdapterPatches(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("superset"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	service, ok := values["service"].(map[string]interface{})
	if !ok || service["type"] != "NodePort" {
		t.Errorf("expected service.type NodePort, got %#v", values["service"])
	}
	nodePort, ok := service["nodePort"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected service.nodePort map, got %#v", service["nodePort"])
	}
	// A real null, not the chart's "nil" string and not a fixed port number.
	if value, exists := nodePort["http"]; !exists || value != nil {
		t.Errorf("expected service.nodePort.http to be null, got %#v", nodePort["http"])
	}
	if key := supersetSecretKey(t, values); len(key) < 32 {
		t.Errorf("expected a long random SECRET_KEY, got %q", key)
	}
	if _, exists := values["configOverrides"]; exists {
		t.Errorf("SECRET_KEY must not land in configOverrides, got %#v", values["configOverrides"])
	}
	bootstrap, _ := values["bootstrapScript"].(string)
	if !strings.Contains(bootstrap, "psycopg2-binary=="+supersetPsycopg2Version) {
		t.Errorf("expected a pinned psycopg2-binary in bootstrapScript, got %q", bootstrap)
	}
	if !strings.Contains(bootstrap, "import psycopg2") {
		t.Errorf("expected bootstrapScript to skip the install when the driver exists, got %q", bootstrap)
	}
	if strings.Contains(bootstrap, "export PYTHONPATH="+supersetDriverTarget+":") {
		t.Errorf("PYTHONPATH must not end in a bare separator, got %q", bootstrap)
	}
}

func TestSupersetAdapterGeneratesDistinctSecretKeys(t *testing.T) {
	first, _, err := prepareHelmInstallValues(testChart("superset"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	second, _, err := prepareHelmInstallValues(testChart("superset"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if supersetSecretKey(t, first) == supersetSecretKey(t, second) {
		t.Error("each install must get its own SECRET_KEY")
	}
}

func TestSupersetAdapterRespectsUserSecret(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("superset"), "https://example.com/charts", map[string]interface{}{
		"extraSecretEnv": map[string]interface{}{"SUPERSET_SECRET_KEY": "user-secret"},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if key := supersetSecretKey(t, values); key != "user-secret" {
		t.Errorf("user SECRET_KEY should win, got %q", key)
	}
}

func TestSupersetAdapterSkipsSecretWhenConfigOverrideSetsIt(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("superset"), "https://example.com/charts", map[string]interface{}{
		"configOverrides": map[string]interface{}{"secret": `SECRET_KEY = "legacy"`},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if _, exists := values["extraSecretEnv"]; exists {
		t.Errorf("no second SECRET_KEY should be generated, got %#v", values["extraSecretEnv"])
	}
}

func TestSupersetValuesPreviewIsStableAndSecretFree(t *testing.T) {
	first, _, err := buildHelmChartInstallValues(testChart("superset"), "https://example.com/charts")
	if err != nil {
		t.Fatalf("build values: %v", err)
	}
	second, _, err := buildHelmChartInstallValues(testChart("superset"), "https://example.com/charts")
	if err != nil {
		t.Fatalf("build values: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("values preview must be reproducible; got %#v then %#v", first, second)
	}
	if _, exists := first["extraSecretEnv"]; exists {
		t.Errorf("values preview must not mint a SECRET_KEY, got %#v", first["extraSecretEnv"])
	}
}

func seedSupersetRelease(t *testing.T, config map[string]interface{}) *action.Configuration {
	t.Helper()
	cfg := &action.Configuration{}
	mem := driver.NewMemory()
	cfg.Releases = storage.Init(mem)
	if err := mem.Create("superset-demo", &release.Release{
		Name:   "superset-demo",
		Chart:  testChart("superset"),
		Info:   &release.Info{Status: release.StatusDeployed},
		Config: config,
	}); err != nil {
		t.Fatalf("seed release: %v", err)
	}
	return cfg
}

func TestPreserveSupersetSecretKeyOnUpgrade(t *testing.T) {
	cfg := seedSupersetRelease(t, map[string]interface{}{
		"extraSecretEnv": map[string]interface{}{"SUPERSET_SECRET_KEY": "installed-key"},
	})
	ch := testChart("superset")

	vals := map[string]interface{}{}
	preserveHelmChartAdapterValues(cfg, ch, "superset-demo", vals)
	if key := supersetSecretKey(t, vals); key != "installed-key" {
		t.Fatalf("expected the installed SECRET_KEY to be reused, got %q", key)
	}

	userVals := map[string]interface{}{"extraSecretEnv": map[string]interface{}{"SUPERSET_SECRET_KEY": "user-key"}}
	preserveHelmChartAdapterValues(cfg, ch, "superset-demo", userVals)
	if key := supersetSecretKey(t, userVals); key != "user-key" {
		t.Fatalf("user-provided SECRET_KEY must win, got %q", key)
	}
}

func TestPreserveLegacySupersetSecretKeyOnUpgrade(t *testing.T) {
	cfg := seedSupersetRelease(t, map[string]interface{}{
		"configOverrides": map[string]interface{}{"secret": `SECRET_KEY = "legacy-key"`},
	})

	vals := map[string]interface{}{}
	preserveHelmChartAdapterValues(cfg, testChart("superset"), "superset-demo", vals)
	overrides, _ := vals["configOverrides"].(map[string]interface{})
	if overrides["secret"] != `SECRET_KEY = "legacy-key"` {
		t.Fatalf("expected the legacy SECRET_KEY to be reused, got %#v", vals["configOverrides"])
	}

	prepared, _, err := prepareHelmInstallValuesWithMode(testChart("superset"), "https://example.com/charts", vals, true)
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	if _, exists := prepared["extraSecretEnv"]; exists {
		t.Errorf("an upgrade must not rotate the key onto a new mechanism, got %#v", prepared["extraSecretEnv"])
	}
}

func TestPreserveHelmChartAdapterValuesIgnoresOtherCharts(t *testing.T) {
	cfg := seedSupersetRelease(t, map[string]interface{}{
		"extraSecretEnv": map[string]interface{}{"SUPERSET_SECRET_KEY": "installed-key"},
	})
	vals := map[string]interface{}{}
	preserveHelmChartAdapterValues(cfg, testChart("grafana"), "superset-demo", vals)
	if len(vals) != 0 {
		t.Errorf("charts without preserved paths must be left alone, got %#v", vals)
	}
}

func TestNextcloudAdapterTrustedDomains(t *testing.T) {
	values, _, err := prepareHelmInstallValuesWithOptions(testChart("nextcloud"), "https://example.com/charts", map[string]interface{}{}, helmInstallValueOptions{
		nodeIPs: func() []string { return []string{"192.168.10.101"} },
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	trusted := values["nextcloud"].(map[string]interface{})["trustedDomains"]
	items, ok := trusted.([]string)
	if !ok || !reflect.DeepEqual(items, []string{"localhost", "nextcloud.kube.home", "192.168.10.101"}) {
		t.Errorf("unexpected trusted domains: %#v", trusted)
	}
}

func TestNextcloudAdapterUsesUserHost(t *testing.T) {
	values, _, err := prepareHelmInstallValuesWithOptions(testChart("nextcloud"), "https://example.com/charts", map[string]interface{}{
		"nextcloud": map[string]interface{}{"host": "cloud.example.com"},
	}, helmInstallValueOptions{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	trusted := values["nextcloud"].(map[string]interface{})["trustedDomains"]
	if !reflect.DeepEqual(trusted, []string{"localhost", "cloud.example.com"}) {
		t.Errorf("unexpected trusted domains: %#v", trusted)
	}
}

func TestNextcloudAdapterChartDefaultHost(t *testing.T) {
	ch := testChart("nextcloud")
	ch.Values["nextcloud"] = map[string]interface{}{"host": "chart-default.example.com"}
	values, _, err := prepareHelmInstallValuesWithOptions(ch, "https://example.com/charts", map[string]interface{}{}, helmInstallValueOptions{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	trusted := values["nextcloud"].(map[string]interface{})["trustedDomains"]
	if !reflect.DeepEqual(trusted, []string{"localhost", "chart-default.example.com"}) {
		t.Errorf("unexpected trusted domains: %#v", trusted)
	}
}

func TestNextcloudAdapterIncludesTrustedDomainsAndHostnames(t *testing.T) {
	values, _, err := prepareHelmInstallValuesWithOptions(testChart("nextcloud"), "https://example.com/charts", map[string]interface{}{
		"nextcloud": map[string]interface{}{
			"host":           "cloud.example.com",
			"trustedDomains": []interface{}{"a.example.com", "b.example.com"},
		},
		"httpRoute": map[string]interface{}{
			"hostnames": []interface{}{"route.example.com", "a.example.com"},
		},
	}, helmInstallValueOptions{
		nodeIPs: func() []string { return []string{"192.168.10.101", "192.168.10.101"} },
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	trusted := values["nextcloud"].(map[string]interface{})["trustedDomains"].([]string)
	expected := []string{"localhost", "cloud.example.com", "a.example.com", "b.example.com", "route.example.com", "192.168.10.101"}
	if !reflect.DeepEqual(trusted, expected) {
		t.Errorf("expected %#v, got %#v", expected, trusted)
	}
}

func TestNextcloudAdapterWithoutNodeIPs(t *testing.T) {
	values, _, err := prepareHelmInstallValuesWithOptions(testChart("nextcloud"), "https://example.com/charts", map[string]interface{}{}, helmInstallValueOptions{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	trusted := values["nextcloud"].(map[string]interface{})["trustedDomains"]
	if !reflect.DeepEqual(trusted, []string{"localhost", "nextcloud.kube.home"}) {
		t.Errorf("unexpected trusted domains: %#v", trusted)
	}
}
