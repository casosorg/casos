package controllers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/store"
)

// The launchpad is the full form behind an image app: everything a container
// needs to be a running, reachable application — resources, a command, config
// files, a private registry, autoscaling and a domain — on top of the workload
// the App Store's one-click install already creates.
//
// The extra objects are named after the app so that reading them back needs no
// index, and all of them carry the app's ownership labels so an uninstall can
// sweep them up.

const appConfigVolumeName = "app-config"

func appConfigMapName(app string) string { return app + "-config" }

func appRegistrySecretName(app string) string { return app + "-registry" }

type appConfigFile struct {
	MountPath string `json:"mountPath"`
	Content   string `json:"content"`
}

type appRegistryLogin struct {
	Server   string `json:"server"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type appHpaRequest struct {
	Enabled     bool  `json:"enabled"`
	MinReplicas int32 `json:"minReplicas"`
	MaxReplicas int32 `json:"maxReplicas"`
	CPUTarget   int32 `json:"cpuTarget"`
}

type appDomain struct {
	Host         string `json:"host"`
	Port         int32  `json:"port"`
	IngressClass string `json:"ingressClass"`
	// Https asks for a Let's Encrypt certificate, and reads back as whether the
	// Ingress already carries one for this host.
	Https bool `json:"https"`
}

type appPodSummary struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Phase      string   `json:"phase"`
	NodeName   string   `json:"nodeName"`
	Restarts   int32    `json:"restarts"`
	Ready      string   `json:"ready"`
	Containers []string `json:"containers"`
	CreatedAt  string   `json:"createdAt"`
}

type imageAppDetail struct {
	imageAppSummary
	resourceSummary
	Command     []string        `json:"command"`
	Args        []string        `json:"args"`
	ConfigFiles []appConfigFile `json:"configFiles"`
	// Only the address is read back: the credentials stay in the cluster.
	RegistryServer string          `json:"registryServer"`
	Hpa            *appHpaRequest  `json:"hpa"`
	Domains        []appDomain     `json:"domains"`
	Urls           []string        `json:"urls"`
	Pods           []appPodSummary `json:"pods"`
}

// configFileKey names one file's entry in the app's ConfigMap. The mount path
// cannot be the key — a key may not contain a slash — so the index is, and the
// path is what the mount puts it back at.
func configFileKey(index int) string {
	return fmt.Sprintf("file-%d", index)
}

// applyAppContainer puts the parts of the form that live on the container
// itself onto it. Shared by install and upgrade so the two cannot disagree
// about what a field means.
func applyAppContainer(container *corev1.Container, req deployAppRequest) error {
	if err := applyResources(container, req.resourceRequest); err != nil {
		return err
	}
	container.Command = trimmedList(req.Command)
	container.Args = trimmedList(req.Args)
	return nil
}

func trimmedList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func validConfigFiles(files []appConfigFile) []appConfigFile {
	result := make([]appConfigFile, 0, len(files))
	for _, file := range files {
		if strings.TrimSpace(file.MountPath) != "" {
			result = append(result, file)
		}
	}
	return result
}

// existingConfigMounts reads back the config volume and mounts a running
// workload already carries, so a payload that says nothing about config files
// leaves them where they are.
func existingConfigMounts(existing *appsv1.Deployment) (*corev1.Volume, []corev1.VolumeMount) {
	if existing == nil || len(existing.Spec.Template.Spec.Containers) == 0 {
		return nil, nil
	}
	var volume *corev1.Volume
	for _, candidate := range existing.Spec.Template.Spec.Volumes {
		if candidate.Name == appConfigVolumeName {
			found := candidate
			volume = &found
			break
		}
	}
	mounts := []corev1.VolumeMount{}
	for _, mount := range existing.Spec.Template.Spec.Containers[0].VolumeMounts {
		if mount.Name == appConfigVolumeName {
			mounts = append(mounts, mount)
		}
	}
	if volume == nil {
		return nil, nil
	}
	return volume, mounts
}

// reconcileAppConfigFiles keeps the ConfigMap holding the app's config files in
// step with the form, and hands back the volume and mounts that put them in the
// container. An app with no config files has no ConfigMap.
func reconcileAppConfigFiles(cfg *rest.Config, req deployAppRequest, labels map[string]string, existingWorkload *appsv1.Deployment) (*corev1.Volume, []corev1.VolumeMount, error) {
	if req.ConfigFiles == nil {
		volume, mounts := existingConfigMounts(existingWorkload)
		return volume, mounts, nil
	}

	name := appConfigMapName(req.Name)
	files := validConfigFiles(*req.ConfigFiles)

	existing, err := object.GetConfigMap(cfg, req.Namespace, name)
	if err != nil && !errors.IsNotFound(err) {
		return nil, nil, err
	}
	if errors.IsNotFound(err) {
		existing = nil
	}

	if len(files) == 0 {
		if existing != nil && ownedByApp(existing.ObjectMeta, req.Name) {
			if err := object.DeleteConfigMap(cfg, req.Namespace, name); err != nil && !errors.IsNotFound(err) {
				return nil, nil, err
			}
		}
		return nil, nil, nil
	}

	data := map[string]string{}
	mounts := make([]corev1.VolumeMount, 0, len(files))
	for i, file := range files {
		key := configFileKey(i)
		data[key] = file.Content
		mounts = append(mounts, corev1.VolumeMount{
			Name:      appConfigVolumeName,
			MountPath: file.MountPath,
			SubPath:   key,
		})
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: req.Namespace},
		Data:       data,
	}
	applyLabels(&cm.ObjectMeta, labels)
	if existing == nil {
		if _, err := object.AddConfigMap(cfg, cm); err != nil {
			return nil, nil, err
		}
	} else {
		existing.Data = data
		applyLabels(&existing.ObjectMeta, labels)
		if _, err := object.UpdateConfigMap(cfg, existing); err != nil {
			return nil, nil, err
		}
	}

	volume := &corev1.Volume{
		Name: appConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: name},
			},
		},
	}
	return volume, mounts, nil
}

// reconcileAppRegistry stores the credentials for a private image as a
// dockerconfigjson secret and returns its name, or an empty name once the form
// no longer asks for one. A form that leaves the password blank on an edit
// keeps the stored one: the password is never read back for it to resend.
func reconcileAppRegistry(cfg *rest.Config, req deployAppRequest, labels map[string]string, existingWorkload *appsv1.Deployment) (string, error) {
	if req.Registry == nil {
		if existingWorkload != nil && len(existingWorkload.Spec.Template.Spec.ImagePullSecrets) > 0 {
			return existingWorkload.Spec.Template.Spec.ImagePullSecrets[0].Name, nil
		}
		return "", nil
	}

	name := appRegistrySecretName(req.Name)
	existing, err := object.GetSecret(cfg, req.Namespace, name)
	if err != nil && !errors.IsNotFound(err) {
		return "", err
	}
	if errors.IsNotFound(err) {
		existing = nil
	}

	login := req.Registry
	if login == nil || strings.TrimSpace(login.Server) == "" || strings.TrimSpace(login.Username) == "" {
		if existing != nil && ownedByApp(existing.ObjectMeta, req.Name) {
			if err := object.DeleteSecret(cfg, req.Namespace, name); err != nil && !errors.IsNotFound(err) {
				return "", err
			}
		}
		return "", nil
	}

	if login.Password == "" {
		if existing == nil {
			return "", fmt.Errorf("a password is required to sign in to %s", login.Server)
		}
		return name, nil
	}

	auth := base64.StdEncoding.EncodeToString([]byte(login.Username + ":" + login.Password))
	config := map[string]any{
		"auths": map[string]any{
			login.Server: map[string]string{
				"username": login.Username,
				"password": login.Password,
				"auth":     auth,
			},
		},
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: req.Namespace},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data:       map[string][]byte{corev1.DockerConfigJsonKey: encoded},
	}
	applyLabels(&secret.ObjectMeta, labels)
	if existing == nil {
		if _, err := object.AddSecret(cfg, secret); err != nil {
			return "", err
		}
		return name, nil
	}
	existing.Type = corev1.SecretTypeDockerConfigJson
	existing.Data = secret.Data
	applyLabels(&existing.ObjectMeta, labels)
	if _, err := object.UpdateSecret(cfg, existing); err != nil {
		return "", err
	}
	return name, nil
}

// reconcileAppHpa creates, updates or removes the autoscaler in front of the
// app's workload.
func reconcileAppHpa(cfg *rest.Config, req deployAppRequest, labels map[string]string) error {
	if req.Hpa == nil {
		return nil
	}
	existing, err := object.GetHPA(cfg, req.Namespace, req.Name)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if errors.IsNotFound(err) {
		existing = nil
	}

	if !req.Hpa.Enabled {
		if existing != nil && ownedByApp(existing.ObjectMeta, req.Name) {
			if err := object.DeleteHPA(cfg, req.Namespace, req.Name); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
		return nil
	}

	target := req.Hpa.CPUTarget
	if target <= 0 {
		target = 60
	}
	maxReplicas := req.Hpa.MaxReplicas
	if maxReplicas < req.Hpa.MinReplicas {
		maxReplicas = req.Hpa.MinReplicas
	}
	if maxReplicas < 1 {
		maxReplicas = 1
	}
	hpaReq := hpaRequest{
		Namespace:            req.Namespace,
		Name:                 req.Name,
		ScaleTargetKind:      "Deployment",
		ScaleTargetName:      req.Name,
		MinReplicas:          req.Hpa.MinReplicas,
		MaxReplicas:          maxReplicas,
		CPUTargetUtilization: &target,
	}
	hpa := buildHpaObject(hpaReq)
	applyLabels(&hpa.ObjectMeta, labels)
	if existing == nil {
		_, err := object.AddHPA(cfg, hpa)
		return err
	}
	hpa.ObjectMeta.ResourceVersion = existing.ResourceVersion
	_, err = object.UpdateHPA(cfg, hpa)
	return err
}

func validDomains(domains []appDomain) []appDomain {
	result := make([]appDomain, 0, len(domains))
	for _, domain := range domains {
		if strings.TrimSpace(domain.Host) != "" && domain.Port > 0 {
			result = append(result, domain)
		}
	}
	return result
}

// reconcileAppIngress publishes the app on the domains the form names, through
// one Ingress carrying every host. Removing the last domain removes it.
func reconcileAppIngress(cfg *rest.Config, req deployAppRequest, labels map[string]string) error {
	if req.Domains == nil {
		return nil
	}
	existing, err := object.GetIngress(cfg, req.Namespace, req.Name)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if errors.IsNotFound(err) {
		existing = nil
	}

	domains := validDomains(*req.Domains)
	if len(domains) == 0 {
		if existing != nil && ownedByApp(existing.ObjectMeta, req.Name) {
			if err := object.DeleteIngress(cfg, req.Namespace, req.Name); err != nil && !errors.IsNotFound(err) {
				return err
			}
		}
		return nil
	}

	rules := make([]ingressRule, 0, len(domains))
	ingressClass := ""
	for _, domain := range domains {
		if ingressClass == "" {
			ingressClass = domain.IngressClass
		}
		rules = append(rules, ingressRule{
			Host:        domain.Host,
			Path:        "/",
			PathType:    "Prefix",
			ServiceName: req.Name,
			ServicePort: domain.Port,
		})
	}
	spec := buildIngressSpec(ingressRequest{
		Namespace:    req.Namespace,
		Name:         req.Name,
		IngressClass: ingressClass,
		Rules:        rules,
	})

	if existing == nil {
		ing := &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace},
			Spec:       spec,
		}
		applyLabels(&ing.ObjectMeta, labels)
		_, err := object.AddIngress(cfg, ing)
		return err
	}
	// TLS is left alone: a certificate is attached to the Ingress by the
	// certificate page or by ACME, and rewriting the spec must not drop it.
	spec.TLS = existing.Spec.TLS
	existing.Spec = spec
	applyLabels(&existing.ObjectMeta, labels)
	_, err = object.UpdateIngress(cfg, existing)
	return err
}

// reconcileAppExtras applies everything an app owns besides its own workload,
// and hands back the config volume and mounts the workload has to carry.
func reconcileAppExtras(cfg *rest.Config, req deployAppRequest, labels map[string]string, existingWorkload *appsv1.Deployment) (*corev1.Volume, []corev1.VolumeMount, string, error) {
	volume, mounts, err := reconcileAppConfigFiles(cfg, req, labels, existingWorkload)
	if err != nil {
		return nil, nil, "", err
	}
	secretName, err := reconcileAppRegistry(cfg, req, labels, existingWorkload)
	if err != nil {
		return nil, nil, "", err
	}
	return volume, mounts, secretName, nil
}

// requestAppCertificate asks Let's Encrypt for a certificate when the form
// ticked HTTPS and the Ingress does not already carry one.
//
// One request covers the app: a certificate is attached to the Ingress as a
// whole, so asking again for a second host would only collide with the first.
// Issuance is asynchronous and its failure is not the deployment's failure —
// the app is already serving on HTTP, and the certificate page reports why.
func requestAppCertificate(cfg *rest.Config, req deployAppRequest) {
	if req.Domains == nil {
		return
	}
	ing, err := object.GetIngress(cfg, req.Namespace, req.Name)
	if err != nil || ing == nil || len(ing.Spec.TLS) > 0 {
		return
	}
	for _, domain := range validDomains(*req.Domains) {
		if domain.Https {
			_, _ = startLECertRequest(cfg, req.Namespace, req.Name, domain.Host, "", 0)
			return
		}
	}
}

// reconcileAppNetworkAndScaling runs the parts that must happen after the
// workload exists, because they point at it.
func reconcileAppNetworkAndScaling(cfg *rest.Config, req deployAppRequest, labels map[string]string) error {
	if err := reconcileAppService(cfg, req, labels); err != nil {
		return fmt.Errorf("the app was saved but its address could not be: %w", err)
	}
	if err := reconcileAppIngress(cfg, req, labels); err != nil {
		return fmt.Errorf("the app was saved but its domain could not be: %w", err)
	}
	requestAppCertificate(cfg, req)
	if err := reconcileAppHpa(cfg, req, labels); err != nil {
		return fmt.Errorf("the app was saved but its autoscaler could not be: %w", err)
	}
	return nil
}

func applyImagePullSecret(spec *corev1.PodSpec, secretName string) {
	if secretName == "" {
		spec.ImagePullSecrets = nil
		return
	}
	spec.ImagePullSecrets = []corev1.LocalObjectReference{{Name: secretName}}
}

func configFilesOf(cm *corev1.ConfigMap, container corev1.Container) []appConfigFile {
	files := []appConfigFile{}
	if cm == nil {
		return files
	}
	for _, mount := range container.VolumeMounts {
		if mount.Name != appConfigVolumeName || mount.SubPath == "" {
			continue
		}
		files = append(files, appConfigFile{MountPath: mount.MountPath, Content: cm.Data[mount.SubPath]})
	}
	return files
}

func hpaOf(hpa *autoscalingv2.HorizontalPodAutoscaler) *appHpaRequest {
	if hpa == nil {
		return nil
	}
	result := &appHpaRequest{Enabled: true, MaxReplicas: hpa.Spec.MaxReplicas, CPUTarget: 60}
	if hpa.Spec.MinReplicas != nil {
		result.MinReplicas = *hpa.Spec.MinReplicas
	}
	for _, metric := range hpa.Spec.Metrics {
		if metric.Resource != nil && metric.Resource.Name == corev1.ResourceCPU && metric.Resource.Target.AverageUtilization != nil {
			result.CPUTarget = *metric.Resource.Target.AverageUtilization
		}
	}
	return result
}

func domainsOf(ing *networkingv1.Ingress) []appDomain {
	domains := []appDomain{}
	if ing == nil {
		return domains
	}
	class := ""
	if ing.Spec.IngressClassName != nil {
		class = *ing.Spec.IngressClassName
	}
	secured := map[string]bool{}
	for _, tls := range ing.Spec.TLS {
		for _, host := range tls.Hosts {
			secured[host] = true
		}
	}
	for _, rule := range ing.Spec.Rules {
		port := int32(0)
		if rule.HTTP != nil && len(rule.HTTP.Paths) > 0 {
			port = rule.HTTP.Paths[0].Backend.Service.Port.Number
		}
		domains = append(domains, appDomain{Host: rule.Host, Port: port, IngressClass: class, Https: secured[rule.Host]})
	}
	return domains
}

// appUrls is where the app answers: its domains first, because a name is what
// someone would rather be handed than an address and a port.
func appUrls(ing *networkingv1.Ingress, svc *corev1.Service, nodeIP string) []string {
	urls := []string{}
	if ing != nil {
		scheme := "http"
		if len(ing.Spec.TLS) > 0 {
			scheme = "https"
		}
		for _, rule := range ing.Spec.Rules {
			if rule.Host != "" {
				urls = append(urls, fmt.Sprintf("%s://%s", scheme, rule.Host))
			}
		}
	}
	if svc == nil {
		return urls
	}
	switch svc.Spec.Type {
	case corev1.ServiceTypeNodePort:
		if nodeIP == "" {
			return urls
		}
		host := nodeIP
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
		for _, port := range svc.Spec.Ports {
			if port.NodePort != 0 {
				urls = append(urls, fmt.Sprintf("%s://%s:%d", schemeForPort(port), host, port.NodePort))
			}
		}
	case corev1.ServiceTypeLoadBalancer:
		for _, ingress := range svc.Status.LoadBalancer.Ingress {
			host := ingress.IP
			if host == "" {
				host = ingress.Hostname
			}
			if host == "" {
				continue
			}
			for _, port := range svc.Spec.Ports {
				urls = append(urls, fmt.Sprintf("%s://%s:%d", schemeForPort(port), host, port.Port))
			}
		}
	}
	return urls
}

func schemeForPort(port corev1.ServicePort) string {
	name := strings.ToLower(port.Name)
	if port.Port == 443 || strings.Contains(name, "https") {
		return "https"
	}
	return "http"
}

// clusterNodeIP stands in for "the cluster" when resolving a node port: an
// external address if the cluster has one, otherwise the internal one.
func clusterNodeIP(cfg *rest.Config) string {
	nodes, err := object.GetNodes(cfg)
	if err != nil {
		return ""
	}
	internal := ""
	for _, node := range nodes {
		for _, address := range node.Status.Addresses {
			if address.Type == corev1.NodeExternalIP && address.Address != "" {
				return address.Address
			}
			if address.Type == corev1.NodeInternalIP && internal == "" {
				internal = address.Address
			}
		}
	}
	return internal
}

func appPodsOf(cfg *rest.Config, namespace, app string) []appPodSummary {
	pods, err := object.GetPods(cfg, namespace)
	if err != nil {
		return []appPodSummary{}
	}
	result := []appPodSummary{}
	for _, pod := range pods {
		if pod.Labels[appInstanceLabel] != app || pod.Labels[appManagedByLabel] != appManagedByValue {
			continue
		}
		restarts := int32(0)
		ready := 0
		for _, status := range pod.Status.ContainerStatuses {
			restarts += status.RestartCount
			if status.Ready {
				ready++
			}
		}
		containers := make([]string, 0, len(pod.Spec.Containers))
		for _, container := range pod.Spec.Containers {
			containers = append(containers, container.Name)
		}
		result = append(result, appPodSummary{
			Name:       pod.Name,
			Namespace:  pod.Namespace,
			Phase:      string(pod.Status.Phase),
			NodeName:   pod.Spec.NodeName,
			Restarts:   restarts,
			Ready:      fmt.Sprintf("%d/%d", ready, len(pod.Spec.Containers)),
			Containers: containers,
			CreatedAt:  pod.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func appSummaryOf(d appsv1.Deployment, svc *corev1.Service) imageAppSummary {
	status, description := deploymentAppStatus(d)
	image := imageOf(d)
	repository, tag := store.SplitImageRef(image)
	replicas := int32(1)
	if d.Spec.Replicas != nil {
		replicas = *d.Spec.Replicas
	}
	summary := imageAppSummary{
		Name:          d.Name,
		Namespace:     d.Namespace,
		Image:         image,
		Repository:    repository,
		Tag:           tag,
		Status:        status,
		Description:   description,
		Replicas:      replicas,
		ReadyReplicas: d.Status.ReadyReplicas,
		Ports:         containerPortsOf(d),
		EnvVars:       extractEnvVars(d.Spec.Template.Spec.Containers),
		Volumes:       extractVolumes(d),
		CreatedAt:     d.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
		Components:    []imageAppComponent{},
	}
	if svc != nil {
		summary.ServiceType = string(svc.Spec.Type)
	}
	return summary
}

// deleteAppExtras removes the objects an app owns besides its workload, its
// Service and its volumes: the domain it answered on, its autoscaler, its
// config files and its registry credentials. Anything it could not delete is
// named back, because a leftover here breaks the app's next install.
func deleteAppExtras(cfg *rest.Config, namespace, app string) []string {
	failures := []string{}

	if ingresses, err := object.GetIngresses(cfg, namespace); err == nil {
		for _, ing := range ingresses {
			if !ownedByApp(ing.ObjectMeta, app) {
				continue
			}
			if err := object.DeleteIngress(cfg, ing.Namespace, ing.Name); err != nil && !errors.IsNotFound(err) {
				failures = append(failures, fmt.Sprintf("%s (%v)", ing.Name, err))
			}
		}
	}

	if hpas, err := object.GetHPAs(cfg, namespace); err == nil {
		for _, hpa := range hpas {
			if !ownedByApp(hpa.ObjectMeta, app) {
				continue
			}
			if err := object.DeleteHPA(cfg, hpa.Namespace, hpa.Name); err != nil && !errors.IsNotFound(err) {
				failures = append(failures, fmt.Sprintf("%s (%v)", hpa.Name, err))
			}
		}
	}

	if configMaps, err := object.GetConfigMaps(cfg, namespace); err == nil {
		for _, cm := range configMaps {
			if !ownedByApp(cm.ObjectMeta, app) {
				continue
			}
			if err := object.DeleteConfigMap(cfg, cm.Namespace, cm.Name); err != nil && !errors.IsNotFound(err) {
				failures = append(failures, fmt.Sprintf("%s (%v)", cm.Name, err))
			}
		}
	}

	if secrets, err := object.GetSecrets(cfg, namespace); err == nil {
		for _, secret := range secrets {
			if !ownedByApp(secret.ObjectMeta, app) {
				continue
			}
			if err := object.DeleteSecret(cfg, secret.Namespace, secret.Name); err != nil && !errors.IsNotFound(err) {
				failures = append(failures, fmt.Sprintf("%s (%v)", secret.Name, err))
			}
		}
	}

	return failures
}

// GetImageApp reads one app back as the form that produced it, plus what it is
// doing right now. One request rather than six, because the detail screen would
// otherwise assemble the same app out of five object lists.
// @router /api/get-image-app [get]
func (c *ApiController) GetImageApp() {
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
	if name == "" {
		c.ResponseError("name is required")
		return
	}

	deployment, err := object.GetDeployment(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if len(deployment.Spec.Template.Spec.Containers) == 0 {
		c.ResponseError(fmt.Sprintf("app %s/%s has no containers", namespace, name))
		return
	}
	container := deployment.Spec.Template.Spec.Containers[0]

	svc, err := object.GetService(cfg, namespace, name)
	if err != nil {
		if !errors.IsNotFound(err) {
			c.ResponseError(err.Error())
			return
		}
		svc = nil
	}
	ing, err := object.GetIngress(cfg, namespace, name)
	if err != nil {
		if !errors.IsNotFound(err) {
			c.ResponseError(err.Error())
			return
		}
		ing = nil
	}
	hpa, err := object.GetHPA(cfg, namespace, name)
	if err != nil {
		if !errors.IsNotFound(err) {
			c.ResponseError(err.Error())
			return
		}
		hpa = nil
	}
	cm, err := object.GetConfigMap(cfg, namespace, appConfigMapName(name))
	if err != nil {
		if !errors.IsNotFound(err) {
			c.ResponseError(err.Error())
			return
		}
		cm = nil
	}

	registryServer := ""
	if len(deployment.Spec.Template.Spec.ImagePullSecrets) > 0 {
		registryServer = registryServerOf(cfg, namespace, deployment.Spec.Template.Spec.ImagePullSecrets[0].Name)
	}

	detail := imageAppDetail{
		imageAppSummary: appSummaryOf(*deployment, svc),
		resourceSummary: extractResources(deployment.Spec.Template.Spec.Containers),
		Command:         container.Command,
		Args:            container.Args,
		ConfigFiles:     configFilesOf(cm, container),
		RegistryServer:  registryServer,
		Hpa:             hpaOf(hpa),
		Domains:         domainsOf(ing),
		Urls:            appUrls(ing, svc, clusterNodeIP(cfg)),
		Pods:            appPodsOf(cfg, namespace, name),
	}
	c.ResponseOk(detail)
}

// registryServerOf reads the address out of a pull secret so the form can show
// which registry an app signs in to without ever handing back the password.
func registryServerOf(cfg *rest.Config, namespace, secretName string) string {
	secret, err := object.GetSecret(cfg, namespace, secretName)
	if err != nil {
		return ""
	}
	raw, ok := secret.Data[corev1.DockerConfigJsonKey]
	if !ok {
		return ""
	}
	var parsed struct {
		Auths map[string]json.RawMessage `json:"auths"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ""
	}
	for server := range parsed.Auths {
		return server
	}
	return ""
}
