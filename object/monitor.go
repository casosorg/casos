package object

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	MonitorStatusHealthy  = "healthy"
	MonitorStatusWarning  = "warning"
	MonitorStatusCritical = "critical"
	MonitorStatusUnknown  = "unknown"

	MonitorSeverityInfo     = "info"
	MonitorSeverityWarning  = "warning"
	MonitorSeverityCritical = "critical"
)

type MonitorSummary struct {
	OverallStatus      string `json:"overallStatus"`
	NodeTotal          int    `json:"nodeTotal"`
	NodeReady          int    `json:"nodeReady"`
	PodTotal           int    `json:"podTotal"`
	PodRunning         int    `json:"podRunning"`
	PodAbnormal        int    `json:"podAbnormal"`
	WarningEventCount  int    `json:"warningEventCount"`
	CriticalCheckCount int    `json:"criticalCheckCount"`
	WarningCheckCount  int    `json:"warningCheckCount"`
	LastCheckedAt      string `json:"lastCheckedAt"`
}

type MonitorOverview struct {
	Summary MonitorSummary      `json:"summary"`
	Checks  []HealthCheckResult `json:"checks"`
}

type HealthCheckResult struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Category         string `json:"category"`
	Status           string `json:"status"`
	Severity         string `json:"severity"`
	Message          string `json:"message"`
	Suggestion       string `json:"suggestion"`
	RelatedKind      string `json:"relatedKind"`
	RelatedNamespace string `json:"relatedNamespace"`
	RelatedName      string `json:"relatedName"`
	LastCheckedAt    string `json:"lastCheckedAt"`
}

type MonitorEvent struct {
	Namespace             string `json:"namespace"`
	InvolvedObjectKind    string `json:"involvedObjectKind"`
	InvolvedObjectName    string `json:"involvedObjectName"`
	Reason                string `json:"reason"`
	Type                  string `json:"type"`
	Message               string `json:"message"`
	Count                 int32  `json:"count"`
	FirstTimestamp        string `json:"firstTimestamp"`
	EventTime             string `json:"eventTime"`
	LastTimestamp         string `json:"lastTimestamp"`
	Source                string `json:"source"`
	ReportingController   string `json:"reportingController"`
	ReportingInstance     string `json:"reportingInstance"`
	InvolvedObjectUID     string `json:"involvedObjectUid"`
	InvolvedObjectVersion string `json:"involvedObjectResourceVersion"`
}

type monitorSnapshot struct {
	checkedAt string
	apiErr    error

	nodes      []corev1.Node
	nodesErr   error
	pods       []corev1.Pod
	podsErr    error
	systemPods []corev1.Pod
	systemErr  error
	pvcs       []corev1.PersistentVolumeClaim
	pvcsErr    error
	events     []corev1.Event
	eventsErr  error
}

func GetMonitorOverview(cfg *rest.Config) MonitorOverview {
	snapshot := loadMonitorSnapshot(cfg)
	checks := buildMonitorChecks(snapshot)
	return MonitorOverview{
		Summary: buildMonitorSummary(snapshot, checks),
		Checks:  checks,
	}
}

func GetMonitorSummary(cfg *rest.Config) MonitorSummary {
	snapshot := loadMonitorSnapshot(cfg)
	checks := buildMonitorChecks(snapshot)
	return buildMonitorSummary(snapshot, checks)
}

func GetMonitorChecks(cfg *rest.Config) []HealthCheckResult {
	return buildMonitorChecks(loadMonitorSnapshot(cfg))
}

