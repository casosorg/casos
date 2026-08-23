package store

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/casosorg/casos/conf"
	proxypkg "github.com/casosorg/casos/proxy"
)

const (
	imageConfigRequestTimeout = 25 * time.Second
	imageConfigMaxBytes       = 4 << 20
	dockerHubRegistryHost     = "registry-1.docker.io"
	dockerHubMirrorHost       = "docker.1ms.run"
)

var registryManifestAccept = strings.Join([]string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.docker.distribution.manifest.v2+json",
}, ", ")

type ImagePort struct {
	Port     int32  `json:"port"`
	Protocol string `json:"protocol"`
}

type ImageEnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	// Whether the variable reads like something an operator would set, rather
	// than base-image plumbing such as PATH or PHP_VERSION. The UI shows these
	// up front and hides the rest behind a toggle.
	Configurable bool `json:"configurable"`
}

// ImageDatabaseHint is the one dependency an image can be made to admit to: the
// env vars it reads its database connection from. Nothing in an image says a
// database must exist, so this is inferred from naming conventions and is only
// ever a suggestion the installer offers.
type ImageDatabaseHint struct {
	Engine      string `json:"engine"`
	HostEnv     string `json:"hostEnv"`
	PortEnv     string `json:"portEnv,omitempty"`
	NameEnv     string `json:"nameEnv,omitempty"`
	UserEnv     string `json:"userEnv,omitempty"`
	PasswordEnv string `json:"passwordEnv,omitempty"`
	Name        string `json:"name,omitempty"`
	User        string `json:"user,omitempty"`
}

