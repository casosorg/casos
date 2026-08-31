package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	helmFailFastPollInterval = 5 * time.Second
	// How many times a container must have died before the install gives up on
	// it. One or two restarts are ordinary while a dependency comes up — a
	// database that is still starting, a webhook that has not registered yet —
	// so the threshold is set past the point where waiting longer would help.
	helmFailFastRestartThreshold = 3
)

// helmInstallFailFast ends an install that Helm's Wait would otherwise sit out
// for the whole install timeout. A container that has crashed several times is
// not going to be fixed by waiting twenty minutes, and while Helm waits the
// operator has no error to act on and no way to stop it.
//
// It never decides the install failed — it only cancels the context Helm is
// waiting on. Helm then returns its own error, and Reason supplies the sentence
// that says what was actually wrong.
type helmInstallFailFast struct {
	cancelWatch context.CancelFunc
	done        chan struct{}
	stopOnce    sync.Once

	mu     sync.Mutex
	reason string
}

// startHelmInstallFailFast watches the release's pods until ctx ends or a
// container has crashed past the threshold. cancel must be the cancel function
// of the context Helm's Wait is using.
func startHelmInstallFailFast(ctx context.Context, cancel context.CancelFunc, cfg *rest.Config, releaseName, namespace string) *helmInstallFailFast {
	watcher := &helmInstallFailFast{done: make(chan struct{})}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil || cancel == nil {
		close(watcher.done)
		return watcher
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if namespace == "" {
		namespace = "default"
	}
	selector := labels.SelectorFromSet(labels.Set{"app.kubernetes.io/instance": releaseName}).String()

	// The watch gets a context of its own so Stop can end it without cancelling
	// the install: the install's own context outlives a successful Wait, so a
	// watch tied to it would keep Stop waiting for the whole install timeout.
	watchCtx, cancelWatch := context.WithCancel(ctx)
	watcher.cancelWatch = cancelWatch

	go func() {
		defer close(watcher.done)
		defer cancelWatch()
		ticker := time.NewTicker(helmFailFastPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
			}
			pods, err := client.CoreV1().Pods(namespace).List(watchCtx, metav1.ListOptions{LabelSelector: selector})
			if err != nil {
				// A transient list failure must not end the install; the next
				// tick tries again.
				continue
			}
			if reason := helmFailFastReason(pods.Items); reason != "" {
				watcher.mu.Lock()
				watcher.reason = reason
				watcher.mu.Unlock()
				cancel()
				return
			}
		}
	}()
	return watcher
}

// Reason returns the sentence describing why the install was cut short, or "" if
// the watcher did not cut it short.
func (w *helmInstallFailFast) Reason() string {
	if w == nil {
		return ""
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.reason
}

// Stop ends the watch and waits for the goroutine, so callers can rely on no
// further cancels arriving once it returns.
func (w *helmInstallFailFast) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		if w.cancelWatch != nil {
			w.cancelWatch()
		}
		<-w.done
	})
}

func helmFailFastReason(pods []corev1.Pod) string {
	for _, pod := range pods {
		statuses := append(append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...), pod.Status.ContainerStatuses...)
		for _, status := range statuses {
			waiting := status.State.Waiting
			if waiting == nil || waiting.Reason != "CrashLoopBackOff" {
				continue
			}
			if status.RestartCount < helmFailFastRestartThreshold {
				continue
			}
			return fmt.Sprintf(
				"container %s in pod %s has restarted %d times and is in CrashLoopBackOff, so the install stopped waiting for it",
				status.Name, pod.Name, status.RestartCount,
			)
		}
	}
	return ""
}

// withHelmFailFastReason replaces Helm's context error with the reason the
// watcher cut the install short. Helm reports a cancelled Wait as a plain
// deadline or cancellation error, which says nothing about the crashing
// container that caused it.
func withHelmFailFastReason(err error, watcher *helmInstallFailFast) error {
	reason := watcher.Reason()
	if err == nil || reason == "" {
		return err
	}
	return fmt.Errorf("%s", reason)
}
