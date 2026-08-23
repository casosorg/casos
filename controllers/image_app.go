package controllers

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/casosorg/casos/object"
	"github.com/casosorg/casos/store"
)

// An app installed from a plain container image has no release object to look
// itself up in the way a Helm release does, so the cluster carries that record
// instead: every Deployment, Service and volume the install created is stamped
// with the app it belongs to. Listing, upgrading and uninstalling all read the
// same labels back.
//
// The instance label is deliberately the one Helm uses. It is what makes an
// image app's Service and volumes group under their app in the UI without a
// second code path (see helmReleaseOf).
const (
	appManagedByLabel     = "app.kubernetes.io/managed-by"
	appManagedByValue     = "casos"
	appInstanceLabel      = helmInstanceLabel
	appComponentLabel     = "app.kubernetes.io/component"
	appImageAnnotation    = "casos.io/image"
	appReplicasAnnotation = "casos.io/stopped-replicas"
)

// appOwnershipLabels marks a resource as belonging to one image app. component
// is empty for the app's own workload and names the part otherwise, so that a
// companion database is uninstalled along with the app that asked for it while
// still being distinguishable from it.
func appOwnershipLabels(instance, component string) map[string]string {
	labels := map[string]string{
		appManagedByLabel: appManagedByValue,
		appInstanceLabel:  instance,
	}
	if component != "" {
		labels[appComponentLabel] = component
	}
	return labels
}

func applyLabels(meta *metav1.ObjectMeta, labels map[string]string) {
	if len(labels) == 0 {
		return
	}
	if meta.Labels == nil {
		meta.Labels = map[string]string{}
	}
	for key, value := range labels {
		meta.Labels[key] = value
	}
}

func applyAnnotation(meta *metav1.ObjectMeta, key, value string) {
	if value == "" {
		return
	}
	if meta.Annotations == nil {
		meta.Annotations = map[string]string{}
	}
	meta.Annotations[key] = value
}

func ownedByApp(meta metav1.ObjectMeta, instance string) bool {
	return meta.Labels[appManagedByLabel] == appManagedByValue && meta.Labels[appInstanceLabel] == instance
}

type imageAppComponent struct {
	Name          string `json:"name"`
	Component     string `json:"component"`
	Image         string `json:"image"`
	Replicas      int32  `json:"replicas"`
	ReadyReplicas int32  `json:"readyReplicas"`
	Status        string `json:"status"`
}

type imageAppSummary struct {
	Name          string              `json:"name"`
	Namespace     string              `json:"namespace"`
	Image         string              `json:"image"`
	Repository    string              `json:"repository"`
	Tag           string              `json:"tag"`
	Status        string              `json:"status"`
	Description   string              `json:"description"`
	Replicas      int32               `json:"replicas"`
	ReadyReplicas int32               `json:"readyReplicas"`
	ServiceType   string              `json:"serviceType"`
	Ports         []appPortRequest    `json:"ports"`
	EnvVars       []envVarSummary     `json:"envVars"`
	Volumes       []volumeSummary     `json:"volumes"`
	CreatedAt     string              `json:"createdAt"`
	Components    []imageAppComponent `json:"components"`
}

// deploymentAppStatus phrases a Deployment's state in the words the app list
// already speaks for Helm releases, so one list can hold both kinds.
func deploymentAppStatus(d appsv1.Deployment) (string, string) {
	replicas := int32(1)
	if d.Spec.Replicas != nil {
		replicas = *d.Spec.Replicas
	}
	if replicas == 0 {
		return "stopped", ""
	}
	for _, cond := range d.Status.Conditions {
		if cond.Type == appsv1.DeploymentReplicaFailure && cond.Status == corev1.ConditionTrue {
			return "failed", cond.Message
		}
		if cond.Type == appsv1.DeploymentProgressing && cond.Status == corev1.ConditionFalse {
			return "failed", cond.Message
		}
	}
	if d.Status.ReadyReplicas >= replicas {
		return "deployed", ""
	}
	return "pending", ""
}