type ImageConfig struct {
	Image       string             `json:"image"`
	Registry    string             `json:"registry"`
	Repository  string             `json:"repository"`
	Tag         string             `json:"tag"`
	Digest      string             `json:"digest"`
	Platform    string             `json:"platform"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Vendor      string             `json:"vendor"`
	Version     string             `json:"version"`
	Source      string             `json:"source"`
	User        string             `json:"user"`
	WorkingDir  string             `json:"workingDir"`
	Entrypoint  []string           `json:"entrypoint"`
	Cmd         []string           `json:"cmd"`
	Ports       []ImagePort        `json:"ports"`
	Volumes     []string           `json:"volumes"`
	Env         []ImageEnvVar      `json:"env"`
	Labels      map[string]string  `json:"labels"`
	Database    *ImageDatabaseHint `json:"database,omitempty"`
}

type ociPlatform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Variant      string `json:"variant"`
}

type ociDescriptor struct {
	MediaType string       `json:"mediaType"`
	Digest    string       `json:"digest"`
	Size      int64        `json:"size"`
	Platform  *ociPlatform `json:"platform"`
}

type registryManifest struct {
	MediaType string          `json:"mediaType"`
	Config    ociDescriptor   `json:"config"`
	Manifests []ociDescriptor `json:"manifests"`
}

type imageConfigBlob struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Config       struct {
		User         string              `json:"User"`
		ExposedPorts map[string]struct{} `json:"ExposedPorts"`
		Env          []string            `json:"Env"`
		Entrypoint   []string            `json:"Entrypoint"`
		Cmd          []string            `json:"Cmd"`
		Volumes      map[string]struct{} `json:"Volumes"`
		WorkingDir   string              `json:"WorkingDir"`
		Labels       map[string]string   `json:"Labels"`
	} `json:"config"`
}

// GetImageConfig reads an image's own runtime metadata straight from the
// registry: the ports it exposes, the paths it declares as volumes, the env it
// ships with, and its OCI labels. Anonymous pull only, so a private image fails
// with the registry's own 401.
func GetImageConfig(ctx context.Context, ref, platform string) (*ImageConfig, error) {
	host, repository, tag, digest, err := parseImageRef(ref)
	if err != nil {
		return nil, err
	}
	osName, arch := parsePlatform(platform)

	cacheKey := fmt.Sprintf("image-config:%s/%s:%s@%s|%s/%s", host, repository, tag, digest, osName, arch)
	if cached, ok := defaultHelmArtifactCache.get(cacheKey); ok {
		var result ImageConfig
		if json.Unmarshal(cached, &result) == nil {
			return &result, nil
		}
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, imageConfigRequestTimeout)
	defer cancel()

	endpoints := []string{registryEndpoint(host)}
	if isDockerHubHost(host) {
		if mirror := dockerHubImageMirrorHost(); mirror != "" {
			endpoints = append(endpoints, mirror)
		}
	}

	var lastErr error
	for _, endpoint := range endpoints {
		result, err := fetchImageConfig(ctx, endpoint, host, repository, tag, digest, osName, arch)
		if err != nil {
			lastErr = err
			continue
		}
		result.Image = formatImageRef(host, repository, tag, digest)
		if data, marshalErr := json.Marshal(result); marshalErr == nil {
			defaultHelmArtifactCache.put(cacheKey, data)
		}
		return result, nil
	}
	return nil, lastErr
}

func fetchImageConfig(ctx context.Context, endpoint, host, repository, tag, digest, osName, arch string) (*ImageConfig, error) {
	client := &registryClient{http: proxypkg.HTTPClient(), host: endpoint, repository: repository}

	reference := digest
	if reference == "" {
		reference = tag
	}
	body, err := client.get(ctx, "manifests/"+url.PathEscape(reference), registryManifestAccept)
	if err != nil {
		return nil, err
	}

	var manifest registryManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest for %s: %w", repository, err)
	}

	manifestDigest := digest
	// A multi-arch image answers with an index; one more round trip resolves the
	// manifest for the platform the cluster will actually run.
	if len(manifest.Manifests) > 0 {
		selected := selectPlatformManifest(manifest.Manifests, osName, arch)
		if selected == nil {
			return nil, fmt.Errorf("image %s has no %s/%s variant", repository, osName, arch)
		}
		manifestDigest = selected.Digest
		body, err = client.get(ctx, "manifests/"+url.PathEscape(selected.Digest), registryManifestAccept)
		if err != nil {
			return nil, err
		}
		manifest = registryManifest{}
		if err := json.Unmarshal(body, &manifest); err != nil {
			return nil, fmt.Errorf("parse manifest for %s: %w", repository, err)
		}
	}

	if manifest.Config.Digest == "" {
		return nil, fmt.Errorf("image %s exposes no config blob; it may still be a legacy v1 manifest", repository)
	}
	blob, err := client.get(ctx, "blobs/"+url.PathEscape(manifest.Config.Digest), "")
	if err != nil {
		return nil, err
	}
	var parsed imageConfigBlob
	if err := json.Unmarshal(blob, &parsed); err != nil {
		return nil, fmt.Errorf("parse image config for %s: %w", repository, err)
	}

	result := buildImageConfig(parsed, host, repository, tag)
	result.Digest = manifestDigest
	return result, nil
}

func buildImageConfig(blob imageConfigBlob, host, repository, tag string) *ImageConfig {
	labels := blob.Config.Labels
	if labels == nil {
		labels = map[string]string{}
	}

	result := &ImageConfig{
		Registry:    host,
		Repository:  repository,
		Tag:         tag,
		Platform:    strings.Trim(fmt.Sprintf("%s/%s", blob.OS, blob.Architecture), "/"),
		Title:       firstNonEmpty(labels["org.opencontainers.image.title"], repositoryDisplayName(repository)),
		Description: firstNonEmpty(labels["org.opencontainers.image.description"], labels["org.label-schema.description"]),
		Vendor:      firstNonEmpty(labels["org.opencontainers.image.vendor"], labels["maintainer"]),
		Version:     labels["org.opencontainers.image.version"],
		Source:      firstNonEmpty(labels["org.opencontainers.image.url"], labels["org.opencontainers.image.source"]),
		User:        blob.Config.User,
		WorkingDir:  blob.Config.WorkingDir,
		Entrypoint:  blob.Config.Entrypoint,
		Cmd:         blob.Config.Cmd,
		Ports:       parseExposedPorts(blob.Config.ExposedPorts),
		Volumes:     sortedKeys(blob.Config.Volumes),
		Env:         parseImageEnv(blob.Config.Env),
		Labels:      labels,
	}
	result.Database = detectDatabaseHint(result.Env)
	return result
}

func parseExposedPorts(exposed map[string]struct{}) []ImagePort {
	ports := make([]ImagePort, 0, len(exposed))
	for key := range exposed {
		spec, protocol, found := strings.Cut(key, "/")
		if !found {
			protocol = "tcp"
		}
		port, err := strconv.Atoi(strings.TrimSpace(spec))
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		ports = append(ports, ImagePort{Port: int32(port), Protocol: strings.ToUpper(protocol)})
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i].Port < ports[j].Port })
	return ports
}

func parseImageEnv(env []string) []ImageEnvVar {
	result := make([]ImageEnvVar, 0, len(env))
	for _, entry := range env {
		name, value, found := strings.Cut(entry, "=")
		if !found || name == "" {
			continue
		}
		result = append(result, ImageEnvVar{Name: name, Value: value, Configurable: isConfigurableEnv(name, value)})
	}
	return result
}

var (
	baseImageEnvNames = map[string]struct{}{
		"PATH": {}, "HOME": {}, "HOSTNAME": {}, "TERM": {}, "PWD": {}, "SHLVL": {},
		"LANG": {}, "LANGUAGE": {}, "LC_ALL": {}, "DEBIAN_FRONTEND": {}, "GPG_KEYS": {},
		"GOPATH": {}, "GOTOOLCHAIN": {}, "PHPIZE_DEPS": {},
	}
	baseImageEnvPrefixes = []string{
		"PHP_", "PYTHON_", "NODE_", "NPM_CONFIG_", "YARN_", "RUBY_", "RUBYGEMS_",
		"GOLANG_", "JAVA_", "PERL_", "COMPOSER_", "APACHE_", "NGINX_", "OPENSSL_",
	}
	baseImageEnvSuffixes = []string{
		"_VERSION", "_SHA256", "_SHA512", "_MD5", "_ASC_URL", "_CFLAGS", "_CPPFLAGS",
		"_LDFLAGS", "_DEPS", "_LIBS", "_KEYS", "_APPS",
	}
)

// isConfigurableEnv separates the handful of variables an operator actually
// sets from the build-time plumbing every base image carries. A presentation
// hint only: the full list is always returned.
func isConfigurableEnv(name, value string) bool {
	upper := strings.ToUpper(name)
	if _, ok := baseImageEnvNames[upper]; ok {
		return false
	}
	for _, prefix := range baseImageEnvPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return false
		}
	}
	for _, suffix := range baseImageEnvSuffixes {
		if strings.HasSuffix(upper, suffix) {
			return false
		}
	}
	// Multi-line values and package lists are build inputs, never settings.
	return len(value) <= 200 && !strings.ContainsAny(value, "\n\t")
}

var databaseHostSuffixes = []string{
	"DB_HOST", "DATABASE_HOST", "MYSQL_HOST", "MARIADB_HOST",
	"POSTGRES_HOST", "POSTGRESQL_HOST", "PGHOST",
}

// detectDatabaseHint recognises the naming convention images use for their
// database connection - PKP_DB_HOST/NAME/USER/PASSWORD, WORDPRESS_DB_*, MYSQL_*,
// PG* - and reports the base that whole group shares.
func detectDatabaseHint(env []ImageEnvVar) *ImageDatabaseHint {
	values := make(map[string]string, len(env))
	for _, item := range env {
		values[strings.ToUpper(item.Name)] = item.Value
	}

	for _, item := range env {
		name := strings.ToUpper(item.Name)
		matched := false
		for _, suffix := range databaseHostSuffixes {
			if name == suffix || strings.HasSuffix(name, "_"+suffix) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		base := strings.TrimSuffix(name, "HOST")

		pick := func(candidates ...string) string {
			for _, candidate := range candidates {
				if _, ok := values[base+candidate]; ok {
					return base + candidate
				}
			}
			return ""
		}
		nameEnv := pick("NAME", "DATABASE", "DB")
		userEnv := pick("USER", "USERNAME")
		passwordEnv := pick("PASSWORD", "PASS", "PASSWD")
		// Without a name, a user and a password there is nothing to wire a
		// database to, and a lone *_HOST is as likely to be a cache or a broker.
		if nameEnv == "" || userEnv == "" || passwordEnv == "" {
			continue
		}

		engine := "mysql"
		if strings.Contains(base, "POSTGRES") || strings.HasPrefix(base, "PG") {
			engine = "postgres"
		}
		return &ImageDatabaseHint{
			Engine:      engine,
			HostEnv:     item.Name,
			PortEnv:     pick("PORT"),
			NameEnv:     nameEnv,
			UserEnv:     userEnv,
			PasswordEnv: passwordEnv,
			Name:        values[nameEnv],
			User:        values[userEnv],
		}
	}
	return nil
}

// ---------- registry plumbing ----------

type registryClient struct {
	http       *http.Client
	host       string
	repository string
	token      string
}

func (c *registryClient) get(ctx context.Context, path, accept string) ([]byte, error) {
	endpoint := fmt.Sprintf("https://%s/v2/%s/%s", c.host, c.repository, path)
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("reach registry %s: %w", c.host, err)
		}
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			challenge := resp.Header.Get("Www-Authenticate")
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			token, tokenErr := fetchRegistryToken(ctx, c.http, challenge)
			if tokenErr != nil {
				return nil, tokenErr
			}
			c.token = token
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, imageConfigMaxBytes))
		statusCode := resp.StatusCode
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read response from registry %s: %w", c.host, readErr)
		}
		if statusCode == http.StatusNotFound {
			return nil, fmt.Errorf("registry %s has no %s for %s", c.host, path, c.repository)
		}
		if statusCode != http.StatusOK {
			return nil, fmt.Errorf("registry %s returned HTTP %d for %s", c.host, statusCode, c.repository)
		}
		return body, nil
	}
	return nil, fmt.Errorf("registry %s rejected an anonymous pull of %s", c.host, c.repository)
}

var authChallengeParam = regexp.MustCompile(`([a-zA-Z_]+)="([^"]*)"`)

