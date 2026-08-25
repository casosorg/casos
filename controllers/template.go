package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/proxy"
	"github.com/casosorg/casos/store"
)

// The template market: the sealos template repository, read as-is.
//
// A template is a description plus a pile of manifests with placeholders in
// them. Deploying one renders the placeholders from the form, applies what
// comes out, and writes down what it created so the whole thing can be removed
// again — casos keeps that record in a ConfigMap rather than in a CRD of its
// own, so an instance is inspectable with kubectl and survives an upgrade.

const (
	templateInstancePrefix  = "casos-template-"
	defaultCertSecretName   = "wildcard-cert"
	templateSyncTimeout     = 5 * time.Minute
	templateInstanceVersion = "1"
)

type templateSummary struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Author      string   `json:"author"`
	URL         string   `json:"url"`
	GitRepo     string   `json:"gitRepo"`
	Categories  []string `json:"categories"`
}

type templateInputField struct {
	Key         string   `json:"key"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Default     string   `json:"default"`
	Required    bool     `json:"required"`
	Options     []string `json:"options,omitempty"`
	If          string   `json:"if,omitempty"`
}

type templateDetail struct {
	templateSummary
	Readme      string               `json:"readme"`
	Screenshots []string             `json:"screenshots"`
	Inputs      []templateInputField `json:"inputs"`
	// Defaults are shown so a reader can see the names the template will use
	// before deploying it.
	Defaults map[string]string `json:"defaults"`
}

type templateInstanceSummary struct {
	Name        string              `json:"name"`
	Namespace   string              `json:"namespace"`
	Template    string              `json:"template"`
	Title       string              `json:"title"`
	Icon        string              `json:"icon"`
	CreatedAt   string              `json:"createdAt"`
	Apps        []templateAppEntry  `json:"apps"`
	Databases   []string            `json:"databases"`
	Objects     []appliedObject     `json:"objects,omitempty"`
	Unsupported []unsupportedObject `json:"unsupported"`
	Inputs      map[string]string   `json:"inputs,omitempty"`
}

func templateLanguage(language string) string {
	if strings.HasPrefix(strings.ToLower(language), "zh") {
		return "zh"
	}
	return "en"
}

// localizedSummary reads the template's own translation table, so a market in
// Chinese describes an app the way its author described it in Chinese.
func localizedSummary(template store.Template, language string) templateSummary {
	spec := template.Spec
	summary := templateSummary{
		Name:        template.Name,
		Title:       spec.Title,
		Description: spec.Description,
		Icon:        spec.Icon,
		Author:      spec.Author,
		URL:         spec.URL,
		GitRepo:     spec.GitRepo,
		Categories:  spec.Categories,
	}
	if summary.Title == "" {
		summary.Title = template.Name
	}
	if summary.Categories == nil {
		summary.Categories = []string{}
	}
	if translated, ok := spec.I18n[language]; ok {
		if translated.Title != "" {
			summary.Title = translated.Title
		}
		if translated.Description != "" {
			summary.Description = translated.Description
		}
	}
	return summary
}

func (c *ApiController) templatesDataDir() (string, bool) {
	srvCfg := getServerConfig()
	if srvCfg == nil {
		c.ResponseError("server config not ready")
		return "", false
	}
	return srvCfg.DataDir, true
}

// GetTemplates lists the market. The first visit to an empty market fetches it,
// so nobody has to know there is a repository behind this at all.
// @router /api/get-templates [get]
func (c *ApiController) GetTemplates() {
	if c.RequireSignedIn() {
		return
	}
	dataDir, ok := c.templatesDataDir()
	if !ok {
		return
	}

	templates, err := store.LoadTemplates(dataDir)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	status := store.ReadTemplateRepoStatus(dataDir)
	if len(templates) == 0 {
		ctx, cancel := context.WithTimeout(c.Ctx.Request.Context(), templateSyncTimeout)
		defer cancel()
		if synced, syncErr := store.SyncTemplates(ctx, proxy.GetHttpClient(""), dataDir, status.Repo, status.Branch); syncErr == nil {
			status = synced
			templates, _ = store.LoadTemplates(dataDir)
		}
	}

	language := templateLanguage(c.GetString("language"))
	search := strings.ToLower(strings.TrimSpace(c.GetString("search")))
	category := strings.TrimSpace(c.GetString("category"))

	categories := map[string]int{}
	result := make([]templateSummary, 0, len(templates))
	for _, template := range templates {
		summary := localizedSummary(template, language)
		for _, item := range summary.Categories {
			categories[item]++
		}
		if category != "" && category != "all" {
			matched := false
			for _, item := range summary.Categories {
				if item == category {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if search != "" {
			haystack := strings.ToLower(summary.Name + " " + summary.Title + " " + summary.Description)
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		result = append(result, summary)
	}

	categoryNames := make([]string, 0, len(categories))
	for name := range categories {
		categoryNames = append(categoryNames, name)
	}
	sort.Strings(categoryNames)

	c.ResponseOk(map[string]any{
		"templates":  result,
		"categories": categoryNames,
		"status":     status,
	})
}

// SyncTemplates pulls the repository again.
// @router /api/sync-templates [post]
func (c *ApiController) SyncTemplates() {
	if c.RequireAdmin() {
		return
	}
	dataDir, ok := c.templatesDataDir()
	if !ok {
		return
	}
	previous := store.ReadTemplateRepoStatus(dataDir)
	ctx, cancel := context.WithTimeout(context.Background(), templateSyncTimeout)
	defer cancel()

	status, err := store.SyncTemplates(ctx, proxy.GetHttpClient(""), dataDir, previous.Repo, previous.Branch)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(status)
}

// renderedDefaults resolves the template's own names — the ones holding
// random() so that two installs of the same app do not collide.
func renderedDefaults(template store.Template, env map[string]string) map[string]string {
	defaults := map[string]string{}
	keys := make([]string, 0, len(template.Spec.Defaults))
	for key := range template.Spec.Defaults {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		data := templateData{Defaults: defaults, Env: env}
		defaults[key] = substitutePlaceholders(template.Spec.Defaults[key].Value, data)
	}
	return defaults
}

func templateInputFields(template store.Template, data templateData, language string) []templateInputField {
	keys := make([]string, 0, len(template.Spec.Inputs))
	for key := range template.Spec.Inputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	fields := make([]templateInputField, 0, len(keys))
	for _, key := range keys {
		input := template.Spec.Inputs[key]
		fields = append(fields, templateInputField{
			Key:         key,
			Description: substitutePlaceholders(input.Description, data),
			Type:        input.Type,
			Default:     substitutePlaceholders(input.Default, data),
			Required:    input.Required,
			Options:     input.Options,
			If:          input.If,
		})
	}
	_ = language
	return fields
}

func (c *ApiController) platformEnv(namespace string) map[string]string {
	domain := strings.TrimSpace(c.GetString("domain"))
	if domain == "" {
		domain = defaultCloudDomain(c.Ctx.Request.Host)
	}
	return map[string]string{
		"SEALOS_NAMESPACE":        namespace,
		"SEALOS_CLOUD_DOMAIN":     domain,
		"SEALOS_CERT_SECRET_NAME": defaultCertSecretName,
		"SEALOS_SERVICE_ACCOUNT":  "default",
		"DESKTOP_DOMAIN":          domain,
	}
}

// defaultCloudDomain is the address templates build their hostnames from. The
// host the reader is already on is the best guess available without asking.
func defaultCloudDomain(host string) string {
	if index := strings.LastIndex(host, ":"); index > 0 {
		host = host[:index]
	}
	if host == "" {
		return "cluster.local"
	}
	return host
}

// GetTemplate reads one template back as the form that deploys it.
// @router /api/get-template [get]
func (c *ApiController) GetTemplate() {
	if c.RequireSignedIn() {
		return
	}
	dataDir, ok := c.templatesDataDir()
	if !ok {
		return
	}
	name := c.GetString("name")
	template, err := store.LoadTemplate(dataDir, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	namespace := c.GetString("namespace")
	if namespace == "" {
		namespace = "default"
	}
	language := templateLanguage(c.GetString("language"))
	env := c.platformEnv(namespace)
	defaults := renderedDefaults(template, env)
	data := templateData{Defaults: defaults, Env: env}

	summary := localizedSummary(template, language)
	detail := templateDetail{
		templateSummary: summary,
		Readme:          template.Spec.Readme,
		Screenshots:     template.Spec.Screenshots,
		Inputs:          templateInputFields(template, data, language),
		Defaults:        defaults,
	}
	if translated, ok := template.Spec.I18n[language]; ok && translated.Readme != "" {
		detail.Readme = translated.Readme
	}
	if detail.Screenshots == nil {
		detail.Screenshots = []string{}
	}
	c.ResponseOk(detail)
}

type templateDeployRequest struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace"`
	Domain    string            `json:"domain"`
	Inputs    map[string]string `json:"inputs"`
}

