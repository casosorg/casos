package object

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestMonitorIssuesFromFakeClientSnapshot(t *testing.T) {
	now := metav1.NewTime(time.Date(2026, 7, 8, 6, 0, 0, 0, time.UTC))
	client := fake.NewSimpleClientset(
		&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
			Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionFalse,
			}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "app",
			}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:  "app",
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				}},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "puller", Namespace: "default"},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{
				Name: "app",
			}}},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:  "app",
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
				}},
			},
		},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "default"},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
		},
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "api-warning", Namespace: "default"},
			InvolvedObject: corev1.ObjectReference{
				Kind:      "Pod",
				Namespace: "default",
				Name:      "api",
			},
			Type:           corev1.EventTypeWarning,
			Reason:         "BackOff",
			Message:        "Back-off restarting failed container app",
			Count:          3,
			FirstTimestamp: now,
			LastTimestamp:  now,
		},
	)

	snapshot := loadMonitorSnapshotFromClient(context.Background(), client, formatMetaTime(now))
	issues := buildMonitorIssues(snapshot)

	requireMonitorIssue(t, issues, "Node", "", "worker-1", "NotReady", MonitorSeverityCritical)
	crashIssue := requireMonitorIssue(t, issues, "Pod", "default", "api", "CrashLoopBackOff", MonitorSeverityCritical)
	if crashIssue.RelatedEventCount == 0 {
		t.Fatalf("expected CrashLoopBackOff issue to include related events")
	}
	requireMonitorIssue(t, issues, "Pod", "default", "puller", "ImagePullBackOff", MonitorSeverityWarning)
	requireMonitorIssue(t, issues, "PersistentVolumeClaim", "default", "data", "Pending", MonitorSeverityWarning)
}

func TestNormalizeMonitorTailLines(t *testing.T) {
	cases := []struct {
		name  string
		input int64
		want  int64
	}{
		{name: "default", input: 0, want: 100},
		{name: "negative", input: -1, want: 100},
		{name: "small", input: 20, want: 20},
		{name: "capped", input: 500, want: 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeMonitorTailLines(tc.input); got != tc.want {
				t.Fatalf("normalizeMonitorTailLines(%d) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func requireMonitorIssue(t *testing.T, issues []MonitorIssue, kind, namespace, name, reason, severity string) MonitorIssue {
	t.Helper()
	for _, issue := range issues {
		if issue.Kind == kind && issue.Namespace == namespace && issue.Name == name && issue.Reason == reason {
			if issue.Severity != severity {
				t.Fatalf("issue %s severity = %s, want %s", issue.ID, issue.Severity, severity)
			}
			return issue
		}
	}
	t.Fatalf("missing issue kind=%s namespace=%s name=%s reason=%s; got %#v", kind, namespace, name, reason, issues)
	return MonitorIssue{}
}
