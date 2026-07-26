package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	githubTreeURL        = "https://api.github.com/repos/labring-actions/templates/git/trees/main?recursive=1"
	rawContentBase       = "https://raw.githubusercontent.com/labring-actions/templates/main/"
	artifactHubSearchURL = "https://artifacthub.io/api/v1/packages/search?kind=0&limit=48&offset=0&sort=relevance"
	templateCacheTTL     = time.Hour
)

const (
	appSourceSealos      = "sealos"
	appSourceArtifactHub = "artifacthub"
	appPackageSealos     = "sealos-template"
	appPackageHelm       = "helm"
)

type TemplateInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Required    bool   `json:"required"`
}

type AppTemplate struct {
	Name        string          `json:"name"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Icon        string          `json:"icon"`
	Categories  []string        `json:"categories"`
	Image       string          `json:"image"`
	Ports       []int32         `json:"ports"`
	Inputs      []TemplateInput `json:"inputs"`
	Source      string          `json:"source"`
	PackageType string          `json:"packageType"`
	RepoName    string          `json:"repoName,omitempty"`
	RepoURL     string          `json:"repoUrl,omitempty"`
	ChartName   string          `json:"chartName,omitempty"`
	Version     string          `json:"version,omitempty"`
}

var (
	templateCache   []AppTemplate
	templateCacheAt time.Time
	templateCacheMu sync.RWMutex
)

// GetAppTemplates returns app templates from the configured app store sources.
// Results are cached in memory for 1 hour.
// @router /api/get-app-templates [get]
func (c *ApiController) GetAppTemplates() {
	templates, err := getCachedTemplates()
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(templates)
}

func getCachedTemplates() ([]AppTemplate, error) {
	templateCacheMu.RLock()
	if !templateCacheAt.IsZero() && time.Since(templateCacheAt) < templateCacheTTL {
		result := templateCache
		templateCacheMu.RUnlock()
		return result, nil
	}
	templateCacheMu.RUnlock()

	templates, partial, err := fetchAllTemplateSources()
	if err != nil {
		return nil, err
	}

	if !partial {
		templateCacheMu.Lock()
		templateCache = templates
		templateCacheAt = time.Now()
		templateCacheMu.Unlock()
	}

	return templates, nil
}

type templateSourceResult struct {
	source    string
	templates []AppTemplate
	err       error
}

func fetchAllTemplateSources() ([]AppTemplate, bool, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resultCh := make(chan templateSourceResult, 2)
	sources := []struct {
		name  string
		fetch func() ([]AppTemplate, error)
	}{
		{name: appSourceSealos, fetch: fetchAllTemplates},
		{name: appSourceArtifactHub, fetch: fetchArtifactHubTemplates},
	}

	for _, source := range sources {
		source := source
		go func() {
			templates, err := source.fetch()
			select {
			case resultCh <- templateSourceResult{source: source.name, templates: templates, err: err}:
			case <-ctx.Done():
			}
		}()
	}

	hardTimer := time.NewTimer(20 * time.Second)
	defer hardTimer.Stop()
	softTimer := time.NewTimer(5 * time.Second)
	defer softTimer.Stop()
	softTimeout := softTimer.C
	hardTimeout := hardTimer.C

	sourceTemplates := map[string][]AppTemplate{}
	var errs []string
	completed := 0
	templateCount := 0
	partial := false
	for completed < len(sources) {
		select {
		case result := <-resultCh:
			completed++
			if result.err != nil {
				errs = append(errs, fmt.Sprintf("%s: %s", result.source, result.err.Error()))
				continue
			}
			sourceTemplates[result.source] = result.templates
			templateCount += len(result.templates)
		case <-softTimeout:
			if templateCount > 0 {
				partial = completed < len(sources)
				completed = len(sources)
			} else {
				softTimeout = nil
			}
		case <-hardTimeout:
			errs = append(errs, "app template source timeout")
			partial = completed < len(sources)
			completed = len(sources)
		}
	}

	var templates []AppTemplate
	for _, source := range sources {
		templates = append(templates, sourceTemplates[source.name]...)
	}

	if len(templates) == 0 {
		if len(errs) == 0 {
			return nil, false, fmt.Errorf("no app templates found")
		}
		return nil, false, fmt.Errorf("fetch app templates: %s", strings.Join(errs, "; "))
	}
	return templates, partial || len(errs) > 0, nil
}

type githubTree struct {
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"tree"`
}

func fetchTemplatePaths() ([]string, error) {
	req, _ := http.NewRequest("GET", githubTreeURL, nil)
	req.Header.Set("User-Agent", "casos-app/1.0")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var tree githubTree
	if err := json.Unmarshal(body, &tree); err != nil {
		return nil, err
	}

	var paths []string
	for _, item := range tree.Tree {
		if item.Type != "blob" || !strings.HasPrefix(item.Path, "template/") || !strings.HasSuffix(item.Path, ".yaml") {
			continue
		}
		parts := strings.Split(item.Path, "/")
		// Accept: template/foo.yaml (depth 2) or template/foo/index.yaml (depth 3)
		if len(parts) == 2 || (len(parts) == 3 && parts[2] == "index.yaml") {
			paths = append(paths, item.Path)
		}
	}
	return paths, nil
}

