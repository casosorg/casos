package casdoor

import "testing"

func TestInitCasdoorConfigSkipsProviderInLocalMode(t *testing.T) {
	t.Setenv("authMode", "local")
	t.Setenv("casdoorEndpoint", "")
	t.Setenv("clientId", "")
	t.Setenv("clientSecret", "")
	if err := InitCasdoorConfig(); err != nil {
		t.Fatalf("InitCasdoorConfig() in local mode: %v", err)
	}
}

func TestInitCasdoorConfigValidatesProviderMode(t *testing.T) {
	t.Setenv("authMode", "casdoor")
	t.Setenv("casdoorEndpoint", "")
	if err := InitCasdoorConfig(); err == nil {
		t.Fatal("InitCasdoorConfig() accepted an incomplete Casdoor configuration")
	}
}