func GetMonitorEvents(cfg *rest.Config, namespace string, limit int) ([]MonitorEvent, error) {
	if cfg == nil {
		return nil, errors.New("apiserver not ready")
	}
	client, err := newClient(cfg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	events, err := listMonitorEvents(ctx, client, namespace, limit)
	if err != nil {
		return nil, err
	}
	result := make([]MonitorEvent, 0, len(events))
	for _, event := range events {
		result = append(result, toMonitorEvent(event))
	}
	return result, nil
}

func buildMonitorSummary(snapshot monitorSnapshot, checks []HealthCheckResult) MonitorSummary {
	summary := MonitorSummary{
		OverallStatus: MonitorStatusHealthy,
		LastCheckedAt: snapshot.checkedAt,
	}

	for _, node := range snapshot.nodes {
		summary.NodeTotal++
		if isNodeReady(node) {
			summary.NodeReady++
		}
	}

	for _, pod := range snapshot.pods {
		summary.PodTotal++
		if pod.Status.Phase == corev1.PodRunning {
			summary.PodRunning++
		}
		if abnormal, _, _ := detectAbnormalPod(pod); abnormal {
			summary.PodAbnormal++
		}
	}

	for _, event := range snapshot.events {
		if event.Type == corev1.EventTypeWarning {
			summary.WarningEventCount++
		}
	}

	hasUnknown := false
	for _, check := range checks {
		switch check.Status {
		case MonitorStatusCritical:
			summary.CriticalCheckCount++
		case MonitorStatusWarning:
			summary.WarningCheckCount++
		case MonitorStatusUnknown:
			hasUnknown = true
		}
	}

	switch {
	case summary.CriticalCheckCount > 0:
		summary.OverallStatus = MonitorStatusCritical
	case summary.WarningCheckCount > 0:
		summary.OverallStatus = MonitorStatusWarning
	case hasUnknown:
		summary.OverallStatus = MonitorStatusUnknown
	default:
		summary.OverallStatus = MonitorStatusHealthy
	}

	return summary
}

func loadMonitorSnapshot(cfg *rest.Config) monitorSnapshot {
	snapshot := monitorSnapshot{
		checkedAt: formatTime(time.Now().UTC()),
	}
	if cfg == nil {
		snapshot.apiErr = errors.New("apiserver not ready")
		return snapshot
	}

	client, err := newClient(cfg)
	if err != nil {
		snapshot.apiErr = err
		return snapshot
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err = client.Discovery().ServerVersion(); err != nil {
		snapshot.apiErr = err
	}

	snapshot.nodes, snapshot.nodesErr = listMonitorNodes(ctx, client)
	snapshot.pods, snapshot.podsErr = listMonitorPods(ctx, client, metav1.NamespaceAll)
	snapshot.systemPods, snapshot.systemErr = listMonitorPods(ctx, client, metav1.NamespaceSystem)
	snapshot.pvcs, snapshot.pvcsErr = listMonitorPVCs(ctx, client)
	snapshot.events, snapshot.eventsErr = listMonitorEvents(ctx, client, metav1.NamespaceAll, 100)

	if snapshot.apiErr == nil && snapshot.nodesErr != nil {
		snapshot.apiErr = snapshot.nodesErr
	}

	return snapshot
}

func listMonitorNodes(ctx context.Context, client *kubernetes.Clientset) ([]corev1.Node, error) {
	list, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func listMonitorPods(ctx context.Context, client *kubernetes.Clientset, namespace string) ([]corev1.Pod, error) {
	list, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func listMonitorPVCs(ctx context.Context, client *kubernetes.Clientset) ([]corev1.PersistentVolumeClaim, error) {
	list, err := client.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

func listMonitorEvents(ctx context.Context, client *kubernetes.Clientset, namespace string, limit int) ([]corev1.Event, error) {
	ns := namespace
	if ns == "" {
		ns = metav1.NamespaceAll
	}
	limit = normalizeMonitorEventLimit(limit)
	list, err := client.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		Limit: int64(limit),
	})
	if err != nil {
		return nil, err
	}
	events := list.Items
	sortEvents(events)
	return events, nil
}

func normalizeMonitorEventLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 500 {
		return 500
	}
	return limit
}

func buildMonitorChecks(snapshot monitorSnapshot) []HealthCheckResult {
	return []HealthCheckResult{
		checkKubernetesAPI(snapshot),
		checkNodeReady(snapshot),
		checkNodePressure(snapshot),
		checkPodAbnormal(snapshot),
		checkSystemComponents(snapshot),
		checkPVCBound(snapshot),
		checkWarningEvents(snapshot),
		checkCasOSService(snapshot),
	}
}

func checkKubernetesAPI(snapshot monitorSnapshot) HealthCheckResult {
	check := newCheck("kubernetes-api", "Kubernetes API Server Connectivity", "cluster", snapshot.checkedAt)
	if snapshot.apiErr != nil {
		check.Status = MonitorStatusUnknown
		check.Severity = MonitorSeverityWarning
		check.Message = fmt.Sprintf("Kubernetes API is not reachable: %v", snapshot.apiErr)
		check.Suggestion = "Check the CasOS apiserver startup status, admin REST config, certificates, and local network connectivity."
		return check
	}
	check.Message = "Kubernetes API is reachable."
	check.Suggestion = "No action required."
	return check
}

func checkNodeReady(snapshot monitorSnapshot) HealthCheckResult {
	check := newCheck("node-ready", "Node Ready Check", "node", snapshot.checkedAt)
	check.RelatedKind = "Node"
	if snapshot.nodesErr != nil {
		return unknownCheck(check, fmt.Sprintf("Unable to list nodes: %v", snapshot.nodesErr), "Check Kubernetes API access and node list permissions.")
	}
	if len(snapshot.nodes) == 0 {
		check.Status = MonitorStatusCritical
		check.Severity = MonitorSeverityCritical
		check.Message = "No nodes are registered in the cluster."
		check.Suggestion = "Start kubelet on at least one machine and verify it can reach the CasOS apiserver."
		return check
	}

	ready := 0
	notReady := []string{}
	for _, node := range snapshot.nodes {
		if isNodeReady(node) {
			ready++
		} else {
			notReady = append(notReady, node.Name)
		}
	}
	if len(notReady) == 0 {
		check.Message = fmt.Sprintf("%d/%d nodes are Ready.", ready, len(snapshot.nodes))
		check.Suggestion = "No action required."
		return check
	}

	check.Status = MonitorStatusWarning
	check.Severity = MonitorSeverityWarning
	if ready == 0 {
		check.Status = MonitorStatusCritical
		check.Severity = MonitorSeverityCritical
	}
	check.Message = fmt.Sprintf("%d/%d nodes are Ready, NotReady nodes: %s.", ready, len(snapshot.nodes), strings.Join(notReady, ", "))
	check.Suggestion = "Check kubelet status, node network connectivity, container runtime, and disk pressure."
	if len(notReady) == 1 {
		check.RelatedName = notReady[0]
	}
	return check
}

func checkNodePressure(snapshot monitorSnapshot) HealthCheckResult {
	check := newCheck("node-pressure", "Node Pressure Check", "node", snapshot.checkedAt)
	check.RelatedKind = "Node"
	if snapshot.nodesErr != nil {
		return unknownCheck(check, fmt.Sprintf("Unable to list node conditions: %v", snapshot.nodesErr), "Check Kubernetes API access and node list permissions.")
	}

	pressure := []string{}
	for _, node := range snapshot.nodes {
		for _, cond := range node.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				continue
			}
			switch cond.Type {
			case corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure, corev1.NodeNetworkUnavailable:
				pressure = append(pressure, fmt.Sprintf("%s:%s", node.Name, cond.Type))
			}
		}
	}
	if len(pressure) == 0 {
		check.Message = "No node pressure conditions are active."
		check.Suggestion = "No action required."
		return check
	}
	check.Status = MonitorStatusWarning
	check.Severity = MonitorSeverityWarning
	check.Message = fmt.Sprintf("Node pressure detected: %s.", strings.Join(limitStrings(pressure, 8), ", "))
	check.Suggestion = "Check node disk, memory, PID usage, network plugin status, and recent kubelet events."
	return check
}

