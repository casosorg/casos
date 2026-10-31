package controllers

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestFlattenNginxCapturePath(t *testing.T) {
	cases := []struct {
		path string
		want string
		ok   bool
	}{
		{path: "/()(.*)", want: "/", ok: true},
		{path: "/?(.*)", want: "/", ok: true},
		{path: "/", want: "/", ok: false},
		{path: "/api/?(.*)", want: "/api/?(.*)", ok: false},
	}
	for _, c := range cases {
		got, ok := flattenNginxCapturePath(c.path)
		if got != c.want || ok != c.ok {
			t.Errorf("flattenNginxCapturePath(%q) = %q, %v; want %q, %v", c.path, got, ok, c.want, c.ok)
		}
	}
}

func TestFlattenNginxCapturePaths(t *testing.T) {
	item := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]any{
				nginxRewriteTargetAnnotation:               "/$2",
				"nginx.ingress.kubernetes.io/ssl-redirect": "true",
			},
		},
		"spec": map[string]any{
			"rules": []any{
				map[string]any{
					"host": "allinssl.localhost",
					"http": map[string]any{
						"paths": []any{
							map[string]any{
								"path":     "/()(.*)",
								"pathType": "Prefix",
							},
						},
					},
				},
			},
		},
	}}
	flattenNginxCapturePaths(item)
	rules, _, _ := unstructured.NestedSlice(item.Object, "spec", "rules")
	rule := rules[0].(map[string]any)
	http := rule["http"].(map[string]any)
	paths := http["paths"].([]any)
	path := paths[0].(map[string]any)["path"]
	if path != "/" {
		t.Fatalf("path = %q, want /", path)
	}
	if item.GetAnnotations()[nginxRewriteTargetAnnotation] != "" {
		t.Fatalf("rewrite-target was kept: %#v", item.GetAnnotations())
	}
	if item.GetAnnotations()["nginx.ingress.kubernetes.io/ssl-redirect"] != "true" {
		t.Fatal("unrelated nginx annotation was dropped")
	}
}
