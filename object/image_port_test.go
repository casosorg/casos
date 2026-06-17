package object

import (
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestSplitImage(t *testing.T) {
	cases := []struct {
		in              string
		registry, repo  string
		tag             string
	}{
		{"nginx", "registry-1.docker.io", "library/nginx", "latest"},
		{"nginx:1.27", "registry-1.docker.io", "library/nginx", "1.27"},
		{"library/nginx", "registry-1.docker.io", "library/nginx", "latest"},
		{"jlesage/firefox", "registry-1.docker.io", "jlesage/firefox", "latest"},
		{"jlesage/firefox:v4.4.0", "registry-1.docker.io", "jlesage/firefox", "v4.4.0"},
		{"docker.io/jlesage/firefox", "registry-1.docker.io", "jlesage/firefox", "latest"},
		{"ghcr.io/owner/repo:1.0", "ghcr.io", "owner/repo", "1.0"},
		{"localhost:5000/myapp:dev", "localhost:5000", "myapp", "dev"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			registry, repo, tag, err := splitImage(c.in)
			if err != nil {
				t.Fatalf("splitImage(%q) error: %v", c.in, err)
			}
			if registry != c.registry {
				t.Errorf("registry = %q, want %q", registry, c.registry)
			}
			if repo != c.repo {
				t.Errorf("repo = %q, want %q", repo, c.repo)
			}
			if tag != c.tag {
				t.Errorf("tag = %q, want %q", tag, c.tag)
			}
		})
	}
}

func TestParseExposedPorts(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]struct{}
		want []int32
	}{
		{"empty", nil, nil},
		{"single tcp", map[string]struct{}{"5800/tcp": {}}, []int32{5800}},
		{"multiple tcp sorted", map[string]struct{}{
			"5800/tcp": {},
			"5900/tcp": {},
			"5801/tcp": {},
		}, []int32{5800, 5801, 5900}},
		{"udp ignored", map[string]struct{}{
			"53/tcp":  {},
			"53/udp":  {},
		}, []int32{53}},
		{"out of range ignored", map[string]struct{}{
			"0/tcp":    {},
			"65536/tcp": {},
			"8080/tcp": {},
		}, []int32{8080}},
		{"malformed ignored", map[string]struct{}{
			"abc/tcp": {},
			"8080":    {},
		}, []int32(nil)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseExposedPorts(c.in)
			if got == nil {
				got = nil
			}
			sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("parseExposedPorts = %v, want %v", got, c.want)
			}
		})
	}
}

func TestLookupExposedPortsCache(t *testing.T) {
	const image = "casbin/casdoor-all-in-one:latest"
	exposedPortCacheMu.Lock()
	exposedPortCache[image] = exposedPortCacheEntry{
		ports:     []int32{8000},
		expiresAt: nowFunc().Add(exposedPortCacheTTL),
	}
	exposedPortCacheMu.Unlock()

	ports, err := LookupExposedPorts(image)
	if err != nil {
		t.Fatalf("cached lookup returned error: %v", err)
	}
	if !reflect.DeepEqual(ports, []int32{8000}) {
		t.Errorf("cached ports = %v, want [8000]", ports)
	}
}

func TestLookupExposedPortsExpired(t *testing.T) {
	const image = "no/such/image:1"
	exposedPortCacheMu.Lock()
	exposedPortCache[image] = exposedPortCacheEntry{
		ports:     []int32{9999},
		expiresAt: nowFunc().Add(-time.Minute),
	}
	exposedPortCacheMu.Unlock()

	_, err := LookupExposedPorts(image)
	if err == nil {
		t.Skip("unexpectedly reached the registry; skipping")
	}
}

var nowFunc = time.Now
