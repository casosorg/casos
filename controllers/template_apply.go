package controllers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	sigsyaml "sigs.k8s.io/yaml"

	"github.com/casosorg/casos/object"
)

// Applying a template means applying whatever objects it names, whichever kind
// they are — so this is the one place in casos that talks to the cluster
// through the dynamic client rather than through a typed one.
//
// Two kinds are not applied but translated, because a sealos template asks for
// them by way of operators casos does not run:
//
//   - apps.kubeblocks.io/Cluster becomes a casos database, with the connection
//     secret named the way KubeBlocks names it so the app that asked for the
//     database finds its credentials unchanged.
//   - app.sealos.io/App becomes a desktop icon recorded on the instance.
//
// Anything else the cluster has no type for is reported back rather than
// swallowed: an app that half-installed should say which piece is missing.

const (
	legacyIngressClassAnnotation = "kubernetes.io/ingress.class"
	templateInstanceLabel        = "casos.io/template-instance"
	templateNameLabel            = "casos.io/template"
	templateManagedBy            = "casos"
)

type appliedObject struct {
	Group     string `json:"group"`
	Version   string `json:"version"`
	Resource  string `json:"resource"`
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type templateAppEntry struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
	URL  string `json:"url"`
}

type unsupportedObject struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Reason     string `json:"reason"`
}

type templateApplyReport struct {
	Applied     []appliedObject     `json:"applied"`
	Apps        []templateAppEntry  `json:"apps"`
	Databases   []string            `json:"databases"`
	Unsupported []unsupportedObject `json:"unsupported"`
}

type templateApplier struct {
	cfg       *rest.Config
	dynamic   dynamic.Interface
	mapper    meta.RESTMapper
	namespace string
	instance  string
	template  string
	// plainHTTP is set when the app gateway serves the instance's addresses.
	plainHTTP      bool
	ingressClasses map[string]bool
	secrets        map[string]bool
	secretsUnknown bool
}

func newTemplateApplier(cfg *rest.Config, namespace, instance, template string) (*templateApplier, error) {
	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	groups, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, err
	}
	return &templateApplier{
		cfg:       cfg,
		dynamic:   dynamicClient,
		mapper:    restmapper.NewDiscoveryRESTMapper(groups),
		namespace: namespace,
		instance:  instance,
		template:  template,
	}, nil
}

func (a *templateApplier) ownershipLabels() map[string]string {
	return map[string]string{
		appManagedByLabel:     templateManagedBy,
		templateInstanceLabel: a.instance,
		templateNameLabel:     a.template,
	}
}

// apply walks the rendered documents in order, which is the order the template
// author wrote them in — databases first in every template that has one.
func (a *templateApplier) apply(ctx context.Context, documents []string) templateApplyReport {
	report := templateApplyReport{
		Applied:     []appliedObject{},
		Apps:        []templateAppEntry{},
		Databases:   []string{},
		Unsupported: []unsupportedObject{},
	}

	for _, document := range documents {
		if strings.TrimSpace(document) == "" {
			continue
		}
		var content map[string]any
		if err := sigsyaml.Unmarshal([]byte(document), &content); err != nil {
			report.Unsupported = append(report.Unsupported, unsupportedObject{
				Reason: "this part of the template is not valid YAML: " + err.Error(),
			})
			continue
		}
		if len(content) == 0 {
			continue
		}
		quoteEnvValues(content)
		stripMissingImagePullSecrets(content, a.secretExists)
		item := &unstructured.Unstructured{Object: content}
		if item.GetKind() == "" || item.GetAPIVersion() == "" {
			continue
		}

		group := item.GroupVersionKind().Group
		switch {
		case group == "app.sealos.io" && item.GetKind() == "App":
			report.Apps = append(report.Apps, appEntryOf(item))
		case group == "app.sealos.io":
			// Instance objects only group a template's parts together, which
			// casos does with labels instead.
			continue
		case group == "apps.kubeblocks.io" && item.GetKind() == "Cluster":
			err := a.applyDatabase(ctx, item, &report)
			if err != nil {
				report.Unsupported = append(report.Unsupported, unsupportedObject{
					APIVersion: item.GetAPIVersion(),
					Kind:       item.GetKind(),
					Name:       item.GetName(),
					Reason:     err.Error(),
				})
				continue
			}
		default:
			if group == "networking.k8s.io" && item.GetKind() == "Ingress" {
				a.adaptIngress(ctx, item)
			}
			applied, err := a.applyObject(ctx, item)
			if err != nil {
				report.Unsupported = append(report.Unsupported, unsupportedObject{
					APIVersion: item.GetAPIVersion(),
					Kind:       item.GetKind(),
					Name:       item.GetName(),
					Reason:     err.Error(),
				})
				continue
			}
			report.Applied = append(report.Applied, applied)
		}
	}

	return report
}

