package controllers

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/casosorg/casos/object"
	"k8s.io/client-go/rest"
)

const (
	helmTaskMaxLogLines = 500
	helmTaskMaxEntries  = 200
	helmTaskTTL         = 2 * time.Hour
)

// helmTask tracks one asynchronous Helm operation so the UI can poll progress
// instead of holding a long HTTP request open.
type helmTask struct {
	Id        string                     `json:"id"`
	Action    string                     `json:"action"` // install | upgrade | uninstall
	Namespace string                     `json:"namespace"`
	Release   string                     `json:"release"`
	Chart     string                     `json:"chart,omitempty"`
	Version   string                     `json:"version,omitempty"`
	Status    string                     `json:"status"` // pending | running | succeeded | failed
	Error     string                     `json:"error,omitempty"`
	Logs      []string                   `json:"logs"`
	Result    *object.HelmReleaseSummary `json:"result,omitempty"`
	StartedAt string                     `json:"startedAt"`
	EndedAt   string                     `json:"endedAt,omitempty"`

	startedAtTime time.Time
}

var (
	helmTaskMu  sync.RWMutex
	helmTasks   = map[string]*helmTask{}
	helmTaskSeq uint64
)

func helmTaskKey(namespace, release string) string {
	return namespace + "/" + release
}

func newHelmTask(action, namespace, release, chart, version string) (*helmTask, error) {
	helmTaskMu.Lock()
	defer helmTaskMu.Unlock()

	key := helmTaskKey(namespace, release)
	for _, task := range helmTasks {
		if helmTaskKey(task.Namespace, task.Release) == key &&
			(task.Status == "pending" || task.Status == "running") {
			return nil, fmt.Errorf("another %s operation is already running for release %q", task.Action, release)
		}
	}

	pruneHelmTasksLocked()

	helmTaskSeq++
	now := time.Now()
	task := &helmTask{
		Id:            fmt.Sprintf("helm-%d-%d", now.UnixNano(), helmTaskSeq),
		Action:        action,
		Namespace:     namespace,
		Release:       release,
		Chart:         chart,
		Version:       version,
		Status:        "pending",
		Logs:          []string{},
		StartedAt:     now.UTC().Format("2006-01-02 15:04:05"),
		startedAtTime: now,
	}
	helmTasks[task.Id] = task
	return task, nil
}

// pruneHelmTasksLocked drops finished tasks that are past the TTL, and trims
// the oldest entries when the map grows beyond the cap. Caller holds the lock.
func pruneHelmTasksLocked() {
	now := time.Now()
	for id, task := range helmTasks {
		if task.Status == "pending" || task.Status == "running" {
			continue
		}
		if now.Sub(task.startedAtTime) > helmTaskTTL {
			delete(helmTasks, id)
		}
	}

	if len(helmTasks) < helmTaskMaxEntries {
		return
	}
	finished := make([]*helmTask, 0, len(helmTasks))
	for _, task := range helmTasks {
		if task.Status == "succeeded" || task.Status == "failed" {
			finished = append(finished, task)
		}
	}
	sort.Slice(finished, func(i, j int) bool {
		return finished[i].startedAtTime.Before(finished[j].startedAtTime)
	})
	for _, task := range finished {
		if len(helmTasks) < helmTaskMaxEntries {
			break
		}
		delete(helmTasks, task.Id)
	}
}

func getHelmTask(id string) *helmTask {
	helmTaskMu.RLock()
	defer helmTaskMu.RUnlock()

	task, ok := helmTasks[id]
	if !ok {
		return nil
	}
	snapshot := *task
	snapshot.Logs = append([]string{}, task.Logs...)
	return &snapshot
}

func updateHelmTask(id string, fn func(task *helmTask)) {
	helmTaskMu.Lock()
	defer helmTaskMu.Unlock()

	if task, ok := helmTasks[id]; ok {
		fn(task)
	}
}

func appendHelmTaskLog(id, line string) {
	updateHelmTask(id, func(task *helmTask) {
		task.Logs = append(task.Logs, line)
		if len(task.Logs) > helmTaskMaxLogLines {
			task.Logs = task.Logs[len(task.Logs)-helmTaskMaxLogLines:]
		}
	})
}

func helmTaskLogger(id string) object.HelmLogger {
	return func(format string, v ...interface{}) {
		appendHelmTaskLog(id, fmt.Sprintf(format, v...))
	}
}

func finishHelmTask(id string, result *object.HelmReleaseSummary, err error) {
	updateHelmTask(id, func(task *helmTask) {
		task.EndedAt = time.Now().UTC().Format("2006-01-02 15:04:05")
		if err != nil {
			task.Status = "failed"
			task.Error = err.Error()
			return
		}
		task.Status = "succeeded"
		task.Result = result
	})
}

// runHelmInstallTask performs an install in the background.
func runHelmInstallTask(cfg *rest.Config, task *helmTask, ref object.HelmChartRef, values map[string]interface{}) {
	updateHelmTask(task.Id, func(t *helmTask) { t.Status = "running" })
	appendHelmTaskLog(task.Id, fmt.Sprintf("installing chart %s into namespace %s", ref.Chart, task.Namespace))

	result, err := object.HelmInstall(cfg, ref, task.Namespace, task.Release, values, helmTaskLogger(task.Id))
	finishHelmTask(task.Id, result, err)
}

// runHelmUpgradeTask performs an upgrade in the background.
func runHelmUpgradeTask(cfg *rest.Config, task *helmTask, ref object.HelmChartRef, values map[string]interface{}) {
	updateHelmTask(task.Id, func(t *helmTask) { t.Status = "running" })
	appendHelmTaskLog(task.Id, fmt.Sprintf("upgrading release %s in namespace %s", task.Release, task.Namespace))

	result, err := object.HelmUpgrade(cfg, ref, task.Namespace, task.Release, values, helmTaskLogger(task.Id))
	finishHelmTask(task.Id, result, err)
}

// runHelmUninstallTask performs an uninstall in the background.
func runHelmUninstallTask(cfg *rest.Config, task *helmTask) {
	updateHelmTask(task.Id, func(t *helmTask) { t.Status = "running" })
	appendHelmTaskLog(task.Id, fmt.Sprintf("uninstalling release %s from namespace %s", task.Release, task.Namespace))

	err := object.HelmUninstall(cfg, task.Namespace, task.Release, helmTaskLogger(task.Id))
	finishHelmTask(task.Id, nil, err)
}
