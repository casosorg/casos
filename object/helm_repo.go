package object

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"
)

const (
	artifactHubAPIBase = "https://artifacthub.io/api/v1"
	artifactHubTimeout = 20 * time.Second
	helmRepoTimeout    = 30 * time.Second
	helmUserAgent      = "casos-app/1.0"
)

// HelmChartListItem is a single chart entry shown in the app store.
type HelmChartListItem struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Version     string   `json:"version"`
	AppVersion  string   `json:"appVersion"`
	Deprecated  bool     `json:"deprecated"`
	Source      string   `json:"source"`
	PackageType string   `json:"packageType"`
	RepoName    string   `json:"repoName"`
	RepoURL     string   `json:"repoUrl"`
	ChartName   string   `json:"chartName"`
	Categories  []string `json:"categories,omitempty"`
	Stars       int      `json:"stars,omitempty"`
	Official    bool     `json:"official,omitempty"`
	Verified    bool     `json:"verified,omitempty"`
}

// HelmChartSearchResult is a paginated chart listing.
type HelmChartSearchResult struct {
	Items    []HelmChartListItem `json:"items"`
	Total    int                 `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
}

type artifactHubSearchResponse struct {
	Packages []artifactHubPackage `json:"packages"`
}

type artifactHubPackage struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Version     string                `json:"version"`
	AppVersion  string                `json:"app_version"`
	Deprecated  bool                  `json:"deprecated"`
	LogoImageID string                `json:"logo_image_id"`
	Stars       int                   `json:"stars"`
	Repository  artifactHubRepository `json:"repository"`
}

type artifactHubRepository struct {
	URL               string `json:"url"`
	Kind              int    `json:"kind"`
	Name              string `json:"name"`
	DisplayName       string `json:"display_name"`
	Official          bool   `json:"official"`
	VerifiedPublisher bool   `json:"verified_publisher"`
	OrganizationName  string `json:"organization_name"`
}

type artifactHubVersion struct {
	Version    string `json:"version"`
	AppVersion string `json:"app_version"`
	Prerelease bool   `json:"prerelease"`
}

type artifactHubPackageDetail struct {
	artifactHubPackage
	AvailableVersions []artifactHubVersion `json:"available_versions"`
}

func httpGet(rawURL string, timeout time.Duration, accept string) ([]byte, http.Header, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", helmUserAgent)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("GET %s returned %d", rawURL, resp.StatusCode)
	}
	return body, resp.Header, nil
}

// SearchArtifactHubCharts performs a server-side chart search against
// ArtifactHub so the UI is not limited to a locally cached subset.
func SearchArtifactHubCharts(query string, page, pageSize int) (*HelmChartSearchResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 60 {
		pageSize = 24
	}

	params := url.Values{}
	params.Set("kind", "0") // Helm charts
	params.Set("limit", strconv.Itoa(pageSize))
	params.Set("offset", strconv.Itoa((page-1)*pageSize))
	params.Set("facets", "false")
	params.Set("deprecated", "false")
	if q := strings.TrimSpace(query); q != "" {
		params.Set("ts_query_web", q)
		params.Set("sort", "relevance")
	} else {
		params.Set("sort", "stars")
	}

	body, header, err := httpGet(artifactHubAPIBase+"/packages/search?"+params.Encode(), artifactHubTimeout, "application/json")
	if err != nil {
		return nil, err
	}

	var parsed artifactHubSearchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse artifacthub response: %w", err)
	}

	items := make([]HelmChartListItem, 0, len(parsed.Packages))
	for _, pkg := range parsed.Packages {
		if item, ok := toArtifactHubItem(pkg); ok {
			items = append(items, item)
		}
	}

	total := len(items)
	if raw := header.Get("Pagination-Total-Count"); raw != "" {
		if parsedTotal, convErr := strconv.Atoi(raw); convErr == nil {
			total = parsedTotal
		}
	}

	return &HelmChartSearchResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func toArtifactHubItem(pkg artifactHubPackage) (HelmChartListItem, bool) {
	if pkg.Name == "" || pkg.Repository.URL == "" {
		return HelmChartListItem{}, false
	}
	repoName := pkg.Repository.DisplayName
	if repoName == "" {
		repoName = pkg.Repository.Name
	}
	icon := ""
	if pkg.LogoImageID != "" {
		icon = "https://artifacthub.io/image/" + pkg.LogoImageID
	}
	title := pkg.Name
	if repoName != "" && !strings.EqualFold(repoName, pkg.Name) {
		title = repoName + " / " + pkg.Name
	}
	return HelmChartListItem{
		Name:        pkg.Name,
		Title:       title,
		Description: pkg.Description,
		Icon:        icon,
		Version:     pkg.Version,
		AppVersion:  pkg.AppVersion,
		Deprecated:  pkg.Deprecated,
		Source:      "artifacthub",
		PackageType: "helm",
		RepoName:    repoName,
		RepoURL:     pkg.Repository.URL,
		ChartName:   pkg.Name,
		Stars:       pkg.Stars,
		Official:    pkg.Repository.Official,
		Verified:    pkg.Repository.VerifiedPublisher,
	}, true
}

// GetArtifactHubChartVersions returns the published versions of a chart hosted
// on ArtifactHub. repoName is the ArtifactHub repository name.
func GetArtifactHubChartVersions(repoName, chartName string) ([]string, error) {
	repoName = strings.TrimSpace(repoName)
	chartName = strings.TrimSpace(chartName)
	if repoName == "" || chartName == "" {
		return nil, fmt.Errorf("repoName and chartName are required")
	}

	endpoint := fmt.Sprintf("%s/packages/helm/%s/%s", artifactHubAPIBase, url.PathEscape(repoName), url.PathEscape(chartName))
	body, _, err := httpGet(endpoint, artifactHubTimeout, "application/json")
	if err != nil {
		return nil, err
	}

	var detail artifactHubPackageDetail
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("parse artifacthub package: %w", err)
	}

	versions := make([]string, 0, len(detail.AvailableVersions))
	for _, v := range detail.AvailableVersions {
		if v.Version != "" && !v.Prerelease {
			versions = append(versions, v.Version)
		}
	}
	return versions, nil
}

// ListHelmRepoCharts fetches and parses index.yaml of an arbitrary Helm
// repository, so users can install charts from any source.
func ListHelmRepoCharts(repoURL string) ([]HelmChartListItem, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(repoURL), "/")
	if trimmed == "" {
		return nil, fmt.Errorf("repoUrl is required")
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "oci://") {
		return nil, fmt.Errorf("oci repositories cannot be listed; install by full chart reference instead")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("invalid repoUrl: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("repoUrl must use http or https")
	}

	body, _, err := httpGet(trimmed+"/index.yaml", helmRepoTimeout, "application/yaml")
	if err != nil {
		return nil, err
	}

	index := &repo.IndexFile{}
	if err := yaml.Unmarshal(body, index); err != nil {
		return nil, fmt.Errorf("parse repository index: %w", err)
	}

	items := make([]HelmChartListItem, 0, len(index.Entries))
	for name, versions := range index.Entries {
		if len(versions) == 0 {
			continue
		}
		latest := versions[0]
		for _, v := range versions {
			if v != nil && v.Metadata != nil && latest != nil && latest.Metadata != nil {
				if v.Created.After(latest.Created) {
					latest = v
				}
			}
		}
		if latest == nil || latest.Metadata == nil {
			continue
		}
		items = append(items, HelmChartListItem{
			Name:        name,
			Title:       name,
			Description: latest.Description,
			Icon:        latest.Icon,
			Version:     latest.Version,
			AppVersion:  latest.AppVersion,
			Deprecated:  latest.Deprecated,
			Source:      "repository",
			PackageType: "helm",
			RepoName:    parsed.Host,
			RepoURL:     trimmed,
			ChartName:   name,
			Categories:  latest.Keywords,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return items, nil
}

// GetHelmRepoChartVersions returns all versions of one chart in a repository.
func GetHelmRepoChartVersions(repoURL, chartName string) ([]string, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(repoURL), "/")
	chartName = strings.TrimSpace(chartName)
	if trimmed == "" || chartName == "" {
		return nil, fmt.Errorf("repoUrl and chart are required")
	}

	body, _, err := httpGet(trimmed+"/index.yaml", helmRepoTimeout, "application/yaml")
	if err != nil {
		return nil, err
	}

	index := &repo.IndexFile{}
	if err := yaml.Unmarshal(body, index); err != nil {
		return nil, fmt.Errorf("parse repository index: %w", err)
	}

	entries, ok := index.Entries[chartName]
	if !ok {
		return nil, fmt.Errorf("chart %q not found in repository", chartName)
	}

	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry != nil && entry.Metadata != nil && entry.Version != "" {
			versions = append(versions, entry.Version)
		}
	}
	return versions, nil
}
