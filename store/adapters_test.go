package store

import (
	"strings"
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

func TestSupersetAdapterPatches(t *testing.T) {
	values, _, err := prepareHelmInstallValues(testChart("superset"), "https://example.com/charts", map[string]interface{}{})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	service, ok := values["service"].(map[string]interface{})
	if !ok || service["type"] != "NodePort" {
		t.Errorf("expected service.type NodePort, got %#v", values["service"])
	}
	overrides, ok := values["configOverrides"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected configOverrides, got %#v", values["configOverrides"])
	}
	secret, _ := overrides["secret"].(string)
	if len(secret) < 32 {
		t.Errorf("expected generated SECRET_KEY, got %q", secret)
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

func TestNextcloudAdapterTrustedDomains(t *testing.T) {
	values, _, err := prepareHelmInstallValuesWithMode(testChart("nextcloud"), "https://example.com/charts", map[string]interface{}{}, false, []string{"192.168.10.101"})
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	service, ok := values["service"].(map[string]interface{})
	if !ok || service["type"] != "NodePort" {
		t.Errorf("expected service.type NodePort, got %#v", values["service"])
	}
	nextcloud, ok := values["nextcloud"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nextcloud values, got %#v", values["nextcloud"])
	}
	configs, ok := nextcloud["configs"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nextcloud configs, got %#v", nextcloud)
	}
	content, _ := configs["trusted_domains.config.php"].(string)
	if !strings.Contains(content, "nextcloud.kube.home") || !strings.Contains(content, "192.168.10.101") {
		t.Errorf("trusted_domains fragment missing probe host or node IP: %q", content)
	}
}

func TestNextcloudAdapterWithoutNodeIPs(t *testing.T) {
	values, _, err := prepareHelmInstallValuesWithMode(testChart("nextcloud"), "https://example.com/charts", map[string]interface{}{}, false, nil)
	if err != nil {
		t.Fatalf("prepare values: %v", err)
	}
	nextcloud, _ := values["nextcloud"].(map[string]interface{})
	configs, _ := nextcloud["configs"].(map[string]interface{})
	content, _ := configs["trusted_domains.config.php"].(string)
	if !strings.Contains(content, "nextcloud.kube.home") {
		t.Errorf("expected probe host in fragment, got %q", content)
	}
	if strings.Contains(content, "192.168") {
		t.Errorf("expected no node IPs, got %q", content)
	}
}
