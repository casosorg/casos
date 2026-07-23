package controllers

import "testing"

func TestCanonicalHelmOperationOwner(t *testing.T) {
	if got := canonicalHelmOperationOwner(" stable-id ", "built-in", "ci-user"); got != "stable-id" {
		t.Fatalf("stable ID was not preferred: %q", got)
	}
	fallback := canonicalHelmOperationOwner("", "built-in", "ci-user")
	if fallback == "" || len(fallback) > 100 {
		t.Fatalf("invalid owner/name fallback: %q", fallback)
	}
	if fallback != canonicalHelmOperationOwner("", "built-in", "ci-user") {
		t.Fatal("owner/name fallback is not deterministic")
	}
	if fallback == canonicalHelmOperationOwner("", "other", "ci-user") {
		t.Fatal("different Casdoor identities produced the same owner")
	}
	if got := canonicalHelmOperationOwner("", "built-in", ""); got != "" {
		t.Fatalf("incomplete identity produced an owner: %q", got)
	}
}
