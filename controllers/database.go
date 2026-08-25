package controllers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/casosorg/casos/object"
)

// A database is a StatefulSet, a Service, a Secret holding its credentials and
// two claims: one for its data and one for its backups. Backups are written by
// the engine itself into the second claim, which the database pod also mounts —
// that is what lets an operator list, download and restore them through the
// pod-file endpoints that already exist, with no backup operator in the cluster.
//
// The alternative, a KubeBlocks-style operator, buys high availability that a
// single-node cluster cannot use anyway. Everything below is deliberately made
// of the primitives casos already manages, so a database is inspectable with the
// same pages as anything else running here.

const (
	databasePasswordAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	databasePasswordLength   = 24
	defaultDatabaseStorage   = "5Gi"
	minimumBackupStorage     = "1Gi"
)

func databaseSecretName(name string) string { return name + "-conn" }

func databaseBackupClaim(name string) string { return name + "-backups" }

func databaseLabels(name, engine, version string) map[string]string {
	return map[string]string{
		databaseManagedByLabel: databaseManagedByValue,
		databaseInstanceLabel:  name,
		databaseEngineLabel:    engine,
		databaseVersionLabel:   version,
		"app":                  name,
	}
}

func ownedDatabase(meta metav1.ObjectMeta) bool {
	return meta.Labels[databaseManagedByLabel] == databaseManagedByValue && meta.Labels[databaseEngineLabel] != ""
}

func generatePassword() (string, error) {
	limit := big.NewInt(int64(len(databasePasswordAlphabet)))
	builder := strings.Builder{}
	for i := 0; i < databasePasswordLength; i++ {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		builder.WriteByte(databasePasswordAlphabet[index.Int64()])
	}
	return builder.String(), nil
}

type databaseRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Engine    string `json:"engine"`
	Version   string `json:"version"`
	Replicas  *int32 `json:"replicas"`
	resourceRequest
	Storage string `json:"storage"`
	// Credentials. A blank password is generated, which is the normal path:
	// nobody should have to invent one to get a database.
	User         string `json:"user"`
	Password     string `json:"password"`
	Database     string `json:"database"`
	PublicAccess bool   `json:"publicAccess"`
	// Params holds engine settings by key. Absent means "whatever the engine
	// does on its own", which is also what a value equal to the default means.
	Params map[string]string `json:"params"`
}

type databaseSummary struct {
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	Engine        string `json:"engine"`
	EngineLabel   string `json:"engineLabel"`
	Version       string `json:"version"`
	Status        string `json:"status"`
	Description   string `json:"description"`
	Replicas      int32  `json:"replicas"`
	ReadyReplicas int32  `json:"readyReplicas"`
	resourceSummary
	Storage      string `json:"storage"`
	Port         int32  `json:"port"`
	PublicAccess bool   `json:"publicAccess"`
	NodePort     int32  `json:"nodePort"`
	CreatedAt    string `json:"createdAt"`
}

type databaseBackupFile struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
}

type databaseDetail struct {
	databaseSummary
	User         string               `json:"user"`
	Database     string               `json:"database"`
	Password     string               `json:"password"`
	InternalHost string               `json:"internalHost"`
	InternalURI  string               `json:"internalUri"`
	ExternalHost string               `json:"externalHost"`
	ExternalURI  string               `json:"externalUri"`
	PodName      string               `json:"podName"`
	Pods         []appPodSummary      `json:"pods"`
	Backups      []databaseBackupFile `json:"backups"`
	// BackupsError says why the backup list is empty when the reason is not
	// "there are none" — an engine that is stopped cannot be asked.
	BackupsError string `json:"backupsError"`
}

type databaseEngineOption struct {
	Key                  string   `json:"key"`
	Label                string   `json:"label"`
	Versions             []string `json:"versions"`
	Port                 int32    `json:"port"`
	DefaultUser          string   `json:"defaultUser"`
	FixedUser            bool     `json:"fixedUser"`
	SupportsDatabaseName bool     `json:"supportsDatabaseName"`
}

