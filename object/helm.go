package object

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	helmgetter "helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/registry"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/yaml"
)

const (
	helmDefaultTimeout = 10 * time.Minute
	helmDriver         = "secrets"
)

// HelmChartRef identifies a chart in a Helm repository (classic HTTP or OCI).
type HelmChartRef struct {
	RepoURL  string `json:"repoUrl"`
	Chart    string `json:"chart"`
	Version  string `json:"version"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// IsOCI reports whether the chart is referenced through an OCI registry.
func (r HelmChartRef) IsOCI() bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.RepoURL)), "oci://") ||
		strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Chart)), "oci://")
}

// Normalize validates the reference and returns the chart argument plus the
// repository URL to pass to Helm. OCI charts are addressed by a single ref and
// must not carry a separate repo URL.
func (r HelmChartRef) Normalize() (chartRef string, repoURL string, err error) {
	repo := strings.TrimSpace(r.RepoURL)
	name := strings.TrimSpace(r.Chart)

	if r.IsOCI() {
		ref := name
		if !strings.HasPrefix(strings.ToLower(ref), "oci://") {
			ref = strings.TrimRight(repo, "/")
			if name != "" && !strings.HasSuffix(strings.ToLower(ref), "/"+strings.ToLower(name)) {
				ref = ref + "/" + name
			}
		}
		if ref == "" {
			return "", "", fmt.Errorf("oci chart reference is required")
		}
		return strings.TrimRight(ref, "/"), "", nil
	}

	if repo == "" {
		return "", "", fmt.Errorf("chart repository url is required")
	}
	if name == "" {
		return "", "", fmt.Errorf("chart name is required")
	}
	return name, repo, nil
}

// HelmReleaseSummary is the API-facing view of a Helm release.
type HelmReleaseSummary struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Chart      string `json:"chart"`
	Version    string `json:"version"`
	AppVersion string `json:"appVersion"`
	Status     string `json:"status"`
	Revision   int    `json:"revision"`
	Updated    string `json:"updated,omitempty"`
	Notes      string `json:"notes,omitempty"`
}

// HelmChartInfo describes a chart and the inputs needed to install it.
type HelmChartInfo struct {
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	AppVersion    string   `json:"appVersion"`
	Description   string   `json:"description"`
	Home          string   `json:"home"`
	Icon          string   `json:"icon"`
	Deprecated    bool     `json:"deprecated"`
	DefaultValues string   `json:"defaultValues"`
	ValuesSchema  string   `json:"valuesSchema,omitempty"`
	Dependencies  []string `json:"dependencies,omitempty"`
}

// HelmLogger receives Helm's debug output during an operation.
type HelmLogger func(format string, v ...interface{})

type helmRESTClientGetter struct {
	cfg       *rest.Config
	namespace string
}

func (g *helmRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	if g.cfg == nil {
		return nil, fmt.Errorf("kubernetes rest config is nil")
	}
	return rest.CopyConfig(g.cfg), nil
}

func (g *helmRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	cfg, err := g.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	client, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(client), nil
}

func (g *helmRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := g.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	return restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient), nil
}

func (g *helmRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return &helmClientConfig{cfg: g.cfg, namespace: g.namespace}
}

type helmClientConfig struct {
	cfg       *rest.Config
	namespace string
}

func (c *helmClientConfig) RawConfig() (clientcmdapi.Config, error) {
	return clientcmdapi.Config{
		CurrentContext: "casos",
		Contexts:       map[string]*clientcmdapi.Context{"casos": {Namespace: c.namespace}},
	}, nil
}

func (c *helmClientConfig) ClientConfig() (*rest.Config, error) {
	if c.cfg == nil {
		return nil, fmt.Errorf("kubernetes rest config is nil")
	}
	return rest.CopyConfig(c.cfg), nil
}

func (c *helmClientConfig) Namespace() (string, bool, error) {
	if c.namespace == "" {
		return "default", false, nil
	}
	return c.namespace, true, nil
}

func (c *helmClientConfig) ConfigAccess() clientcmd.ConfigAccess {
	return nil
}

// helmEnv bundles the per-operation Helm environment. Everything lives in a
// temporary directory so the server never reads or writes the user's ~/.helm.
type helmEnv struct {
	settings       *cli.EnvSettings
	registryClient *registry.Client
	cleanup        func()
}

func newHelmEnv(namespace string) (*helmEnv, error) {
	workDir, err := os.MkdirTemp("", "casos-helm-*")
	if err != nil {
		return nil, fmt.Errorf("create helm temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(workDir) }

	settings := cli.New()
	settings.SetNamespace(namespace)
	settings.RepositoryCache = filepath.Join(workDir, "repository")
	settings.RepositoryConfig = filepath.Join(workDir, "repositories.yaml")
	settings.RegistryConfig = filepath.Join(workDir, "registry.json")
	if err := os.MkdirAll(settings.RepositoryCache, 0o755); err != nil {
		cleanup()
		return nil, fmt.Errorf("create helm cache dir: %w", err)
	}

	registryClient, err := registry.NewClient(registry.ClientOptCredentialsFile(settings.RegistryConfig))
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("create helm registry client: %w", err)
	}

	return &helmEnv{settings: settings, registryClient: registryClient, cleanup: cleanup}, nil
}

func newHelmActionConfig(cfg *rest.Config, namespace string, env *helmEnv, log HelmLogger) (*action.Configuration, error) {
	if log == nil {
		log = func(string, ...interface{}) {}
	}
	actionConfig := new(action.Configuration)
	getter := &helmRESTClientGetter{cfg: cfg, namespace: namespace}
	if err := actionConfig.Init(getter, namespace, helmDriver, action.DebugLog(log)); err != nil {
		return nil, fmt.Errorf("init helm action config: %w", err)
	}
	actionConfig.RegistryClient = env.registryClient
	return actionConfig, nil
}

func applyChartPathOptions(opts *action.ChartPathOptions, ref HelmChartRef, repoURL string) {
	opts.RepoURL = repoURL
	opts.Version = strings.TrimSpace(ref.Version)
	opts.Username = ref.Username
	opts.Password = ref.Password
}

// loadChart downloads (if needed) and loads a chart, updating dependencies when
// the chart declares ones that are not vendored in the archive.
func loadChart(opts *action.ChartPathOptions, ref HelmChartRef, env *helmEnv) (*chart.Chart, error) {
	chartRef, repoURL, err := ref.Normalize()
	if err != nil {
		return nil, err
	}
	applyChartPathOptions(opts, ref, repoURL)

	chartPath, err := opts.LocateChart(chartRef, env.settings)
	if err != nil {
		return nil, fmt.Errorf("locate helm chart: %w", err)
	}

	loaded, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("load helm chart: %w", err)
	}

	if loaded.Metadata != nil && len(loaded.Metadata.Dependencies) > 0 {
		if depErr := action.CheckDependencies(loaded, loaded.Metadata.Dependencies); depErr != nil {
			manager := &downloader.Manager{
				ChartPath:        chartPath,
				Keyring:          opts.Keyring,
				Getters:          helmgetter.All(env.settings),
				RepositoryConfig: env.settings.RepositoryConfig,
				RepositoryCache:  env.settings.RepositoryCache,
				RegistryClient:   env.registryClient,
			}
			if updateErr := manager.Update(); updateErr != nil {
				return nil, fmt.Errorf("update chart dependencies: %w", updateErr)
			}
			loaded, err = loader.Load(chartPath)
			if err != nil {
				return nil, fmt.Errorf("reload helm chart after dependency update: %w", err)
			}
		}
	}
	return loaded, nil
}

// ParseHelmValues parses user supplied values. YAML is accepted, and because
// JSON is a subset of YAML, JSON input works too. An empty input yields an
// empty (non-nil) map.
func ParseHelmValues(raw string) (map[string]interface{}, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]interface{}{}, nil
	}
	values := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(trimmed), &values); err != nil {
		return nil, fmt.Errorf("values must be a valid YAML or JSON object: %w", err)
	}
	if values == nil {
		return map[string]interface{}{}, nil
	}
	return values, nil
}

// ValidateHelmReleaseName checks the release name against Helm's own rules.
func ValidateHelmReleaseName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("release name is required")
	}
	if err := chartutil.ValidateReleaseName(name); err != nil {
		return fmt.Errorf("invalid release name: %w", err)
	}
	return nil
}

// HelmShowChart loads a chart without touching the cluster and reports the
// metadata, default values and values schema needed to render an install form.
func HelmShowChart(ref HelmChartRef) (*HelmChartInfo, error) {
	env, err := newHelmEnv("default")
	if err != nil {
		return nil, err
	}
	defer env.cleanup()

	install := action.NewInstall(new(action.Configuration))
	install.SetRegistryClient(env.registryClient)

	loaded, err := loadChart(&install.ChartPathOptions, ref, env)
	if err != nil {
		return nil, err
	}

	info := &HelmChartInfo{}
	if loaded.Metadata != nil {
		info.Name = loaded.Metadata.Name
		info.Version = loaded.Metadata.Version
		info.AppVersion = loaded.Metadata.AppVersion
		info.Description = loaded.Metadata.Description
		info.Home = loaded.Metadata.Home
		info.Icon = loaded.Metadata.Icon
		info.Deprecated = loaded.Metadata.Deprecated
		for _, dep := range loaded.Metadata.Dependencies {
			info.Dependencies = append(info.Dependencies, dep.Name)
		}
	}
	if len(loaded.Schema) > 0 {
		info.ValuesSchema = string(loaded.Schema)
	}
	for _, file := range loaded.Raw {
		if file.Name == chartutil.ValuesfileName {
			info.DefaultValues = string(file.Data)
			break
		}
	}
	if info.DefaultValues == "" && len(loaded.Values) > 0 {
		if data, marshalErr := yaml.Marshal(loaded.Values); marshalErr == nil {
			info.DefaultValues = string(data)
		}
	}
	return info, nil
}

// HelmRenderChart renders a chart locally (no cluster access) and returns the
// generated manifests. It is used for validation and smoke tests.
func HelmRenderChart(ref HelmChartRef, namespace, releaseName string, values map[string]interface{}) (string, error) {
	if err := ValidateHelmReleaseName(releaseName); err != nil {
		return "", err
	}
	env, err := newHelmEnv(namespace)
	if err != nil {
		return "", err
	}
	defer env.cleanup()

	actionConfig := new(action.Configuration)
	install := action.NewInstall(actionConfig)
	install.SetRegistryClient(env.registryClient)
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.DryRun = true
	install.ClientOnly = true
	install.IncludeCRDs = true
	// ClientOnly rendering defaults to a very old Kubernetes version; charts
	// with a kubeVersion constraint would fail. Match the embedded control
	// plane version instead.
	install.KubeVersion = &chartutil.KubeVersion{Version: "v1.36.0", Major: "1", Minor: "36"}

	loaded, err := loadChart(&install.ChartPathOptions, ref, env)
	if err != nil {
		return "", err
	}
	if values == nil {
		values = map[string]interface{}{}
	}
	rel, err := install.Run(loaded, values)
	if err != nil {
		return "", fmt.Errorf("render helm chart: %w", err)
	}
	return rel.Manifest, nil
}

// HelmInstall installs a chart into the cluster. Installation is atomic: a
// failed install is rolled back so no partial resources are left behind.
func HelmInstall(cfg *rest.Config, ref HelmChartRef, namespace, releaseName string, values map[string]interface{}, log HelmLogger) (*HelmReleaseSummary, error) {
	if err := ValidateHelmReleaseName(releaseName); err != nil {
		return nil, err
	}
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	env, err := newHelmEnv(namespace)
	if err != nil {
		return nil, err
	}
	defer env.cleanup()

	actionConfig, err := newHelmActionConfig(cfg, namespace, env, log)
	if err != nil {
		return nil, err
	}

	install := action.NewInstall(actionConfig)
	install.SetRegistryClient(env.registryClient)
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.CreateNamespace = false
	install.Atomic = true
	install.Wait = true
	install.Timeout = helmDefaultTimeout

	loaded, err := loadChart(&install.ChartPathOptions, ref, env)
	if err != nil {
		return nil, err
	}
	if values == nil {
		values = map[string]interface{}{}
	}

	rel, err := install.Run(loaded, values)
	if err != nil {
		return nil, fmt.Errorf("install helm chart: %w", err)
	}
	return toHelmReleaseSummary(rel), nil
}

// HelmUpgrade upgrades an existing release, rolling back on failure.
func HelmUpgrade(cfg *rest.Config, ref HelmChartRef, namespace, releaseName string, values map[string]interface{}, log HelmLogger) (*HelmReleaseSummary, error) {
	if err := ValidateHelmReleaseName(releaseName); err != nil {
		return nil, err
	}
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	env, err := newHelmEnv(namespace)
	if err != nil {
		return nil, err
	}
	defer env.cleanup()

	actionConfig, err := newHelmActionConfig(cfg, namespace, env, log)
	if err != nil {
		return nil, err
	}

	upgrade := action.NewUpgrade(actionConfig)
	upgrade.SetRegistryClient(env.registryClient)
	upgrade.Namespace = namespace
	upgrade.Atomic = true
	upgrade.Wait = true
	upgrade.Timeout = helmDefaultTimeout

	loaded, err := loadChart(&upgrade.ChartPathOptions, ref, env)
	if err != nil {
		return nil, err
	}
	if values == nil {
		values = map[string]interface{}{}
	}

	rel, err := upgrade.Run(releaseName, loaded, values)
	if err != nil {
		return nil, fmt.Errorf("upgrade helm release: %w", err)
	}
	return toHelmReleaseSummary(rel), nil
}

// HelmUninstall removes a release and its resources.
func HelmUninstall(cfg *rest.Config, namespace, releaseName string, log HelmLogger) error {
	if err := ValidateHelmReleaseName(releaseName); err != nil {
		return err
	}
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}

	env, err := newHelmEnv(namespace)
	if err != nil {
		return err
	}
	defer env.cleanup()

	actionConfig, err := newHelmActionConfig(cfg, namespace, env, log)
	if err != nil {
		return err
	}

	uninstall := action.NewUninstall(actionConfig)
	uninstall.Wait = false
	uninstall.Timeout = helmDefaultTimeout
	if _, err := uninstall.Run(releaseName); err != nil {
		return fmt.Errorf("uninstall helm release: %w", err)
	}
	return nil
}

// HelmListReleases lists releases in a namespace, or in all namespaces when
// namespace is empty.
func HelmListReleases(cfg *rest.Config, namespace string) ([]HelmReleaseSummary, error) {
	env, err := newHelmEnv(namespace)
	if err != nil {
		return nil, err
	}
	defer env.cleanup()

	actionConfig, err := newHelmActionConfig(cfg, namespace, env, nil)
	if err != nil {
		return nil, err
	}

	list := action.NewList(actionConfig)
	list.All = true
	list.SetStateMask()
	if namespace == "" {
		list.AllNamespaces = true
	}

	releases, err := list.Run()
	if err != nil {
		return nil, fmt.Errorf("list helm releases: %w", err)
	}

	result := make([]HelmReleaseSummary, 0, len(releases))
	for _, rel := range releases {
		if summary := toHelmReleaseSummary(rel); summary != nil {
			result = append(result, *summary)
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// HelmGetRelease returns a single release.
func HelmGetRelease(cfg *rest.Config, namespace, releaseName string) (*HelmReleaseSummary, error) {
	env, err := newHelmEnv(namespace)
	if err != nil {
		return nil, err
	}
	defer env.cleanup()

	actionConfig, err := newHelmActionConfig(cfg, namespace, env, nil)
	if err != nil {
		return nil, err
	}

	rel, err := action.NewGet(actionConfig).Run(releaseName)
	if err != nil {
		return nil, fmt.Errorf("get helm release: %w", err)
	}
	return toHelmReleaseSummary(rel), nil
}

func toHelmReleaseSummary(rel *release.Release) *HelmReleaseSummary {
	if rel == nil {
		return nil
	}
	summary := &HelmReleaseSummary{
		Name:      rel.Name,
		Namespace: rel.Namespace,
		Revision:  rel.Version,
	}
	if rel.Chart != nil && rel.Chart.Metadata != nil {
		summary.Chart = rel.Chart.Metadata.Name
		summary.Version = rel.Chart.Metadata.Version
		summary.AppVersion = rel.Chart.Metadata.AppVersion
	}
	if rel.Info != nil {
		summary.Status = rel.Info.Status.String()
		summary.Notes = rel.Info.Notes
		if !rel.Info.LastDeployed.IsZero() {
			summary.Updated = rel.Info.LastDeployed.UTC().Format("2006-01-02 15:04:05")
		} else if !rel.Info.FirstDeployed.IsZero() {
			summary.Updated = rel.Info.FirstDeployed.UTC().Format("2006-01-02 15:04:05")
		}
	}
	return summary
}