func checkPodAbnormal(snapshot monitorSnapshot) HealthCheckResult {
	check := newCheck("pod-abnormal", "Pod Abnormal Status Check", "pod", snapshot.checkedAt)
	check.RelatedKind = "Pod"
	if snapshot.podsErr != nil {
		return unknownCheck(check, fmt.Sprintf("Unable to list pods: %v", snapshot.podsErr), "Check Kubernetes API access and pod list permissions.")
	}

	abnormal := []string{}
	critical := false
	var related *corev1.Pod
	for i, pod := range snapshot.pods {
		isAbnormal, reason, isCritical := detectAbnormalPod(pod)
		if !isAbnormal {
			continue
		}
		if related == nil {
			related = &snapshot.pods[i]
		}
		if isCritical {
			critical = true
		}
		abnormal = append(abnormal, fmt.Sprintf("%s/%s(%s)", pod.Namespace, pod.Name, reason))
	}
	if len(abnormal) == 0 {
		check.Message = "No abnormal pods found."
		check.Suggestion = "No action required."
		return check
	}
	check.Status = MonitorStatusWarning
	check.Severity = MonitorSeverityWarning
	if critical {
		check.Status = MonitorStatusCritical
		check.Severity = MonitorSeverityCritical
	}
	check.Message = fmt.Sprintf("%d abnormal pods found: %s.", len(abnormal), strings.Join(limitStrings(abnormal, 8), ", "))
	check.Suggestion = podSuggestion(abnormal[0])
	if related != nil {
		check.RelatedNamespace = related.Namespace
		check.RelatedName = related.Name
	}
	return check
}

