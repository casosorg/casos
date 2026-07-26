package controllers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/casosorg/casos/object"
	"k8s.io/client-go/rest"
)

type helmChartRequest struct {
	Namespace string `json:"namespace"`
	Release   string `json:"release"`
	RepoURL   string `json:"repoUrl"`
	Chart     string `json:"chart"`
	Version   string `json:"version"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Values    string `json:"values"`
}

// SearchHelmCharts performs a server-side chart search on ArtifactHub.
// @router /api/search-helm-charts [get]
func (c *ApiController) SearchHelmCharts() {
	if c.RequireSignedIn() {
		return
	}

	page, _ := strconv.Atoi(c.GetString("page"))
	pageSize, _ := strconv.Atoi(c.GetString("pageSize"))
	result, err := object.SearchArtifactHubCharts(c.GetString("q"), page, pageSize)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(result)
}

// GetHelmRepoCharts lists all charts published by an arbitrary Helm repository.
// @router /api/get-helm-repo-charts [get]
func (c *ApiController) GetHelmRepoCharts() {
	if c.RequireSignedIn() {
		return
	}

	items, err := object.ListHelmRepoCharts(c.GetString("repoUrl"))
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(items)
}

// GetHelmChartInfo returns chart metadata, default values and values schema.
// @router /api/get-helm-chart-info [get]
func (c *ApiController) GetHelmChartInfo() {
	if c.RequireSignedIn() {
		return
	}

	ref, err := helmChartRefFromRequest(
		c.GetString("repoUrl"), c.GetString("chart"), c.GetString("version"),
		c.GetString("username"), c.GetString("password"),
	)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	info, err := object.HelmShowChart(ref)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	versions := helmChartVersions(c.GetString("source"), c.GetString("repoName"), ref)
	c.ResponseOk(map[string]interface{}{
		"info":              info,
		"availableVersions": versions,
	})
}

// helmChartVersions resolves the selectable chart versions, preferring the
// repository index and falling back to ArtifactHub metadata.
func helmChartVersions(source, repoName string, ref object.HelmChartRef) []string {
	if ref.IsOCI() {
		return nil
	}
	if versions, err := object.GetHelmRepoChartVersions(ref.RepoURL, ref.Chart); err == nil && len(versions) > 0 {
		return versions
	}
	if source == "artifacthub" && repoName != "" {
		if versions, err := object.GetArtifactHubChartVersions(repoName, ref.Chart); err == nil {
			return versions
		}
	}
	return nil
}

// GetHelmReleases lists Helm releases in a namespace (or all namespaces).
// @router /api/get-helm-releases [get]
func (c *ApiController) GetHelmReleases() {
	if c.RequireSignedIn() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	releases, err := object.HelmListReleases(cfg, c.GetString("namespace"))
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(releases)
}

// GetHelmTask returns the state of an asynchronous Helm operation.
// @router /api/get-helm-task [get]
func (c *ApiController) GetHelmTask() {
	if c.RequireSignedIn() {
		return
	}

	task := getHelmTask(c.GetString("id"))
	if task == nil {
		c.ResponseError("helm task not found")
		return
	}
	c.ResponseOk(task)
}

// InstallHelmChart starts an asynchronous Helm install and returns a task id.
// @router /api/install-helm-chart [post]
func (c *ApiController) InstallHelmChart() {
	c.runHelmChartTask("install")
}

// UpgradeHelmRelease starts an asynchronous Helm upgrade and returns a task id.
// @router /api/upgrade-helm-release [post]
func (c *ApiController) UpgradeHelmRelease() {
	c.runHelmChartTask("upgrade")
}

func (c *ApiController) runHelmChartTask(action string) {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	var req helmChartRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}

	namespace := strings.TrimSpace(req.Namespace)
	release := strings.TrimSpace(req.Release)
	if err := validateHelmTarget(cfg, namespace, release); err != nil {
		c.ResponseError(err.Error())
		return
	}

	ref, err := helmChartRefFromRequest(req.RepoURL, req.Chart, req.Version, req.Username, req.Password)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	values, err := object.ParseHelmValues(req.Values)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	task, err := newHelmTask(action, namespace, release, ref.Chart, ref.Version)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	if action == "upgrade" {
		go runHelmUpgradeTask(cfg, task, ref, values)
	} else {
		go runHelmInstallTask(cfg, task, ref, values)
	}
	c.ResponseOk(map[string]string{"taskId": task.Id, "status": task.Status})
}

// UninstallHelmRelease starts an asynchronous Helm uninstall.
// @router /api/uninstall-helm-release [post]
func (c *ApiController) UninstallHelmRelease() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}

	var req helmChartRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}

	namespace := strings.TrimSpace(req.Namespace)
	release := strings.TrimSpace(req.Release)
	if err := validateHelmTarget(cfg, namespace, release); err != nil {
		c.ResponseError(err.Error())
		return
	}

	task, err := newHelmTask("uninstall", namespace, release, "", "")
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	go runHelmUninstallTask(cfg, task)
	c.ResponseOk(map[string]string{"taskId": task.Id, "status": task.Status})
}

// validateHelmTarget verifies the release name and that the target namespace
// already exists. Namespaces are never created implicitly.
func validateHelmTarget(cfg *rest.Config, namespace, release string) error {
	if namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if err := object.ValidateHelmReleaseName(release); err != nil {
		return err
	}
	if _, err := object.GetNamespace(cfg, namespace); err != nil {
		return fmt.Errorf("namespace %q is not available: %w", namespace, err)
	}
	return nil
}
