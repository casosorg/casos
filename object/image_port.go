package object

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/casosorg/casos/proxy"
)

type imageConfig struct {
	ExposedPorts map[string]struct{} `json:"exposedPorts"`
}

type manifestList struct {
	SchemaVersion int        `json:"schemaVersion"`
	MediaType     string     `json:"mediaType"`
	Manifests     []manifest `json:"manifests"`
}

type manifest struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Platform  struct {
		Architecture string `json:"architecture"`
		OS           string `json:"os"`
	} `json:"platform"`
}

type exposedPortCacheEntry struct {
	ports     []int32
	expiresAt time.Time
}

var (
	exposedPortCacheMu sync.Mutex
	exposedPortCache   = map[string]exposedPortCacheEntry{}
)

const (
	exposedPortCacheTTL = 24 * time.Hour
	httpTimeout         = 10 * time.Second
)

func LookupExposedPorts(image string) ([]int32, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return nil, fmt.Errorf("empty image reference")
	}

	exposedPortCacheMu.Lock()
	if entry, ok := exposedPortCache[image]; ok && time.Now().Before(entry.expiresAt) {
		exposedPortCacheMu.Unlock()
		return entry.ports, nil
	}
	exposedPortCacheMu.Unlock()

	ports, err := fetchExposedPorts(image)
	if err != nil {
		return nil, err
	}

	exposedPortCacheMu.Lock()
	exposedPortCache[image] = exposedPortCacheEntry{
		ports:     ports,
		expiresAt: time.Now().Add(exposedPortCacheTTL),
	}
	exposedPortCacheMu.Unlock()

	return ports, nil
}

func fetchExposedPorts(image string) ([]int32, error) {
	registry, repo, tag, err := splitImage(image)
	if err != nil {
		return nil, err
	}

	token, err := fetchBearerToken(registry, repo)
	if err != nil {
		return nil, err
	}

	manifestBytes, mediaType, err := fetchManifest(registry, repo, tag, token)
	if err != nil {
		return nil, err
	}

	if strings.Contains(mediaType, "manifest.list") || strings.Contains(mediaType, "image.index") {
		var list manifestList
		if err := json.Unmarshal(manifestBytes, &list); err != nil {
			return nil, fmt.Errorf("parse manifest list: %w", err)
		}
		var picked string
		for _, m := range list.Manifests {
			if m.Platform.OS == "linux" && m.Platform.Architecture == "amd64" {
				picked = m.Digest
				break
			}
		}
		if picked == "" && len(list.Manifests) > 0 {
			picked = list.Manifests[0].Digest
		}
		if picked == "" {
			return nil, fmt.Errorf("manifest list has no entries for image %q", image)
		}
		manifestBytes, _, err = fetchManifestDigest(registry, repo, picked, token)
		if err != nil {
			return nil, err
		}
	}

	var m struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.Config.Digest == "" {
		return nil, fmt.Errorf("manifest has no config digest for image %q", image)
	}

	configBytes, err := fetchBlob(registry, repo, m.Config.Digest, token)
	if err != nil {
		return nil, err
	}

	var cfg imageConfig
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		return nil, fmt.Errorf("parse image config: %w", err)
	}

	return parseExposedPorts(cfg.ExposedPorts), nil
}

func parseExposedPorts(m map[string]struct{}) []int32 {
	if len(m) == 0 {
		return nil
	}
	out := make([]int32, 0, len(m))
	for k := range m {
		parts := strings.SplitN(k, "/", 2)
		if len(parts) != 2 || parts[1] != "tcp" {
			continue
		}
		var p int32
		if _, err := fmt.Sscanf(parts[0], "%d", &p); err != nil {
			continue
		}
		if p > 0 && p <= 65535 {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func splitImage(image string) (registry, repo, tag string, err error) {
	ref := image
	if i := strings.LastIndex(ref, ":"); i > 0 && !strings.Contains(ref[i:], "/") {
		tag = ref[i+1:]
		ref = ref[:i]
	} else {
		tag = "latest"
	}
	if tag == "" {
		tag = "latest"
	}

	parts := strings.Split(ref, "/")
	switch len(parts) {
	case 1:
		registry = "registry-1.docker.io"
		repo = "library/" + parts[0]
	case 2:
		if strings.ContainsAny(parts[0], ".:") || parts[0] == "localhost" {
			registry = parts[0]
			if registry == "docker.io" {
				registry = "registry-1.docker.io"
			}
			repo = parts[1]
		} else {
			registry = "registry-1.docker.io"
			repo = parts[0] + "/" + parts[1]
		}
	default:
		registry = parts[0]
		if registry == "docker.io" {
			registry = "registry-1.docker.io"
		}
		repo = strings.Join(parts[1:], "/")
	}
	if repo == "" {
		return "", "", "", fmt.Errorf("could not parse repository from %q", image)
	}
	return registry, repo, tag, nil
}

func fetchBearerToken(registry, repo string) (string, error) {
	url := fmt.Sprintf("https://%s/token?service=registry.docker.io&scope=repository:%s:pull", registry, repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	body, _, err := doJSON(req)
	if err != nil {
		return "", fmt.Errorf("fetch bearer token for %s/%s: %w", registry, repo, err)
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse bearer token: %w", err)
	}
	if resp.Token == "" {
		return "", fmt.Errorf("registry returned empty bearer token for %s/%s", registry, repo)
	}
	return resp.Token, nil
}

func fetchManifest(registry, repo, tag, token string) ([]byte, string, error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ","))
	return doJSON(req)
}

func fetchManifestDigest(registry, repo, digest, token string) ([]byte, string, error) {
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, digest)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
	}, ","))
	return doJSON(req)
}

func fetchBlob(registry, repo, digest, token string) ([]byte, error) {
	url := fmt.Sprintf("https://%s/v2/%s/blobs/%s", registry, repo, digest)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	body, _, err := doJSON(req)
	return body, err
}

func doJSON(req *http.Request) ([]byte, string, error) {
	client := proxy.GetHttpClient(req.URL.String())
	if client == nil {
		client = &http.Client{Timeout: httpTimeout}
	} else if client.Timeout == 0 {
		client.Timeout = httpTimeout
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode/100 != 2 {
		return nil, resp.Header.Get("Content-Type"), fmt.Errorf("registry returned %d: %s", resp.StatusCode, truncate(body, 200))
	}
	return body, resp.Header.Get("Content-Type"), nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
