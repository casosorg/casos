package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
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

	"github.com/casosorg/casos/conf"
	proxypkg "github.com/casosorg/casos/proxy"
)

const (
	helmOperationTimeout      = 5 * time.Minute
	helmCompatibilityTimeout  = 2 * time.Minute
	helmChartLoadTimeout      = 2 * time.Minute
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
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Revision     string `json:"revision"`
	Updated      string `json:"updated"`
	Status       string `json:"status"`
	Chart        string `json:"chart"`
	ChartName    string `json:"chartName"`
	ChartVersion string `json:"chartVersion"`
	RepoURL      string `json:"repoURL,omitempty"`
	AppVersion   string `json:"app_version"`
	Description  string `json:"description"`
	Icon         string `json:"icon,omitempty"`
}

const helmRepoURLAnnotation = "casos.org/helm-repository-url"

func setHelmChartRepoURL(ch *chart.Chart, repoURL string) {
	if ch == nil || ch.Metadata == nil {
		return
	}
	repoURL = helmRepositoryIdentity(repoURL)
	if repoURL == "" {
		delete(ch.Metadata.Annotations, helmRepoURLAnnotation)
		return
	}
	if ch.Metadata.Annotations == nil {
		ch.Metadata.Annotations = map[string]string{}
	}
	ch.Metadata.Annotations[helmRepoURLAnnotation] = repoURL
}

func helmChartRepoURL(ch *chart.Chart) string {
	if ch == nil || ch.Metadata == nil {
		return ""
	}
	return helmRepositoryIdentity(ch.Metadata.Annotations[helmRepoURLAnnotation])
}

