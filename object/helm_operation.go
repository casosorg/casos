package object

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"xorm.io/xorm"
)

const (
	HelmOperationInstall = "install"

	HelmOperationStatusPending   = "pending"
	HelmOperationStatusRunning   = "running"
	HelmOperationStatusSucceeded = "succeeded"
	HelmOperationStatusFailed    = "failed"

	HelmOperationPhaseQueued     = "queued"
	HelmOperationPhaseLoading    = "loading"
	HelmOperationPhaseInstalling = "installing"
	HelmOperationPhaseReady      = "ready"
	HelmOperationPhaseFailed     = "failed"

	HelmOperationLogLevelInfo  = "info"
	HelmOperationLogLevelError = "error"

	HelmOperationPersistenceTimeout = 5 * time.Second
	helmOperationStaleAfter         = 11 * time.Minute
)

var (
	ErrHelmOperationAlreadyActive   = errors.New("Helm operation already active")
	ErrHelmOperationAlreadyFinished = errors.New("Helm operation already finished")
)

type HelmOperationTask struct {
	Id          int64     `xorm:"pk autoincr" json:"id"`
	ActiveKey   *string   `xorm:"char(64) unique" json:"-"`
	Owner       string    `xorm:"varchar(100) notnull index" json:"owner"`
	Operation   string    `xorm:"varchar(30) notnull" json:"operation"`
	ReleaseName string    `xorm:"varchar(253) notnull index" json:"releaseName"`
	Namespace   string    `xorm:"varchar(253) notnull index" json:"namespace"`
	ChartName   string    `xorm:"varchar(253) notnull" json:"chartName"`
	Version     string    `xorm:"varchar(100)" json:"version"`
	Status      string    `xorm:"varchar(30) notnull index" json:"status"`
	Phase       string    `xorm:"varchar(30) notnull" json:"phase"`
	ErrorMsg    string    `xorm:"text" json:"errorMsg"`
	CreatedAt   time.Time `json:"createdAt"`
	StartedAt   time.Time `json:"startedAt"`
	FinishedAt  time.Time `json:"finishedAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type HelmOperationLog struct {
	Id        int64     `xorm:"pk autoincr" json:"id"`
	TaskId    int64     `xorm:"notnull index" json:"taskId"`
	Level     string    `xorm:"varchar(20) notnull" json:"level"`
	Message   string    `xorm:"text" json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

func CreateHelmOperationTask(owner, operation, releaseName, namespace, chartName, version string) (*HelmOperationTask, error) {
	owner = strings.TrimSpace(owner)
	operation = strings.TrimSpace(operation)
	releaseName = strings.TrimSpace(releaseName)
	namespace = strings.TrimSpace(namespace)
	chartName = strings.TrimSpace(chartName)
	if owner == "" || operation == "" || releaseName == "" || namespace == "" || chartName == "" {
		return nil, fmt.Errorf("owner, operation, releaseName, namespace, and chartName are required")
	}
	if operation != HelmOperationInstall {
		return nil, fmt.Errorf("unsupported Helm operation: %s", operation)
	}
	activeKey := helmOperationActiveKey(namespace, releaseName)

	result, err := withHelmOperationTransaction(func(session *xorm.Session) (interface{}, error) {
		active := &HelmOperationTask{}
		found, err := session.
			Where("namespace = ? AND release_name = ? AND status IN (?, ?)", namespace, releaseName, HelmOperationStatusPending, HelmOperationStatusRunning).
			Desc("id").
			ForUpdate().
			Get(active)
		if err != nil {
			return nil, err
		}
		if found {
			if active.UpdatedAt.After(time.Now().UTC().Add(-helmOperationStaleAfter)) {
				return nil, fmt.Errorf("%w: task %d is already active for %s/%s", ErrHelmOperationAlreadyActive, active.Id, namespace, releaseName)
			}
			now := time.Now().UTC()
			if _, err := session.ID(active.Id).
				Cols("active_key", "status", "phase", "error_msg", "finished_at", "updated_at").
				Update(&HelmOperationTask{
					ActiveKey:  nil,
					Status:     HelmOperationStatusFailed,
					Phase:      HelmOperationPhaseFailed,
					ErrorMsg:   "Helm operation expired before completion",
					FinishedAt: now,
					UpdatedAt:  now,
				}); err != nil {
				return nil, err
			}
		}

		now := time.Now().UTC()
		task := &HelmOperationTask{
			ActiveKey:   &activeKey,
			Owner:       owner,
			Operation:   operation,
			ReleaseName: releaseName,
			Namespace:   namespace,
			ChartName:   chartName,
			Version:     strings.TrimSpace(version),
			Status:      HelmOperationStatusPending,
			Phase:       HelmOperationPhaseQueued,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if _, err := session.Insert(task); err != nil {
			conflict := &HelmOperationTask{}
			if found, lookupErr := session.Where("active_key = ?", activeKey).Get(conflict); lookupErr == nil && found {
				return nil, fmt.Errorf("%w: task %d is already active for %s/%s", ErrHelmOperationAlreadyActive, conflict.Id, namespace, releaseName)
			}
			return nil, err
		}
		return task, nil
	})
	if err != nil {
		return nil, err
	}
	task, ok := result.(*HelmOperationTask)
	if !ok || task == nil {
		return nil, fmt.Errorf("create Helm operation task returned invalid result")
	}
	return task, nil
}

func helmOperationActiveKey(namespace, releaseName string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(namespace+"\x00"+releaseName)))
}

