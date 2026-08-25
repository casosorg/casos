package store

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	sigsyaml "sigs.k8s.io/yaml"
)

// The template market reads the same repository the sealos template provider
// reads, in the same format, so every app published for that store works here.
// sealos clones the repository with git; casos pulls the tarball over HTTPS
// instead — there is no git binary to depend on, and only one file per template
// is kept, which is what makes a 40 MB repository a few hundred kilobytes on
// disk.

const (
	DefaultTemplateRepo   = "https://github.com/labring-actions/templates"
	DefaultTemplateBranch = "kb-0.9"

	// Templates are small; anything larger is not one.
	maxTemplateBytes = 4 << 20
)

// templatePathPattern matches "<repo>-<ref>/template/<name>/index.yaml", the
// one file per template that carries both its description and its manifests.
var templatePathPattern = regexp.MustCompile(`^[^/]+/template/([^/]+)/index\.ya?ml$`)

type TemplateValue struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type TemplateInput struct {
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Default     string   `json:"default"`
	Required    bool     `json:"required"`
	Options     []string `json:"options,omitempty"`
	// If is a condition deciding whether the input is asked for at all.
	If string `json:"if,omitempty"`
}

type TemplateI18n struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Readme      string `json:"readme,omitempty"`
}

type TemplateSpec struct {
	Title        string                   `json:"title"`
	URL          string                   `json:"url"`
	GitRepo      string                   `json:"gitRepo"`
	Author       string                   `json:"author"`
	Description  string                   `json:"description"`
	Readme       string                   `json:"readme"`
	Icon         string                   `json:"icon"`
	Screenshots  []string                 `json:"screenshots,omitempty"`
	TemplateType string                   `json:"templateType"`
	Locale       string                   `json:"locale,omitempty"`
	I18n         map[string]TemplateI18n  `json:"i18n,omitempty"`
	Categories   []string                 `json:"categories,omitempty"`
	Draft        bool                     `json:"draft,omitempty"`
	Defaults     map[string]TemplateValue `json:"defaults,omitempty"`
	Inputs       map[string]TemplateInput `json:"inputs,omitempty"`
}

type templateDocument struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Spec TemplateSpec `json:"spec"`
}

// Template is one app from the market: what to show about it, and the
// manifests behind it with their placeholders still in place. The manifests
// stay as text because a placeholder may sit where YAML expects a number, and
// parsing before rendering would reject the file.
type Template struct {
	Name      string       `json:"name"`
	Spec      TemplateSpec `json:"spec"`
	Manifests string       `json:"-"`
}

type templateCacheEntry struct {
	template Template
	modTime  time.Time
}

var (
	templateCacheMu sync.RWMutex
	templateCache   = map[string]templateCacheEntry{}
)

type TemplateRepoStatus struct {
	Repo      string `json:"repo"`
	Branch    string `json:"branch"`
	Count     int    `json:"count"`
	UpdatedAt string `json:"updatedAt"`
}

func TemplatesDir(dataDir string) string {
	return filepath.Join(dataDir, "templates")
}

func templateStatusPath(dataDir string) string {
	return filepath.Join(TemplatesDir(dataDir), "repo.json")
}

// tarballURL turns a GitHub repository address into the archive endpoint for
// one branch. Anything that is not GitHub is assumed to already be an archive.
func tarballURL(repoURL, branch string) string {
	trimmed := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(repoURL), "/"), ".git")
	if strings.HasSuffix(trimmed, ".tar.gz") || strings.HasSuffix(trimmed, ".tgz") {
		return trimmed
	}
	const githubPrefix = "https://github.com/"
	if strings.HasPrefix(trimmed, githubPrefix) {
		return fmt.Sprintf("https://codeload.github.com/%s/tar.gz/refs/heads/%s",
			strings.TrimPrefix(trimmed, githubPrefix), branch)
	}
	return trimmed
}

