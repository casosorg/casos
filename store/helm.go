package store

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/repo"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"gopkg.in/yaml.v3"
	sigsyaml "sigs.k8s.io/yaml"

	proxypkg "github.com/casosorg/casos/proxy"
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
		return nil, fmt.Errorf("fetch index: %w", err)
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

// FetchRepoIndex returns all charts listed in a Helm repo's index.yaml.
func FetchRepoIndex(repoURL string) ([]HelmChartSummary, error) {
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

// ---------- Chart loader ----------

func loadChart(chartName, repoURL, version string) (*chart.Chart, error) {
	idx, err := fetchIndexFile(repoURL)
	if err != nil {
		return nil, err
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
	if !strings.HasPrefix(chartURL, "http") {
		chartURL = strings.TrimRight(repoURL, "/") + "/" + strings.TrimLeft(chartURL, "/")
	}

	resp, err := helmGet(chartURL)
	if err != nil {
		return nil, fmt.Errorf("download chart: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download chart: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return loader.LoadArchive(bytes.NewReader(data))
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

	install := action.NewInstall(actionConfig)
	install.ReleaseName = releaseName
	install.Namespace = namespace
	install.CreateNamespace = true
	install.Wait = true
	install.Timeout = 5 * time.Minute

	_, err = install.Run(ch, vals)
	return err
}

// InstallHelmChartStream runs helm install asynchronously and pushes log lines to the returned channel.
// The channel is closed when the operation finishes; a final line of "ERROR: <msg>" or "DONE" signals the outcome.
func InstallHelmChartStream(cfg *rest.Config, releaseName, namespace, chartName, repoURL, version, valuesYAML string) <-chan string {
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		logFn := func(format string, args ...interface{}) {
			ch <- fmt.Sprintf(format, args...)
		}
		actionConfig, err := newHelmConfigWithLog(cfg, namespace, logFn)
		if err != nil {
			ch <- "ERROR: " + err.Error()
			return
		}
		chart, err := loadChart(chartName, repoURL, version)
		if err != nil {
			ch <- "ERROR: " + err.Error()
			return
		}
		vals, err := parseValues(valuesYAML)
		if err != nil {
			ch <- "ERROR: " + err.Error()
			return
		}
		install := action.NewInstall(actionConfig)
		install.ReleaseName = releaseName
		install.Namespace = namespace
		install.CreateNamespace = true
		install.Wait = true
		install.Timeout = 5 * time.Minute
		if _, err = install.Run(chart, vals); err != nil {
			ch <- "ERROR: " + err.Error()
			return
		}
		ch <- "DONE"
	}()
	return ch
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

	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Namespace = namespace
	upgrade.Wait = true
	upgrade.Timeout = 5 * time.Minute

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
	rollback.Timeout = 5 * time.Minute
	return rollback.Run(releaseName)
}

func UninstallHelmRelease(cfg *rest.Config, releaseName, namespace string) error {
	actionConfig, err := newHelmConfig(cfg, namespace)
	if err != nil {
		return err
	}
	uninstall := action.NewUninstall(actionConfig)
	uninstall.Wait = true
	uninstall.Timeout = 5 * time.Minute
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