func checkSystemComponents(snapshot monitorSnapshot) HealthCheckResult {
	check := newCheck("system-components", "System Component Check", "system", snapshot.checkedAt)
	check.RelatedKind = "Pod"
	if snapshot.systemErr != nil {
		return unknownCheck(check, fmt.Sprintf("Unable to list kube-system pods: %v", snapshot.systemErr), "Check Kubernetes API access and kube-system pod permissions.")
	}

	recognized := []corev1.Pod{}
	for _, pod := range snapshot.systemPods {
		if systemComponentName(pod.Name) != "" {
			recognized = append(recognized, pod)
		}
	}
	if len(recognized) == 0 {
		check.Status = MonitorStatusUnknown
		check.Severity = MonitorSeverityWarning
		check.Message = "No known kube-system component pods were found."
		check.Suggestion = "Verify cluster DNS, kube-proxy, and network plugin components if this cluster is expected to run workloads."
		return check
	}

	abnormal := []string{}
	corednsReady := false
	corednsSeen := false
	for _, pod := range recognized {
		component := systemComponentName(pod.Name)
		if component == "coredns" {
			corednsSeen = true
			if isPodAvailable(pod) {
				corednsReady = true
			}
		}
		if !isPodAvailable(pod) {
			reason := string(pod.Status.Phase)
			if abnormalPod, podReason, _ := detectAbnormalPod(pod); abnormalPod {
				reason = podReason
			}
			abnormal = append(abnormal, fmt.Sprintf("%s/%s(%s)", pod.Namespace, pod.Name, reason))
		}
	}
	if len(abnormal) == 0 {
		check.Message = fmt.Sprintf("%d kube-system component pods look healthy.", len(recognized))
		check.Suggestion = "No action required."
		return check
	}

	check.Status = MonitorStatusWarning
	check.Severity = MonitorSeverityWarning
	if corednsSeen && !corednsReady {
		check.Status = MonitorStatusCritical
		check.Severity = MonitorSeverityCritical
		check.Suggestion = "Check coredns pods, kube-system events, and the cluster DNS service."
	} else {
		check.Suggestion = "Check kube-system pod logs, related events, and node networking."
	}
	check.Message = fmt.Sprintf("Abnormal system components: %s.", strings.Join(limitStrings(abnormal, 8), ", "))
	return check
}

func checkPVCBound(snapshot monitorSnapshot) HealthCheckResult {
	check := newCheck("pvc-bound", "PVC Bound Check", "storage", snapshot.checkedAt)
	check.RelatedKind = "PersistentVolumeClaim"
	if snapshot.pvcsErr != nil {
		return unknownCheck(check, fmt.Sprintf("Unable to list PVCs: %v", snapshot.pvcsErr), "Check Kubernetes API access and PVC list permissions.")
	}
	if len(snapshot.pvcs) == 0 {
		check.Message = "No PVCs found."
		check.Suggestion = "No action required."
		return check
	}
	notBound := []string{}
	for _, pvc := range snapshot.pvcs {
		if pvc.Status.Phase != corev1.ClaimBound {
			notBound = append(notBound, fmt.Sprintf("%s/%s(%s)", pvc.Namespace, pvc.Name, pvc.Status.Phase))
		}
	}
	if len(notBound) == 0 {
		check.Message = fmt.Sprintf("%d/%d PVCs are Bound.", len(snapshot.pvcs), len(snapshot.pvcs))
		check.Suggestion = "No action required."
		return check
	}
	check.Status = MonitorStatusWarning
	check.Severity = MonitorSeverityWarning
	check.Message = fmt.Sprintf("%d/%d PVCs are not Bound: %s.", len(notBound), len(snapshot.pvcs), strings.Join(limitStrings(notBound, 8), ", "))
	check.Suggestion = "Check whether a default StorageClass exists and whether the requested storage can be provisioned."
	return check
}