// renderTemplate is the whole of the rendering half: the defaults first, then
// the form on top of them, then the manifests.
func renderTemplate(template store.Template, req templateDeployRequest, env map[string]string) (string, templateData, []string, []string) {
	defaults := renderedDefaults(template, env)
	data := templateData{Defaults: defaults, Inputs: map[string]string{}, Env: env}

	keys := make([]string, 0, len(template.Spec.Inputs))
	for key := range template.Spec.Inputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	missing := []string{}
	for _, key := range keys {
		input := template.Spec.Inputs[key]
		value, given := req.Inputs[key]
		if !given || value == "" {
			value = substitutePlaceholders(input.Default, data)
		}
		// A required field left empty does not merely deploy a half-configured
		// app: the empty string lands in the middle of a YAML line and takes
		// the whole manifest with it.
		if input.Required && strings.TrimSpace(value) == "" && conditionHolds(input.If, data) {
			missing = append(missing, key)
		}
		data.Inputs[key] = value
	}

	instance := defaults["app_name"]
	if instance == "" {
		instance = fmt.Sprintf("%s-%s", template.Name, randomString(6))
	}

	rendered := renderTemplateText(template.Manifests, data)
	return instance, data, store.SplitYamlDocuments(rendered), missing
}

// conditionHolds answers whether an input is being asked for at all: a
// template may hide one behind another's value.
func conditionHolds(condition string, data templateData) bool {
	if strings.TrimSpace(condition) == "" {
		return true
	}
	return truthy(evaluateOrEmpty(condition, data))
}