func fetchRegistryToken(ctx context.Context, client *http.Client, challenge string) (string, error) {
	params := map[string]string{}
	for _, match := range authChallengeParam.FindAllStringSubmatch(challenge, -1) {
		params[strings.ToLower(match[1])] = match[2]
	}
	realm := params["realm"]
	if realm == "" {
		return "", fmt.Errorf("registry did not offer a bearer token realm")
	}
	tokenURL, err := url.Parse(realm)
	if err != nil || tokenURL.Host == "" || tokenURL.Scheme != "https" {
		return "", fmt.Errorf("registry offered an unusable token realm")
	}
	query := tokenURL.Query()
	for _, key := range []string{"service", "scope"} {
		if value := params[key]; value != "" {
			query.Set(key, value)
		}
	}
	tokenURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch registry token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry token endpoint returned HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", fmt.Errorf("parse registry token: %w", err)
	}
	token := firstNonEmpty(payload.Token, payload.AccessToken)
	if token == "" {
		return "", fmt.Errorf("registry token endpoint returned no token")
	}
	return token, nil
}

func selectPlatformManifest(manifests []ociDescriptor, osName, arch string) *ociDescriptor {
	var fallback *ociDescriptor
	for i := range manifests {
		entry := &manifests[i]
		if entry.Platform == nil {
			continue
		}
		// Attestation manifests ride along in the same index but carry no image.
		if entry.Platform.OS == "unknown" || entry.Platform.Architecture == "unknown" {
			continue
		}
		if entry.Platform.OS == osName && entry.Platform.Architecture == arch {
			return entry
		}
		if fallback == nil && entry.Platform.OS == osName {
			fallback = entry
		}
	}
	return fallback
}