func GetHelmOperationTask(id int64) (*HelmOperationTask, error) {
	task := &HelmOperationTask{Id: id}
	found, err := ormer.Engine.Get(task)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return task, nil
}

// GetHelmOperationTaskForOwner deliberately returns (nil, nil) for both a
// missing task and an owner mismatch so callers cannot reveal another owner's
// task through their response behavior.
func GetHelmOperationTaskForOwner(id int64, owner string) (*HelmOperationTask, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return nil, fmt.Errorf("Helm operation owner is required")
	}
	task, err := GetHelmOperationTask(id)
	if err != nil || task == nil {
		return task, err
	}
	if task.Owner != owner {
		return nil, nil
	}
	return task, nil
}

func IsHelmOperationTaskStale(task *HelmOperationTask, now time.Time) bool {
	if task == nil || (task.Status != HelmOperationStatusPending && task.Status != HelmOperationStatusRunning) {
		return false
	}
	return task.UpdatedAt.Before(now.UTC().Add(-helmOperationStaleAfter))
}

func ExpireStaleHelmOperationTask(id int64) error {
	now := time.Now().UTC()
	_, err := ormer.Engine.ID(id).
		Where("status IN (?, ?) AND updated_at < ?", HelmOperationStatusPending, HelmOperationStatusRunning, now.Add(-helmOperationStaleAfter)).
		Cols("active_key", "status", "phase", "error_msg", "finished_at", "updated_at").
		Update(&HelmOperationTask{
			ActiveKey:  nil,
			Status:     HelmOperationStatusFailed,
			Phase:      HelmOperationPhaseFailed,
			ErrorMsg:   "Helm operation expired before completion",
			FinishedAt: now,
			UpdatedAt:  now,
		})
	return err
}

func StartHelmOperationTask(id int64, phase string) error {
	return StartHelmOperationTaskContext(context.Background(), id, phase)
}

func StartHelmOperationTaskContext(ctx context.Context, id int64, phase string) error {
	if phase != HelmOperationPhaseLoading {
		return fmt.Errorf("invalid Helm operation start phase: %s", phase)
	}
	now := time.Now().UTC()
	affected, err := ormer.Engine.Context(ctx).ID(id).
		Where("status = ? AND phase = ?", HelmOperationStatusPending, HelmOperationPhaseQueued).
		Cols("status", "phase", "started_at", "updated_at").
		Update(&HelmOperationTask{Status: HelmOperationStatusRunning, Phase: phase, StartedAt: now, UpdatedAt: now})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("Helm operation task %d is not pending", id)
	}
	return nil
}

func UpdateHelmOperationTaskPhase(id int64, phase string) error {
	return UpdateHelmOperationTaskPhaseContext(context.Background(), id, phase)
}

func UpdateHelmOperationTaskPhaseContext(ctx context.Context, id int64, phase string) error {
	if phase != HelmOperationPhaseInstalling {
		return fmt.Errorf("invalid Helm operation phase: %s", phase)
	}
	affected, err := ormer.Engine.Context(ctx).ID(id).
		Where("status = ? AND phase = ?", HelmOperationStatusRunning, HelmOperationPhaseLoading).
		Cols("phase", "updated_at").
		Update(&HelmOperationTask{Phase: phase, UpdatedAt: time.Now().UTC()})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("Helm operation task %d is not running", id)
	}
	return nil
}