// PreviewTemplate renders a template without applying it, so the deploy form
// can show exactly what the cluster is about to be asked for.
// @router /api/preview-template [post]
func (c *ApiController) PreviewTemplate() {
	if c.RequireSignedIn() {
		return
	}
	dataDir, ok := c.templatesDataDir()
	if !ok {
		return
	}
	var req templateDeployRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	template, err := store.LoadTemplate(dataDir, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	env := c.platformEnv(req.Namespace)
	if req.Domain != "" {
		env["SEALOS_CLOUD_DOMAIN"] = req.Domain
		env["DESKTOP_DOMAIN"] = req.Domain
	}
	instance, data, documents, _ := renderTemplate(template, req, env)

	c.ResponseOk(map[string]any{
		"instance":  instance,
		"defaults":  data.Defaults,
		"inputs":    data.Inputs,
		"documents": documents,
		"yaml":      strings.Join(documents, "\n---\n"),
	})
}

// DeployTemplate renders a template and applies it.
// @router /api/deploy-template [post]
func (c *ApiController) DeployTemplate() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	dataDir, ok := c.templatesDataDir()
	if !ok {
		return
	}
	var req templateDeployRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	template, err := store.LoadTemplate(dataDir, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	env := c.platformEnv(req.Namespace)
	if req.Domain != "" {
		env["SEALOS_CLOUD_DOMAIN"] = req.Domain
		env["DESKTOP_DOMAIN"] = req.Domain
	}
	instance, data, documents, missing := renderTemplate(template, req, env)
	if len(missing) > 0 {
		c.ResponseError("these fields are required: " + strings.Join(missing, ", "))
		return
	}

	applier, err := newTemplateApplier(cfg, req.Namespace, instance, template.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	report := applier.apply(c.Ctx.Request.Context(), documents)

	language := templateLanguage(c.GetString("language"))
	summary := localizedSummary(template, language)
	instanceSummary := templateInstanceSummary{
		Name:        instance,
		Namespace:   req.Namespace,
		Template:    template.Name,
		Title:       summary.Title,
		Icon:        summary.Icon,
		CreatedAt:   time.Now().UTC().Format("2006-01-02 15:04:05"),
		Apps:        report.Apps,
		Databases:   report.Databases,
		Objects:     report.Applied,
		Unsupported: report.Unsupported,
		Inputs:      data.Inputs,
	}
	if err := writeTemplateInstance(cfg, instanceSummary); err != nil {
		c.ResponseError("the app was deployed but casos could not record it: " + err.Error())
		return
	}
	c.ResponseOk(instanceSummary)
}

func writeTemplateInstance(cfg *rest.Config, instance templateInstanceSummary) error {
	objects, _ := json.Marshal(instance.Objects)
	apps, _ := json.Marshal(instance.Apps)
	unsupported, _ := json.Marshal(instance.Unsupported)
	inputs, _ := json.Marshal(instance.Inputs)
	databases, _ := json.Marshal(instance.Databases)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      templateInstancePrefix + instance.Name,
			Namespace: instance.Namespace,
			Labels: map[string]string{
				appManagedByLabel:     templateManagedBy,
				templateInstanceLabel: instance.Name,
				templateNameLabel:     instance.Template,
			},
		},
		Data: map[string]string{
			"version":     templateInstanceVersion,
			"template":    instance.Template,
			"title":       instance.Title,
			"icon":        instance.Icon,
			"createdAt":   instance.CreatedAt,
			"objects":     string(objects),
			"apps":        string(apps),
			"unsupported": string(unsupported),
			"inputs":      string(inputs),
			"databases":   string(databases),
		},
	}

	existing, err := object.GetConfigMap(cfg, instance.Namespace, configMap.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		_, err := object.AddConfigMap(cfg, configMap)
		return err
	}
	existing.Data = configMap.Data
	existing.Labels = configMap.Labels
	_, err = object.UpdateConfigMap(cfg, existing)
	return err
}