func parseImageRef(ref string) (string, string, string, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", "", "", fmt.Errorf("image reference is required")
	}
	digest := ""
	if name, suffix, found := strings.Cut(ref, "@"); found {
		digest = suffix
		ref = name
	}
	host := ""
	remainder := ref
	if candidate, rest, found := strings.Cut(ref, "/"); found {
		if strings.ContainsAny(candidate, ".:") || candidate == "localhost" {
			host = candidate
			remainder = rest
		}
	}
	tag := ""
	if idx := strings.LastIndex(remainder, ":"); idx >= 0 && !strings.Contains(remainder[idx+1:], "/") {
		tag = remainder[idx+1:]
		remainder = remainder[:idx]
	}
	if host == "" {
		host = "docker.io"
	}
	if remainder == "" {
		return "", "", "", "", fmt.Errorf("image reference %q names no repository", ref)
	}
	// Docker Hub's official images live under an implicit "library" namespace.
	if isDockerHubHost(host) && !strings.Contains(remainder, "/") {
		remainder = "library/" + remainder
	}
	if tag == "" && digest == "" {
		tag = "latest"
	}
	return host, remainder, tag, digest, nil
}

func formatImageRef(host, repository, tag, digest string) string {
	name := repository
	if isDockerHubHost(host) {
		name = strings.TrimPrefix(name, "library/")
	} else {
		name = host + "/" + repository
	}
	if digest != "" {
		return name + "@" + digest
	}
	return name + ":" + tag
}

func parsePlatform(platform string) (string, string) {
	osName, arch, found := strings.Cut(strings.TrimSpace(platform), "/")
	if !found || osName == "" || arch == "" {
		return "linux", "amd64"
	}
	return osName, arch
}

func registryEndpoint(host string) string {
	if isDockerHubHost(host) {
		return dockerHubRegistryHost
	}
	return host
}

func isDockerHubHost(host string) bool {
	switch strings.ToLower(host) {
	case "docker.io", "index.docker.io", dockerHubRegistryHost:
		return true
	}
	return false
}

func dockerHubImageMirrorHost() string {
	if strings.EqualFold(strings.TrimSpace(conf.GetConfigStringDefault("imageRegistryMirror", "auto")), "never") {
		return ""
	}
	return dockerHubMirrorHost
}

func repositoryDisplayName(repository string) string {
	name := repository
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}

func sortedKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