// GetDatabaseEngines lists what casos can run, so the form is described by the
// backend that has to honour it rather than by a copy kept in the browser.
// @router /api/get-database-engines [get]
func (c *ApiController) GetDatabaseEngines() {
	if c.RequireSignedIn() {
		return
	}
	options := make([]databaseEngineOption, 0, len(databaseEngineOrder))
	for _, key := range databaseEngineOrder {
		engine := databaseEngines[key]
		options = append(options, databaseEngineOption{
			Key:                  engine.Key,
			Label:                engine.Label,
			Versions:             engine.Versions,
			Port:                 engine.Port,
			DefaultUser:          engine.DefaultUser,
			FixedUser:            engine.FixedUser,
			SupportsDatabaseName: engine.SupportsDatabaseName,
		})
	}
	c.ResponseOk(options)
}

func databaseStatus(sts appsv1.StatefulSet) (string, string) {
	replicas := int32(1)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}
	if replicas == 0 {
		return "stopped", ""
	}
	if sts.Status.ReadyReplicas >= replicas {
		return "running", ""
	}
	return "pending", ""
}

func storageOf(sts appsv1.StatefulSet) string {
	for _, claim := range sts.Spec.VolumeClaimTemplates {
		if claim.Name != databaseDataVolume {
			continue
		}
		if quantity, ok := claim.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			return quantity.String()
		}
	}
	return ""
}

func versionOfImage(image string) string {
	if index := strings.LastIndex(image, ":"); index >= 0 {
		return image[index+1:]
	}
	return ""
}

func toDatabaseSummary(sts appsv1.StatefulSet, svc *corev1.Service) databaseSummary {
	engineKey := sts.Labels[databaseEngineLabel]
	engine := databaseEngines[engineKey]
	status, description := databaseStatus(sts)
	replicas := int32(1)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}
	summary := databaseSummary{
		Namespace:       sts.Namespace,
		Name:            sts.Name,
		Engine:          engineKey,
		EngineLabel:     engine.Label,
		Version:         versionOfImage(imageOfStatefulSet(sts)),
		Status:          status,
		Description:     description,
		Replicas:        replicas,
		ReadyReplicas:   sts.Status.ReadyReplicas,
		resourceSummary: extractResources(sts.Spec.Template.Spec.Containers),
		Storage:         storageOf(sts),
		Port:            engine.Port,
		CreatedAt:       sts.CreationTimestamp.UTC().Format("2006-01-02 15:04:05"),
	}
	if svc != nil {
		summary.PublicAccess = svc.Spec.Type == corev1.ServiceTypeNodePort
		for _, port := range svc.Spec.Ports {
			if port.NodePort != 0 {
				summary.NodePort = port.NodePort
			}
		}
	}
	return summary
}

func imageOfStatefulSet(sts appsv1.StatefulSet) string {
	if len(sts.Spec.Template.Spec.Containers) == 0 {
		return ""
	}
	return sts.Spec.Template.Spec.Containers[0].Image
}

func buildDatabaseStatefulSet(req databaseRequest, engine databaseEngine, version string, storage resource.Quantity) (*appsv1.StatefulSet, error) {
	labels := databaseLabels(req.Name, engine.Key, version)
	replicas := replicasOrDefault(req.Replicas)

	container := corev1.Container{
		Name:  databaseContainerName,
		Image: fmt.Sprintf("%s:%s", engine.Image, version),
		Env:   engine.Env(databaseSecretName(req.Name)),
		Ports: []corev1.ContainerPort{{Name: "db", ContainerPort: engine.Port, Protocol: corev1.ProtocolTCP}},
		VolumeMounts: []corev1.VolumeMount{
			{Name: databaseDataVolume, MountPath: engine.DataPath},
			{Name: databaseBackupVolume, MountPath: databaseBackupPath},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(engine.Port)},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
		},
	}
	if err := applyDatabaseParams(&container, engine, req.Params); err != nil {
		return nil, err
	}
	if err := applyResources(&container, req.resourceRequest); err != nil {
		return nil, err
	}

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace, Labels: labels},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: req.Name,
			Replicas:    &replicas,
			Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": req.Name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{container},
					Volumes: []corev1.Volume{{
						Name: databaseBackupVolume,
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: databaseBackupClaim(req.Name),
							},
						},
					}},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{Name: databaseDataVolume, Labels: labels},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{corev1.ResourceStorage: storage},
					},
				},
			}},
		},
	}, nil
}