func containerPortsOf(d appsv1.Deployment) []appPortRequest {
	ports := []appPortRequest{}
	if len(d.Spec.Template.Spec.Containers) == 0 {
		return ports
	}
	for _, port := range d.Spec.Template.Spec.Containers[0].Ports {
		protocol := string(port.Protocol)
		if protocol == "" {
			protocol = string(corev1.ProtocolTCP)
		}
		ports = append(ports, appPortRequest{Name: port.Name, ContainerPort: port.ContainerPort, Protocol: protocol})
	}
	return ports
}

func imageOf(d appsv1.Deployment) string {
	if len(d.Spec.Template.Spec.Containers) == 0 {
		return ""
	}
	return d.Spec.Template.Spec.Containers[0].Image
}

// GetImageApps lists the apps installed from a container image, one entry per
// app rather than one per workload: a companion database is reported as a part
// of the app that owns it.
// @router /api/get-image-apps [get]
func (c *ApiController) GetImageApps() {
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
	deployments, err := object.GetDeployments(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	services, err := object.GetServices(cfg, namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	serviceByKey := map[string]corev1.Service{}
	for _, svc := range services {
		serviceByKey[svc.Namespace+"/"+svc.Name] = svc
	}

	// A part whose app is missing is skipped rather than listed as an app of
	// its own, which is what a half-finished install would otherwise look like.
	mains := []appsv1.Deployment{}
	parts := map[string][]appsv1.Deployment{}
	for _, d := range deployments {
		if d.Labels[appManagedByLabel] != appManagedByValue {
			continue
		}
		instance := d.Labels[appInstanceLabel]
		if instance == "" {
			continue
		}
		if d.Labels[appComponentLabel] == "" {
			mains = append(mains, d)
			continue
		}
		key := d.Namespace + "/" + instance
		parts[key] = append(parts[key], d)
	}

	result := make([]imageAppSummary, 0, len(mains))
	for _, d := range mains {
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
		if svc, ok := serviceByKey[d.Namespace+"/"+d.Name]; ok {
			summary.ServiceType = string(svc.Spec.Type)
		}
		for _, part := range parts[d.Namespace+"/"+d.Name] {
			partStatus, _ := deploymentAppStatus(part)
			partReplicas := int32(1)
			if part.Spec.Replicas != nil {
				partReplicas = *part.Spec.Replicas
			}
			summary.Components = append(summary.Components, imageAppComponent{
				Name:          part.Name,
				Component:     part.Labels[appComponentLabel],
				Image:         imageOf(part),
				Replicas:      partReplicas,
				ReadyReplicas: part.Status.ReadyReplicas,
				Status:        partStatus,
			})
		}
		sort.Slice(summary.Components, func(i, j int) bool {
			return summary.Components[i].Name < summary.Components[j].Name
		})
		result = append(result, summary)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		return result[i].Name < result[j].Name
	})
	c.ResponseOk(result)
}

func servicePortsFor(ports []appPortRequest) []portRequest {
	result := make([]portRequest, 0, len(ports))
	for _, p := range ports {
		protocol := p.Protocol
		if protocol == "" {
			protocol = "TCP"
		}
		result = append(result, portRequest{
			Name:       p.Name,
			Protocol:   protocol,
			Port:       p.ContainerPort,
			TargetPort: fmt.Sprintf("%d", p.ContainerPort),
		})
	}
	return result
}

// keepNodePorts carries the node ports an existing Service was assigned over to
// the ports replacing it. Left out, every upgrade would draw a fresh port from
// the range and move the address the app answers on.
func keepNodePorts(ports []portRequest, existing *corev1.Service) []portRequest {
	if existing == nil {
		return ports
	}
	assigned := map[int32]int32{}
	for _, port := range existing.Spec.Ports {
		if port.NodePort != 0 {
			assigned[port.Port] = port.NodePort
		}
	}
	for i := range ports {
		if nodePort, ok := assigned[ports[i].Port]; ok {
			ports[i].NodePort = nodePort
		}
	}
	return ports
}

// reconcileAppService brings the app's Service in line with the ports it now
// asks to expose: created when there was none, updated in place when there
// was, and deleted once the app exposes nothing.
func reconcileAppService(cfg *rest.Config, req deployAppRequest, labels map[string]string) error {
	existing, err := object.GetService(cfg, req.Namespace, req.Name)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if errors.IsNotFound(err) {
		existing = nil
	}

	if len(req.Ports) == 0 {
		if existing == nil {
			return nil
		}
		if err := object.DeleteService(cfg, req.Namespace, req.Name); err != nil && !errors.IsNotFound(err) {
			return err
		}
		return nil
	}

	svcType := req.ServiceType
	if svcType == "" {
		svcType = "NodePort"
	}
	ports := servicePortsFor(req.Ports)
	if svcType != string(corev1.ServiceTypeClusterIP) {
		ports = keepNodePorts(ports, existing)
	}
	svcReq := serviceRequest{
		Namespace: req.Namespace,
		Name:      req.Name,
		Type:      svcType,
		Selector:  map[string]string{"app": req.Name},
		Ports:     ports,
	}
	if existing == nil {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: svcReq.Name, Namespace: svcReq.Namespace},
			Spec:       buildServiceSpec(svcReq),
		}
		applyLabels(&svc.ObjectMeta, labels)
		_, err := object.AddService(cfg, svc)
		return err
	}

	// A cluster IP is assigned once and rejected on update if it changes, so
	// only the parts the form owns are replaced.
	spec := buildServiceSpec(svcReq)
	spec.ClusterIP = existing.Spec.ClusterIP
	spec.ClusterIPs = existing.Spec.ClusterIPs
	spec.IPFamilies = existing.Spec.IPFamilies
	spec.IPFamilyPolicy = existing.Spec.IPFamilyPolicy
	existing.Spec = spec
	applyLabels(&existing.ObjectMeta, labels)
	_, err = object.UpdateService(cfg, existing)
	return err
}