func helmRepositoryIdentity(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	switch parsed.Scheme {
	case "http", "https", "oci":
	default:
		return ""
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	return strings.TrimSpace(parsed.String())
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

// newClusterContext gives install adapters the cluster facts a host binding
// needs. Node addresses resolve lazily and only once, so charts without a
// binding never reach the API server and charts with several bindings list the
// nodes a single time.
func newClusterContext(cfg *rest.Config, releaseName, namespace string) clusterContext {
	services := memoizedServices(func() []corev1.Service { return getClusterServices(cfg) })
	return clusterContext{
		nodeIPs: memoizedNodeIPs(func() []string { return getClusterNodeIPs(cfg) }),
		usedNodePorts: func() map[int32]bool {
			return clusterUsedNodePorts(services())
		},
		releaseNodePorts: func() map[int32]int32 {
			return releaseAssignedNodePorts(services(), releaseName, namespace)
		},
		releaseName: releaseName,
		namespace:   namespace,
	}
}

// memoizedServices defers the Service listing until an adapter asks for it and
// keeps the answer, so the two node-port questions share one call.
func memoizedServices(resolve func() []corev1.Service) func() []corev1.Service {
	var once sync.Once
	var services []corev1.Service
	return func() []corev1.Service {
		once.Do(func() { services = resolve() })
		return services
	}
}

// getClusterServices lists every Service in the cluster. Returns nil on any
// failure; the callers treat that as "cannot tell" and leave port selection to
// Kubernetes rather than risk pinning one that is already taken.
func getClusterServices(cfg *rest.Config) []corev1.Service {
	if cfg == nil {
		return nil
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logrus.Warnf("build service client for install adapter: %v", err)
		return nil
	}
	services, err := client.CoreV1().Services("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		logrus.Warnf("list services for install adapter: %v", err)
		return nil
	}
	return services.Items
}

// clusterUsedNodePorts collects the node ports already bound cluster-wide. A nil
// result means the Service list could not be read, which is distinct from an
// empty one: nothing may be pinned on a guess.
func clusterUsedNodePorts(services []corev1.Service) map[int32]bool {
	if services == nil {
		return nil
	}
	used := map[int32]bool{}
	for _, service := range services {
		for _, port := range service.Spec.Ports {
			if port.NodePort != 0 {
				used[port.NodePort] = true
			}
		}
	}
	return used
}

// releaseAssignedNodePorts maps a release's Service ports to the node ports they
// already hold, so an upgrade republishes the app on the address it is already
// reachable at instead of moving it.
func releaseAssignedNodePorts(services []corev1.Service, releaseName, namespace string) map[int32]int32 {
	assigned := map[int32]int32{}
	for _, service := range services {
		if namespace != "" && service.Namespace != namespace {
			continue
		}
		if service.Labels["app.kubernetes.io/instance"] != releaseName {
			continue
		}
		for _, port := range service.Spec.Ports {
			if port.NodePort != 0 && assigned[port.Port] == 0 {
				assigned[port.Port] = port.NodePort
			}
		}
	}
	return assigned
}

// memoizedNodeIPs defers a node lookup until a host binding asks for it and
// keeps the answer, so several bindings on one chart share a single list call.
func memoizedNodeIPs(resolve func() []string) func() []string {
	var once sync.Once
	var ips []string
	return func() []string {
		once.Do(func() { ips = resolve() })
		return ips
	}
}

// getClusterNodeIPs returns the addresses users reach the cluster through:
// every node's internal and external IP. The app store publishes apps as
// NodePort services, so these are the hosts an app must be willing to answer
// to. Returns nil on any failure, leaving installs to degrade rather than fail.
func getClusterNodeIPs(cfg *rest.Config) []string {
	if cfg == nil {
		return nil
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logrus.Warnf("build node client for install adapter: %v", err)
		return nil
	}
	nodes, err := client.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		logrus.Warnf("list nodes for install adapter: %v", err)
		return nil
	}
	var ips []string
	seen := map[string]bool{}
	for _, node := range nodes.Items {
		for _, address := range node.Status.Addresses {
			if address.Type != corev1.NodeInternalIP && address.Type != corev1.NodeExternalIP {
				continue
			}
			if address.Address == "" || seen[address.Address] {
				continue
			}
			seen[address.Address] = true
			ips = append(ips, address.Address)
		}
	}
	return ips
}

func newHelmConfigWithLog(cfg *rest.Config, namespace string, logFn func(string, ...interface{})) (*action.Configuration, error) {
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(newRESTClientGetter(cfg, namespace), namespace, "secret", logFn); err != nil {
		return nil, fmt.Errorf("helm config init: %w", err)
	}
	return actionConfig, nil
}

func attachHelmCapabilities(ctx context.Context, actionConfig *action.Configuration, cfg *rest.Config, logFn func(string, ...interface{})) {
	capabilities, err := buildHelmCapabilities(ctx, cfg, logFn)
	if err != nil {
		logFn("WARNING: failed to build helm capabilities, using defaults: %v", err)
		capabilities = chartutil.DefaultCapabilities
	}
	actionConfig.Capabilities = capabilities
}

func helmWarningLog(format string, args ...interface{}) {
	logrus.Warnf(format, args...)
}

func buildHelmCapabilities(ctx context.Context, cfg *rest.Config, logFn func(string, ...interface{})) (*chartutil.Capabilities, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, fmt.Errorf("helm discovery HTTP client: %w", err)
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfigAndClient(cfg, httpClientWithContext(ctx, httpClient))
	if err != nil {
		return nil, fmt.Errorf("helm discovery client: %w", err)
	}
	dc := memory.NewMemCacheClient(discoveryClient)
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

type contextRoundTripper struct {
	ctx  context.Context
	next http.RoundTripper
}

func (t contextRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.next.RoundTrip(req.Clone(t.ctx))
}

func httpClientWithContext(ctx context.Context, base *http.Client) *http.Client {
	if ctx == nil {
		ctx = context.Background()
	}
	if base == nil {
		base = http.DefaultClient
	}
	client := *base
	transport := base.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = contextRoundTripper{ctx: ctx, next: transport}
	return &client
}

// ---------- Repo index ----------

func fetchIndexFile(ctx context.Context, repoURL string) (*repo.IndexFile, error) {
	indexURL := strings.TrimRight(repoURL, "/") + "/index.yaml"
	data, err := downloadHelmRepoIndex(ctx, indexURL)
	if err != nil {
		return nil, fmt.Errorf(
			"fetch index %q: %w",
			redactURLForError(indexURL),
			sanitizeErrorMessage(err, indexURL, repoURL),
		)
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
	ctx, cancel := context.WithTimeout(context.Background(), helmChartLoadTimeout)
	defer cancel()
	if isOCIRepo(repoURL) {
		return fetchOCIChartSummary(ctx, repoURL)
	}

	idx, err := fetchIndexFile(ctx, repoURL)
	if err != nil {
		return nil, err
	}
	charts := make([]HelmChartSummary, 0, len(idx.Entries))
	for name, versions := range idx.Entries {
		if len(versions) == 0 {
			continue
		}
		v := versions[0]
		if !isInstallableHelmChartMetadata(v.Metadata) {
			continue
		}
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

func newOCIRegistryClient(ctx context.Context) (*registry.Client, error) {
	client := newOCIRegistryHTTPClient(proxypkg.ProxyHttpClient)
	return registry.NewClient(registry.ClientOptHTTPClient(httpClientWithContext(ctx, client)))
}

// pullOCIChart pulls the chart hosted at repoURL, resolving to the newest published
// semver tag when version is empty.
func pullOCIChart(ctx context.Context, repoURL, version string) (*registry.PullResult, error) {
	ref, resolvedVersion := resolveOCIChartRef(repoURL, version)

	rc, err := newOCIRegistryClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("oci registry client: %w", err)
	}

	if resolvedVersion == "" {
		if !strings.Contains(ref, "@") {
			var tags []string
			err = retryOCIRegistryOperation(ctx, func() error {
				var tagsErr error
				tags, tagsErr = rc.Tags(ref)
				return tagsErr
			})
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
	if cached, ok, err := cachedOCIChartPull(pullRef); err != nil {
		return nil, err
	} else if ok {
		return cached, nil
	}

	var pull *registry.PullResult
	err = retryOCIRegistryOperation(ctx, func() error {
		var pullErr error
		pull, pullErr = rc.Pull(pullRef, registry.PullOptWithChart(true))
		return pullErr
	})
	if err != nil {
		return nil, fmt.Errorf(
			"pull oci chart %q: %w",
			redactURLForError(repoURL),
			sanitizeErrorMessage(err, repoURL, ref, pullRef),
		)
	}
	cacheOCIChartPull(pullRef, pull)
	return pull, nil
}

func loadOCIChartWithMirrorFallback(ctx context.Context, repoURL, version string) (*chart.Chart, error) {
	ch, err := loadOCIChart(ctx, repoURL, version)
	if err == nil {
		return ch, nil
	}
	mirrorURL := dockerHubOCIChartMirror(repoURL)
	if mirrorURL == "" {
		return nil, err
	}
	mirrored, mirrorErr := loadOCIChart(ctx, mirrorURL, version)
	if mirrorErr == nil {
		return mirrored, nil
	}
	return nil, errors.Join(err, fmt.Errorf("Docker Hub OCI mirror %q failed: %w", redactURLForError(mirrorURL), mirrorErr))
}

func dockerHubOCIChartMirror(repoURL string) string {
	if strings.EqualFold(strings.TrimSpace(conf.GetConfigStringDefault("imageRegistryMirror", "auto")), "never") {
		return ""
	}
	prefix := fmt.Sprintf("%s://", registry.OCIScheme)
	ref := strings.TrimPrefix(strings.TrimSpace(repoURL), prefix)
	host, path, ok := strings.Cut(ref, "/")
	if !ok || path == "" {
		return ""
	}
	switch strings.ToLower(host) {
	case "docker.io", "registry-1.docker.io":
		return prefix + "docker.1ms.run/" + path
	default:
		return ""
	}
}

func cachedOCIChartPull(pullRef string) (*registry.PullResult, bool, error) {
	data, ok := defaultHelmArtifactCache.get("oci-chart:" + pullRef)
	if !ok {
		return nil, false, nil
	}
	loaded, err := loader.LoadArchive(bytes.NewReader(data))
	if err != nil {
		return nil, true, fmt.Errorf("load cached oci chart %q: %w", redactURLForError(pullRef), err)
	}
	return &registry.PullResult{
		Chart: &registry.DescriptorPullSummaryWithMeta{
			DescriptorPullSummary: registry.DescriptorPullSummary{Data: data, Size: int64(len(data))},
			Meta:                  loaded.Metadata,
		},
		Ref: pullRef,
	}, true, nil
}

func cacheOCIChartPull(pullRef string, pull *registry.PullResult) {
	if pull == nil || pull.Chart == nil || len(pull.Chart.Data) == 0 {
		return
	}
	defaultHelmArtifactCache.put("oci-chart:"+pullRef, pull.Chart.Data)
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

func loadOCIChart(ctx context.Context, repoURL, version string) (*chart.Chart, error) {
	pull, err := pullOCIChart(ctx, repoURL, version)
	if err != nil {
		return nil, err
	}
	return loader.LoadArchive(bytes.NewReader(pull.Chart.Data))
}

func fetchOCIChartSummary(ctx context.Context, repoURL string) ([]HelmChartSummary, error) {
	pull, err := pullOCIChart(ctx, repoURL, "")
	if err != nil {
		return nil, err
	}
	meta := pull.Chart.Meta
	if meta == nil {
		return nil, fmt.Errorf("chart metadata not found for %s", redactURLForError(repoURL))
	}
	if !isInstallableHelmChartMetadata(meta) {
		return []HelmChartSummary{}, nil
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

func isInstallableHelmChartMetadata(metadata *chart.Metadata) bool {
	return metadata != nil && !strings.EqualFold(strings.TrimSpace(metadata.Type), "library")
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
	return loadChartWithContext(context.Background(), chartName, repoURL, version)
}

func loadChartWithContext(parent context.Context, chartName, repoURL, version string) (*chart.Chart, error) {
	return loadChartWithFallbackContext(parent, chartName, repoURL, version, "")
}

func loadChartWithFallbackContext(parent context.Context, chartName, repoURL, version, contentURL string) (*chart.Chart, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, helmChartLoadTimeout)
	defer cancel()

	if isOCIRepo(repoURL) {
		reportHelmChartLoadStage(ctx, HelmChartLoadStageOCI)
		ch, err := loadOCIChartWithMirrorFallback(withHelmChartLoadStage(ctx, HelmChartLoadStageOCI), repoURL, version)
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

	idx, err := fetchIndexFile(withHelmChartLoadStage(ctx, HelmChartLoadStageIndex), repoURL)
	if err != nil {
		return loadChartContentURLFallback(ctx, chartName, repoURL, version, contentURL, fmt.Errorf("load chart %q from repo %q version %q: fetch index.yaml failed: %w", chartName, redactURLForError(repoURL), version, err))
	}

	versions, ok := idx.Entries[chartName]
	if !ok || len(versions) == 0 {
		return loadChartContentURLFallback(ctx, chartName, repoURL, version, contentURL, fmt.Errorf("chart %q not found in repo", chartName))
	}

	var entry *repo.ChartVersion
	for _, v := range versions {
		if version == "" || helmVersionsEqual(v.Version, version) {
			entry = v
			break
		}
	}
	if entry == nil {
		return loadChartContentURLFallback(ctx, chartName, repoURL, version, contentURL, fmt.Errorf("chart %q version %q not found", chartName, version))
	}
	if len(entry.URLs) == 0 {
		return loadChartContentURLFallback(ctx, chartName, repoURL, version, contentURL, fmt.Errorf("chart %q has no download URLs", chartName))
	}

	chartURL := entry.URLs[0]
	if isOCIRepo(chartURL) {
		ociVersion := version
		if ociVersion == "" {
			ociVersion = entry.Version
		}
		reportHelmChartLoadStage(ctx, HelmChartLoadStageOCI)
		ch, err := loadOCIChartWithMirrorFallback(withHelmChartLoadStage(ctx, HelmChartLoadStageOCI), chartURL, ociVersion)
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

	data, err := downloadHelmArtifact(withHelmChartLoadStage(ctx, HelmChartLoadStageChart), chartURL)
	if err != nil {
		return loadChartContentURLFallback(ctx, chartName, repoURL, version, contentURL, fmt.Errorf(
			"load chart %q from repo %q version %q: download chart archive %q failed: %w",
			chartName,
			redactURLForError(repoURL),
			entry.Version,
			redactURLForError(chartURL),
			sanitizeErrorMessage(err, chartURL, repoURL),
		))
	}
	ch, err := loader.LoadArchive(bytes.NewReader(data))
	if err != nil {
		return loadChartContentURLFallback(ctx, chartName, repoURL, version, contentURL, fmt.Errorf(
			"load chart %q from repo %q version %q: parse chart archive %q failed: %w",
			chartName,
			redactURLForError(repoURL),
			entry.Version,
			redactURLForError(chartURL),
			err,
		))
	}
	return ch, nil
}

func loadChartContentURLFallback(ctx context.Context, chartName, repoURL, version, contentURL string, primaryErr error) (*chart.Chart, error) {
	if strings.TrimSpace(contentURL) == "" {
		return nil, primaryErr
	}
	data, err := downloadHelmArtifact(withHelmChartLoadStage(ctx, HelmChartLoadStageChart), contentURL)
	if err != nil {
		return nil, errors.Join(primaryErr, fmt.Errorf("download ArtifactHub content URL %q failed: %w", redactURLForError(contentURL), sanitizeErrorMessage(err, contentURL, repoURL)))
	}
	ch, err := loader.LoadArchive(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Join(primaryErr, fmt.Errorf("parse ArtifactHub chart archive %q failed: %w", redactURLForError(contentURL), err))
	}
	if ch.Metadata == nil || ch.Name() != chartName || (version != "" && !helmVersionsEqual(ch.Metadata.Version, version)) {
		actualName, actualVersion := "", ""
		if ch.Metadata != nil {
			actualName, actualVersion = ch.Name(), ch.Metadata.Version
		}
		return nil, errors.Join(primaryErr, fmt.Errorf("ArtifactHub content URL resolved to chart %q version %q, expected %q version %q", actualName, actualVersion, chartName, version))
	}
	return ch, nil
}

func helmVersionsEqual(left, right string) bool {
	if left == right {
		return true
	}
	leftVersion, leftErr := semver.NewVersion(left)
	rightVersion, rightErr := semver.NewVersion(right)
	return leftErr == nil && rightErr == nil && leftVersion.Equal(rightVersion)
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
		lines = appendContainerLogDiagnostics(ctx, client, lines, namespace, pods.Items)
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
	chartStr, chartName, chartVersion, appVersion, repoURL, icon := "", "", "", "", "", ""
	if r.Chart != nil && r.Chart.Metadata != nil {
		chartName = r.Chart.Metadata.Name
		chartVersion = r.Chart.Metadata.Version
		chartStr = chartName + "-" + chartVersion
		appVersion = r.Chart.Metadata.AppVersion
		repoURL = helmChartRepoURL(r.Chart)
		icon = r.Chart.Metadata.Icon
	}
	return HelmReleaseSummary{
		Name:         r.Name,
		Namespace:    r.Namespace,
		Revision:     fmt.Sprintf("%d", r.Version),
		Updated:      r.Info.LastDeployed.UTC().Format(time.RFC3339),
		Status:       string(r.Info.Status),
		Chart:        chartStr,
		ChartName:    chartName,
		ChartVersion: chartVersion,
		RepoURL:      repoURL,
		AppVersion:   appVersion,
		Description:  r.Info.Description,
		Icon:         icon,
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
	return installHelmChart(cfg, releaseName, namespace, chartName, repoURL, version, valuesYAML, "", "")
}

func InstallHelmChartWithValuesBaseline(cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML string) error {
	return installHelmChart(cfg, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML, "")
}

func InstallHelmChartWithValuesBaselineAndFallback(cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML, contentURL string) error {
	return installHelmChart(cfg, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML, contentURL)
}

func installHelmChart(cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML, contentURL string) error {
	installTimeout, err := configuredHelmInstallTimeout()
	if err != nil {
		return err
	}
	actionConfig, err := newHelmConfig(cfg, namespace)
	if err != nil {
		return err
	}
	if err := validateHelmInstallNodes(context.Background(), cfg, func(message string) {
		logrus.Warn(message)
	}); err != nil {
		return err
	}
	ch, err := loadChartWithFallbackContext(context.Background(), chartName, repoURL, version, contentURL)
	if err != nil {
		return err
	}
	setHelmChartRepoURL(ch, repoURL)
	valuesYAML, inputIsOverrides, err := getHelmValueOverrides(valuesYAML, valuesBaselineYAML)
	if err != nil {
		return err
	}
	vals, err := parseValues(valuesYAML)
	if err != nil {
		return err
	}
	vals, adjustments, err := prepareHelmInstallValuesWithOptions(ch, repoURL, vals, helmInstallValueOptions{
		inputIsOverrides: inputIsOverrides,
		cluster:          newClusterContext(cfg, releaseName, namespace),
	})
	if err != nil {
		return err
	}
	for _, warning := range adjustments.warnings() {
		logrus.Warn(warning)
	}

	compatibilityCtx, cancelCompatibility := context.WithTimeout(context.Background(), helmCompatibilityTimeout)
	attachHelmCapabilities(compatibilityCtx, actionConfig, cfg, helmWarningLog)
	err = validateHelmChartCompatibility(compatibilityCtx, cfg, actionConfig, releaseName, namespace, ch, vals)
	if err == nil {
		err = checkHelmInstallImages(compatibilityCtx, actionConfig, releaseName, namespace, ch, vals)
	}
	cancelCompatibility()
	if err != nil {
		return err
	}
	install := action.NewInstall(actionConfig)
	configureHelmInstall(install, releaseName, namespace, installTimeout)

	installCtx, cancelInstall := context.WithCancel(context.Background())
	defer cancelInstall()
	failFast := startHelmInstallFailFast(installCtx, cancelInstall, cfg, releaseName, namespace)
	failedRelease, err := install.RunWithContext(installCtx, ch, vals)
	failFast.Stop()
	if err != nil {
		return finishFailedHelmInstall(
			withHelmFailFastReason(err, failFast),
			func(installErr error) error {
				return withHelmReleaseDiagnostics(context.Background(), cfg, releaseName, namespace, installErr)
			},
			func() error { return cleanupFailedHelmRelease(actionConfig, install, failedRelease) },
		)
	}
	reportHelmReadiness(inspectHelmReleaseResources(context.Background(), cfg, releaseName, namespace), func(message string) {
		logrus.Warn(message)
	})
	return nil
}

type HelmInstallLifecycle interface {
	StartLoading() error
	MarkInstalling() error
	RecordLog(line string) error
	Finish(installErr error) error
}

const (
	HelmInstallStreamEventLog     = "log"
	HelmInstallStreamEventWarning = "warning"
	HelmInstallStreamEventError   = "error"
	HelmInstallStreamEventDone    = "done"
)

type HelmInstallStreamEvent struct {
	Type    string                      `json:"type"`
	Message string                      `json:"message,omitempty"`
	Error   *HelmCompatibilityErrorInfo `json:"error,omitempty"`
}

func newHelmErrorEvent(err error) HelmInstallStreamEvent {
	event := HelmInstallStreamEvent{Type: HelmInstallStreamEventError, Message: err.Error()}
	if info, ok := HelmCompatibilityErrorInfoFrom(err); ok {
		event.Error = &info
	}
	return event
}

// InstallHelmChartStream runs a Helm install independently of the browser
// request. Lifecycle persistence is supplied by the caller so store remains
// independent of the database layer.
func InstallHelmChartStream(ctx context.Context, lifecycle HelmInstallLifecycle, cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML string) <-chan HelmInstallStreamEvent {
	return installHelmChartStream(ctx, lifecycle, cfg, releaseName, namespace, chartName, repoURL, version, valuesYAML, "", "")
}

func InstallHelmChartStreamWithValuesBaseline(ctx context.Context, lifecycle HelmInstallLifecycle, cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML string) <-chan HelmInstallStreamEvent {
	return installHelmChartStream(ctx, lifecycle, cfg, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML, "")
}

func InstallHelmChartStreamWithValuesBaselineAndFallback(ctx context.Context, lifecycle HelmInstallLifecycle, cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML, contentURL string) <-chan HelmInstallStreamEvent {
	return installHelmChartStream(ctx, lifecycle, cfg, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML, contentURL)
}

func installHelmChartStream(ctx context.Context, lifecycle HelmInstallLifecycle, cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML, contentURL string) <-chan HelmInstallStreamEvent {
	eventCh := make(chan HelmInstallStreamEvent, 64)
	if lifecycle == nil {
		eventCh <- newHelmErrorEvent(errors.New("Helm install lifecycle is required"))
		close(eventCh)
		return eventCh
	}
	go func() {
		defer close(eventCh)
		streamCtx := ctx
		if streamCtx == nil {
			streamCtx = context.Background()
		}
		logThrottle := newHelmLogThrottle()
		send := func(event HelmInstallStreamEvent) bool {
			persistedMessage := event.Message
			switch event.Type {
			case HelmInstallStreamEventError:
				persistedMessage = "ERROR: " + event.Message
			case HelmInstallStreamEventWarning:
				persistedMessage = "WARNING: " + event.Message
			case HelmInstallStreamEventLog:
				// A repeat carries nothing the reader has not already seen, and
				// only warnings and errors are exempt from that.
				if !logThrottle.allow(event.Message) {
					return true
				}
			}
			if persistedMessage != "" {
				if err := lifecycle.RecordLog(persistedMessage); err != nil {
					logrus.Warnf("failed to persist Helm operation log: %v", err)
				}
			}
			select {
			case eventCh <- event:
				return true
			case <-streamCtx.Done():
				return false
			}
		}
		sendLog := func(message string) bool {
			return send(HelmInstallStreamEvent{Type: HelmInstallStreamEventLog, Message: message})
		}
		sendWarning := func(message string) bool {
			return send(HelmInstallStreamEvent{Type: HelmInstallStreamEventWarning, Message: message})
		}
		sendError := func(err error) bool {
			return send(newHelmErrorEvent(err))
		}
		finishWithError := func(err error, context string) {
			sendError(err)
			if finishErr := lifecycle.Finish(err); finishErr != nil {
				logrus.Errorf("failed to finish Helm operation after %s: %v", context, finishErr)
			}
		}
		if err := lifecycle.StartLoading(); err != nil {
			finishWithError(err, "loading error")
			return
		}
		installTimeout, err := configuredHelmInstallTimeout()
		if err != nil {
			finishWithError(err, "configuration error")
			return
		}
		installCtx, cancelInstall := context.WithTimeout(
			context.WithoutCancel(streamCtx),
			helmInstallOperationDeadline(installTimeout),
		)
		defer cancelInstall()
		logFn := func(format string, args ...interface{}) {
			sendLog(fmt.Sprintf(format, args...))
		}
		actionConfig, err := newHelmConfigWithLog(cfg, namespace, logFn)
		if err != nil {
			finishWithError(err, "configuration error")
			return
		}
		if err := validateHelmInstallNodes(installCtx, cfg, func(message string) {
			sendWarning(message)
		}); err != nil {
			finishWithError(err, "node preflight error")
			return
		}
		helmChart, err := loadChartWithFallbackContext(installCtx, chartName, repoURL, version, contentURL)
		if err != nil {
			finishWithError(err, "chart loading error")
			return
		}
		setHelmChartRepoURL(helmChart, repoURL)
		valuesYAML, inputIsOverrides, err := getHelmValueOverrides(valuesYAML, valuesBaselineYAML)
		if err != nil {
			finishWithError(err, "value override parsing error")
			return
		}
		vals, err := parseValues(valuesYAML)
		if err != nil {
			finishWithError(err, "values parsing error")
			return
		}
		vals, adjustments, err := prepareHelmInstallValuesWithOptions(helmChart, repoURL, vals, helmInstallValueOptions{
			inputIsOverrides: inputIsOverrides,
			cluster:          newClusterContext(cfg, releaseName, namespace),
		})
		if err != nil {
			finishWithError(err, "values preparation error")
			return
		}
		for _, warning := range adjustments.warnings() {
			sendWarning(warning)
		}
		compatibilityCtx, cancelCompatibility := context.WithTimeout(installCtx, helmCompatibilityTimeout)
		attachHelmCapabilities(compatibilityCtx, actionConfig, cfg, logFn)
		err = validateHelmChartCompatibility(compatibilityCtx, cfg, actionConfig, releaseName, namespace, helmChart, vals)
		if err == nil {
			err = checkHelmInstallImages(compatibilityCtx, actionConfig, releaseName, namespace, helmChart, vals)
		}
		cancelCompatibility()
		if err != nil {
			finishWithError(err, "compatibility validation error")
			return
		}
		if err := lifecycle.MarkInstalling(); err != nil {
			finishWithError(err, "phase transition error")
			return
		}
		install := action.NewInstall(actionConfig)
		configureHelmInstall(install, releaseName, namespace, installTimeout)
		progress := startHelmProgressReporter(installCtx, cfg, releaseName, namespace, func(line string) {
			sendLog(line)
		})
		failFast := startHelmInstallFailFast(installCtx, cancelInstall, cfg, releaseName, namespace)
		failedRelease, installErr := install.RunWithContext(installCtx, helmChart, vals)
		failFast.Stop()
		// Stopped before anything else, because the reporter holds sendLog and
		// this goroutine closes the channel sendLog writes to.
		progress.Stop()
		if installErr != nil {
			// The diagnostics and the cleanup below both need to reach the
			// cluster, and installCtx is cancelled once the watcher fires.
			diagnosticsCtx := context.WithoutCancel(installCtx)
			err = finishFailedHelmInstall(
				withHelmFailFastReason(installErr, failFast),
				func(installErr error) error {
					for _, line := range helmReleaseDiagnostics(diagnosticsCtx, cfg, releaseName, namespace) {
						sendLog(line)
					}
					return installErr
				},
				func() error { return cleanupFailedHelmRelease(actionConfig, install, failedRelease) },
			)
			finishWithError(err, "install error")
			return
		}
		reportHelmReadiness(inspectHelmReleaseResources(installCtx, cfg, releaseName, namespace), func(message string) {
			sendWarning(message)
		})
		if err := lifecycle.Finish(nil); err != nil {
			logrus.Warnf("failed to finish Helm operation: %v", err)
			sendError(err)
			return
		}
		select {
		case eventCh <- HelmInstallStreamEvent{Type: HelmInstallStreamEventDone}:
		case <-streamCtx.Done():
		}
	}()
	return eventCh
}

type helmUpgradeStreamRunner func(context.Context, func(string, ...interface{}), func(string)) error

func runHelmUpgradeStream(ctx context.Context, lifecycle HelmInstallLifecycle, run helmUpgradeStreamRunner) <-chan HelmInstallStreamEvent {
	eventCh := make(chan HelmInstallStreamEvent, 64)
	if lifecycle == nil || run == nil {
		eventCh <- newHelmErrorEvent(errors.New("Helm upgrade lifecycle is required"))
		close(eventCh)
		return eventCh
	}
	go func() {
		defer close(eventCh)
		streamCtx := ctx
		if streamCtx == nil {
			streamCtx = context.Background()
		}
		logThrottle := newHelmLogThrottle()
		send := func(event HelmInstallStreamEvent) bool {
			persistedMessage := event.Message
			switch event.Type {
			case HelmInstallStreamEventError:
				persistedMessage = "ERROR: " + event.Message
			case HelmInstallStreamEventWarning:
				persistedMessage = "WARNING: " + event.Message
			case HelmInstallStreamEventLog:
				if !logThrottle.allow(event.Message) {
					return true
				}
			}
			if persistedMessage != "" {
				if err := lifecycle.RecordLog(persistedMessage); err != nil {
					logrus.Warnf("failed to persist Helm operation log: %v", err)
				}
			}
			select {
			case eventCh <- event:
				return true
			case <-streamCtx.Done():
				return false
			}
		}
		sendError := func(err error) bool { return send(newHelmErrorEvent(err)) }
		finishWithError := func(err error, operationContext string) {
			sendError(err)
			if finishErr := lifecycle.Finish(err); finishErr != nil {
				logrus.Errorf("failed to finish Helm operation after %s: %v", operationContext, finishErr)
			}
		}
		if err := lifecycle.StartLoading(); err != nil {
			finishWithError(err, "loading error")
			return
		}
		if err := lifecycle.MarkInstalling(); err != nil {
			finishWithError(err, "phase transition error")
			return
		}
		upgradeCtx, cancelUpgrade := context.WithTimeout(
			context.WithoutCancel(streamCtx),
			helmChartLoadTimeout+helmCompatibilityTimeout+helmOperationTimeout,
		)
		defer cancelUpgrade()
		logFn := func(format string, args ...interface{}) {
			send(HelmInstallStreamEvent{Type: HelmInstallStreamEventLog, Message: fmt.Sprintf(format, args...)})
		}
		warnFn := func(message string) {
			send(HelmInstallStreamEvent{Type: HelmInstallStreamEventWarning, Message: message})
		}
		if err := run(upgradeCtx, logFn, warnFn); err != nil {
			finishWithError(err, "upgrade error")
			return
		}
		if err := lifecycle.Finish(nil); err != nil {
			logrus.Warnf("failed to finish Helm operation: %v", err)
			sendError(err)
			return
		}
		select {
		case eventCh <- HelmInstallStreamEvent{Type: HelmInstallStreamEventDone}:
		case <-streamCtx.Done():
		}
	}()
	return eventCh
}

func UpgradeHelmReleaseStream(ctx context.Context, lifecycle HelmInstallLifecycle, cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML string) <-chan HelmInstallStreamEvent {
	return UpgradeHelmReleaseStreamWithValuesBaseline(ctx, lifecycle, cfg, releaseName, namespace, chartName, repoURL, version, valuesYAML, "")
}

func UpgradeHelmReleaseStreamWithValuesBaseline(ctx context.Context, lifecycle HelmInstallLifecycle, cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML string) <-chan HelmInstallStreamEvent {
	return runHelmUpgradeStream(ctx, lifecycle, func(upgradeCtx context.Context, logFn func(string, ...interface{}), warnFn func(string)) error {
		return upgradeHelmReleaseWithContext(upgradeCtx, logFn, warnFn, cfg, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML)
	})
}

func UpgradeHelmRelease(cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML string) error {
	return upgradeHelmRelease(cfg, releaseName, namespace, chartName, repoURL, version, valuesYAML, "")
}

func UpgradeHelmReleaseWithValuesBaseline(cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML string) error {
	return upgradeHelmRelease(cfg, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML)
}

type helmUpgradeDiagnosticCollector struct {
	logFn      func(string, ...interface{})
	lines      []string
	collecting bool
}

func newHelmUpgradeDiagnosticCollector(logFn func(string, ...interface{})) *helmUpgradeDiagnosticCollector {
	return &helmUpgradeDiagnosticCollector{logFn: logFn}
}

func (c *helmUpgradeDiagnosticCollector) log(format string, args ...interface{}) {
	if c.logFn != nil {
		c.logFn(format, args...)
	}
	line := fmt.Sprintf(format, args...)
	if strings.HasPrefix(line, "Helm release diagnostics") {
		c.collecting = true
	}
	if c.collecting {
		c.lines = append(c.lines, line)
	}
}

func (c *helmUpgradeDiagnosticCollector) wrap(err error) error {
	if err == nil || len(c.lines) == 0 {
		return err
	}
	return fmt.Errorf("%w\n%s", err, strings.Join(c.lines, "\n"))
}

func upgradeHelmRelease(cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML string) error {
	diagnostics := newHelmUpgradeDiagnosticCollector(helmWarningLog)
	err := upgradeHelmReleaseWithContext(context.Background(), diagnostics.log, func(message string) {
		logrus.Warn(message)
	}, cfg, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML)
	return diagnostics.wrap(err)
}

func upgradeHelmReleaseWithContext(ctx context.Context, logFn func(string, ...interface{}), warnFn func(string), cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML, valuesBaselineYAML string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if logFn == nil {
		logFn = func(string, ...interface{}) {}
	}
	if warnFn == nil {
		warnFn = func(string) {}
	}
	actionConfig, err := newHelmConfigWithLog(cfg, namespace, logFn)
	if err != nil {
		return err
	}
	ch, err := loadChartWithContext(ctx, chartName, repoURL, version)
	if err != nil {
		return err
	}
	setHelmChartRepoURL(ch, repoURL)
	valuesYAML, inputIsOverrides, err := getHelmValueOverrides(valuesYAML, valuesBaselineYAML)
	if err != nil {
		return err
	}
	vals, err := parseValues(valuesYAML)
	if err != nil {
		return err
	}
	preserveHelmChartAdapterValues(actionConfig, ch, releaseName, vals)
	vals, adjustments, err := prepareHelmInstallValuesWithOptions(ch, repoURL, vals, helmInstallValueOptions{
		inputIsOverrides: inputIsOverrides,
		cluster:          newClusterContext(cfg, releaseName, namespace),
	})
	if err != nil {
		return err
	}
	for _, warning := range adjustments.warnings() {
		warnFn(warning)
	}

	compatibilityCtx, cancelCompatibility := context.WithTimeout(ctx, helmCompatibilityTimeout)
	attachHelmCapabilities(compatibilityCtx, actionConfig, cfg, logFn)
	err = validateHelmReleaseCompatibility(compatibilityCtx, actionConfig, releaseName, namespace, ch, vals)
	cancelCompatibility()
	if err != nil {
		return err
	}
	upgrade := newHelmUpgrade(actionConfig, namespace)

	progress := startHelmProgressReporter(ctx, cfg, releaseName, namespace, func(line string) {
		logFn("%s", line)
	})
	_, err = upgrade.RunWithContext(ctx, releaseName, ch, vals)
	progress.Stop()
	if err != nil {
		for _, line := range helmReleaseDiagnostics(ctx, cfg, releaseName, namespace) {
			logFn("%s", line)
		}
		return err
	}
	reportHelmReadiness(inspectHelmReleaseResources(ctx, cfg, releaseName, namespace), warnFn)
	return nil
}

func newHelmUpgrade(actionConfig *action.Configuration, namespace string) *action.Upgrade {
	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Namespace = namespace
	upgrade.Wait = true
	upgrade.WaitForJobs = true
	upgrade.Timeout = helmOperationTimeout
	upgrade.ResetThenReuseValues = true
	upgrade.PostRenderer = configuredLocalImagePullPolicyPostRenderer()
	return upgrade
}

func RollbackHelmRelease(cfg *rest.Config, releaseName, namespace string, revision int) error {
	actionConfig, err := newHelmConfig(cfg, namespace)
	if err != nil {
		return err
	}
	if err := validateHelmRollbackCompatibility(context.Background(), actionConfig, releaseName, namespace, revision); err != nil {
		return err
	}
	rollback := action.NewRollback(actionConfig)
	rollback.Version = revision
	rollback.Wait = true
	rollback.WaitForJobs = true
	rollback.Timeout = helmOperationTimeout
	if err := rollback.Run(releaseName); err != nil {
		return withHelmReleaseDiagnostics(context.Background(), cfg, releaseName, namespace, err)
	}
	reportHelmReadiness(inspectHelmReleaseResources(context.Background(), cfg, releaseName, namespace), func(message string) {
		logrus.Warn(message)
	})
	return nil
}

func UninstallHelmRelease(cfg *rest.Config, releaseName, namespace string) error {
	return UninstallHelmReleaseWithOptions(cfg, releaseName, namespace, false)
}

// UninstallHelmReleaseWithOptions removes a release and, when deleteData is set,
// the PersistentVolumeClaims it left behind.
//
// Helm never deletes a StatefulSet's volumeClaimTemplate claims, so without this
// the next install of the same app finds the old volumes and a freshly generated
// password in its Secret, and can no longer authenticate against its own
// database. The claims are listed before the uninstall, because that is the only
// moment the release's own resources are still there to identify them.
func UninstallHelmReleaseWithOptions(cfg *rest.Config, releaseName, namespace string, deleteData bool) error {
	actionConfig, err := newHelmConfig(cfg, namespace)
	if err != nil {
		return err
	}
	var claims []string
	if deleteData {
		claims, err = helmReleaseClaimNames(context.Background(), cfg, releaseName, namespace)
		if err != nil {
			return err
		}
	}
	uninstall := action.NewUninstall(actionConfig)
	uninstall.Wait = true
	uninstall.Timeout = helmUninstallTimeout()
	if _, err = uninstall.Run(releaseName); err != nil {
		return err
	}
	if !deleteData {
		return nil
	}
	return deleteHelmReleaseClaims(context.Background(), cfg, namespace, claims)
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