func ensureBackupClaim(cfg *rest.Config, req databaseRequest, labels map[string]string, storage resource.Quantity) error {
	minimum := resource.MustParse(minimumBackupStorage)
	if storage.Cmp(minimum) < 0 {
		storage = minimum
	}
	claim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: databaseBackupClaim(req.Name), Namespace: req.Namespace, Labels: labels},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: storage},
			},
		},
	}
	if _, err := object.AddPersistentVolumeClaim(cfg, claim); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func databaseServiceType(publicAccess bool) string {
	if publicAccess {
		return string(corev1.ServiceTypeNodePort)
	}
	return string(corev1.ServiceTypeClusterIP)
}

func reconcileDatabaseService(cfg *rest.Config, req databaseRequest, engine databaseEngine, labels map[string]string) error {
	spec := buildServiceSpec(serviceRequest{
		Namespace: req.Namespace,
		Name:      req.Name,
		Type:      databaseServiceType(req.PublicAccess),
		Selector:  map[string]string{"app": req.Name},
		Ports: []portRequest{{
			Name:       "db",
			Protocol:   "TCP",
			Port:       engine.Port,
			TargetPort: fmt.Sprintf("%d", engine.Port),
		}},
	})

	existing, err := object.GetService(cfg, req.Namespace, req.Name)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if errors.IsNotFound(err) {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: req.Name, Namespace: req.Namespace, Labels: labels},
			Spec:       spec,
		}
		_, err := object.AddService(cfg, svc)
		return err
	}

	// The address an application was given must survive an edit, so the node
	// port already assigned is carried over.
	if req.PublicAccess {
		for _, port := range existing.Spec.Ports {
			if port.NodePort != 0 && len(spec.Ports) > 0 {
				spec.Ports[0].NodePort = port.NodePort
			}
		}
	}
	spec.ClusterIP = existing.Spec.ClusterIP
	spec.ClusterIPs = existing.Spec.ClusterIPs
	spec.IPFamilies = existing.Spec.IPFamilies
	spec.IPFamilyPolicy = existing.Spec.IPFamilyPolicy
	existing.Spec = spec
	applyLabels(&existing.ObjectMeta, labels)
	_, err = object.UpdateService(cfg, existing)
	return err
}

// CreateDatabase brings up one database: its credentials, its storage, the
// engine itself and the address applications reach it on.
// @router /api/create-database [post]
func (c *ApiController) CreateDatabase() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req databaseRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	summary, _, err := createDatabase(cfg, req)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(summary)
}

// databaseCredentials is what an application needs to reach the database that
// was just created for it.
type databaseCredentials struct {
	User     string
	Password string
	Database string
	Host     string
	Port     int32
}

