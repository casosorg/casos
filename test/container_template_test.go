// Package test contains the project's test files.
//
// CasOS did not ship with tests historically; this package is the
// starting point for the project's testing effort (issue #65).
//
// Files here follow Go's "external test package" pattern: the package
// name is `test` (not `object` or any other internal package), so
// tests exercise the public API only.
package test

import (
	"strings"
	"testing"

	"github.com/casosorg/casos/object"
)

// TestGetContainerTemplates covers the only piece of the App Store
// feature that is pure (no K8s, no DB) and therefore cheap to test:
// the built-in template list. The contract is small:
//
//  1. At least one template ships.
//  2. The "firefox" template exists and is well-formed.
//  3. Its image carries a registry prefix so K8s does not silently
//     default to docker.io/library/<name>:latest.
func TestGetContainerTemplates(t *testing.T) {
	templates := object.GetContainerTemplates()
	if len(templates) == 0 {
		t.Fatal("expected at least one built-in template")
	}

	var firefox *object.ContainerTemplate
	for i := range templates {
		if templates[i].Name == "firefox" {
			firefox = &templates[i]
			break
		}
	}
	if firefox == nil {
		t.Fatal("firefox template not found")
	}

	if firefox.DisplayName == "" {
		t.Error("DisplayName must not be empty")
	}
	if !strings.Contains(firefox.Image, "/") {
		t.Errorf("Image %q must include a registry prefix (e.g. docker.io/...)", firefox.Image)
	}
	if firefox.DefaultPort <= 0 || firefox.DefaultPort > 65535 {
		t.Errorf("DefaultPort %d is out of valid TCP range", firefox.DefaultPort)
	}
}