// UpgradeImageApp re-deploys an installed image app: a newer tag, a changed
// environment, a port it should now expose. The workload is updated in place so
// the volumes holding the app's data stay where they are.
// @router /api/upgrade-image-app [post]
func (c *ApiController) UpgradeImageApp() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req deployAppRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Name == "" {
		c.ResponseError("name is required")
		return
	}

	existing, err := object.GetDeployment(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if len(existing.Spec.Template.Spec.Containers) == 0 {
		c.ResponseError(fmt.Sprintf("app %s/%s has no containers", req.Namespace, req.Name))
		return
	}

	component := existing.Labels[appComponentLabel]
	labels := appOwnershipLabels(req.Name, component)
	if err := ensureDeploymentPVCs(cfg, req.Namespace, req.Name, req.Volumes, labels); err != nil {
		c.ResponseError(err.Error())
		return
	}

	applyLabels(&existing.ObjectMeta, labels)
	applyLabels(&existing.Spec.Template.ObjectMeta, labels)
	applyAnnotation(&existing.ObjectMeta, appImageAnnotation, req.Image)

	if req.Replicas != nil {
		replicas := replicasOrDefault(req.Replicas)
		existing.Spec.Replicas = &replicas
	}

	container := &existing.Spec.Template.Spec.Containers[0]
	if req.Image != "" {
		container.Image = req.Image
	}
	container.Env = buildEnvVars(req.EnvVars)
	container.Ports = containerPortsFor(req.Ports)
	podVolumes, mounts := buildPodVolumes(req.Name, req.Volumes)
	container.VolumeMounts = mounts
	existing.Spec.Template.Spec.Volumes = podVolumes

	updated, err := object.UpdateDeployment(cfg, existing)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if err := reconcileAppService(cfg, req, labels); err != nil {
		c.ResponseError(fmt.Sprintf("the app was updated but its address could not be: %s", err.Error()))
		return
	}
	c.ResponseOk(toDeploymentSummary(*updated))
}

type uninstallImageAppRequest struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	DeleteData bool   `json:"deleteData"`
}