// createDatabase brings up one database and hands back both what it looks like
// and how to connect to it. It is the whole of the create path: the HTTP
// handler above and the template market's KubeBlocks translation both go
// through here, so a database created either way is the same database.
func createDatabase(cfg *rest.Config, req databaseRequest) (databaseSummary, databaseCredentials, error) {
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Name == "" {
		return databaseSummary{}, databaseCredentials{}, fmt.Errorf("name is required")
	}
	engine, ok := engineByKey(req.Engine)
	if !ok {
		return databaseSummary{}, databaseCredentials{}, fmt.Errorf("unknown database engine %q", req.Engine)
	}
	version := engineVersionOrDefault(engine, req.Version)

	storageText := req.Storage
	if storageText == "" {
		storageText = defaultDatabaseStorage
	}
	storage, err := resource.ParseQuantity(storageText)
	if err != nil {
		return databaseSummary{}, databaseCredentials{}, fmt.Errorf("invalid storage size %q", storageText)
	}

	user := strings.TrimSpace(req.User)
	if user == "" || engine.FixedUser {
		user = engine.DefaultUser
	}
	database := strings.TrimSpace(req.Database)
	if database == "" {
		database = req.Name
	}
	password := req.Password
	if password == "" {
		password, err = generatePassword()
		if err != nil {
			return databaseSummary{}, databaseCredentials{}, err
		}
	}

	labels := databaseLabels(req.Name, engine.Key, version)
	credentials := databaseCredentials{
		User:     user,
		Password: password,
		Database: database,
		Host:     fmt.Sprintf("%s.%s.svc.cluster.local", req.Name, req.Namespace),
		Port:     engine.Port,
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: databaseSecretName(req.Name), Namespace: req.Namespace, Labels: labels},
		Type:       corev1.SecretTypeOpaque,
		StringData: map[string]string{"username": user, "password": password, "database": database},
	}
	if existing, err := object.GetSecret(cfg, req.Namespace, databaseSecretName(req.Name)); err == nil {
		// Recreating a database over one that is already there must not hand
		// out credentials the engine does not have.
		credentials.User = string(existing.Data["username"])
		credentials.Password = string(existing.Data["password"])
		credentials.Database = string(existing.Data["database"])
	} else if _, err := object.AddSecret(cfg, secret); err != nil && !errors.IsAlreadyExists(err) {
		return databaseSummary{}, databaseCredentials{}, err
	}

	if err := ensureBackupClaim(cfg, req, labels, storage); err != nil {
		return databaseSummary{}, databaseCredentials{}, err
	}

	sts, err := buildDatabaseStatefulSet(req, engine, version, storage)
	if err != nil {
		return databaseSummary{}, databaseCredentials{}, err
	}
	created, err := object.AddStatefulSet(cfg, sts)
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return databaseSummary{}, databaseCredentials{}, err
		}
		created, err = object.GetStatefulSet(cfg, req.Namespace, req.Name)
		if err != nil {
			return databaseSummary{}, databaseCredentials{}, err
		}
	}

	if err := reconcileDatabaseService(cfg, req, engine, labels); err != nil {
		return databaseSummary{}, databaseCredentials{}, fmt.Errorf("the database was created but its address could not be: %w", err)
	}

	svc, _ := object.GetService(cfg, req.Namespace, req.Name)
	return toDatabaseSummary(*created, svc), credentials, nil
}

// UpdateDatabase changes what an existing database may use, which version it
// runs and whether it is reachable from outside the cluster. Its credentials
// and its data are not this call's business.
// @router /api/update-database [post]
func (c *ApiController) UpdateDatabase() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req databaseRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	existing, err := object.GetStatefulSet(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !ownedDatabase(existing.ObjectMeta) {
		c.ResponseError(fmt.Sprintf("%s/%s is not a database managed by casos", req.Namespace, req.Name))
		return
	}
	engine, ok := engineByKey(existing.Labels[databaseEngineLabel])
	if !ok {
		c.ResponseError("unknown database engine " + existing.Labels[databaseEngineLabel])
		return
	}
	version := engineVersionOrDefault(engine, req.Version)

	if req.Replicas != nil {
		replicas := replicasOrDefault(req.Replicas)
		existing.Spec.Replicas = &replicas
	}
	container := &existing.Spec.Template.Spec.Containers[0]
	container.Image = fmt.Sprintf("%s:%s", engine.Image, version)
	if err := applyResources(container, req.resourceRequest); err != nil {
		c.ResponseError(err.Error())
		return
	}
	labels := databaseLabels(req.Name, engine.Key, version)
	applyLabels(&existing.ObjectMeta, labels)
	applyLabels(&existing.Spec.Template.ObjectMeta, labels)

	updated, err := object.UpdateStatefulSet(cfg, existing)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}

	// A claim can grow but never shrink, and the template it came from cannot
	// change at all, so growth is applied to the claims themselves.
	if req.Storage != "" {
		if err := growDatabaseStorage(cfg, req); err != nil {
			c.ResponseError("the database was updated but its disk could not grow: " + err.Error())
			return
		}
	}

	if err := reconcileDatabaseService(cfg, req, engine, labels); err != nil {
		c.ResponseError("the database was updated but its address could not be: " + err.Error())
		return
	}

	svc, _ := object.GetService(cfg, req.Namespace, req.Name)
	c.ResponseOk(toDatabaseSummary(*updated, svc))
}

