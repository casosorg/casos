package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/rest"

	"github.com/casosorg/casos/object"
)

type mcpToolAnnotations struct {
	ReadOnlyHint    bool `json:"readOnlyHint"`
	DestructiveHint bool `json:"destructiveHint"`
	IdempotentHint  bool `json:"idempotentHint"`
	OpenWorldHint   bool `json:"openWorldHint"`
}

type mcpTool struct {
	Name        string                 `json:"name"`
	Title       string                 `json:"title,omitempty"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	Annotations mcpToolAnnotations     `json:"annotations"`

	handler func(ctx context.Context, cfg *rest.Config, args json.RawMessage) (interface{}, error)
}

// This audited allowlist is the permission boundary; protocol hints alone grant nothing.
func mcpReadOnlyAllowed(tool *mcpTool) bool {
	if !tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint {
		return false
	}
	switch tool.Name {
	case "list_apps", "get_app", "get_app_logs", "list_app_revisions", "list_app_builds",
		"get_cluster_health", "list_cluster_issues", "diagnose_object":
		return true
	default:
		return false
	}
}

const (
	mcpDefaultWait = 90 * time.Second
	mcpMaxWait     = 600 * time.Second
	mcpMaxLogBytes = 64 * 1024
)

// Waiting reasons that do not clear up by waiting longer.
var mcpFatalWaitingReasons = map[string]bool{
	"ImagePullBackOff":           true,
	"InvalidImageName":           true,
	"ErrImageNeverPull":          true,
	"CrashLoopBackOff":           true,
	"CreateContainerConfigError": true,
	"CreateContainerError":       true,
	"RunContainerError":          true,
}

func mcpSchema(required []string, properties map[string]interface{}) map[string]interface{} {
	schema := map[string]interface{}{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func mcpProp(kind, description string) map[string]interface{} {
	return map[string]interface{}{"type": kind, "description": description}
}

var (
	mcpNameProp      = mcpProp("string", "App name: lowercase letters, digits and '-', at most 63 characters. It also names the app's Service.")
	mcpNamespaceProp = mcpProp("string", "Kubernetes namespace. Defaults to \"default\".")
	mcpWaitProp      = mcpProp("integer", "Seconds to wait for the rollout to finish before returning, at most 600. Defaults to 90; 0 returns at once.")
)

var mcpTools = []*mcpTool{
	{
		Name:        "list_apps",
		Title:       "List apps",
		Description: "List the apps deployed on casos with their image, status and replica counts.",
		InputSchema: mcpSchema(nil, map[string]interface{}{
			"namespace": mcpProp("string", "Only list apps in this namespace. Lists every namespace when omitted."),
		}),
		Annotations: mcpToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
		handler:     mcpListApps,
	},
	{
		Name:        "get_app",
		Title:       "Get app status",
		Description: "Show one app: its image, rollout status, the URLs it answers on, environment variables, pods with the problem each one has, and recent warning events.",
		InputSchema: mcpSchema([]string{"name"}, map[string]interface{}{
			"name":      mcpNameProp,
			"namespace": mcpNamespaceProp,
		}),
		Annotations: mcpToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
		handler:     mcpGetApp,
	},
	{
		Name:  "deploy_app",
		Title: "Deploy app",
		Description: "Deploy a container image to casos. Creates the app when it does not exist, otherwise updates it in place: only the fields given change, and env is merged into the existing variables. " +
			"Deploying the image the app already runs restarts it, which picks up a re-pushed tag. " +
			"The image must already be pushed to a registry the cluster can pull from. " +
			"Waits for the rollout and returns its outcome together with the app's URLs.",
		InputSchema: mcpSchema([]string{"name"}, map[string]interface{}{
			"name":         mcpNameProp,
			"namespace":    mcpNamespaceProp,
			"image":        mcpProp("string", "Image reference, e.g. ghcr.io/me/web:1.4.2. Required when creating the app."),
			"port":         mcpProp("integer", "Port the container listens on. The app is exposed on it through a Service."),
			"env":          map[string]interface{}{"type": "object", "additionalProperties": map[string]interface{}{"type": "string"}, "description": "Environment variables to set, as NAME: value."},
			"remove_env":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Names of environment variables to remove."},
			"replicas":     mcpProp("integer", "Number of replicas. Defaults to 1 for a new app; 0 stops the app."),
			"command":      map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Overrides the image's entrypoint."},
			"args":         map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "Overrides the image's command arguments."},
			"cpu":          mcpProp("string", "CPU limit, e.g. 500m or 2. An empty string removes the limit."),
			"memory":       mcpProp("string", "Memory limit, e.g. 512Mi or 2Gi. An empty string removes the limit."),
			"domain":       mcpProp("string", "Host name to publish the app on through the ingress, e.g. app.example.com. Needs port."),
			"https":        mcpProp("boolean", "Request a Let's Encrypt certificate for domain."),
			"wait_seconds": mcpWaitProp,
		}),
		Annotations: mcpToolAnnotations{DestructiveHint: true, IdempotentHint: true},
		handler:     mcpDeployApp,
	},
	{
		Name:        "get_app_logs",
		Title:       "Get app logs",
		Description: "Read the logs of every pod of an app, newest lines last. Use previous=true to read the crashed container of a pod in CrashLoopBackOff.",
		InputSchema: mcpSchema([]string{"name"}, map[string]interface{}{
			"name":          mcpNameProp,
			"namespace":     mcpNamespaceProp,
			"tail_lines":    mcpProp("integer", "Lines to read from the end of each container's log, at most 1000. Defaults to 100."),
			"since_minutes": mcpProp("integer", "Only read lines logged in the last this many minutes."),
			"previous":      mcpProp("boolean", "Read the previous, terminated instance of each container instead of the running one."),
			"keyword":       mcpProp("string", "Only return lines containing this text, case-insensitively."),
			"pod":           mcpProp("string", "Only read this pod."),
		}),
		Annotations: mcpToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
		handler:     mcpGetAppLogs,
	},
	{
		Name:        "list_app_revisions",
		Title:       "List app revisions",
		Description: "List the revisions an app can be rolled back to, newest first, with the image each one ran.",
		InputSchema: mcpSchema([]string{"name"}, map[string]interface{}{
			"name":      mcpNameProp,
			"namespace": mcpNamespaceProp,
		}),
		Annotations: mcpToolAnnotations{ReadOnlyHint: true, IdempotentHint: true},
		handler:     mcpListAppRevisions,
	},
	{
		Name:        "rollback_app",
		Title:       "Roll back app",
		Description: "Return an app to an earlier revision, by default the one before the current. The rollback is itself a new revision, so it can be undone the same way. Waits for the rollout.",
		InputSchema: mcpSchema([]string{"name"}, map[string]interface{}{
			"name":         mcpNameProp,
			"namespace":    mcpNamespaceProp,
			"revision":     mcpProp("integer", "Revision to return to, from list_app_revisions. Defaults to the previous revision."),
			"wait_seconds": mcpWaitProp,
		}),
		Annotations: mcpToolAnnotations{DestructiveHint: true},
		handler:     mcpRollbackApp,
	},
	{
		Name:        "restart_app",
		Title:       "Restart app",
		Description: "Replace every pod of an app with a fresh one, one at a time. Waits for the rollout.",
		InputSchema: mcpSchema([]string{"name"}, map[string]interface{}{
			"name":         mcpNameProp,
			"namespace":    mcpNamespaceProp,
			"wait_seconds": mcpWaitProp,
		}),
		Annotations: mcpToolAnnotations{DestructiveHint: true},
		handler:     mcpRestartApp,
	},
}

func findMCPTool(name string) *mcpTool {
	for _, tool := range mcpTools {
		if tool.Name == name {
			return tool
		}
	}
	return nil
}

// Unknown fields are refused so that a misspelt argument fails loudly instead
// of deploying something other than what the agent meant.
func decodeMCPArgs(raw json.RawMessage, target interface{}) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid arguments: %v", err)
	}
	return nil
}

type mcpAppRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (ref *mcpAppRef) normalize() error {
	ref.Name = strings.TrimSpace(ref.Name)
	ref.Namespace = strings.TrimSpace(ref.Namespace)
	if ref.Namespace == "" {
		ref.Namespace = "default"
	}
	if ref.Name == "" {
		return fmt.Errorf("name is required")
	}
	if problems := validation.IsDNS1123Label(ref.Name); len(problems) > 0 {
		return fmt.Errorf("name %q is invalid: %s", ref.Name, strings.Join(problems, "; "))
	}
	if problems := validation.IsDNS1123Label(ref.Namespace); len(problems) > 0 {
		return fmt.Errorf("namespace %q is invalid: %s", ref.Namespace, strings.Join(problems, "; "))
	}
	return nil
}

func mcpWaitDuration(seconds *int) time.Duration {
	if seconds == nil {
		return mcpDefaultWait
	}
	wait := time.Duration(*seconds) * time.Second
	if wait < 0 {
		return 0
	}
	if wait > mcpMaxWait {
		return mcpMaxWait
	}
	return wait
}

type mcpAppListItem struct {
	Name          string   `json:"name"`
	Namespace     string   `json:"namespace"`
	Image         string   `json:"image"`
	Status        string   `json:"status"`
	Replicas      int32    `json:"replicas"`
	ReadyReplicas int32    `json:"readyReplicas"`
	Components    []string `json:"components,omitempty"`
}

func mcpListApps(_ context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args struct {
		Namespace string `json:"namespace"`
	}
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	apps, err := listImageApps(cfg, strings.TrimSpace(args.Namespace))
	if err != nil {
		return nil, err
	}
	if len(apps) == 0 {
		return "No apps are deployed yet. Use deploy_app to deploy one.", nil
	}
	items := make([]mcpAppListItem, 0, len(apps))
	for _, app := range apps {
		item := mcpAppListItem{
			Name:          app.Name,
			Namespace:     app.Namespace,
			Image:         app.Image,
			Status:        app.Status,
			Replicas:      app.Replicas,
			ReadyReplicas: app.ReadyReplicas,
		}
		for _, component := range app.Components {
			item.Components = append(item.Components, fmt.Sprintf("%s (%s, %s)", component.Name, component.Component, component.Status))
		}
		items = append(items, item)
	}
	return items, nil
}

type mcpPodStatus struct {
	Name     string `json:"name"`
	Phase    string `json:"phase"`
	Ready    string `json:"ready"`
	Restarts int32  `json:"restarts"`
	Node     string `json:"node,omitempty"`
	Problem  string `json:"problem,omitempty"`
}

type mcpAppStatus struct {
	Name          string          `json:"name"`
	Namespace     string          `json:"namespace"`
	Image         string          `json:"image"`
	Status        string          `json:"status"`
	Message       string          `json:"message,omitempty"`
	Revision      string          `json:"revision,omitempty"`
	Replicas      int32           `json:"replicas"`
	ReadyReplicas int32           `json:"readyReplicas"`
	Ports         []int32         `json:"ports,omitempty"`
	Urls          []string        `json:"urls"`
	Env           []envVarSummary `json:"env,omitempty"`
	Pods          []mcpPodStatus  `json:"pods"`
	Warnings      []string        `json:"recentWarnings,omitempty"`
}

func mcpGetApp(_ context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args mcpAppRef
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	if err := args.normalize(); err != nil {
		return nil, err
	}
	return mcpAppStatusOf(cfg, args.Namespace, args.Name)
}

func mcpAppStatusOf(cfg *rest.Config, namespace, name string) (*mcpAppStatus, error) {
	deployment, err := object.GetDeployment(cfg, namespace, name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("no app named %s in namespace %s", name, namespace)
		}
		return nil, err
	}
	detail, err := getImageAppDetail(cfg, namespace, name)
	if err != nil {
		return nil, err
	}

	status := &mcpAppStatus{
		Name:          detail.Name,
		Namespace:     detail.Namespace,
		Image:         detail.Image,
		Status:        detail.Status,
		Message:       detail.Description,
		Revision:      deployment.Annotations["deployment.kubernetes.io/revision"],
		Replicas:      detail.Replicas,
		ReadyReplicas: detail.ReadyReplicas,
		Urls:          detail.Urls,
		Env:           detail.EnvVars,
		Pods:          []mcpPodStatus{},
	}
	for _, port := range detail.Ports {
		status.Ports = append(status.Ports, port.ContainerPort)
	}

	pods, err := mcpDeploymentPods(cfg, deployment)
	if err != nil {
		return nil, err
	}
	for _, pod := range pods {
		status.Pods = append(status.Pods, mcpPodStatusOf(pod))
	}
	status.Warnings = mcpRecentWarnings(cfg, namespace, name)
	return status, nil
}

func mcpDeploymentPods(cfg *rest.Config, deployment *appsv1.Deployment) ([]corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(deployment.Spec.Selector)
	if err != nil {
		return nil, err
	}
	pods, err := object.GetPods(cfg, deployment.Namespace)
	if err != nil {
		return nil, err
	}
	result := []corev1.Pod{}
	for _, pod := range pods {
		if selector.Matches(labels.Set(pod.Labels)) {
			result = append(result, pod)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func mcpPodStatusOf(pod corev1.Pod) mcpPodStatus {
	ready := 0
	restarts := int32(0)
	for _, container := range pod.Status.ContainerStatuses {
		restarts += container.RestartCount
		if container.Ready {
			ready++
		}
	}
	problem, _ := mcpPodProblem(pod)
	return mcpPodStatus{
		Name:     pod.Name,
		Phase:    string(pod.Status.Phase),
		Ready:    fmt.Sprintf("%d/%d", ready, len(pod.Spec.Containers)),
		Restarts: restarts,
		Node:     pod.Spec.NodeName,
		Problem:  problem,
	}
}

// mcpPodProblem names what is holding a pod back, and whether waiting longer
// could fix it.
func mcpPodProblem(pod corev1.Pod) (string, bool) {
	statuses := make([]corev1.ContainerStatus, 0, len(pod.Status.InitContainerStatuses)+len(pod.Status.ContainerStatuses))
	statuses = append(statuses, pod.Status.InitContainerStatuses...)
	statuses = append(statuses, pod.Status.ContainerStatuses...)
	for _, status := range statuses {
		if waiting := status.State.Waiting; waiting != nil && waiting.Reason != "" &&
			waiting.Reason != "ContainerCreating" && waiting.Reason != "PodInitializing" {
			problem := fmt.Sprintf("%s: %s", status.Name, waiting.Reason)
			if waiting.Message != "" {
				problem += ": " + waiting.Message
			}
			if last := status.LastTerminationState.Terminated; last != nil {
				problem += fmt.Sprintf(" (last exit code %d, %s)", last.ExitCode, last.Reason)
			}
			return problem, mcpFatalWaitingReasons[waiting.Reason]
		}
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodScheduled && condition.Status == corev1.ConditionFalse {
			return "not scheduled: " + condition.Message, false
		}
	}
	return "", false
}

func mcpRecentWarnings(cfg *rest.Config, namespace, name string) []string {
	events, err := object.GetEvents(cfg, namespace, string(corev1.EventTypeWarning), 0)
	if err != nil {
		return nil
	}
	warnings := []string{}
	for _, event := range events {
		involved := event.InvolvedObject.Name
		if involved != name && !strings.HasPrefix(involved, name+"-") {
			continue
		}
		when := object.EventTimestamp(event).UTC().Format(time.RFC3339)
		warnings = append(warnings, fmt.Sprintf("%s %s %s/%s: %s", when, event.Reason, strings.ToLower(event.InvolvedObject.Kind), involved, event.Message))
		if len(warnings) == 8 {
			break
		}
	}
	return warnings
}

type mcpRollout struct {
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
}

const (
	mcpRolloutLive       = "live"
	mcpRolloutFailed     = "failed"
	mcpRolloutInProgress = "in_progress"
	mcpRolloutStopped    = "stopped"
)

func waitForMCPRollout(ctx context.Context, cfg *rest.Config, namespace, name string, wait time.Duration) mcpRollout {
	deadline := time.Now().Add(wait)
	for {
		rollout := checkMCPRollout(cfg, namespace, name)
		if rollout.State != mcpRolloutInProgress || !time.Now().Before(deadline) {
			return rollout
		}
		select {
		case <-ctx.Done():
			return rollout
		case <-time.After(2 * time.Second):
		}
	}
}

func checkMCPRollout(cfg *rest.Config, namespace, name string) mcpRollout {
	deployment, revisions, err := object.DeploymentRevisions(cfg, namespace, name)
	if err != nil {
		return mcpRollout{State: mcpRolloutFailed, Reason: err.Error()}
	}
	replicas := int32(1)
	if deployment.Spec.Replicas != nil {
		replicas = *deployment.Spec.Replicas
	}
	if replicas == 0 {
		return mcpRollout{State: mcpRolloutStopped, Reason: "the app is scaled to 0 replicas"}
	}
	// Until the controller has seen the change, the status still describes
	// the previous rollout, a failed one included.
	if deployment.Status.ObservedGeneration < deployment.Generation {
		return mcpRollout{State: mcpRolloutInProgress, Reason: "waiting for the rollout to start"}
	}
	if status, message := deploymentAppStatus(*deployment); status == "failed" {
		return mcpRollout{State: mcpRolloutFailed, Reason: message}
	}
	st := deployment.Status
	if st.UpdatedReplicas >= replicas && st.Replicas == st.UpdatedReplicas && st.AvailableReplicas >= replicas {
		return mcpRollout{State: mcpRolloutLive}
	}

	currentHash := ""
	for _, rs := range revisions {
		if object.IsCurrentRevision(deployment, rs) {
			currentHash = rs.Labels["pod-template-hash"]
			break
		}
	}
	reason := fmt.Sprintf("%d of %d replicas updated, %d available", st.UpdatedReplicas, replicas, st.AvailableReplicas)
	pods, err := mcpDeploymentPods(cfg, deployment)
	if err != nil {
		return mcpRollout{State: mcpRolloutInProgress, Reason: reason}
	}
	for _, pod := range pods {
		// Pods of the revision being replaced may be failing for reasons the
		// new one has already fixed.
		if currentHash != "" && pod.Labels["pod-template-hash"] != currentHash {
			continue
		}
		problem, fatal := mcpPodProblem(pod)
		if fatal {
			return mcpRollout{State: mcpRolloutFailed, Reason: fmt.Sprintf("pod %s: %s", pod.Name, problem)}
		}
		if problem != "" {
			reason = fmt.Sprintf("%s; pod %s: %s", reason, pod.Name, problem)
		}
	}
	return mcpRollout{State: mcpRolloutInProgress, Reason: reason}
}

func mcpRolloutHint(rollout mcpRollout) string {
	switch rollout.State {
	case mcpRolloutFailed:
		return "The rollout failed. Read get_app_logs (previous=true for a crash loop) and get_app to find out why, then redeploy a fix or call rollback_app."
	case mcpRolloutInProgress:
		return "The rollout has not finished yet. Call get_app to follow it."
	}
	return ""
}

type mcpDeployArgs struct {
	mcpAppRef
	Image       string            `json:"image"`
	Port        *int32            `json:"port"`
	Env         map[string]string `json:"env"`
	RemoveEnv   []string          `json:"remove_env"`
	Replicas    *int32            `json:"replicas"`
	Command     []string          `json:"command"`
	Args        []string          `json:"args"`
	CPU         *string           `json:"cpu"`
	Memory      *string           `json:"memory"`
	Domain      string            `json:"domain"`
	HTTPS       bool              `json:"https"`
	WaitSeconds *int              `json:"wait_seconds"`
}

type mcpDeployResult struct {
	Action  string        `json:"action"`
	Rollout mcpRollout    `json:"rollout"`
	App     *mcpAppStatus `json:"app,omitempty"`
	Notes   []string      `json:"notes,omitempty"`
	Next    string        `json:"next,omitempty"`
}

func mcpDeployApp(ctx context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args mcpDeployArgs
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	if err := args.normalize(); err != nil {
		return nil, err
	}
	args.Image = strings.TrimSpace(args.Image)
	args.Domain = strings.TrimSpace(args.Domain)
	if args.Port != nil && (*args.Port < 1 || *args.Port > 65535) {
		return nil, fmt.Errorf("port must be between 1 and 65535")
	}
	if args.Replicas != nil && *args.Replicas < 0 {
		return nil, fmt.Errorf("replicas must not be negative")
	}
	for name := range args.Env {
		if problems := validation.IsEnvVarName(name); len(problems) > 0 {
			return nil, fmt.Errorf("environment variable name %q is invalid: %s", name, strings.Join(problems, "; "))
		}
	}

	existing, err := object.GetDeployment(cfg, args.Namespace, args.Name)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}

	result := mcpDeployResult{}
	if errors.IsNotFound(err) {
		if err := mcpCreateApp(cfg, args); err != nil {
			return nil, err
		}
		result.Action = "created"
	} else {
		notes, err := mcpUpdateApp(cfg, args, existing)
		if err != nil {
			return nil, err
		}
		result.Action = "updated"
		result.Notes = notes
	}

	result.Rollout = waitForMCPRollout(ctx, cfg, args.Namespace, args.Name, mcpWaitDuration(args.WaitSeconds))
	result.Next = mcpRolloutHint(result.Rollout)
	if status, err := mcpAppStatusOf(cfg, args.Namespace, args.Name); err == nil {
		result.App = status
	}
	return result, nil
}

func mcpEnvRequests(env map[string]string) []envVarRequest {
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]envVarRequest, 0, len(names))
	for _, name := range names {
		result = append(result, envVarRequest{Name: name, Value: env[name]})
	}
	return result
}

func mcpEnsureNamespace(cfg *rest.Config, namespace string) error {
	_, err := object.GetNamespace(cfg, namespace)
	if err == nil || !errors.IsNotFound(err) {
		return err
	}
	_, err = object.AddNamespace(cfg, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func mcpCreateApp(cfg *rest.Config, args mcpDeployArgs) error {
	if args.Image == "" {
		return fmt.Errorf("image is required to create app %s", args.Name)
	}
	if args.Domain != "" && args.Port == nil {
		return fmt.Errorf("domain needs port: the port the domain should reach")
	}
	if err := mcpEnsureNamespace(cfg, args.Namespace); err != nil {
		return err
	}

	req := deployAppRequest{
		Namespace: args.Namespace,
		Name:      args.Name,
		Image:     args.Image,
		Replicas:  args.Replicas,
		EnvVars:   mcpEnvRequests(args.Env),
		Command:   args.Command,
		Args:      args.Args,
	}
	req.CpuLimit = args.CPU
	req.MemoryLimit = args.Memory
	if args.Port != nil {
		req.Ports = []appPortRequest{{Name: "http", ContainerPort: *args.Port, Protocol: "TCP"}}
	}
	if args.Domain != "" {
		req.Domains = &[]appDomain{{Host: args.Domain, Port: *args.Port, Https: args.HTTPS}}
	}
	_, err := deployAppWorkload(cfg, req, workloadOptions{})
	return err
}

// mcpUpdateApp changes only what the agent asked for. The launchpad's upgrade
// replaces the whole form, which from a partial request would wipe the app's
// volumes, ports and environment.
func mcpUpdateApp(cfg *rest.Config, args mcpDeployArgs, existing *appsv1.Deployment) ([]string, error) {
	if !ownedByApp(existing.ObjectMeta, args.Name) || existing.Labels[appComponentLabel] != "" {
		return nil, fmt.Errorf("deployment %s/%s was not created as a casos app, so deploy_app will not overwrite it; choose another name", args.Namespace, args.Name)
	}
	if len(existing.Spec.Template.Spec.Containers) == 0 {
		return nil, fmt.Errorf("app %s/%s has no containers", args.Namespace, args.Name)
	}
	notes := []string{}
	container := &existing.Spec.Template.Spec.Containers[0]

	restart := false
	if args.Image != "" {
		if args.Image == container.Image {
			restart = true
		} else {
			container.Image = args.Image
			applyAnnotation(&existing.ObjectMeta, appImageAnnotation, args.Image)
		}
	}

	for _, env := range mcpEnvRequests(args.Env) {
		found := false
		for i := range container.Env {
			if container.Env[i].Name == env.Name {
				container.Env[i].Value = env.Value
				container.Env[i].ValueFrom = nil
				found = true
				break
			}
		}
		if !found {
			container.Env = append(container.Env, corev1.EnvVar{Name: env.Name, Value: env.Value})
		}
	}
	if len(args.RemoveEnv) > 0 {
		remove := map[string]bool{}
		for _, name := range args.RemoveEnv {
			remove[strings.TrimSpace(name)] = true
		}
		kept := container.Env[:0]
		for _, env := range container.Env {
			if !remove[env.Name] {
				kept = append(kept, env)
			}
		}
		container.Env = kept
	}

	if args.Command != nil {
		container.Command = trimmedList(args.Command)
	}
	if args.Args != nil {
		container.Args = trimmedList(args.Args)
	}
	if err := applyResources(container, resourceRequest{CpuLimit: args.CPU, MemoryLimit: args.Memory}); err != nil {
		return nil, err
	}
	if args.Port != nil {
		container.Ports = containerPortsFor([]appPortRequest{{Name: "http", ContainerPort: *args.Port, Protocol: "TCP"}})
	}

	if args.Replicas != nil {
		_, err := object.GetHPA(cfg, args.Namespace, args.Name)
		switch {
		case err == nil:
			notes = append(notes, "replicas was ignored: an autoscaler sets this app's replica count")
		case errors.IsNotFound(err):
			replicas := *args.Replicas
			existing.Spec.Replicas = &replicas
		default:
			return nil, err
		}
	}

	if restart {
		if existing.Spec.Template.Annotations == nil {
			existing.Spec.Template.Annotations = map[string]string{}
		}
		existing.Spec.Template.Annotations["casos.io/restartedAt"] = time.Now().UTC().Format(time.RFC3339)
		notes = append(notes, "the image is unchanged, so the app was restarted to pull it again")
	}

	if _, err := object.UpdateDeployment(cfg, existing); err != nil {
		return nil, err
	}

	ownership := appOwnershipLabels(args.Name, "")
	if args.Port != nil {
		serviceType := ""
		if svc, err := object.GetService(cfg, args.Namespace, args.Name); err == nil {
			serviceType = string(svc.Spec.Type)
		}
		req := deployAppRequest{
			Namespace:   args.Namespace,
			Name:        args.Name,
			Ports:       []appPortRequest{{Name: "http", ContainerPort: *args.Port, Protocol: "TCP"}},
			ServiceType: serviceType,
		}
		if err := reconcileAppService(cfg, req, ownership); err != nil {
			return notes, fmt.Errorf("the app was updated but its Service could not be: %w", err)
		}
	}

	if args.Domain != "" {
		if err := mcpPublishDomain(cfg, args, container); err != nil {
			return notes, fmt.Errorf("the app was updated but its domain could not be: %w", err)
		}
	}
	return notes, nil
}

// mcpPublishDomain adds a host to the app's ingress, keeping the hosts it
// already answers on.
func mcpPublishDomain(cfg *rest.Config, args mcpDeployArgs, container *corev1.Container) error {
	port := int32(0)
	if args.Port != nil {
		port = *args.Port
	} else if len(container.Ports) > 0 {
		port = container.Ports[0].ContainerPort
	}
	if port == 0 {
		return fmt.Errorf("the app exposes no port; pass port as well")
	}

	domains := []appDomain{}
	ing, err := object.GetIngress(cfg, args.Namespace, args.Name)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}
	if err == nil {
		domains = domainsOf(ing)
	}
	replaced := false
	for i := range domains {
		if strings.EqualFold(domains[i].Host, args.Domain) {
			domains[i].Port = port
			domains[i].Https = domains[i].Https || args.HTTPS
			replaced = true
		}
	}
	if !replaced {
		domains = append(domains, appDomain{Host: args.Domain, Port: port, Https: args.HTTPS})
	}

	req := deployAppRequest{Namespace: args.Namespace, Name: args.Name, Domains: &domains}
	if err := reconcileAppIngress(cfg, req, appOwnershipLabels(args.Name, "")); err != nil {
		return err
	}
	requestAppCertificate(cfg, req)
	return nil
}

func mcpGetAppLogs(_ context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args struct {
		mcpAppRef
		TailLines    *int64 `json:"tail_lines"`
		SinceMinutes *int64 `json:"since_minutes"`
		Previous     bool   `json:"previous"`
		Keyword      string `json:"keyword"`
		Pod          string `json:"pod"`
	}
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	if err := args.normalize(); err != nil {
		return nil, err
	}

	tail := int64(100)
	if args.TailLines != nil && *args.TailLines > 0 {
		tail = *args.TailLines
	}
	if tail > 1000 {
		tail = 1000
	}

	deployment, err := object.GetDeployment(cfg, args.Namespace, args.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("no app named %s in namespace %s", args.Name, args.Namespace)
		}
		return nil, err
	}
	pods, err := mcpDeploymentPods(cfg, deployment)
	if err != nil {
		return nil, err
	}

	keyword := strings.ToLower(strings.TrimSpace(args.Keyword))
	var out strings.Builder
	read := 0
	for _, pod := range pods {
		if args.Pod != "" && pod.Name != args.Pod {
			continue
		}
		containers := []string{}
		// An init container is only worth reading while it is what holds the
		// pod back.
		for _, status := range pod.Status.InitContainerStatuses {
			if status.State.Terminated == nil || status.State.Terminated.ExitCode != 0 {
				containers = append(containers, status.Name)
			}
		}
		for _, container := range pod.Spec.Containers {
			containers = append(containers, container.Name)
		}

		for _, container := range containers {
			read++
			opts := corev1.PodLogOptions{Container: container, TailLines: &tail, Previous: args.Previous, Timestamps: true}
			if args.SinceMinutes != nil && *args.SinceMinutes > 0 {
				since := *args.SinceMinutes * 60
				opts.SinceSeconds = &since
			}
			fmt.Fprintf(&out, "==> %s/%s <==\n", pod.Name, container)
			text, err := object.GetPodLogsWithOptions(cfg, args.Namespace, pod.Name, opts)
			if err != nil {
				fmt.Fprintf(&out, "(logs unavailable: %v)\n\n", err)
				continue
			}
			lines := 0
			for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
				if line == "" || (keyword != "" && !strings.Contains(strings.ToLower(line), keyword)) {
					continue
				}
				out.WriteString(line)
				out.WriteByte('\n')
				lines++
			}
			if lines == 0 && keyword != "" {
				out.WriteString("(no matching lines)\n")
			} else if lines == 0 {
				out.WriteString("(no output)\n")
			}
			out.WriteByte('\n')
		}
	}

	if read == 0 {
		if args.Pod != "" {
			return nil, fmt.Errorf("app %s has no pod named %s", args.Name, args.Pod)
		}
		return fmt.Sprintf("App %s has no pods right now; get_app shows why.", args.Name), nil
	}
	text := out.String()
	if len(text) > mcpMaxLogBytes {
		text = "(earlier output truncated)\n" + text[len(text)-mcpMaxLogBytes:]
	}
	return text, nil
}

type mcpRevision struct {
	Revision  int64  `json:"revision"`
	Image     string `json:"image"`
	Current   bool   `json:"current"`
	CreatedAt string `json:"createdAt"`
	Change    string `json:"change,omitempty"`
}

func mcpListAppRevisions(_ context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args mcpAppRef
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	if err := args.normalize(); err != nil {
		return nil, err
	}
	deployment, revisions, err := object.DeploymentRevisions(cfg, args.Namespace, args.Name)
	if err != nil {
		return nil, err
	}
	result := make([]mcpRevision, 0, len(revisions))
	for _, rs := range revisions {
		image := ""
		if len(rs.Spec.Template.Spec.Containers) > 0 {
			image = rs.Spec.Template.Spec.Containers[0].Image
		}
		result = append(result, mcpRevision{
			Revision:  object.RevisionNumber(rs),
			Image:     image,
			Current:   object.IsCurrentRevision(deployment, rs),
			CreatedAt: rs.CreationTimestamp.UTC().Format(time.RFC3339),
			Change:    rs.Annotations["kubernetes.io/change-cause"],
		})
	}
	return result, nil
}

type mcpRollbackResult struct {
	RolledBackTo int64         `json:"rolledBackTo"`
	Image        string        `json:"image"`
	Rollout      mcpRollout    `json:"rollout"`
	App          *mcpAppStatus `json:"app,omitempty"`
	Next         string        `json:"next,omitempty"`
}

func mcpRollbackApp(ctx context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args struct {
		mcpAppRef
		Revision    *int64 `json:"revision"`
		WaitSeconds *int   `json:"wait_seconds"`
	}
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	if err := args.normalize(); err != nil {
		return nil, err
	}

	deployment, revisions, err := object.DeploymentRevisions(cfg, args.Namespace, args.Name)
	if err != nil {
		return nil, err
	}
	var target *appsv1.ReplicaSet
	for i := range revisions {
		if args.Revision != nil {
			if object.RevisionNumber(revisions[i]) == *args.Revision {
				target = &revisions[i]
				break
			}
			continue
		}
		// Newest first, so the first one not running is the previous revision.
		if !object.IsCurrentRevision(deployment, revisions[i]) {
			target = &revisions[i]
			break
		}
	}
	if target == nil {
		if args.Revision != nil {
			return nil, fmt.Errorf("revision %d is not kept for app %s; list_app_revisions shows the ones that are", *args.Revision, args.Name)
		}
		return nil, fmt.Errorf("app %s has no earlier revision to roll back to", args.Name)
	}
	if object.IsCurrentRevision(deployment, *target) {
		return nil, fmt.Errorf("app %s is already running revision %d", args.Name, object.RevisionNumber(*target))
	}

	revision := object.RevisionNumber(*target)
	if err := object.RollbackDeployment(cfg, args.Namespace, args.Name, revision); err != nil {
		return nil, err
	}

	result := mcpRollbackResult{RolledBackTo: revision}
	if len(target.Spec.Template.Spec.Containers) > 0 {
		result.Image = target.Spec.Template.Spec.Containers[0].Image
	}
	result.Rollout = waitForMCPRollout(ctx, cfg, args.Namespace, args.Name, mcpWaitDuration(args.WaitSeconds))
	result.Next = mcpRolloutHint(result.Rollout)
	if status, err := mcpAppStatusOf(cfg, args.Namespace, args.Name); err == nil {
		result.App = status
	}
	return result, nil
}

func mcpRestartApp(ctx context.Context, cfg *rest.Config, raw json.RawMessage) (interface{}, error) {
	var args struct {
		mcpAppRef
		WaitSeconds *int `json:"wait_seconds"`
	}
	if err := decodeMCPArgs(raw, &args); err != nil {
		return nil, err
	}
	if err := args.normalize(); err != nil {
		return nil, err
	}
	if err := object.RestartDeployment(cfg, args.Namespace, args.Name); err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("no app named %s in namespace %s", args.Name, args.Namespace)
		}
		return nil, err
	}
	result := mcpDeployResult{Action: "restarted"}
	result.Rollout = waitForMCPRollout(ctx, cfg, args.Namespace, args.Name, mcpWaitDuration(args.WaitSeconds))
	result.Next = mcpRolloutHint(result.Rollout)
	if status, err := mcpAppStatusOf(cfg, args.Namespace, args.Name); err == nil {
		result.App = status
	}
	return result, nil
}