// UninstallImageApp removes everything one install created: the app's workload,
// whatever was deployed alongside it, and the Service in front of them. Volumes
// are kept unless asked for, so reinstalling the app finds its data where it
// left it.
// @router /api/uninstall-image-app [post]
func (c *ApiController) UninstallImageApp() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req uninstallImageAppRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Name == "" {
		c.ResponseError("name is required")
		return
	}

	deployments, err := object.GetDeployments(cfg, req.Namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	removed := 0
	var failures []string
	for _, d := range deployments {
		if !ownedByApp(d.ObjectMeta, req.Name) {
			continue
		}
		if err := object.DeleteDeployment(cfg, d.Namespace, d.Name); err != nil && !errors.IsNotFound(err) {
			failures = append(failures, fmt.Sprintf("%s (%v)", d.Name, err))
			continue
		}
		removed++
	}
	if removed == 0 && len(failures) == 0 {
		c.ResponseError(fmt.Sprintf("no app named %s is installed in %s", req.Name, req.Namespace))
		return
	}

	services, err := object.GetServices(cfg, req.Namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	for _, svc := range services {
		if !ownedByApp(svc.ObjectMeta, req.Name) {
			continue
		}
		if err := object.DeleteService(cfg, svc.Namespace, svc.Name); err != nil && !errors.IsNotFound(err) {
			failures = append(failures, fmt.Sprintf("%s (%v)", svc.Name, err))
		}
	}

	if req.DeleteData {
		claims, err := object.GetPersistentVolumeClaims(cfg, req.Namespace)
		if err != nil {
			c.ResponseError(err.Error())
			return
		}
		for _, claim := range claims {
			if !ownedByApp(claim.ObjectMeta, req.Name) {
				continue
			}
			if err := object.DeletePersistentVolumeClaim(cfg, claim.Namespace, claim.Name); err != nil && !errors.IsNotFound(err) {
				failures = append(failures, fmt.Sprintf("%s (%v)", claim.Name, err))
			}
		}
	}

	if len(failures) > 0 {
		// Reported as a partial success: the app itself is gone by now, and
		// saying nothing would leave resources behind to break its next install.
		c.ResponseError(fmt.Sprintf("the app was uninstalled but these could not be deleted: %s", strings.Join(failures, ", ")))
		return
	}
	c.ResponseOk()
}

type scaleImageAppRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Running   bool   `json:"running"`
}

// scaleAppDeployment stops a workload by scaling it to zero and starts it again
// at the count it was stopped at. Kubernetes keeps no memory of that count, so
// it is written down on the way out; without it every restart would silently
// bring a scaled-out app back as a single replica.
func scaleAppDeployment(cfg *rest.Config, d *appsv1.Deployment, running bool) error {
	current := int32(1)
	if d.Spec.Replicas != nil {
		current = *d.Spec.Replicas
	}
	if running {
		if current > 0 {
			return nil
		}
		restored := int32(1)
		if saved, err := strconv.ParseInt(d.Annotations[appReplicasAnnotation], 10, 32); err == nil && saved > 0 {
			restored = int32(saved)
		}
		delete(d.Annotations, appReplicasAnnotation)
		d.Spec.Replicas = &restored
	} else {
		if current == 0 {
			return nil
		}
		applyAnnotation(&d.ObjectMeta, appReplicasAnnotation, strconv.FormatInt(int64(current), 10))
		zero := int32(0)
		d.Spec.Replicas = &zero
	}
	_, err := object.UpdateDeployment(cfg, d)
	return err
}

// ScaleImageApp stops or starts an installed app, its companion parts included:
// an app whose database kept running would be stopped in name only.
// @router /api/scale-image-app [post]
func (c *ApiController) ScaleImageApp() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req scaleImageAppRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Name == "" {
		c.ResponseError("name is required")
		return
	}

	deployments, err := object.GetDeployments(cfg, req.Namespace)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	scaled := 0
	var failures []string
	for i := range deployments {
		if !ownedByApp(deployments[i].ObjectMeta, req.Name) {
			continue
		}
		if err := scaleAppDeployment(cfg, &deployments[i], req.Running); err != nil {
			failures = append(failures, fmt.Sprintf("%s (%v)", deployments[i].Name, err))
			continue
		}
		scaled++
	}
	if scaled == 0 && len(failures) == 0 {
		c.ResponseError(fmt.Sprintf("no app named %s is installed in %s", req.Name, req.Namespace))
		return
	}
	if len(failures) > 0 {
		c.ResponseError(strings.Join(failures, ", "))
		return
	}
	c.ResponseOk()
}