func growDatabaseStorage(cfg *rest.Config, req databaseRequest) error {
	wanted, err := resource.ParseQuantity(req.Storage)
	if err != nil {
		return fmt.Errorf("invalid storage size %q", req.Storage)
	}
	claims, err := object.GetPersistentVolumeClaims(cfg, req.Namespace)
	if err != nil {
		return err
	}
	for _, claim := range claims {
		if claim.Labels[databaseInstanceLabel] != req.Name {
			continue
		}
		current := claim.Spec.Resources.Requests[corev1.ResourceStorage]
		if current.Cmp(wanted) >= 0 {
			continue
		}
		updated := claim
		updated.Spec.Resources.Requests[corev1.ResourceStorage] = wanted
		if _, err := object.UpdatePersistentVolumeClaim(cfg, &updated); err != nil {
			return err
		}
	}
	return nil
}

// GetDatabases lists the databases casos manages.
// @router /api/get-databases [get]
func (c *ApiController) GetDatabases() {
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
	sets, err := object.GetStatefulSets(cfg, namespace)
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

	result := []databaseSummary{}
	for _, sts := range sets {
		if !ownedDatabase(sts.ObjectMeta) {
			continue
		}
		var svc *corev1.Service
		if found, ok := serviceByKey[sts.Namespace+"/"+sts.Name]; ok {
			svc = &found
		}
		result = append(result, toDatabaseSummary(sts, svc))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Namespace != result[j].Namespace {
			return result[i].Namespace < result[j].Namespace
		}
		return result[i].Name < result[j].Name
	})
	c.ResponseOk(result)
}

func databasePodsOf(cfg *rest.Config, namespace, name string) []appPodSummary {
	pods, err := object.GetPods(cfg, namespace)
	if err != nil {
		return []appPodSummary{}
	}
	result := []appPodSummary{}
	for _, pod := range pods {
		if pod.Labels[databaseInstanceLabel] != name || pod.Labels[databaseManagedByLabel] != databaseManagedByValue {
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

// readyDatabasePod is the pod commands run in: backups, restores and the
// console all need one that is actually up.
func readyDatabasePod(pods []appPodSummary) string {
	for _, pod := range pods {
		if pod.Phase == string(corev1.PodRunning) {
			return pod.Name
		}
	}
	return ""
}

// execInPod runs one command in a container and collects what it wrote. Used
// for the short-lived work — listing, dumping, restoring — rather than for the
// console, which streams.
func execInPod(cfg *rest.Config, namespace, pod, container string, command []string) (string, error) {
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", err
	}
	var stdout, stderr bytes.Buffer
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").Name(pod).Namespace(namespace).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: container,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return "", err
	}
	if err := executor.StreamWithContext(context.Background(), remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr}); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s", message)
	}
	return stdout.String(), nil
}