func instanceFromConfigMap(configMap corev1.ConfigMap) templateInstanceSummary {
	instance := templateInstanceSummary{
		Name:        configMap.Labels[templateInstanceLabel],
		Namespace:   configMap.Namespace,
		Template:    configMap.Data["template"],
		Title:       configMap.Data["title"],
		Icon:        configMap.Data["icon"],
		CreatedAt:   configMap.Data["createdAt"],
		Apps:        []templateAppEntry{},
		Databases:   []string{},
		Objects:     []appliedObject{},
		Unsupported: []unsupportedObject{},
	}
	_ = json.Unmarshal([]byte(configMap.Data["apps"]), &instance.Apps)
	_ = json.Unmarshal([]byte(configMap.Data["databases"]), &instance.Databases)
	_ = json.Unmarshal([]byte(configMap.Data["objects"]), &instance.Objects)
	_ = json.Unmarshal([]byte(configMap.Data["unsupported"]), &instance.Unsupported)
	_ = json.Unmarshal([]byte(configMap.Data["inputs"]), &instance.Inputs)
	if instance.Name == "" {
		instance.Name = strings.TrimPrefix(configMap.Name, templateInstancePrefix)
	}
	return instance
}

// GetTemplateInstances lists what the market has installed.
// @router /api/get-template-instances [get]
func (c *ApiController) GetTemplateInstances() {
	if c.RequireSignedIn() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	if namespace == "all" {
		namespace = ""
	}
	configMaps, err := object.GetConfigMaps(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	result := []templateInstanceSummary{}
	for _, configMap := range configMaps {
		if configMap.Labels[templateInstanceLabel] == "" || configMap.Labels[appManagedByLabel] != templateManagedBy {
			continue
		}
		instance := instanceFromConfigMap(configMap)
		// The object list is long and only the detail view wants it.
		instance.Objects = nil
		result = append(result, instance)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		return result[i].Name < result[j].Name
	})
	c.ResponseOk(result)
}

// GetTemplateInstance reads one installed app, including everything it created.
// @router /api/get-template-instance [get]
func (c *ApiController) GetTemplateInstance() {
	if c.RequireSignedIn() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	namespace := c.GetString("namespace")
	if namespace == "" {
		namespace = "default"
	}
	name := c.GetString("name")
	configMap, err := object.GetConfigMap(cfg, namespace, templateInstancePrefix+name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(instanceFromConfigMap(*configMap))
}

type templateInstanceActionRequest struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	DeleteData bool   `json:"deleteData"`
}

// DeleteTemplateInstance removes everything one deploy created.
// @router /api/delete-template-instance [post]
func (c *ApiController) DeleteTemplateInstance() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req templateInstanceActionRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	configMap, err := object.GetConfigMap(cfg, req.Namespace, templateInstancePrefix+req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	instance := instanceFromConfigMap(*configMap)

	applier, err := newTemplateApplier(cfg, req.Namespace, instance.Name, instance.Template)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	failures := applier.deleteApplied(c.Ctx.Request.Context(), instance.Objects)

	for _, database := range instance.Databases {
		failures = append(failures, deleteDatabaseObjects(cfg, req.Namespace, database, req.DeleteData)...)
	}

	if err := object.DeleteConfigMap(cfg, req.Namespace, configMap.Name); err != nil && !errors.IsNotFound(err) {
		failures = append(failures, fmt.Sprintf("record %s (%v)", configMap.Name, err))
	}

	if len(failures) > 0 {
		c.ResponseError("the app was removed but these could not be: " + strings.Join(failures, ", "))
		return
	}
	c.ResponseOk()
}