func fetchAllTemplates() ([]AppTemplate, error) {
	paths, err := fetchTemplatePaths()
	if err != nil {
		return nil, fmt.Errorf("fetch template list: %w", err)
	}

	type result struct {
		tpl AppTemplate
		err error
	}
	results := make([]result, len(paths))

	sem := make(chan struct{}, 15)
	var wg sync.WaitGroup
	for i, path := range paths {
		wg.Add(1)
		go func(idx int, p string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			tpl, err := fetchAndParseTemplate(p)
			results[idx] = result{tpl, err}
		}(i, path)
	}
	wg.Wait()

	templates := make([]AppTemplate, 0, len(paths))
	for _, r := range results {
		if r.err == nil && r.tpl.Title != "" {
			templates = append(templates, r.tpl)
		}
	}
	return templates, nil
}

type sealosInput struct {
	Description string `yaml:"description"`
	Type        string `yaml:"type"`
	Default     string `yaml:"default"`
	Required    bool   `yaml:"required"`
}

type sealosTemplateCRD struct {
	Spec struct {
		Title       string                 `yaml:"title"`
		Description string                 `yaml:"description"`
		Icon        string                 `yaml:"icon"`
		GitRepo     string                 `yaml:"gitRepo"`
		URL         string                 `yaml:"url"`
		Categories  []string               `yaml:"categories"`
		Inputs      map[string]sealosInput `yaml:"inputs"`
	} `yaml:"spec"`
}

var (
	imageRegex         = regexp.MustCompile(`(?m)^\s+image:\s+([^\s${\n]+)`)
	containerPortRegex = regexp.MustCompile(`containerPort:\s+(\d+)`)
	templateVarRegex   = regexp.MustCompile(`\$\{\{[^}]*\}\}`)
)

func fetchAndParseTemplate(path string) (AppTemplate, error) {
	url := rawContentBase + path
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "casos-app/1.0")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return AppTemplate{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return AppTemplate{}, fmt.Errorf("HTTP %d for %s", resp.StatusCode, path)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return AppTemplate{}, err
	}
	content := string(raw)

	// First YAML document is the Template CRD; subsequent docs are K8s resources
	firstDoc := content
	if idx := strings.Index(content, "\n---"); idx >= 0 {
		firstDoc = content[:idx]
	}

	// Strip Sealos template expressions so the YAML is valid
	cleanDoc := templateVarRegex.ReplaceAllString(firstDoc, `""`)

	var crd sealosTemplateCRD
	_ = yaml.Unmarshal([]byte(cleanDoc), &crd)
	if crd.Spec.Title == "" {
		return AppTemplate{}, nil
	}

	// Extract the first non-sealos image from the resource section
	var image string
	for _, m := range imageRegex.FindAllStringSubmatch(content, -1) {
		img := strings.Trim(strings.TrimSpace(m[1]), `"'`)
		if img == "" || strings.Contains(img, "sealos") || strings.Contains(img, "labring") {
			continue
		}
		image = img
		break
	}

	// Extract unique container ports
	portSeen := map[int32]bool{}
	var ports []int32
	for _, m := range containerPortRegex.FindAllStringSubmatch(content, -1) {
		var p int32
		fmt.Sscanf(m[1], "%d", &p)
		if p > 0 && !portSeen[p] {
			portSeen[p] = true
			ports = append(ports, p)
		}
	}

	// Derive app name from file path
	parts := strings.Split(path, "/")
	name := strings.TrimSuffix(parts[len(parts)-1], ".yaml")
	if name == "index" && len(parts) >= 2 {
		name = parts[len(parts)-2]
	}

	// Convert inputs map to ordered slice: required inputs first, then optional
	var requiredInputs, optionalInputs []TemplateInput
	for k, v := range crd.Spec.Inputs {
		inp := TemplateInput{
			Name:        k,
			Description: v.Description,
			Type:        v.Type,
			Default:     v.Default,
			Required:    v.Required,
		}
		if v.Required {
			requiredInputs = append(requiredInputs, inp)
		} else {
			optionalInputs = append(optionalInputs, inp)
		}
	}
	sort.Slice(requiredInputs, func(i, j int) bool { return requiredInputs[i].Name < requiredInputs[j].Name })
	sort.Slice(optionalInputs, func(i, j int) bool { return optionalInputs[i].Name < optionalInputs[j].Name })
	inputs := append(requiredInputs, optionalInputs...)

	return AppTemplate{
		Name:        name,
		Title:       crd.Spec.Title,
		Description: crd.Spec.Description,
		Icon:        crd.Spec.Icon,
		Categories:  crd.Spec.Categories,
		Image:       image,
		Ports:       ports,
		Inputs:      inputs,
		Source:      appSourceSealos,
		PackageType: appPackageSealos,
	}, nil
}