func checkWarningEvents(snapshot monitorSnapshot) HealthCheckResult {
	check := newCheck("warning-events", "Kubernetes Warning Event Check", "event", snapshot.checkedAt)
	check.RelatedKind = "Event"
	if snapshot.eventsErr != nil {
		return unknownCheck(check, fmt.Sprintf("Unable to list Kubernetes events: %v", snapshot.eventsErr), "Check Kubernetes API access and event list permissions.")
	}

	warnings := []string{}
	for _, event := range snapshot.events {
		if event.Type != corev1.EventTypeWarning {
			continue
		}
		objectName := event.InvolvedObject.Name
		if event.InvolvedObject.Namespace != "" {
			objectName = event.InvolvedObject.Namespace + "/" + objectName
		}
		warnings = append(warnings, fmt.Sprintf("%s:%s", objectName, event.Reason))
	}
	if len(warnings) == 0 {
		check.Message = "No Warning events found in the recent Kubernetes event window."
		check.Suggestion = "No action required."
		return check
	}
	check.Status = MonitorStatusWarning
	check.Severity = MonitorSeverityWarning
	check.Message = fmt.Sprintf("%d Warning events found: %s.", len(warnings), strings.Join(limitStrings(warnings, 8), ", "))
	check.Suggestion = "Open the event center, inspect the newest Warning events, and check the related object logs or configuration."
	return check
}

func checkCasOSService(snapshot monitorSnapshot) HealthCheckResult {
	check := newCheck("casos-service", "CasOS Backend and Database Check", "casos", snapshot.checkedAt)
	if err := PingDatabase(); err != nil {
		check.Status = MonitorStatusCritical
		check.Severity = MonitorSeverityCritical
		check.Message = fmt.Sprintf("CasOS backend is alive, but database ping failed: %v", err)
		check.Suggestion = "Check database service reachability, credentials, and CasOS database configuration."
		return check
	}
	check.Message = "CasOS backend is alive and database ping succeeded."
	check.Suggestion = "No action required."
	return check
}

func newCheck(id, name, category, checkedAt string) HealthCheckResult {
	return HealthCheckResult{
		ID:            id,
		Name:          name,
		Category:      category,
		Status:        MonitorStatusHealthy,
		Severity:      MonitorSeverityInfo,
		Message:       "",
		Suggestion:    "No action required.",
		LastCheckedAt: checkedAt,
	}
}

func unknownCheck(check HealthCheckResult, message, suggestion string) HealthCheckResult {
	check.Status = MonitorStatusUnknown
	check.Severity = MonitorSeverityWarning
	check.Message = message
	check.Suggestion = suggestion
	return check
}

func isNodeReady(node corev1.Node) bool {
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

func detectAbnormalPod(pod corev1.Pod) (bool, string, bool) {
	switch pod.Status.Phase {
	case corev1.PodPending:
		reason := pod.Status.Reason
		if reason == "" {
			reason = "Pending"
		}
		if waitingReason := firstWaitingReason(pod); waitingReason != "" {
			reason = waitingReason
		}
		return true, reason, isCriticalPodReason(reason)
	case corev1.PodFailed:
		reason := pod.Status.Reason
		if reason == "" {
			reason = "Failed"
		}
		return true, reason, true
	case corev1.PodUnknown:
		return true, "Unknown", true
	}

	if reason := firstWaitingReason(pod); reason != "" && isAbnormalPodReason(reason) {
		return true, reason, isCriticalPodReason(reason)
	}
	if reason := firstTerminatedReason(pod); reason != "" && isAbnormalPodReason(reason) {
		return true, reason, isCriticalPodReason(reason)
	}
	return false, "", false
}

func firstWaitingReason(pod corev1.Pod) string {
	statuses := append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...)
	statuses = append(statuses, pod.Status.ContainerStatuses...)
	for _, cs := range statuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			return cs.State.Waiting.Reason
		}
	}
	return ""
}

func firstTerminatedReason(pod corev1.Pod) string {
	statuses := append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...)
	statuses = append(statuses, pod.Status.ContainerStatuses...)
	for _, cs := range statuses {
		if cs.State.Terminated != nil && cs.State.Terminated.Reason != "" {
			return cs.State.Terminated.Reason
		}
	}
	return ""
}

func isAbnormalPodReason(reason string) bool {
	switch reason {
	case "CrashLoopBackOff",
		"ImagePullBackOff",
		"ErrImagePull",
		"CreateContainerConfigError",
		"RunContainerError",
		"CreateContainerError",
		"InvalidImageName",
		"OOMKilled",
		"Evicted":
		return true
	}
	return false
}