// Sealos templates name a pull secret CasOS never creates.
func stripMissingImagePullSecrets(node any, exists func(string) bool) {
	switch value := node.(type) {
	case map[string]any:
		if refs, ok := value["imagePullSecrets"].([]any); ok {
			kept := make([]any, 0, len(refs))
			for _, ref := range refs {
				entry, ok := ref.(map[string]any)
				if !ok {
					kept = append(kept, ref)
					continue
				}
				name, _ := entry["name"].(string)
				if name != "" && exists(name) {
					kept = append(kept, ref)
				}
			}
			if len(kept) == 0 {
				delete(value, "imagePullSecrets")
			} else {
				value["imagePullSecrets"] = kept
			}
		}
		for key, child := range value {
			if key == "imagePullSecrets" {
				continue
			}
			stripMissingImagePullSecrets(child, exists)
		}
	case []any:
		for _, child := range value {
			stripMissingImagePullSecrets(child, exists)
		}
	}
}

func (a *templateApplier) secretExists(name string) bool {
	// Keep the template's secret if the namespace cannot be listed.
	if a.secretsUnknown {
		return true
	}
	if a.secrets == nil {
		list, err := object.GetSecrets(a.cfg, a.namespace)
		if err != nil {
			a.secretsUnknown = true
			return true
		}
		a.secrets = make(map[string]bool, len(list))
		for _, secret := range list {
			a.secrets[secret.Name] = true
		}
	}
	return a.secrets[name]
}

// quoteEnvValues turns env values such as `value: ${{ inputs.port }}` back into
// strings: once rendered they parse as numbers, which the apiserver rejects.
func quoteEnvValues(node any) {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			if env, ok := child.([]any); ok && key == "env" {
				for _, entry := range env {
					if variable, ok := entry.(map[string]any); ok {
						switch scalar := variable["value"].(type) {
						case float64:
							variable["value"] = strconv.FormatFloat(scalar, 'f', -1, 64)
						case int64, bool:
							variable["value"] = fmt.Sprint(scalar)
						}
					}
				}
				continue
			}
			quoteEnvValues(child)
		}
	case []any:
		for _, child := range value {
			quoteEnvValues(child)
		}
	}
}

// adaptIngress fits a sealos Ingress to this cluster. sealos names its nginx
// class in the legacy annotation, and an Ingress of a class nobody runs is
// served by nobody, so the annotation goes and the default class takes it.
// Behind the app gateway there is no TLS to terminate, and a TLS section would
// keep the route off the plain HTTP entrypoint the gateway talks to.
func (a *templateApplier) adaptIngress(ctx context.Context, item *unstructured.Unstructured) {
	if class := item.GetAnnotations()[legacyIngressClassAnnotation]; class != "" && !a.hasIngressClass(ctx, class) {
		annotations := item.GetAnnotations()
		delete(annotations, legacyIngressClassAnnotation)
		item.SetAnnotations(annotations)
	}
	if className, _, _ := unstructured.NestedString(item.Object, "spec", "ingressClassName"); className != "" && !a.hasIngressClass(ctx, className) {
		unstructured.RemoveNestedField(item.Object, "spec", "ingressClassName")
	}
	if a.plainHTTP {
		unstructured.RemoveNestedField(item.Object, "spec", "tls")
	}
	flattenNginxCapturePaths(item)
}

const nginxRewriteTargetAnnotation = "nginx.ingress.kubernetes.io/rewrite-target"

// Sealos writes nginx capture paths such as /()(.*); Traefik treats them as
// literal strings, so / never matches.
func flattenNginxCapturePaths(item *unstructured.Unstructured) {
	rules, found, _ := unstructured.NestedSlice(item.Object, "spec", "rules")
	if !found {
		return
	}
	changed := false
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]any)
		if !ok {
			continue
		}
		http, ok := ruleMap["http"].(map[string]any)
		if !ok {
			continue
		}
		paths, ok := http["paths"].([]any)
		if !ok {
			continue
		}
		for _, entry := range paths {
			pathMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			path, _ := pathMap["path"].(string)
			if next, ok := flattenNginxCapturePath(path); ok {
				pathMap["path"] = next
				changed = true
			}
		}
	}
	if !changed {
		return
	}
	_ = unstructured.SetNestedSlice(item.Object, rules, "spec", "rules")
	annotations := item.GetAnnotations()
	if len(annotations) == 0 {
		return
	}
	delete(annotations, nginxRewriteTargetAnnotation)
	item.SetAnnotations(annotations)
}

