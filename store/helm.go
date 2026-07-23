package store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	semver "github.com/Masterminds/semver/v3"
	"github.com/sirupsen/logrus"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	k8sversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"gopkg.in/yaml.v3"
	sigsyaml "sigs.k8s.io/yaml"

	proxypkg "github.com/casosorg/casos/proxy"
)

const (
	helmOperationTimeout      = 5 * time.Minute
	helmInstallTimeout        = 10 * time.Minute
	helmDiagnosticsTimeout    = 15 * time.Second
	helmDiagnosticsMaxEvents  = 20
	helmDiagnosticsMessageLen = 240
	helmDiagnosticsEventLen   = 360
)

// ---------- Types ----------

type HelmChartSummary struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	AppVersion  string   `json:"appVersion"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Keywords    []string `json:"keywords"`
}

type HelmReleaseSummary struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Revision    string `json:"revision"`
	Updated     string `json:"updated"`
	Status      string `json:"status"`
	Chart       string `json:"chart"`
	AppVersion  string `json:"app_version"`
	Description string `json:"description"`
}

type HelmReleaseHistory struct {
	Revision    int    `json:"revision"`
	Updated     string `json:"updated"`
	Status      string `json:"status"`
	Chart       string `json:"chart"`
	AppVersion  string `json:"app_version"`
	Description string `json:"description"`
}

// ---------- RESTClientGetter: adapts *rest.Config to Helm's action.Configuration ----------

type restClientGetter struct {
	cfg       *rest.Config
	namespace string
}

func newRESTClientGetter(cfg *rest.Config, namespace string) *restClientGetter {
	return &restClientGetter{cfg: cfg, namespace: namespace}
}

func (r *restClientGetter) ToRESTConfig() (*rest.Config, error) {
	return r.cfg, nil
}

func (r *restClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(r.cfg)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(dc), nil
}

func (r *restClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	dc, err := r.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(dc), nil
}

func (r *restClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["casos"] = &clientcmdapi.Cluster{
		Server:                   r.cfg.Host,
		CertificateAuthorityData: r.cfg.CAData,
		InsecureSkipTLSVerify:    r.cfg.Insecure,
	}
	cfg.AuthInfos["casos"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: r.cfg.CertData,
		ClientKeyData:         r.cfg.KeyData,
		Token:                 r.cfg.BearerToken,
	}
	cfg.Contexts["casos"] = &clientcmdapi.Context{
		Cluster:   "casos",
		AuthInfo:  "casos",
		Namespace: r.namespace,
	}
	cfg.CurrentContext = "casos"
	return clientcmd.NewDefaultClientConfig(*cfg, &clientcmd.ConfigOverrides{})
}

// ---------- action.Configuration builder ----------

func newHelmConfig(cfg *rest.Config, namespace string) (*action.Configuration, error) {
	return newHelmConfigWithLog(cfg, namespace, func(string, ...interface{}) {})
}

func newHelmConfigWithLog(cfg *rest.Config, namespace string, logFn func(string, ...interface{})) (*action.Configuration, error) {
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(newRESTClientGetter(cfg, namespace), namespace, "secret", logFn); err != nil {
		return nil, fmt.Errorf("helm config init: %w", err)
	}
	return actionConfig, nil
}

func attachHelmCapabilities(actionConfig *action.Configuration, cfg *rest.Config, namespace string, logFn func(string, ...interface{})) {
	capabilities, err := buildHelmCapabilities(cfg, namespace, logFn)
	if err != nil {
		logFn("WARNING: failed to build helm capabilities, using defaults: %v", err)
		capabilities = chartutil.DefaultCapabilities
	}
	actionConfig.Capabilities = capabilities
}

func helmWarningLog(format string, args ...interface{}) {
	logrus.Warnf(format, args...)
}

func buildHelmCapabilities(cfg *rest.Config, namespace string, logFn func(string, ...interface{})) (*chartutil.Capabilities, error) {
	dc, err := newRESTClientGetter(cfg, namespace).ToDiscoveryClient()
	if err != nil {
		return nil, fmt.Errorf("helm discovery client: %w", err)
	}
	dc.Invalidate()

	kubeVersion, err := dc.ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("helm server version: %w", err)
	}

	apiVersions, err := action.GetVersionSet(dc)
	if err != nil {
		if discovery.IsGroupDiscoveryFailedError(err) {
			logFn("WARNING: The Kubernetes server has an orphaned API service. Server reports: %s", err)
			logFn("WARNING: To fix this, kubectl delete apiservice <service-name>")
			if apiVersions == nil {
				apiVersions = chartutil.VersionSet{}
			}
		} else {
			return nil, fmt.Errorf("helm api versions: %w", err)
		}
	}

	normalizedVersion := normalizeHelmKubeVersion(kubeVersion.GitVersion, kubeVersion.Major, kubeVersion.Minor)

	return &chartutil.Capabilities{
		APIVersions: apiVersions,
		KubeVersion: normalizedVersion,
		HelmVersion: chartutil.DefaultCapabilities.HelmVersion,
	}, nil
}

func normalizeHelmKubeVersion(gitVersion, major, minor string) chartutil.KubeVersion {
	normalizedGitVersion := gitVersion
	if idx := strings.Index(normalizedGitVersion, "+"); idx >= 0 {
		normalizedGitVersion = normalizedGitVersion[:idx]
	}

	semanticVersion, err := k8sversion.ParseSemantic(normalizedGitVersion)
	if err == nil {
		normalized := chartutil.KubeVersion{
			Version: "v" + semanticVersion.String(),
			Major:   strconv.Itoa(int(semanticVersion.Major())),
			Minor:   strconv.Itoa(int(semanticVersion.Minor())),
		}
		if shouldKeepHelmKubePrerelease(semanticVersion.PreRelease()) {
			return normalized
		}
		normalized.Version = fmt.Sprintf("v%d.%d.%d", semanticVersion.Major(), semanticVersion.Minor(), semanticVersion.Patch())
		return normalized
	}

	parsedVersion, err := chartutil.ParseKubeVersion(normalizedGitVersion)
	if err == nil {
		return *parsedVersion
	}

	if _, err := strconv.Atoi(major); err != nil {
		major = "0"
	}
	if _, err := strconv.Atoi(minor); err != nil {
		minor = "0"
	}

	return chartutil.KubeVersion{
		Version: normalizedGitVersion,
		Major:   major,
		Minor:   minor,
	}
}

func shouldKeepHelmKubePrerelease(preRelease string) bool {
	// Preserve canonical Kubernetes prereleases, but strip distro/vendor
	// suffixes such as k3s/eks so Helm's kubeVersion checks see the base
	// upstream version instead of a distribution tag.
	return preRelease == "" ||
		strings.HasPrefix(preRelease, "alpha") ||
		strings.HasPrefix(preRelease, "beta") ||
		strings.HasPrefix(preRelease, "rc")
}

// ---------- HTTP helper ----------

func helmGet(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Helm/3.21.2")
	return proxypkg.ProxyHttpClient.Do(req)
}

// ---------- Repo index ----------

func fetchIndexFile(repoURL string) (*repo.IndexFile, error) {
	indexURL := strings.TrimRight(repoURL, "/") + "/index.yaml"
	resp, err := helmGet(indexURL)
	if err != nil {
		return nil, fmt.Errorf(
			"fetch index %q: %w",
			redactURLForError(indexURL),
			sanitizeErrorMessage(err, indexURL, repoURL),
		)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("index returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// Use sigs.k8s.io/yaml (YAML→JSON→struct) so that embedded pointer fields
	// like *chart.Metadata inside ChartVersion are properly allocated. Plain
	// gopkg.in/yaml.v3 leaves those pointers nil, causing panics in SortEntries.
	var idx repo.IndexFile
	if err := sigsyaml.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse index: %w", err)
	}
	// Safety: drop any entry whose Metadata is still nil.
	for name, versions := range idx.Entries {
		filtered := versions[:0]
		for _, v := range versions {
			if v != nil && v.Metadata != nil {
				filtered = append(filtered, v)
			}
		}
		if len(filtered) == 0 {
			delete(idx.Entries, name)
		} else {
			idx.Entries[name] = filtered
		}
	}
	return &idx, nil
}

// FetchRepoIndex returns all charts listed in a Helm repo's index.yaml, or, for an
// "oci://" repoURL, the single chart hosted at that OCI reference.
func FetchRepoIndex(repoURL string) ([]HelmChartSummary, error) {
	if isOCIRepo(repoURL) {
		return fetchOCIChartSummary(repoURL)
	}

	idx, err := fetchIndexFile(repoURL)
	if err != nil {
		return nil, err
	}
	charts := make([]HelmChartSummary, 0, len(idx.Entries))
	for name, versions := range idx.Entries {
		if len(versions) == 0 {
			continue
		}
		v := versions[0]
		charts = append(charts, HelmChartSummary{
			Name:        name,
			Version:     v.Version,
			AppVersion:  v.AppVersion,
			Description: v.Description,
			Icon:        v.Icon,
			Keywords:    v.Keywords,
		})
	}
	return charts, nil
}

// ---------- OCI chart support ----------

// isOCIRepo reports whether repoURL is an OCI registry reference (e.g.
// "oci://registry-1.docker.io/casbin/casdoor-helm-charts") rather than a classic
// index.yaml-based Helm repo.
func isOCIRepo(repoURL string) bool {
	return strings.HasPrefix(repoURL, fmt.Sprintf("%s://", registry.OCIScheme))
}

func resolveOCIChartRef(repoURL, version string) (string, string) {
	ref := strings.TrimPrefix(repoURL, fmt.Sprintf("%s://", registry.OCIScheme))
	ref, taggedVersion := splitOCIChartTag(ref)
	if version != "" {
		return ref, version
	}
	return ref, taggedVersion
}

func splitOCIChartTag(ref string) (string, string) {
	if strings.Contains(ref, "@") {
		return ref, ""
	}
	lastSlashIdx := strings.LastIndex(ref, "/")
	tagIdx := strings.LastIndex(ref, ":")
	if lastSlashIdx < 0 || tagIdx <= lastSlashIdx {
		return ref, ""
	}
	tag := ref[tagIdx+1:]
	repoAndTag := ref[lastSlashIdx+1 : tagIdx]
	if strings.Contains(repoAndTag, ":") || !isOCIChartTag(tag) {
		return ref, ""
	}
	return ref[:tagIdx], tag
}

func isOCIChartTag(tag string) bool {
	if len(tag) == 0 || len(tag) > 128 {
		return false
	}
	for i := 0; i < len(tag); i++ {
		ch := tag[i]
		isAlphaNum := (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
		if i == 0 {
			if !isAlphaNum && ch != '_' {
				return false
			}
			continue
		}
		if !isAlphaNum && ch != '_' && ch != '.' && ch != '-' {
			return false
		}
	}
	return true
}

func newOCIRegistryClient() (*registry.Client, error) {
	return registry.NewClient(registry.ClientOptHTTPClient(proxypkg.ProxyHttpClient))
}

// pullOCIChart pulls the chart hosted at repoURL, resolving to the newest published
// semver tag when version is empty.
func pullOCIChart(repoURL, version string) (*registry.PullResult, error) {
	ref, resolvedVersion := resolveOCIChartRef(repoURL, version)

	rc, err := newOCIRegistryClient()
	if err != nil {
		return nil, fmt.Errorf("oci registry client: %w", err)
	}

	if resolvedVersion == "" {
		if !strings.Contains(ref, "@") {
			tags, err := rc.Tags(ref)
			if err != nil {
				return nil, fmt.Errorf(
					"list oci tags for %q: %w",
					redactURLForError(repoURL),
					sanitizeErrorMessage(err, repoURL, ref),
				)
			}
			if len(tags) == 0 {
				return nil, fmt.Errorf("no tags found for %s", redactURLForError(repoURL))
			}
			resolvedVersion = latestOCISemverTag(tags)
			if resolvedVersion == "" {
				return nil, fmt.Errorf("no semver tags found for %s", redactURLForError(repoURL))
			}
		}
	}

	pullRef := ref
	if resolvedVersion != "" {
		if strings.Contains(ref, "@") {
			return nil, fmt.Errorf("oci digest reference %s cannot be used with version %q", redactURLForError(repoURL), resolvedVersion)
		}
		pullRef = fmt.Sprintf("%s:%s", ref, resolvedVersion)
	}

	pull, err := rc.Pull(pullRef, registry.PullOptWithChart(true))
	if err != nil {
		return nil, fmt.Errorf(
			"pull oci chart %q: %w",
			redactURLForError(repoURL),
			sanitizeErrorMessage(err, repoURL, ref, pullRef),
		)
	}
	return pull, nil
}

func latestOCISemverTag(tags []string) string {
	type versionedTag struct {
		tag     string
		version *semver.Version
	}

	versionedTags := make([]versionedTag, 0, len(tags))
	for _, tag := range tags {
		version, err := semver.NewVersion(tag)
		if err != nil {
			continue
		}
		versionedTags = append(versionedTags, versionedTag{tag: tag, version: version})
	}
	if len(versionedTags) == 0 {
		return ""
	}

	sort.SliceStable(versionedTags, func(i, j int) bool {
		return versionedTags[i].version.GreaterThan(versionedTags[j].version)
	})
	return versionedTags[0].tag
}

func loadOCIChart(repoURL, version string) (*chart.Chart, error) {
	pull, err := pullOCIChart(repoURL, version)
	if err != nil {
		return nil, err
	}
	return loader.LoadArchive(bytes.NewReader(pull.Chart.Data))
}

func fetchOCIChartSummary(repoURL string) ([]HelmChartSummary, error) {
	pull, err := pullOCIChart(repoURL, "")
	if err != nil {
		return nil, err
	}
	meta := pull.Chart.Meta
	if meta == nil {
		return nil, fmt.Errorf("chart metadata not found for %s", redactURLForError(repoURL))
	}
	return []HelmChartSummary{
		{
			Name:        meta.Name,
			Version:     meta.Version,
			AppVersion:  meta.AppVersion,
			Description: meta.Description,
			Icon:        meta.Icon,
			Keywords:    meta.Keywords,
		},
	}, nil
}

// ---------- Chart loader ----------

func redactURLForError(raw string) string {
	if raw == "" {
		return raw
	}
	parsed, addedScheme, err := parseURLForRedaction(raw)
	if err != nil {
		return fallbackRedactCredentialSegment(raw)
	}
	if parsed.User != nil {
		parsed.User = url.User("REDACTED")
	}
	if parsed.RawQuery != "" {
		query := parsed.Query()
		for key := range query {
			if isCredentialQueryKey(key) {
				query.Set(key, "REDACTED")
			}
		}
		parsed.RawQuery = query.Encode()
	}
	redacted := parsed.String()
	if addedScheme {
		return strings.TrimPrefix(redacted, "redact://")
	}
	return redacted
}

func parseURLForRedaction(raw string) (*url.URL, bool, error) {
	work := raw
	addedScheme := false
	if !strings.Contains(raw, "://") {
		work = "redact://" + raw
		addedScheme = true
	}
	parsed, err := url.Parse(work)
	return parsed, addedScheme, err
}

func isCredentialQueryKey(key string) bool {
	switch strings.ToLower(key) {
	case "access_key", "accesskey", "access_token", "apikey", "api_key", "api_secret", "app_key", "app_secret", "auth", "auth_token", "cert", "client_secret", "credential", "key", "password", "passwd", "private_key", "refresh_token", "sas_token", "secret", "secret_key", "secretkey", "session_token", "shared_access_key", "sign", "signature", "token":
		return true
	default:
		return false
	}
}

func fallbackRedactCredentialSegment(raw string) string {
	redacted := fallbackRedactCredentialQuery(raw)
	at := strings.Index(raw, "@")
	if at <= 0 {
		return redacted
	}
	start := strings.LastIndex(raw[:at], "://")
	if start >= 0 {
		start += 3
	} else {
		start = 0
	}
	candidate := raw[start:at]
	if candidate == "" || strings.ContainsAny(candidate, "/?#") {
		return redacted
	}
	if looksLikeHostPort(candidate) {
		return redacted
	}
	return redacted[:start] + "REDACTED" + redacted[at:]
}

// fallbackRedactCredentialQuery handles malformed URLs that url.Parse cannot
// process, for example invalid percent escapes in sensitive query values.
func fallbackRedactCredentialQuery(raw string) string {
	queryStart := strings.Index(raw, "?")
	if queryStart < 0 {
		return raw
	}
	queryEnd := len(raw)
	if fragmentStart := strings.Index(raw[queryStart+1:], "#"); fragmentStart >= 0 {
		queryEnd = queryStart + 1 + fragmentStart
	}
	query := raw[queryStart+1 : queryEnd]
	parts := strings.Split(query, "&")
	changed := false
	for i, part := range parts {
		key, value, ok := strings.Cut(part, "=")
		if !ok || value == "" || !isCredentialQueryKey(key) {
			continue
		}
		parts[i] = key + "=REDACTED"
		changed = true
	}
	if !changed {
		return raw
	}
	return raw[:queryStart+1] + strings.Join(parts, "&") + raw[queryEnd:]
}

// looksLikeHostPort avoids treating malformed host:port text before an "@"
// as userinfo when the parser has already rejected the whole string.
func looksLikeHostPort(value string) bool {
	host, port, ok := strings.Cut(value, ":")
	if !ok || host == "" || port == "" {
		return false
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return false
		}
	}
	for _, r := range host {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return true
}

type sanitizedDisplayError struct {
	message string
	cause   error
}

func (e *sanitizedDisplayError) Error() string {
	return e.message
}

func (e *sanitizedDisplayError) Unwrap() error {
	return e.cause
}

func sanitizeErrorMessage(err error, rawValues ...string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	values := make([]string, 0, len(rawValues)*2)
	seen := make(map[string]struct{}, len(rawValues)*2)
	for _, raw := range rawValues {
		for _, candidate := range redactionCandidates(raw) {
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			values = append(values, candidate)
		}
	}
	sort.Slice(values, func(i, j int) bool {
		return len(values[i]) > len(values[j])
	})
	for _, raw := range values {
		message = strings.ReplaceAll(message, raw, redactURLForError(raw))
	}
	if message == err.Error() {
		return err
	}
	return &sanitizedDisplayError{message: message, cause: err}
}

func redactionCandidates(raw string) []string {
	if raw == "" {
		return nil
	}
	candidates := []string{raw}
	parsed, addedScheme, err := parseURLForRedaction(raw)
	if err != nil {
		return candidates
	}
	normalized := parsed.String()
	if addedScheme {
		normalized = strings.TrimPrefix(normalized, "redact://")
	}
	if normalized != raw {
		candidates = append(candidates, normalized)
	}
	return candidates
}

func loadChart(chartName, repoURL, version string) (*chart.Chart, error) {
	if isOCIRepo(repoURL) {
		ch, err := loadOCIChart(repoURL, version)
		if err != nil {
			return nil, fmt.Errorf(
				"load chart %q from OCI repo %q version %q: %w",
				chartName,
				redactURLForError(repoURL),
				version,
				err,
			)
		}
		return ch, nil
	}

	idx, err := fetchIndexFile(repoURL)
	if err != nil {
		return nil, fmt.Errorf("load chart %q from repo %q version %q: fetch index.yaml failed: %w", chartName, redactURLForError(repoURL), version, err)
	}

	versions, ok := idx.Entries[chartName]
	if !ok || len(versions) == 0 {
		return nil, fmt.Errorf("chart %q not found in repo", chartName)
	}

	var entry *repo.ChartVersion
	for _, v := range versions {
		if version == "" || v.Version == version {
			entry = v
			break
		}
	}
	if entry == nil {
		return nil, fmt.Errorf("chart %q version %q not found", chartName, version)
	}
	if len(entry.URLs) == 0 {
		return nil, fmt.Errorf("chart %q has no download URLs", chartName)
	}

	chartURL := entry.URLs[0]
	if isOCIRepo(chartURL) {
		ociVersion := version
		if ociVersion == "" {
			ociVersion = entry.Version
		}
		ch, err := loadOCIChart(chartURL, ociVersion)
		if err != nil {
			return nil, fmt.Errorf(
				"load chart %q from repo %q version %q: index.yaml resolved to OCI chart URL %q: %w",
				chartName,
				redactURLForError(repoURL),
				ociVersion,
				redactURLForError(chartURL),
				err,
			)
		}
		return ch, nil
	}
	if !strings.HasPrefix(chartURL, "http") {
		chartURL = strings.TrimRight(repoURL, "/") + "/" + strings.TrimLeft(chartURL, "/")
	}

	resp, err := helmGet(chartURL)
	if err != nil {
		return nil, fmt.Errorf(
			"load chart %q from repo %q version %q: download chart archive %q failed: %w",
			chartName,
			redactURLForError(repoURL),
			entry.Version,
			redactURLForError(chartURL),
			sanitizeErrorMessage(err, chartURL, repoURL),
		)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf(
			"load chart %q from repo %q version %q: chart archive %q returned HTTP %d",
			chartName,
			redactURLForError(repoURL),
			entry.Version,
			redactURLForError(chartURL),
			resp.StatusCode,
		)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf(
			"load chart %q from repo %q version %q: read chart archive %q failed: %w",
			chartName,
			redactURLForError(repoURL),
			entry.Version,
			redactURLForError(chartURL),
			err,
		)
	}
	ch, err := loader.LoadArchive(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf(
			"load chart %q from repo %q version %q: parse chart archive %q failed: %w",
			chartName,
			redactURLForError(repoURL),
			entry.Version,
			redactURLForError(chartURL),
			err,
		)
	}
	return ch, nil
}

func parseValues(valuesYAML string) (map[string]interface{}, error) {
	if strings.TrimSpace(valuesYAML) == "" {
		return map[string]interface{}{}, nil
	}
	vals := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(valuesYAML), &vals); err != nil {
		return nil, fmt.Errorf("parse values: %w", err)
	}
	return vals, nil
}

func withHelmReleaseDiagnostics(ctx context.Context, cfg *rest.Config, releaseName, namespace string, err error) error {
	if err == nil {
		return nil
	}
	lines := helmReleaseDiagnostics(ctx, cfg, releaseName, namespace)
	if len(lines) == 0 {
		return err
	}
	return fmt.Errorf("%w\n%s", err, strings.Join(lines, "\n"))
}

func helmReleaseDiagnostics(parent context.Context, cfg *rest.Config, releaseName, namespace string) []string {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, helmDiagnosticsTimeout)
	defer cancel()

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return []string{fmt.Sprintf("Helm release diagnostics unavailable: %v", err)}
	}
	if namespace == "" {
		namespace = "default"
	}

	selector := labels.SelectorFromSet(labels.Set{"app.kubernetes.io/instance": releaseName}).String()
	lines := []string{
		fmt.Sprintf("Helm release diagnostics for %s/%s:", namespace, releaseName),
		fmt.Sprintf("  selector: %s", selector),
	}
	objectNames := map[string]bool{releaseName: true}
	addObjectName := func(name string) {
		if name != "" {
			objectNames[name] = true
		}
	}

	deployments, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		lines = append(lines, fmt.Sprintf("  list deployments failed: %v", err))
	} else if len(deployments.Items) == 0 {
		lines = append(lines, "  no deployments found for release")
	} else {
		for _, deployment := range deployments.Items {
			addObjectName(deployment.Name)
			lines = appendDeploymentDiagnostics(lines, deployment)
		}
	}

	replicaSets, err := client.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		lines = append(lines, fmt.Sprintf("  list replicasets failed: %v", err))
	} else if len(replicaSets.Items) == 0 {
		lines = append(lines, "  no replicasets found for release")
	} else {
		for _, replicaSet := range replicaSets.Items {
			addObjectName(replicaSet.Name)
			lines = appendReplicaSetDiagnostics(lines, replicaSet)
		}
	}

	services, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		lines = append(lines, fmt.Sprintf("  list services failed: %v", err))
	} else if len(services.Items) == 0 {
		lines = append(lines, "  no services found for release")
	} else {
		for _, service := range services.Items {
			addObjectName(service.Name)
			lines = appendServiceDiagnostics(lines, service)
		}
	}

	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		lines = append(lines, fmt.Sprintf("  list pods failed: %v", err))
	} else if len(pods.Items) == 0 {
		lines = append(lines, "  no pods found for release")
	} else {
		for _, pod := range pods.Items {
			addObjectName(pod.Name)
			lines = appendPodDiagnostics(lines, pod)
		}
	}

	lines = appendEventDiagnostics(ctx, client, lines, namespace, releaseName, objectNames)
	return lines
}

func appendDeploymentDiagnostics(lines []string, deployment appsv1.Deployment) []string {
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	lines = append(lines, fmt.Sprintf(
		"  Deployment %s: ready=%d/%d available=%d updated=%d observedGeneration=%d generation=%d",
		deployment.Name,
		deployment.Status.ReadyReplicas,
		desired,
		deployment.Status.AvailableReplicas,
		deployment.Status.UpdatedReplicas,
		deployment.Status.ObservedGeneration,
		deployment.Generation,
	))
	for _, condition := range deployment.Status.Conditions {
		lines = append(lines, fmt.Sprintf(
			"    condition %s=%s reason=%s message=%s",
			condition.Type,
			condition.Status,
			emptyDash(condition.Reason),
			oneLineDiagnosticText(condition.Message, helmDiagnosticsMessageLen),
		))
	}
	return lines
}

func appendReplicaSetDiagnostics(lines []string, replicaSet appsv1.ReplicaSet) []string {
	desired := int32(1)
	if replicaSet.Spec.Replicas != nil {
		desired = *replicaSet.Spec.Replicas
	}
	lines = append(lines, fmt.Sprintf(
		"  ReplicaSet %s: ready=%d/%d available=%d observedGeneration=%d generation=%d",
		replicaSet.Name,
		replicaSet.Status.ReadyReplicas,
		desired,
		replicaSet.Status.AvailableReplicas,
		replicaSet.Status.ObservedGeneration,
		replicaSet.Generation,
	))
	for _, condition := range replicaSet.Status.Conditions {
		lines = append(lines, fmt.Sprintf(
			"    condition %s=%s reason=%s message=%s",
			condition.Type,
			condition.Status,
			emptyDash(condition.Reason),
			oneLineDiagnosticText(condition.Message, helmDiagnosticsMessageLen),
		))
	}
	return lines
}

func appendServiceDiagnostics(lines []string, service corev1.Service) []string {
	ports := make([]string, 0, len(service.Spec.Ports))
	for _, port := range service.Spec.Ports {
		portText := fmt.Sprintf("%s/%d->%s", port.Protocol, port.Port, port.TargetPort.String())
		if port.NodePort != 0 {
			portText = fmt.Sprintf("%s nodePort=%d", portText, port.NodePort)
		}
		ports = append(ports, portText)
	}
	lines = append(lines, fmt.Sprintf(
		"  Service %s: type=%s clusterIP=%s ports=[%s] selector=%v",
		service.Name,
		service.Spec.Type,
		service.Spec.ClusterIP,
		strings.Join(ports, ", "),
		service.Spec.Selector,
	))
	return lines
}

func appendPodDiagnostics(lines []string, pod corev1.Pod) []string {
	readyContainers, totalContainers := podReadyContainers(pod)
	lines = append(lines, fmt.Sprintf(
		"  Pod %s: phase=%s ready=%d/%d restarts=%d node=%s podIP=%s reason=%s message=%s",
		pod.Name,
		pod.Status.Phase,
		readyContainers,
		totalContainers,
		podRestartCount(pod),
		emptyDash(pod.Spec.NodeName),
		emptyDash(pod.Status.PodIP),
		emptyDash(pod.Status.Reason),
		oneLineDiagnosticText(pod.Status.Message, helmDiagnosticsMessageLen),
	))
	for _, condition := range pod.Status.Conditions {
		lines = append(lines, fmt.Sprintf(
			"    condition %s=%s reason=%s message=%s",
			condition.Type,
			condition.Status,
			emptyDash(condition.Reason),
			oneLineDiagnosticText(condition.Message, helmDiagnosticsMessageLen),
		))
	}

	for _, status := range pod.Status.InitContainerStatuses {
		lines = appendContainerStatusDiagnostics(lines, "init container", status)
	}
	for _, status := range pod.Status.ContainerStatuses {
		lines = appendContainerStatusDiagnostics(lines, "container", status)
	}
	return lines
}

func appendContainerStatusDiagnostics(lines []string, kind string, status corev1.ContainerStatus) []string {
	lines = append(lines, fmt.Sprintf(
		"    %s %s: ready=%t restarts=%d state=%s",
		kind,
		status.Name,
		status.Ready,
		status.RestartCount,
		containerStateText(status.State),
	))
	lastState := containerStateText(status.LastTerminationState)
	if lastState != "none" {
		lines = append(lines, fmt.Sprintf("      lastState=%s", lastState))
	}
	return lines
}

func appendEventDiagnostics(ctx context.Context, client kubernetes.Interface, lines []string, namespace, releaseName string, objectNames map[string]bool) []string {
	matchedEventsByKey := map[string]corev1.Event{}
	for objectName := range objectNames {
		events, err := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
			FieldSelector: fields.OneTermEqualSelector("involvedObject.name", objectName).String(),
		})
		if err != nil {
			lines = append(lines, fmt.Sprintf("  list events for %s failed: %v", objectName, err))
			continue
		}
		for _, event := range events.Items {
			matchedEventsByKey[eventKey(event)] = event
		}
	}

	matchedEvents := make([]corev1.Event, 0, len(matchedEventsByKey))
	for _, event := range matchedEventsByKey {
		if objectNames[event.InvolvedObject.Name] || strings.Contains(event.Message, releaseName) {
			matchedEvents = append(matchedEvents, event)
		}
	}
	sort.Slice(matchedEvents, func(i, j int) bool {
		return eventTime(matchedEvents[i]).Before(eventTime(matchedEvents[j]))
	})
	if len(matchedEvents) == 0 {
		return append(lines, "  no related events found")
	}
	if len(matchedEvents) > helmDiagnosticsMaxEvents {
		matchedEvents = matchedEvents[len(matchedEvents)-helmDiagnosticsMaxEvents:]
	}
	lines = append(lines, "  recent related events:")
	for _, event := range matchedEvents {
		lines = append(lines, fmt.Sprintf(
			"    %s %s %s %s/%s count=%d message=%s",
			eventTime(event).Format(time.RFC3339),
			event.Type,
			event.Reason,
			event.InvolvedObject.Kind,
			event.InvolvedObject.Name,
			event.Count,
			oneLineDiagnosticText(event.Message, helmDiagnosticsEventLen),
		))
	}
	return lines
}

func eventKey(event corev1.Event) string {
	if event.UID != "" {
		return string(event.UID)
	}
	return fmt.Sprintf(
		"%s/%s/%s/%s/%s/%s",
		event.Namespace,
		event.InvolvedObject.Kind,
		event.InvolvedObject.Name,
		event.Type,
		event.Reason,
		eventTime(event).Format(time.RFC3339Nano),
	)
}

func podReadyContainers(pod corev1.Pod) (int, int) {
	ready := 0
	for _, status := range pod.Status.ContainerStatuses {
		if status.Ready {
			ready++
		}
	}
	return ready, len(pod.Spec.Containers)
}

func podRestartCount(pod corev1.Pod) int32 {
	var restarts int32
	for _, status := range pod.Status.InitContainerStatuses {
		restarts += status.RestartCount
	}
	for _, status := range pod.Status.ContainerStatuses {
		restarts += status.RestartCount
	}
	return restarts
}

func containerStateText(state corev1.ContainerState) string {
	switch {
	case state.Waiting != nil:
		return fmt.Sprintf(
			"waiting reason=%s message=%s",
			emptyDash(state.Waiting.Reason),
			oneLineDiagnosticText(state.Waiting.Message, helmDiagnosticsMessageLen),
		)
	case state.Running != nil:
		return fmt.Sprintf("running startedAt=%s", state.Running.StartedAt.Time.Format(time.RFC3339))
	case state.Terminated != nil:
		return fmt.Sprintf(
			"terminated reason=%s exitCode=%d signal=%d finishedAt=%s message=%s",
			emptyDash(state.Terminated.Reason),
			state.Terminated.ExitCode,
			state.Terminated.Signal,
			state.Terminated.FinishedAt.Time.Format(time.RFC3339),
			oneLineDiagnosticText(state.Terminated.Message, helmDiagnosticsMessageLen),
		)
	default:
		return "none"
	}
}

func eventTime(event corev1.Event) time.Time {
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	if !event.FirstTimestamp.IsZero() {
		return event.FirstTimestamp.Time
	}
	return event.CreationTimestamp.Time
}

func emptyDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func oneLineDiagnosticText(text string, maxLen int) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "-"
	}
	if len(text) <= maxLen {
		return text
	}
	if maxLen <= 3 {
		return text[:maxLen]
	}
	return text[:maxLen-3] + "..."
}

// ---------- Release helpers ----------

func relToSummary(r *release.Release) HelmReleaseSummary {
	chartStr, appVersion := "", ""
	if r.Chart != nil && r.Chart.Metadata != nil {
		chartStr = r.Chart.Metadata.Name + "-" + r.Chart.Metadata.Version
		appVersion = r.Chart.Metadata.AppVersion
	}
	return HelmReleaseSummary{
		Name:        r.Name,
		Namespace:   r.Namespace,
		Revision:    fmt.Sprintf("%d", r.Version),
		Updated:     r.Info.LastDeployed.UTC().Format(time.RFC3339),
		Status:      string(r.Info.Status),
		Chart:       chartStr,
		AppVersion:  appVersion,
		Description: r.Info.Description,
	}
}

// ---------- Lifecycle operations ----------

func GetHelmReleases(cfg *rest.Config, namespace string) ([]HelmReleaseSummary, error) {
	ns := namespace
	if ns == "all" {
		ns = ""
	}
	actionConfig, err := newHelmConfig(cfg, ns)
	if err != nil {
		return nil, err
	}

	listAction := action.NewList(actionConfig)
	listAction.StateMask = action.ListAll
	if ns == "" {
		listAction.AllNamespaces = true
	}

	releases, err := listAction.Run()
	if err != nil {
		return nil, err
	}

	result := make([]HelmReleaseSummary, 0, len(releases))
	for _, r := range releases {
		result = append(result, relToSummary(r))
	}
	return result, nil
}

func InstallHelmChart(cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML string) error {
	actionConfig, err := newHelmConfig(cfg, namespace)
	if err != nil {
		return err
	}
	ch, err := loadChart(chartName, repoURL, version)
	if err != nil {
		return err
	}
	vals, err := parseValues(valuesYAML)
	if err != nil {
		return err
	}

	attachHelmCapabilities(actionConfig, cfg, namespace, helmWarningLog)
	install := action.NewInstall(actionConfig)
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.CreateNamespace = true
	install.Wait = true
	install.Timeout = helmInstallTimeout

	_, err = install.Run(ch, vals)
	if err != nil {
		return withHelmReleaseDiagnostics(context.Background(), cfg, releaseName, namespace, err)
	}
	return nil
}

type HelmInstallLifecycle interface {
	StartLoading() error
	MarkInstalling() error
	RecordLog(line string) error
	Finish(installErr error) error
}

// InstallHelmChartStream runs a Helm install independently of the browser
// request. Lifecycle persistence is supplied by the caller so store remains
// independent of the database layer.
func InstallHelmChartStream(ctx context.Context, lifecycle HelmInstallLifecycle, cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML string) <-chan string {
	logCh := make(chan string, 64)
	if lifecycle == nil {
		logCh <- "ERROR: Helm install lifecycle is required"
		close(logCh)
		return logCh
	}
	go func() {
		defer close(logCh)
		streamCtx := ctx
		if streamCtx == nil {
			streamCtx = context.Background()
		}
		installCtx := context.WithoutCancel(streamCtx)
		send := func(line string) bool {
			if err := lifecycle.RecordLog(line); err != nil {
				logrus.Warnf("failed to persist Helm operation log: %v", err)
			}
			select {
			case logCh <- line:
				return true
			case <-streamCtx.Done():
				return false
			}
		}
		if err := lifecycle.StartLoading(); err != nil {
			send("ERROR: " + err.Error())
			_ = lifecycle.Finish(err)
			return
		}
		logFn := func(format string, args ...interface{}) {
			send(fmt.Sprintf(format, args...))
		}
		actionConfig, err := newHelmConfigWithLog(cfg, namespace, logFn)
		if err != nil {
			send("ERROR: " + err.Error())
			_ = lifecycle.Finish(err)
			return
		}
		helmChart, err := loadChart(chartName, repoURL, version)
		if err != nil {
			send("ERROR: " + err.Error())
			_ = lifecycle.Finish(err)
			return
		}
		vals, err := parseValues(valuesYAML)
		if err != nil {
			send("ERROR: " + err.Error())
			_ = lifecycle.Finish(err)
			return
		}
		attachHelmCapabilities(actionConfig, cfg, namespace, logFn)
		if err := lifecycle.MarkInstalling(); err != nil {
			send("ERROR: " + err.Error())
			_ = lifecycle.Finish(err)
			return
		}
		install := action.NewInstall(actionConfig)
		install.ReleaseName = releaseName
		install.Namespace = namespace
		install.CreateNamespace = true
		install.Wait = true
		install.Timeout = helmInstallTimeout
		if _, err = install.RunWithContext(installCtx, helmChart, vals); err != nil {
			for _, line := range helmReleaseDiagnostics(installCtx, cfg, releaseName, namespace) {
				send(line)
			}
			send("ERROR: " + err.Error())
			_ = lifecycle.Finish(err)
			return
		}
		if err := lifecycle.Finish(nil); err != nil {
			logrus.Warnf("failed to finish Helm operation: %v", err)
			select {
			case logCh <- "ERROR: " + err.Error():
			case <-streamCtx.Done():
			}
			return
		}
		select {
		case logCh <- "DONE":
		case <-streamCtx.Done():
		}
	}()
	return logCh
}

func UpgradeHelmRelease(cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML string) error {
	actionConfig, err := newHelmConfig(cfg, namespace)
	if err != nil {
		return err
	}
	ch, err := loadChart(chartName, repoURL, version)
	if err != nil {
		return err
	}
	vals, err := parseValues(valuesYAML)
	if err != nil {
		return err
	}

	attachHelmCapabilities(actionConfig, cfg, namespace, helmWarningLog)
	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Namespace = namespace
	upgrade.Wait = true
	upgrade.Timeout = helmOperationTimeout

	_, err = upgrade.Run(releaseName, ch, vals)
	return err
}

func RollbackHelmRelease(cfg *rest.Config, releaseName, namespace string, revision int) error {
	actionConfig, err := newHelmConfig(cfg, namespace)
	if err != nil {
		return err
	}
	rollback := action.NewRollback(actionConfig)
	rollback.Version = revision
	rollback.Wait = true
	rollback.Timeout = helmOperationTimeout
	return rollback.Run(releaseName)
}

func UninstallHelmRelease(cfg *rest.Config, releaseName, namespace string) error {
	actionConfig, err := newHelmConfig(cfg, namespace)
	if err != nil {
		return err
	}
	uninstall := action.NewUninstall(actionConfig)
	uninstall.Wait = true
	uninstall.Timeout = helmOperationTimeout
	_, err = uninstall.Run(releaseName)
	return err
}

func GetHelmReleaseHistory(cfg *rest.Config, releaseName, namespace string) ([]HelmReleaseHistory, error) {
	actionConfig, err := newHelmConfig(cfg, namespace)
	if err != nil {
		return nil, err
	}
	histAction := action.NewHistory(actionConfig)
	histAction.Max = 20

	releases, err := histAction.Run(releaseName)
	if err != nil {
		return nil, err
	}

	result := make([]HelmReleaseHistory, 0, len(releases))
	for _, r := range releases {
		chartStr, appVersion := "", ""
		if r.Chart != nil && r.Chart.Metadata != nil {
			chartStr = r.Chart.Metadata.Name + "-" + r.Chart.Metadata.Version
			appVersion = r.Chart.Metadata.AppVersion
		}
		result = append(result, HelmReleaseHistory{
			Revision:    r.Version,
			Updated:     r.Info.LastDeployed.UTC().Format(time.RFC3339),
			Status:      string(r.Info.Status),
			Chart:       chartStr,
			AppVersion:  appVersion,
			Description: r.Info.Description,
		})
	}
	return result, nil
}

// GetHelmChartDefaultValues downloads a chart and returns its default values.yaml content.
func GetHelmChartDefaultValues(chartName, repoURL, version string) (string, error) {
	ch, err := loadChart(chartName, repoURL, version)
	if err != nil {
		return "", err
	}
	if ch.Values == nil {
		return "", nil
	}
	data, err := yaml.Marshal(ch.Values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