func FinishHelmOperationTask(id int64, success bool, errorMsg string) error {
	return FinishHelmOperationTaskContext(context.Background(), id, success, errorMsg)
}

func FinishHelmOperationTaskContext(ctx context.Context, id int64, success bool, errorMsg string) error {
	status := HelmOperationStatusSucceeded
	phase := HelmOperationPhaseReady
	if success {
		errorMsg = ""
	}
	if !success {
		status = HelmOperationStatusFailed
		phase = HelmOperationPhaseFailed
	}
	now := time.Now().UTC()
	affected, err := ormer.Engine.Context(ctx).ID(id).
		Where("status IN (?, ?)", HelmOperationStatusPending, HelmOperationStatusRunning).
		Cols("active_key", "status", "phase", "error_msg", "finished_at", "updated_at").
		Update(&HelmOperationTask{ActiveKey: nil, Status: status, Phase: phase, ErrorMsg: errorMsg, FinishedAt: now, UpdatedAt: now})
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("%w: task %d", ErrHelmOperationAlreadyFinished, id)
	}
	return nil
}

func HelmOperationTaskHasTerminalOutcomeContext(ctx context.Context, id int64, success bool, errorMsg string) (bool, error) {
	task := &HelmOperationTask{Id: id}
	found, err := ormer.Engine.Context(ctx).Get(task)
	if err != nil || !found {
		return false, err
	}
	if success {
		return task.Status == HelmOperationStatusSucceeded && task.Phase == HelmOperationPhaseReady && task.ErrorMsg == "", nil
	}
	return task.Status == HelmOperationStatusFailed && task.Phase == HelmOperationPhaseFailed && task.ErrorMsg == errorMsg, nil
}

func addHelmOperationLogs(taskID int64, logs []*HelmOperationLog) error {
	return addHelmOperationLogsContext(context.Background(), taskID, logs)
}

func addHelmOperationLogsContext(ctx context.Context, taskID int64, logs []*HelmOperationLog) error {
	if len(logs) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for _, entry := range logs {
		if entry == nil || entry.TaskId != taskID {
			return fmt.Errorf("Helm operation log task id does not match task %d", taskID)
		}
		if entry.CreatedAt.IsZero() {
			entry.CreatedAt = now
		}
	}
	_, err := withHelmOperationTransactionContext(ctx, func(session *xorm.Session) (interface{}, error) {
		if _, err := session.Insert(&logs); err != nil {
			return nil, err
		}
		_, err := session.ID(taskID).
			Where("status IN (?, ?)", HelmOperationStatusPending, HelmOperationStatusRunning).
			Cols("updated_at").
			Update(&HelmOperationTask{UpdatedAt: now})
		return nil, err
	})
	return err
}

// GetHelmOperationLogs returns the most recent limit entries in chronological
// order (oldest to newest within that window).
func GetHelmOperationLogs(taskID int64, limit int) ([]*HelmOperationLog, error) {
	if taskID <= 0 {
		return nil, fmt.Errorf("invalid taskId")
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 1000 {
		return nil, fmt.Errorf("limit must not exceed 1000")
	}
	logs := []*HelmOperationLog{}
	err := ormer.Engine.Where("task_id = ?", taskID).Desc("id").Limit(limit).Find(&logs)
	for left, right := 0, len(logs)-1; left < right; left, right = left+1, right-1 {
		logs[left], logs[right] = logs[right], logs[left]
	}
	return logs, err
}

func isValidHelmOperationPhase(phase string) bool {
	switch phase {
	case HelmOperationPhaseQueued, HelmOperationPhaseLoading, HelmOperationPhaseInstalling, HelmOperationPhaseReady, HelmOperationPhaseFailed:
		return true
	default:
		return false
	}
}

func withHelmOperationTransaction(fn func(*xorm.Session) (interface{}, error)) (interface{}, error) {
	return withHelmOperationTransactionContext(context.Background(), fn)
}

func withHelmOperationTransactionContext(ctx context.Context, fn func(*xorm.Session) (interface{}, error)) (interface{}, error) {
	session := ormer.Engine.NewSession().Context(ctx)
	defer func() {
		if v := recover(); v != nil {
			_ = session.Rollback()
			session.Close()
			panic(v)
		}
		session.Close()
	}()
	if err := session.Begin(); err != nil {
		return nil, err
	}
	result, err := fn(session)
	if err != nil {
		_ = session.Rollback()
		return nil, err
	}
	if err := session.Commit(); err != nil {
		_ = session.Rollback()
		return nil, err
	}
	return result, nil
}
