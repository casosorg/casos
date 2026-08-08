package store

import (
	"testing"

	"helm.sh/helm/v3/pkg/chart"
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
	values, adjustments, err := prepareHelmInstallValues(testChart("grafana"), bitnamiChartRepoURL, map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	service, ok := values["service"].(map[string]interface{})
	if !ok || service["type"] != "NodePort" {
		t.Errorf("grafana should get NodePort even on Bitnami repo, got %#v", values["service"])
	}
	_ = adjustments
}

func TestN8nAdapterInjectsSecureCookieEnv(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("n8n"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	env, ok := values["extraEnv"].([]interface{})
	if !ok || len(env) != 1 {
		t.Fatalf("expected injected env entry, got %#v", values["extraEnv"])
	}
	entry, _ := env[0].(map[string]interface{})
	if entry["name"] != "N8N_SECURE_COOKIE" || entry["value"] != "false" {
		t.Errorf("unexpected env entry: %#v", entry)
	}
}

func TestN8nAdapterMergesUserEnvByName(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("n8n"), "https://example.com/charts", map[string]interface{}{
		"extraEnv": []interface{}{
			map[string]interface{}{"name": "TZ", "value": "Asia/Shanghai"},
		},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	env, ok := values["extraEnv"].([]interface{})
	if !ok || len(env) != 2 {
		t.Fatalf("expected 2 merged env entries, got %#v", values["extraEnv"])
	}
	names := map[string]string{}
	for _, item := range env {
		m, _ := item.(map[string]interface{})
		names[m["name"].(string)] = m["value"].(string)
	}
	if names["TZ"] != "Asia/Shanghai" || names["N8N_SECURE_COOKIE"] != "false" {
		t.Errorf("unexpected merged env: %#v", names)
	}
}

func TestN8nAdapterUserEnvWinsByName(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("n8n"), "https://example.com/charts", map[string]interface{}{
		"extraEnv": []interface{}{
			map[string]interface{}{"name": "N8N_SECURE_COOKIE", "value": "true"},
		},
	})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	env, _ := values["extraEnv"].([]interface{})
	if len(env) != 1 {
		t.Fatalf("expected single user env entry, got %#v", values["extraEnv"])
	}
	entry, _ := env[0].(map[string]interface{})
	if entry["value"] != "true" {
		t.Errorf("user value should win, got %#v", entry)
	}
}

func TestGetHelmChartAdapterVisibleOverrides(t *testing.T) {
	overrides := GetHelmChartAdapterVisibleOverrides("n8n")
	if len(overrides) != 1 || overrides[0].Key != "extraEnv.N8N_SECURE_COOKIE" || overrides[0].Default != "false" {
		t.Fatalf("unexpected visible overrides: %#v", overrides)
	}
	if got := GetHelmChartAdapterVisibleOverrides("grafana"); got != nil {
		t.Errorf("grafana should have no visible overrides, got %#v", got)
	}
}
