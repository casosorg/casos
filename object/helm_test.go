package object

import (
	"os"
	"strings"
	"testing"
)

func TestParseHelmValues(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, values map[string]interface{})
	}{
		{
			name:  "empty input yields empty map",
			input: "   ",
			check: func(t *testing.T, values map[string]interface{}) {
				if len(values) != 0 {
					t.Fatalf("expected empty map, got %v", values)
				}
			},
		},
		{
			name:  "json object",
			input: `{"replicaCount": 2, "image": {"tag": "1.2.3"}}`,
			check: func(t *testing.T, values map[string]interface{}) {
				image, ok := values["image"].(map[string]interface{})
				if !ok {
					t.Fatalf("expected nested image map, got %T", values["image"])
				}
				if image["tag"] != "1.2.3" {
					t.Fatalf("expected image.tag=1.2.3, got %v", image["tag"])
				}
			},
		},
		{
			name:  "yaml object",
			input: "replicaCount: 3\nservice:\n  type: NodePort\n",
			check: func(t *testing.T, values map[string]interface{}) {
				service, ok := values["service"].(map[string]interface{})
				if !ok {
					t.Fatalf("expected nested service map, got %T", values["service"])
				}
				if service["type"] != "NodePort" {
					t.Fatalf("expected service.type=NodePort, got %v", service["type"])
				}
			},
		},
		{
			name:    "list is rejected",
			input:   "- a\n- b\n",
			wantErr: true,
		},
		{
			name:    "scalar is rejected",
			input:   "just-a-string",
			wantErr: true,
		},
		{
			name:    "malformed yaml is rejected",
			input:   "foo: [unclosed",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values, err := ParseHelmValues(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if values == nil {
				t.Fatal("expected non-nil values map")
			}
			if tc.check != nil {
				tc.check(t, values)
			}
		})
	}
}

func TestHelmChartRefNormalize(t *testing.T) {
	cases := []struct {
		name      string
		ref       HelmChartRef
		wantChart string
		wantRepo  string
		wantOCI   bool
		wantErr   bool
	}{
		{
			name:      "classic http repository",
			ref:       HelmChartRef{RepoURL: "https://charts.bitnami.com/bitnami", Chart: "redis"},
			wantChart: "redis",
			wantRepo:  "https://charts.bitnami.com/bitnami",
		},
		{
			name:      "oci repo url with chart name already included",
			ref:       HelmChartRef{RepoURL: "oci://registry-1.docker.io/cloudpirates/nginx", Chart: "nginx"},
			wantChart: "oci://registry-1.docker.io/cloudpirates/nginx",
			wantOCI:   true,
		},
		{
			name:      "oci repo url without chart suffix",
			ref:       HelmChartRef{RepoURL: "oci://registry-1.docker.io/bitnamicharts", Chart: "redis"},
			wantChart: "oci://registry-1.docker.io/bitnamicharts/redis",
			wantOCI:   true,
		},
		{
			name:      "oci passed through chart field",
			ref:       HelmChartRef{Chart: "oci://ghcr.io/some/chart"},
			wantChart: "oci://ghcr.io/some/chart",
			wantOCI:   true,
		},
		{
			name:    "missing repo url",
			ref:     HelmChartRef{Chart: "redis"},
			wantErr: true,
		},
		{
			name:    "missing chart name",
			ref:     HelmChartRef{RepoURL: "https://charts.bitnami.com/bitnami"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ref.IsOCI(); got != tc.wantOCI {
				t.Fatalf("IsOCI() = %v, want %v", got, tc.wantOCI)
			}
			chartRef, repoURL, err := tc.ref.Normalize()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if chartRef != tc.wantChart {
				t.Fatalf("chartRef = %q, want %q", chartRef, tc.wantChart)
			}
			if repoURL != tc.wantRepo {
				t.Fatalf("repoURL = %q, want %q", repoURL, tc.wantRepo)
			}
		})
	}
}

func TestValidateHelmReleaseName(t *testing.T) {
	if err := ValidateHelmReleaseName("my-release"); err != nil {
		t.Fatalf("expected valid release name, got %v", err)
	}
	for _, name := range []string{"", "   ", "Invalid_Name", strings.Repeat("a", 100)} {
		if err := ValidateHelmReleaseName(name); err == nil {
			t.Fatalf("expected error for release name %q", name)
		}
	}
}

// TestHelmRenderChartE2E downloads a real chart and renders it locally with
// Helm's dry-run engine. It needs network access but no Kubernetes cluster.
// Enable with CASOS_HELM_E2E=1.
func TestHelmRenderChartE2E(t *testing.T) {
	if os.Getenv("CASOS_HELM_E2E") != "1" {
		t.Skip("set CASOS_HELM_E2E=1 to run the Helm chart rendering smoke test")
	}

	// charts.jetstack.io serves chart archives from the same host as its
	// index.yaml, which keeps the smoke test independent from GitHub and
	// Docker Hub connectivity.
	ref := HelmChartRef{
		RepoURL: "https://charts.jetstack.io",
		Chart:   "cert-manager",
	}

	info, err := HelmShowChart(ref)
	if err != nil {
		t.Fatalf("HelmShowChart failed: %v", err)
	}
	if info.Name == "" || info.Version == "" {
		t.Fatalf("expected chart metadata, got %+v", info)
	}
	if strings.TrimSpace(info.DefaultValues) == "" {
		t.Fatal("expected non-empty default values")
	}

	values, err := ParseHelmValues(info.DefaultValues)
	if err != nil {
		t.Fatalf("default values are not parseable: %v", err)
	}

	manifest, err := HelmRenderChart(ref, "default", "casos-smoke", values)
	if err != nil {
		t.Fatalf("HelmRenderChart failed: %v", err)
	}
	if !strings.Contains(manifest, "kind: Service") {
		t.Fatalf("rendered manifest looks wrong, first 200 chars: %.200s", manifest)
	}
}
