package proxy

import "testing"

func TestMain(m *testing.M) {
	InitHttpClient()
	m.Run()
}

func TestShouldUseProxy(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"hub.docker.com", true},
		{"registry-1.docker.io", true},
		{"docker.io", true},
		{"ghcr.io", false},
		{"localhost:5000", false},
		{"127.0.0.1:5000", false},
		{"quay.io", false},
		{"evil.com", false},
		{"docker.io.evil.com", false},
	}
	for _, c := range cases {
		t.Run(c.host, func(t *testing.T) {
			if got := shouldUseProxy(c.host); got != c.want {
				t.Errorf("shouldUseProxy(%q) = %v, want %v", c.host, got, c.want)
			}
		})
	}
}
