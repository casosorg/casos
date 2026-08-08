package store

import (
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
func TestSupersetAdapterPatches(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("superset"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	service, ok := values["service"].(map[string]interface{})
	if !ok || service["type"] != "NodePort" {
		t.Errorf("expected service.type NodePort, got %#v", values["service"])
	}
	nodePort, _ := service["nodePort"].(map[string]interface{})
	if nodePort["http"] != 30088 {
		t.Errorf("expected explicit numeric nodePort, got %#v", service["nodePort"])
	}
	overrides, ok := values["configOverrides"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected configOverrides, got %#v", values["configOverrides"])
	}
	secret, _ := overrides["secret"].(string)
	if !strings.HasPrefix(secret, "SECRET_KEY = ") || len(secret) < 32 {
		t.Errorf("expected SECRET_KEY assignment, got %q", secret)
	}
	bootstrap, _ := values["bootstrapScript"].(string)
	if !strings.Contains(bootstrap, "psycopg2") {
		t.Errorf("expected psycopg2 in bootstrapScript, got %q", bootstrap)
	}
}

func TestSupersetAdapterRespectsUserSecret(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("superset"), "https://example.com/charts", map[string]interface{}{
		"configOverrides": map[string]interface{}{"secret": "user-secret"},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	overrides, _ := values["configOverrides"].(map[string]interface{})
	if overrides["secret"] != "user-secret" {
		t.Errorf("user SECRET_KEY should win, got %#v", overrides["secret"])
	}
}

func TestReuseSupersetSecretKeyOnUpgrade(t *testing.T) {

	cfg := &action.Configuration{}
	mem := driver.NewMemory()
	cfg.Releases = storage.Init(mem)
	chart := &chart.Chart{Metadata: &chart.Metadata{Name: "superset"}}
	if err := mem.Create("superset-demo", &release.Release{
		Name:  "superset-demo",
		Chart: chart,
		Info:  &release.Info{Status: release.StatusDeployed},
		Config: map[string]interface{}{
			"configOverrides": map[string]interface{}{"secret": "old-secret"},
		},
	}); err != nil {
		t.Fatalf("seed release: %v", err)
	}

	vals := map[string]interface{}{}
	reuseSupersetSecretKey(cfg, "superset-demo", vals)
	overrides, _ := vals["configOverrides"].(map[string]interface{})
	if overrides["secret"] != "old-secret" {
		t.Fatalf("expected existing SECRET_KEY reused, got %#v", vals["configOverrides"])
	}

	vals2 := map[string]interface{}{"configOverrides": map[string]interface{}{"secret": "user-key"}}
	reuseSupersetSecretKey(cfg, "superset-demo", vals2)
	overrides2, _ := vals2["configOverrides"].(map[string]interface{})
	if overrides2["secret"] != "user-key" {
		t.Fatalf("user-provided SECRET_KEY must win, got %#v", vals2["configOverrides"])
	}
}