func flattenNginxCapturePath(path string) (string, bool) {
	switch path {
	case "/()(.*)", "/?(.*)":
		return "/", true
	default:
		return path, false
	}
}

func (a *templateApplier) hasIngressClass(ctx context.Context, name string) bool {
	if a.ingressClasses == nil {
		client, err := kubernetes.NewForConfig(a.cfg)
		if err != nil {
			return true
		}
		classes, err := client.NetworkingV1().IngressClasses().List(ctx, metav1.ListOptions{})
		if err != nil {
			// Unable to tell, so the template's choice stands.
			return true
		}
		a.ingressClasses = map[string]bool{}
		for _, class := range classes.Items {
			a.ingressClasses[class.Name] = true
		}
	}
	return a.ingressClasses[name]
}

func appEntryOf(item *unstructured.Unstructured) templateAppEntry {
	entry := templateAppEntry{Name: item.GetName()}
	if icon, ok, _ := unstructured.NestedString(item.Object, "spec", "icon"); ok {
		entry.Icon = icon
	}
	if name, ok, _ := unstructured.NestedString(item.Object, "spec", "name"); ok && name != "" {
		entry.Name = name
	}
	if url, ok, _ := unstructured.NestedString(item.Object, "spec", "data", "url"); ok {
		entry.URL = url
	}
	return entry
}

func (a *templateApplier) applyObject(ctx context.Context, item *unstructured.Unstructured) (appliedObject, error) {
	gvk := item.GroupVersionKind()
	mapping, err := a.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return appliedObject{}, fmt.Errorf("this cluster has no %s (%s); the operator that provides it is not installed", gvk.Kind, gvk.GroupVersion().String())
	}

	labels := item.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	for key, value := range a.ownershipLabels() {
		labels[key] = value
	}
	item.SetLabels(labels)
	// Pods carry the instance label too, so the instance can say whether its
	// app is up. A selector names only the template's own labels, so adding
	// to the Pod template never breaks it.
	if _, found, _ := unstructured.NestedMap(item.Object, "spec", "template"); found {
		podLabels, _, _ := unstructured.NestedStringMap(item.Object, "spec", "template", "metadata", "labels")
		if podLabels == nil {
			podLabels = map[string]string{}
		}
		podLabels[templateInstanceLabel] = a.instance
		_ = unstructured.SetNestedStringMap(item.Object, podLabels, "spec", "template", "metadata", "labels")
	}

	namespaced := mapping.Scope.Name() == meta.RESTScopeNameNamespace
	if namespaced {
		item.SetNamespace(a.namespace)
	}

	var resourceInterface dynamic.ResourceInterface = a.dynamic.Resource(mapping.Resource)
	if namespaced {
		resourceInterface = a.dynamic.Resource(mapping.Resource).Namespace(a.namespace)
	}

	created, err := resourceInterface.Create(ctx, item, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		existing, getErr := resourceInterface.Get(ctx, item.GetName(), metav1.GetOptions{})
		if getErr != nil {
			return appliedObject{}, getErr
		}
		// The parts of an object the cluster owns are not the template's to
		// rewrite, so the update carries them over.
		item.SetResourceVersion(existing.GetResourceVersion())
		if clusterIP, ok, _ := unstructured.NestedString(existing.Object, "spec", "clusterIP"); ok && clusterIP != "" {
			_ = unstructured.SetNestedField(item.Object, clusterIP, "spec", "clusterIP")
		}
		created, err = resourceInterface.Update(ctx, item, metav1.UpdateOptions{})
	}
	if err != nil {
		return appliedObject{}, err
	}
	if created.GetKind() == "Secret" && a.secrets != nil {
		a.secrets[created.GetName()] = true
	}

	result := appliedObject{
		Group:    mapping.Resource.Group,
		Version:  mapping.Resource.Version,
		Resource: mapping.Resource.Resource,
		Kind:     gvk.Kind,
		Name:     created.GetName(),
	}
	if namespaced {
		result.Namespace = a.namespace
	}
	return result, nil
}