// SyncTemplates replaces the local copy of the market with what the repository
// holds now. The whole set is written before anything is removed, so a failed
// sync leaves the previous market intact rather than an empty one.
func SyncTemplates(ctx context.Context, client *http.Client, dataDir, repoURL, branch string) (TemplateRepoStatus, error) {
	if repoURL == "" {
		repoURL = DefaultTemplateRepo
	}
	if branch == "" {
		branch = DefaultTemplateBranch
	}
	status := TemplateRepoStatus{Repo: repoURL, Branch: branch}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, tarballURL(repoURL, branch), nil)
	if err != nil {
		return status, err
	}
	response, err := client.Do(request)
	if err != nil {
		return status, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return status, fmt.Errorf("template repository returned %s", response.Status)
	}

	gzipReader, err := gzip.NewReader(response.Body)
	if err != nil {
		return status, err
	}
	defer func() { _ = gzipReader.Close() }()

	dir := TemplatesDir(dataDir)
	staging := dir + ".new"
	if err := os.RemoveAll(staging); err != nil {
		return status, err
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return status, err
	}

	reader := tar.NewReader(gzipReader)
	count := 0
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return status, err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		match := templatePathPattern.FindStringSubmatch(header.Name)
		if match == nil {
			continue
		}
		content, err := io.ReadAll(io.LimitReader(reader, maxTemplateBytes))
		if err != nil {
			return status, err
		}
		name := match[1]
		if err := os.WriteFile(filepath.Join(staging, name+".yaml"), content, 0o644); err != nil {
			return status, err
		}
		count++
	}

	if count == 0 {
		_ = os.RemoveAll(staging)
		return status, fmt.Errorf("no templates found in %s", repoURL)
	}

	previous := dir + ".old"
	_ = os.RemoveAll(previous)
	if _, err := os.Stat(dir); err == nil {
		if err := os.Rename(dir, previous); err != nil {
			return status, err
		}
	}
	if err := os.Rename(staging, dir); err != nil {
		_ = os.Rename(previous, dir)
		return status, err
	}
	_ = os.RemoveAll(previous)

	status.Count = count
	status.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if encoded, err := json.Marshal(status); err == nil {
		_ = os.WriteFile(templateStatusPath(dataDir), encoded, 0o644)
	}

	templateCacheMu.Lock()
	templateCache = map[string]templateCacheEntry{}
	templateCacheMu.Unlock()

	return status, nil
}

func ReadTemplateRepoStatus(dataDir string) TemplateRepoStatus {
	status := TemplateRepoStatus{Repo: DefaultTemplateRepo, Branch: DefaultTemplateBranch}
	content, err := os.ReadFile(templateStatusPath(dataDir))
	if err != nil {
		return status
	}
	_ = json.Unmarshal(content, &status)
	return status
}

// SplitYamlDocuments cuts a multi-document file the way the sealos template
// provider does — on a line that is exactly "---" — so a template renders here
// into the same set of objects it renders into there.
func SplitYamlDocuments(text string) []string {
	normalised := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(normalised, "\n")
	documents := []string{}
	current := []string{}
	for _, line := range lines {
		if strings.TrimRight(line, " \t") == "---" {
			documents = append(documents, strings.Join(current, "\n"))
			current = current[:0]
			continue
		}
		current = append(current, line)
	}
	documents = append(documents, strings.Join(current, "\n"))
	return documents
}

func parseTemplateFile(name string, content []byte) (Template, error) {
	documents := SplitYamlDocuments(string(content))
	if len(documents) == 0 {
		return Template{}, fmt.Errorf("template %s is empty", name)
	}

	var header templateDocument
	if err := sigsyaml.Unmarshal([]byte(documents[0]), &header); err != nil {
		return Template{}, fmt.Errorf("template %s: %w", name, err)
	}
	if header.Kind != "Template" {
		return Template{}, fmt.Errorf("template %s does not start with a Template document", name)
	}
	if header.Metadata.Name != "" {
		name = header.Metadata.Name
	}

	return Template{
		Name:      name,
		Spec:      header.Spec,
		Manifests: strings.Join(documents[1:], "\n---\n"),
	}, nil
}

func loadTemplateFile(path, name string) (Template, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Template{}, err
	}

	templateCacheMu.RLock()
	cached, ok := templateCache[path]
	templateCacheMu.RUnlock()
	if ok && cached.modTime.Equal(info.ModTime()) {
		return cached.template, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return Template{}, err
	}
	parsed, err := parseTemplateFile(name, content)
	if err != nil {
		return Template{}, err
	}

	templateCacheMu.Lock()
	templateCache[path] = templateCacheEntry{template: parsed, modTime: info.ModTime()}
	templateCacheMu.Unlock()
	return parsed, nil
}

// LoadTemplates reads every template in the local copy of the market. A file
// that cannot be parsed is left out rather than failing the whole market: one
// broken template upstream must not empty the store.
func LoadTemplates(dataDir string) ([]Template, error) {
	entries, err := os.ReadDir(TemplatesDir(dataDir))
	if err != nil {
		if os.IsNotExist(err) {
			return []Template{}, nil
		}
		return nil, err
	}

	templates := make([]Template, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".yaml")
		parsed, err := loadTemplateFile(filepath.Join(TemplatesDir(dataDir), entry.Name()), name)
		if err != nil {
			continue
		}
		if parsed.Spec.Draft {
			continue
		}
		templates = append(templates, parsed)
	}
	sort.Slice(templates, func(i, j int) bool {
		return strings.ToLower(templates[i].Name) < strings.ToLower(templates[j].Name)
	})
	return templates, nil
}

func LoadTemplate(dataDir, name string) (Template, error) {
	if name == "" || strings.ContainsAny(name, `/\`) {
		return Template{}, fmt.Errorf("invalid template name %q", name)
	}
	return loadTemplateFile(filepath.Join(TemplatesDir(dataDir), name+".yaml"), name)
}
