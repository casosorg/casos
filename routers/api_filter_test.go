package routers

import "testing"

func TestPublicAPIAllowlist(t *testing.T) {
	publicPaths := []string{
		"/api/signin",
		"/api/signout",
		"/api/get-account",
		"/api/get-signin-options",
		"/api/setup",
		"/api/e2e/signin",
		"/api/get-built-in-site",
	}
	for _, path := range publicPaths {
		if !isPublicAPI(path) {
			t.Fatalf("expected %s to be public", path)
		}
	}
	if isPublicAPI("/api/get-pods") {
		t.Fatal("cluster API must require authentication")
	}
}