// kubeBlocksEngines maps what a sealos template asks KubeBlocks for onto what
// casos runs. The names on the left are KubeBlocks cluster definitions.
var kubeBlocksEngines = map[string]string{
	"postgresql":     "postgresql",
	"apecloud-mysql": "mysql",
	"mysql":          "mysql",
	"mongodb":        "mongodb",
	"redis":          "redis",
}

var sealosDefaultDatabaseName = map[string]string{
	"mysql":      "mydb",
	"postgresql": "postgres",
}

// engineVersionFromRef reads "postgresql-16.4.0" or "ac-mysql-8.0.30" and
// picks the closest version casos offers, so a template that asked for
// PostgreSQL 16 does not silently get 17.
func engineVersionFromRef(engine databaseEngine, ref string) string {
	digits := strings.IndexFunc(ref, func(r rune) bool { return r >= '0' && r <= '9' })
	if digits < 0 {
		return engine.Versions[0]
	}
	wanted := ref[digits:]
	major := strings.SplitN(wanted, ".", 2)[0]
	for _, candidate := range engine.Versions {
		if strings.HasPrefix(candidate, wanted) {
			return candidate
		}
	}
	for _, candidate := range engine.Versions {
		if strings.HasPrefix(candidate, major+".") || strings.HasPrefix(candidate, major+"-") || candidate == major {
			return candidate
		}
	}
	return engine.Versions[0]
}

// applyDatabase turns a KubeBlocks Cluster into a casos database, and leaves
// behind the connection secret under the name KubeBlocks would have used.
func (a *templateApplier) applyDatabase(ctx context.Context, item *unstructured.Unstructured, report *templateApplyReport) error {
	definition, _, _ := unstructured.NestedString(item.Object, "spec", "clusterDefinitionRef")
	versionRef, _, _ := unstructured.NestedString(item.Object, "spec", "clusterVersionRef")
	components, _, _ := unstructured.NestedSlice(item.Object, "spec", "componentSpecs")
	var component map[string]any
	if len(components) > 0 {
		component, _ = components[0].(map[string]any)
	}
	componentBased := definition == ""
	if componentBased {
		definition, _, _ = unstructured.NestedString(component, "componentDef")
		if versionRef == "" {
			versionRef, _, _ = unstructured.NestedString(component, "serviceVersion")
		}
	}
	engineKey, ok := kubeBlocksEngines[definition]
	if !ok {
		return fmt.Errorf("casos has no engine for a %q database", definition)
	}
	engine, ok := engineByKey(engineKey)
	if !ok {
		return fmt.Errorf("casos has no engine for a %q database", definition)
	}

	request := databaseRequest{
		Namespace: a.namespace,
		Name:      item.GetName(),
		Engine:    engine.Key,
		Version:   engineVersionFromRef(engine, versionRef),
		Database:  sealosDefaultDatabaseName[engine.Key],
	}

	if component != nil {
		if cpu, ok, _ := unstructured.NestedString(component, "resources", "limits", "cpu"); ok && cpu != "" {
			request.CpuLimit = &cpu
		}
		if memory, ok, _ := unstructured.NestedString(component, "resources", "limits", "memory"); ok && memory != "" {
			request.MemoryLimit = &memory
		}
		claims, _, _ := unstructured.NestedSlice(component, "volumeClaimTemplates")
		for _, entry := range claims {
			claim, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if storage, ok, _ := unstructured.NestedString(claim, "spec", "resources", "requests", "storage"); ok && storage != "" {
				request.Storage = storage
				break
			}
		}
	}

	_, credentials, err := createDatabase(a.cfg, request)
	if err != nil {
		return err
	}

	report.Databases = append(report.Databases, item.GetName())
	if err := a.writeConnCredential(item.GetName(), credentials); err != nil {
		return fmt.Errorf("the database was created but its connection secret could not be: %w", err)
	}
	report.Applied = append(report.Applied, appliedObject{
		Version: "v1", Resource: "secrets", Kind: "Secret",
		Namespace: a.namespace, Name: item.GetName() + "-conn-credential",
	})
	if componentBased && engine.Key == "mongodb" {
		componentName, _, _ := unstructured.NestedString(component, "name")
		if componentName == "" {
			return fmt.Errorf("the MongoDB component has no name")
		}
		return a.writeMongoDBAliases(ctx, item.GetName(), componentName, credentials, report)
	}
	return nil
}

