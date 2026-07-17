package store

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/chart"
)

func TestBitnamiLegacyFallbackIsNarrowAndVisible(t *testing.T) {
	values := map[string]interface{}{
		"image": map[string]interface{}{
			"registry":   "docker.io",
			"repository": "bitnami/nginx",
			"tag":        "1.2.3",
		},
		"privateImage": map[string]interface{}{
			"registry":   "registry.example.com",
			"repository": "bitnami/private",
			"tag":        "1.2.3",
		},
		"latestImage": map[string]interface{}{
			"repository": "bitnami/latest",
			"tag":        "latest",
		},
	}
	if err := applyBitnamiLegacyImageFallback(values); err != nil {
		t.Fatalf("apply legacy fallback: %v", err)
	}
	image := values["image"].(map[string]interface{})
	if image["repository"] != "bitnamilegacy/nginx" {
		t.Fatalf("versioned Bitnami image was not rewritten: %#v", image)
	}
	privateImage := values["privateImage"].(map[string]interface{})
	if privateImage["repository"] != "bitnami/private" {
		t.Fatalf("private registry image was rewritten: %#v", privateImage)
	}
	latestImage := values["latestImage"].(map[string]interface{})
	if latestImage["repository"] != "bitnami/latest" {
		t.Fatalf("latest image was rewritten: %#v", latestImage)
	}
	global := values["global"].(map[string]interface{})
	security := global["security"].(map[string]interface{})
	if security["allowInsecureImages"] != true {
		t.Fatalf("legacy fallback was not surfaced in install values: %#v", security)
	}
}

func TestLegacyFallbackOnlyAppliesToBitnamiCommunityRepository(t *testing.T) {
	chartWithBitnamiImage := func() *chart.Chart {
		return &chart.Chart{
			Metadata: &chart.Metadata{Name: "demo", Version: "1.0.0"},
			Values: map[string]interface{}{"image": map[string]interface{}{
				"repository": "bitnami/nginx",
				"tag":        "1.2.3",
			}},
		}
	}
	privateValues, err := buildHelmChartInstallValues(chartWithBitnamiImage(), "demo", "https://charts.example.com")
	if err != nil {
		t.Fatalf("build private-repo values: %v", err)
	}
	if privateValues["image"].(map[string]interface{})["repository"] != "bitnami/nginx" {
		t.Fatal("non-Bitnami repository values were rewritten")
	}
	bitnamiValues, err := buildHelmChartInstallValues(chartWithBitnamiImage(), "demo", bitnamiChartRepoURL)
	if err != nil {
		t.Fatalf("build Bitnami values: %v", err)
	}
	if bitnamiValues["image"].(map[string]interface{})["repository"] != "bitnamilegacy/nginx" {
		t.Fatal("Bitnami community repository did not receive its compatibility fallback")
	}
}

func TestInstallValueDefaultsPreserveScalarKeys(t *testing.T) {
	values := map[string]interface{}{
		"master": "custom-master-shape",
		"global": "custom-global-shape",
		"image": map[string]interface{}{
			"repository": "bitnami/nginx",
			"tag":        "1.2.3",
		},
	}
	applyBitnamiAppStoreChartDefaults("elasticsearch", values)
	err := applyBitnamiLegacyImageFallback(values)
	if err == nil {
		t.Fatal("malformed global value did not return a visible error")
	}
	if values["master"] != "custom-master-shape" {
		t.Fatalf("chart scalar was overwritten: %#v", values["master"])
	}
	if values["global"] != "custom-global-shape" {
		t.Fatalf("global scalar was overwritten: %#v", values["global"])
	}
	if values["image"].(map[string]interface{})["repository"] != "bitnami/nginx" {
		t.Fatal("image was rewritten even though the required security value could not be represented")
	}
}

func TestDependencyDefaultsHandleCyclesAndPreserveAliases(t *testing.T) {
	a := &chart.Chart{Metadata: &chart.Metadata{Name: "a", Version: "1.0.0", Dependencies: []*chart.Dependency{{Name: "b"}}}}
	b := &chart.Chart{Metadata: &chart.Metadata{Name: "b", Version: "1.0.0", Dependencies: []*chart.Dependency{{Name: "a"}}}, Values: map[string]interface{}{"enabled": true}}
	a.SetDependencies(b)
	b.SetDependencies(a)
	values := map[string]interface{}{"b": "user-scalar"}
	applyHelmDependencyDefaults(a, values)
	if values["b"] != "user-scalar" {
		t.Fatalf("dependency alias scalar was overwritten: %#v", values["b"])
	}
}

func TestOCIRetryHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	err := retryOCIRegistryOperationWithDelays(ctx, func() error {
		attempts++
		cancel()
		return io.ErrUnexpectedEOF
	}, []time.Duration{time.Hour})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("retry returned %v, want context cancellation", err)
	}
	if attempts != 1 {
		t.Fatalf("retry attempts = %d, want 1", attempts)
	}
}