func listDatabaseBackups(cfg *rest.Config, namespace, pod string) ([]databaseBackupFile, error) {
	output, err := execInPod(cfg, namespace, pod, databaseContainerName, []string{"ls", "-la", databaseBackupPath})
	if err != nil {
		return nil, err
	}
	files := []databaseBackupFile{}
	for _, entry := range parseLsOutput(output) {
		if entry.Type != "file" {
			continue
		}
		files = append(files, databaseBackupFile{Name: entry.Name, Size: entry.Size, ModTime: entry.ModTime})
	}
	// Newest first is what a restore dialog wants at the top.
	sort.Slice(files, func(i, j int) bool { return files[i].Name > files[j].Name })
	return files, nil
}

// GetDatabase reads one database back in full: how it is configured, how to
// connect to it, what is running and which backups it holds.
// @router /api/get-database [get]
func (c *ApiController) GetDatabase() {
	if c.RequireAdmin() {
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

	sts, err := object.GetStatefulSet(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !ownedDatabase(sts.ObjectMeta) {
		c.ResponseError(fmt.Sprintf("%s/%s is not a database managed by casos", namespace, name))
		return
	}
	engine, ok := engineByKey(sts.Labels[databaseEngineLabel])
	if !ok {
		c.ResponseError("unknown database engine " + sts.Labels[databaseEngineLabel])
		return
	}

	svc, err := object.GetService(cfg, namespace, name)
	if err != nil {
		if !errors.IsNotFound(err) {
			c.ResponseError(err.Error())
			return
		}
		svc = nil
	}

	user, password, database := engine.DefaultUser, "", name
	if secret, err := object.GetSecret(cfg, namespace, databaseSecretName(name)); err == nil {
		user = string(secret.Data["username"])
		password = string(secret.Data["password"])
		database = string(secret.Data["database"])
	}

	summary := toDatabaseSummary(*sts, svc)
	internalHost := fmt.Sprintf("%s.%s.svc.cluster.local", name, namespace)
	detail := databaseDetail{
		databaseSummary: summary,
		User:            user,
		Database:        database,
		Password:        password,
		InternalHost:    internalHost,
		InternalURI:     engine.URI(user, password, database, internalHost, engine.Port),
		Pods:            databasePodsOf(cfg, namespace, name),
		Backups:         []databaseBackupFile{},
	}

	if summary.PublicAccess && summary.NodePort != 0 {
		if nodeIP := clusterNodeIP(cfg); nodeIP != "" {
			detail.ExternalHost = fmt.Sprintf("%s:%d", nodeIP, summary.NodePort)
			detail.ExternalURI = engine.URI(user, password, database, nodeIP, summary.NodePort)
		}
	}

	detail.PodName = readyDatabasePod(detail.Pods)
	if detail.PodName == "" {
		detail.BackupsError = "the database is not running"
	} else if backups, err := listDatabaseBackups(cfg, namespace, detail.PodName); err != nil {
		detail.BackupsError = err.Error()
	} else {
		detail.Backups = backups
	}

	c.ResponseOk(detail)
}

type databaseActionRequest struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	Running    bool   `json:"running"`
	DeleteData bool   `json:"deleteData"`
	File       string `json:"file"`
}

// ScaleDatabase stops a database by scaling it to zero and starts it again.
// @router /api/scale-database [post]
func (c *ApiController) ScaleDatabase() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req databaseActionRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	sts, err := object.GetStatefulSet(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !ownedDatabase(sts.ObjectMeta) {
		c.ResponseError(fmt.Sprintf("%s/%s is not a database managed by casos", req.Namespace, req.Name))
		return
	}
	replicas := int32(0)
	if req.Running {
		replicas = 1
	}
	sts.Spec.Replicas = &replicas
	updated, err := object.UpdateStatefulSet(cfg, sts)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	svc, _ := object.GetService(cfg, req.Namespace, req.Name)
	c.ResponseOk(toDatabaseSummary(*updated, svc))
}

// deleteDatabaseObjects removes one database and reports what it could not
// remove. Deleting a template instance goes through here, so a database that
// came with an app is torn down exactly like one created by hand.
func deleteDatabaseObjects(cfg *rest.Config, namespace, name string, deleteData bool) []string {
	failures := []string{}
	if err := object.DeleteStatefulSet(cfg, namespace, name); err != nil && !errors.IsNotFound(err) {
		failures = append(failures, fmt.Sprintf("database %s (%v)", name, err))
	}
	if err := object.DeleteService(cfg, namespace, name); err != nil && !errors.IsNotFound(err) {
		failures = append(failures, fmt.Sprintf("service %s (%v)", name, err))
	}
	if deleteData {
		if err := object.DeleteSecret(cfg, namespace, databaseSecretName(name)); err != nil && !errors.IsNotFound(err) {
			failures = append(failures, fmt.Sprintf("secret %s (%v)", databaseSecretName(name), err))
		}
		claims, err := object.GetPersistentVolumeClaims(cfg, namespace)
		if err == nil {
			for _, claim := range claims {
				if claim.Labels[databaseInstanceLabel] != name {
					continue
				}
				if err := object.DeletePersistentVolumeClaim(cfg, claim.Namespace, claim.Name); err != nil && !errors.IsNotFound(err) {
					failures = append(failures, fmt.Sprintf("%s (%v)", claim.Name, err))
				}
			}
		}
	}
	return failures
}

// DeleteDatabase removes the engine and its address. The data and the backups
// are kept unless asked for, because a database deleted by mistake is otherwise
// unrecoverable.
// @router /api/delete-database [post]
func (c *ApiController) DeleteDatabase() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req databaseActionRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	sts, err := object.GetStatefulSet(cfg, req.Namespace, req.Name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	if !ownedDatabase(sts.ObjectMeta) {
		c.ResponseError(fmt.Sprintf("%s/%s is not a database managed by casos", req.Namespace, req.Name))
		return
	}

	failures := []string{}
	if err := object.DeleteStatefulSet(cfg, req.Namespace, req.Name); err != nil && !errors.IsNotFound(err) {
		c.ResponseError(err.Error())
		return
	}
	if err := object.DeleteService(cfg, req.Namespace, req.Name); err != nil && !errors.IsNotFound(err) {
		failures = append(failures, fmt.Sprintf("service %s (%v)", req.Name, err))
	}

	if req.DeleteData {
		if err := object.DeleteSecret(cfg, req.Namespace, databaseSecretName(req.Name)); err != nil && !errors.IsNotFound(err) {
			failures = append(failures, fmt.Sprintf("secret %s (%v)", databaseSecretName(req.Name), err))
		}
		claims, err := object.GetPersistentVolumeClaims(cfg, req.Namespace)
		if err == nil {
			for _, claim := range claims {
				if claim.Labels[databaseInstanceLabel] != req.Name {
					continue
				}
				if err := object.DeletePersistentVolumeClaim(cfg, claim.Namespace, claim.Name); err != nil && !errors.IsNotFound(err) {
					failures = append(failures, fmt.Sprintf("%s (%v)", claim.Name, err))
				}
			}
		}
	}

	if len(failures) > 0 {
		c.ResponseError("the database was deleted but these could not be: " + strings.Join(failures, ", "))
		return
	}
	c.ResponseOk()
}

func backupFileName(name string, engine databaseEngine) string {
	return fmt.Sprintf("%s-%s%s", name, time.Now().UTC().Format("20060102-150405"), engine.BackupSuffix)
}

// resolveDatabase reads back the pieces every command below needs: the engine,
// and a pod of it that is running.
func (c *ApiController) resolveDatabase(cfg *rest.Config, namespace, name string) (databaseEngine, string, bool) {
	sts, err := object.GetStatefulSet(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return databaseEngine{}, "", false
	}
	if !ownedDatabase(sts.ObjectMeta) {
		c.ResponseError(fmt.Sprintf("%s/%s is not a database managed by casos", namespace, name))
		return databaseEngine{}, "", false
	}
	engine, ok := engineByKey(sts.Labels[databaseEngineLabel])
	if !ok {
		c.ResponseError("unknown database engine " + sts.Labels[databaseEngineLabel])
		return databaseEngine{}, "", false
	}
	pod := readyDatabasePod(databasePodsOf(cfg, namespace, name))
	if pod == "" {
		c.ResponseError("the database is not running")
		return databaseEngine{}, "", false
	}
	return engine, pod, true
}

// BackupDatabase has the engine dump itself into the claim it shares with the
// backup list, and reports the file it wrote.
// @router /api/backup-database [post]
func (c *ApiController) BackupDatabase() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req databaseActionRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	engine, pod, ok := c.resolveDatabase(cfg, req.Namespace, req.Name)
	if !ok {
		return
	}

	file := backupFileName(req.Name, engine)
	path := databaseBackupPath + "/" + file
	if _, err := execInPod(cfg, req.Namespace, pod, databaseContainerName, []string{"sh", "-c", engine.Dump(path)}); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk(databaseBackupFile{Name: file})
}

