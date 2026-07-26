package controllers

import (
	"fmt"
	"sort"
	"strings"

	"github.com/casosorg/casos/object"
)

// fetchArtifactHubTemplates returns a curated first page of popular Helm charts
// for the app store landing view. Full discovery goes through
// /api/search-helm-charts, which queries ArtifactHub server-side.
func fetchArtifactHubTemplates() ([]AppTemplate, error) {
	result, err := object.SearchArtifactHubCharts("", 1, 48)
	if err != nil {
		return nil, err
	}

	templates := make([]AppTemplate, 0, len(result.Items))
	seen := map[string]bool{}
	for _, item := range result.Items {
		key := item.RepoURL + "/" + item.ChartName
		if seen[key] {
			continue
		}
		seen[key] = true
		templates = append(templates, toAppTemplate(item))
	}

	sort.SliceStable(templates, func(i, j int) bool {
		return strings.ToLower(templates[i].Title) < strings.ToLower(templates[j].Title)
	})
	return templates, nil
}

func toAppTemplate(item object.HelmChartListItem) AppTemplate {
	categories := append([]string{"Helm"}, item.Categories...)
	if item.Official {
		categories = append(categories, "Official")
	} else if item.Verified {
		categories = append(categories, "Verified")
	}

	return AppTemplate{
		Name:        safeAppTemplateName(item.ChartName),
		Title:       item.Title,
		Description: item.Description,
		Icon:        item.Icon,
		Categories:  categories,
		Source:      item.Source,
		PackageType: item.PackageType,
		RepoName:    item.RepoName,
		RepoURL:     item.RepoURL,
		ChartName:   item.ChartName,
		Version:     item.Version,
	}
}

func safeAppTemplateName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "app"
	}
	return result
}

func helmChartRefFromRequest(repoURL, chartName, version, username, password string) (object.HelmChartRef, error) {
	ref := object.HelmChartRef{
		RepoURL:  strings.TrimSpace(repoURL),
		Chart:    strings.TrimSpace(chartName),
		Version:  strings.TrimSpace(version),
		Username: username,
		Password: password,
	}
	if _, _, err := ref.Normalize(); err != nil {
		return object.HelmChartRef{}, fmt.Errorf("invalid chart reference: %w", err)
	}
	return ref, nil
}