func isCriticalPodReason(reason string) bool {
	switch reason {
	case "CrashLoopBackOff",
		"CreateContainerConfigError",
		"RunContainerError",
		"CreateContainerError",
		"Failed",
		"Unknown",
		"OOMKilled",
		"Evicted":
		return true
	}
	return false
}

func podSuggestion(sample string) string {
	switch {
	case strings.Contains(sample, "ImagePullBackOff"), strings.Contains(sample, "ErrImagePull"), strings.Contains(sample, "InvalidImageName"):
		return "Check image name, registry access, imagePullSecrets, and network connectivity."
	case strings.Contains(sample, "CrashLoopBackOff"):
		return "Check container logs, command args, environment variables, and mounted configuration."
	case strings.Contains(sample, "CreateContainerConfigError"), strings.Contains(sample, "RunContainerError"), strings.Contains(sample, "CreateContainerError"):
		return "Check container command, environment variables, secrets, config maps, and mounted volumes."
	case strings.Contains(sample, "Pending"):
		return "Check scheduling constraints, node capacity, image pulls, and PVC binding status."
	default:
		return "Check pod events, container logs, image configuration, and node health."
	}
}

func isPodAvailable(pod corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
		return false
	}
	for _, cs := range pod.Status.ContainerStatuses {
		if !cs.Ready && pod.Status.Phase != corev1.PodSucceeded {
			return false
		}
	}
	return true
}

func systemComponentName(podName string) string {
	name := strings.ToLower(podName)
	switch {
	case strings.Contains(name, "coredns"):
		return "coredns"
	case strings.Contains(name, "kube-proxy"):
		return "kube-proxy"
	case strings.Contains(name, "metrics-server"):
		return "metrics-server"
	case strings.Contains(name, "flannel"):
		return "flannel"
	case strings.Contains(name, "calico"):
		return "calico"
	case strings.Contains(name, "cilium"):
		return "cilium"
	}
	return ""
}

func sortEvents(events []corev1.Event) {
	sort.Slice(events, func(i, j int) bool {
		if events[i].Type == corev1.EventTypeWarning && events[j].Type != corev1.EventTypeWarning {
			return true
		}
		if events[i].Type != corev1.EventTypeWarning && events[j].Type == corev1.EventTypeWarning {
			return false
		}
		return eventSortTime(events[i]).After(eventSortTime(events[j]))
	})
}

func toMonitorEvent(event corev1.Event) MonitorEvent {
	return MonitorEvent{
		Namespace:             event.Namespace,
		InvolvedObjectKind:    event.InvolvedObject.Kind,
		InvolvedObjectName:    event.InvolvedObject.Name,
		Reason:                event.Reason,
		Type:                  event.Type,
		Message:               event.Message,
		Count:                 event.Count,
		FirstTimestamp:        formatMetaTime(event.FirstTimestamp),
		EventTime:             formatMicroTime(event.EventTime),
		LastTimestamp:         formatMetaTime(event.LastTimestamp),
		Source:                formatEventSource(event.Source),
		ReportingController:   event.ReportingController,
		ReportingInstance:     event.ReportingInstance,
		InvolvedObjectUID:     string(event.InvolvedObject.UID),
		InvolvedObjectVersion: event.InvolvedObject.ResourceVersion,
	}
}

func eventSortTime(event corev1.Event) time.Time {
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	if !event.FirstTimestamp.IsZero() {
		return event.FirstTimestamp.Time
	}
	return event.CreationTimestamp.Time
}

func formatEventSource(source corev1.EventSource) string {
	parts := []string{}
	if source.Component != "" {
		parts = append(parts, source.Component)
	}
	if source.Host != "" {
		parts = append(parts, source.Host)
	}
	return strings.Join(parts, "/")
}

func formatMetaTime(t metav1.Time) string {
	if t.IsZero() {
		return ""
	}
	return formatTime(t.Time)
}

func formatMicroTime(t metav1.MicroTime) string {
	if t.IsZero() {
		return ""
	}
	return formatTime(t.Time)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func limitStrings(items []string, limit int) []string {
	if len(items) <= limit {
		return items
	}
	return append(items[:limit], fmt.Sprintf("+%d more", len(items)-limit))
}