// Component-based MongoDB templates use KubeBlocks account and service names.
func (a *templateApplier) writeMongoDBAliases(ctx context.Context, name, component string, credentials databaseCredentials, report *templateApplyReport) error {
	prefix := name + "-" + component
	objects := []runtime.Object{
		&corev1.Secret{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
			ObjectMeta: metav1.ObjectMeta{Name: prefix + "-account-root"},
			Type:       corev1.SecretTypeOpaque,
			StringData: map[string]string{"username": credentials.User, "password": credentials.Password},
		},
		&corev1.Service{
			TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
			ObjectMeta: metav1.ObjectMeta{Name: prefix},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": name},
				Ports:    []corev1.ServicePort{{Name: "db", Port: credentials.Port}},
			},
		},
	}
	for _, alias := range objects {
		content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(alias)
		if err != nil {
			return err
		}
		applied, err := a.applyObject(ctx, &unstructured.Unstructured{Object: content})
		if err != nil {
			return err
		}
		report.Applied = append(report.Applied, applied)
	}
	return nil
}

// writeConnCredential is the compatibility shim that makes the market work: a
// template's app reads its database's address out of "<cluster>-conn-credential"
// with the keys KubeBlocks writes, so casos writes exactly that.
func (a *templateApplier) writeConnCredential(name string, credentials databaseCredentials) error {
	secretName := name + "-conn-credential"
	data := map[string]string{
		"username": credentials.User,
		"password": credentials.Password,
		"host":     credentials.Host,
		"port":     fmt.Sprintf("%d", credentials.Port),
		"endpoint": fmt.Sprintf("%s:%d", credentials.Host, credentials.Port),
		"database": credentials.Database,
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: a.namespace,
			Labels:    a.ownershipLabels(),
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: data,
	}
	existing, err := object.GetSecret(a.cfg, a.namespace, secretName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		_, err := object.AddSecret(a.cfg, secret)
		return err
	}
	existing.StringData = data
	existing.Labels = secret.Labels
	_, err = object.UpdateSecret(a.cfg, existing)
	return err
}

// statefulSetClaims names the PVCs minted from volumeClaimTemplates: they
// outlive their StatefulSet, so call this before deleteApplied removes it.
func (a *templateApplier) statefulSetClaims(ctx context.Context, objects []appliedObject) []appliedObject {
	claims := []appliedObject{}
	for _, item := range objects {
		if item.Kind != "StatefulSet" || item.Namespace == "" {
			continue
		}
		gvr := schema.GroupVersionResource{Group: item.Group, Version: item.Version, Resource: item.Resource}
		set, err := a.dynamic.Resource(gvr).Namespace(item.Namespace).Get(ctx, item.Name, metav1.GetOptions{})
		if err != nil {
			continue
		}
		replicas, found, _ := unstructured.NestedInt64(set.Object, "spec", "replicas")
		if !found || replicas < 1 {
			replicas = 1
		}
		templates, _, _ := unstructured.NestedSlice(set.Object, "spec", "volumeClaimTemplates")
		for _, entry := range templates {
			claim, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			claimName, _, _ := unstructured.NestedString(claim, "metadata", "name")
			if claimName == "" {
				continue
			}
			for ordinal := int64(0); ordinal < replicas; ordinal++ {
				claims = append(claims, appliedObject{
					Version:   "v1",
					Resource:  "persistentvolumeclaims",
					Kind:      "PersistentVolumeClaim",
					Namespace: item.Namespace,
					Name:      fmt.Sprintf("%s-%s-%d", claimName, item.Name, ordinal),
				})
			}
		}
	}
	return claims
}

// deleteApplied removes what an instance created, newest first so that an
// object is never left pointing at one that is already gone.
func (a *templateApplier) deleteApplied(ctx context.Context, objects []appliedObject) []string {
	failures := []string{}
	for index := len(objects) - 1; index >= 0; index-- {
		item := objects[index]
		gvr := schema.GroupVersionResource{Group: item.Group, Version: item.Version, Resource: item.Resource}
		var resourceInterface dynamic.ResourceInterface = a.dynamic.Resource(gvr)
		if item.Namespace != "" {
			resourceInterface = a.dynamic.Resource(gvr).Namespace(item.Namespace)
		}
		// A Job left to the API's default would orphan its Pods.
		propagation := metav1.DeletePropagationBackground
		if err := resourceInterface.Delete(ctx, item.Name, metav1.DeleteOptions{PropagationPolicy: &propagation}); err != nil && !errors.IsNotFound(err) {
			failures = append(failures, fmt.Sprintf("%s %s (%v)", item.Kind, item.Name, err))
		}
	}
	return failures
}
