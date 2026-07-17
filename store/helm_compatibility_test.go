package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
)

func TestInstallableHelmChartMetadata(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata *chart.Metadata
		want     bool
	}{
		{name: "nil", want: false},
		{name: "application", metadata: &chart.Metadata{Name: "app", Type: "application"}, want: true},
		{name: "default type", metadata: &chart.Metadata{Name: "app"}, want: true},
		{name: "library", metadata: &chart.Metadata{Name: "lib", Type: " Library "}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isInstallableHelmChartMetadata(tc.metadata); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDeprecatedChartsAreHiddenButClassifiedSeparately(t *testing.T) {
	metadata := &chart.Metadata{Name: "old-app", Type: "application", Deprecated: true}
	if !isInstallableHelmChartMetadata(metadata) {
		t.Fatal("deprecated application was misclassified as a library chart")
	}
	if isVisibleAppStoreChartMetadata(metadata) {
		t.Fatal("deprecated chart remained visible in the App Store")
	}
}

func TestCompatibilityDryRunMirrorsInstallSettings(t *testing.T) {
	dryRun := newHelmCompatibilityDryRun(new(action.Configuration), "demo", "new-namespace")
	if !dryRun.DryRun || dryRun.DryRunOption != "server" {
		t.Fatalf("unexpected dry-run mode: DryRun=%v option=%q", dryRun.DryRun, dryRun.DryRunOption)
	}
	if !dryRun.CreateNamespace {
		t.Fatal("compatibility dry-run would reject a namespace the real install creates")
	}
	if dryRun.Timeout != 2*time.Minute {
		t.Fatalf("dry-run timeout = %v, want %v", dryRun.Timeout, 2*time.Minute)
	}
}

func TestValidateHelmChartCompatibilityRejectsInvalidMetadata(t *testing.T) {
	for _, tc := range []struct {
		name  string
		chart *chart.Chart
		want  string
	}{
		{name: "nil chart", want: "chart metadata is missing"},
		{name: "nil metadata", chart: &chart.Chart{}, want: "chart metadata is missing"},
		{name: "deprecated", chart: &chart.Chart{Metadata: &chart.Metadata{Name: "old", Deprecated: true}}, want: "deprecated"},
		{name: "library", chart: &chart.Chart{Metadata: &chart.Metadata{Name: "lib", Type: "library"}}, want: "library chart"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHelmChartCompatibility(context.Background(), new(action.Configuration), "demo", "default", tc.chart, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected compatibility result: %v", err)
			}
		})
	}
}