// RestoreDatabase reads one of those files back into the engine.
// @router /api/restore-database [post]
func (c *ApiController) RestoreDatabase() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req databaseActionRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if strings.TrimSpace(req.File) == "" || strings.Contains(req.File, "/") {
		c.ResponseError("a backup file name is required")
		return
	}
	engine, pod, ok := c.resolveDatabase(cfg, req.Namespace, req.Name)
	if !ok {
		return
	}

	path := databaseBackupPath + "/" + req.File
	if _, err := execInPod(cfg, req.Namespace, pod, databaseContainerName, []string{"sh", "-c", engine.Restore(path)}); err != nil {
		c.ResponseError(err.Error())
		return
	}
	// Engines that only read their data at startup need the pod cycled; the
	// StatefulSet brings it straight back.
	if engine.RestoreNeedsRestart {
		if err := object.DeletePod(cfg, req.Namespace, pod); err != nil && !errors.IsNotFound(err) {
			c.ResponseError("the backup was restored but the database could not be restarted: " + err.Error())
			return
		}
	}
	c.ResponseOk()
}

// DeleteDatabaseBackup removes one dump from the backup claim.
// @router /api/delete-database-backup [post]
func (c *ApiController) DeleteDatabaseBackup() {
	if c.RequireAdmin() {
		return
	}
	cfg := getAdminRestConfig()
	if cfg == nil {
		c.ResponseError("apiserver not ready")
		return
	}
	var req databaseActionRequest
	if err := json.Unmarshal(c.Ctx.Input.RequestBody, &req); err != nil {
		c.ResponseError("invalid request body: " + err.Error())
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if strings.TrimSpace(req.File) == "" || strings.Contains(req.File, "/") {
		c.ResponseError("a backup file name is required")
		return
	}
	_, pod, ok := c.resolveDatabase(cfg, req.Namespace, req.Name)
	if !ok {
		return
	}
	if _, err := execInPod(cfg, req.Namespace, pod, databaseContainerName, []string{"rm", "-f", databaseBackupPath + "/" + req.File}); err != nil {
		c.ResponseError(err.Error())
		return
	}
	c.ResponseOk()
}

// DatabaseConsole attaches the engine's own client to a websocket, so an
// operator gets psql or redis-cli rather than a shell they then have to know
// the password for.
// @router /api/database-console [get]
func (c *ApiController) DatabaseConsole() {
	if c.RequireAdmin() {
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
	sts, err := object.GetStatefulSet(cfg, namespace, name)
	if err != nil {
		c.ResponseError(err.Error())
		return
	}
	engine, ok := engineByKey(sts.Labels[databaseEngineLabel])
	if !ok || !ownedDatabase(sts.ObjectMeta) {
		c.ResponseError(fmt.Sprintf("%s/%s is not a database managed by casos", namespace, name))
		return
	}
	pod := readyDatabasePod(databasePodsOf(cfg, namespace, name))
	if pod == "" {
		c.ResponseError("the database is not running")
		return
	}
	c.streamPodExec(cfg, namespace, pod, databaseContainerName, engine.Console())
}
