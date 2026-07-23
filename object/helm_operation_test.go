package object

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func TestHelmOperationTaskStateMachine(t *testing.T) {
	previousOrmer := ormer
	engine, err := xorm.NewEngine("sqlite", "file:helm-operation-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("create test engine: %v", err)
	}
	engine.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = engine.Close()
		ormer = previousOrmer
	})
	ormer = &Ormer{Engine: engine}

	if err := engine.Sync2(new(HelmOperationTask), new(HelmOperationLog)); err != nil {
		t.Fatalf("create task tables: %v", err)
	}
	if _, err := CreateHelmOperationTask("admin", "delete", "demo", "default", "demo", "1.0.0"); err == nil {
		t.Fatal("expected an unsupported operation to be rejected")
	}

	task, err := CreateHelmOperationTask("admin", HelmOperationInstall, "demo", "default", "demo", "1.0.0")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.Status != HelmOperationStatusPending || task.Phase != HelmOperationPhaseQueued {
		t.Fatalf("unexpected initial state: %s/%s", task.Status, task.Phase)
	}
	owned, err := GetHelmOperationTaskForOwner(task.Id, "admin")
	if err != nil || owned == nil || owned.Id != task.Id {
		t.Fatalf("get task for owner: task=%v err=%v", owned, err)
	}
	if other, err := GetHelmOperationTaskForOwner(task.Id, "other-admin"); err != nil || other != nil {
		t.Fatalf("expected owner mismatch to look missing, task=%v err=%v", other, err)
	}
	if missing, err := GetHelmOperationTaskForOwner(task.Id+1000, "admin"); err != nil || missing != nil {
		t.Fatalf("expected missing task, task=%v err=%v", missing, err)
	}
	if err := StartHelmOperationTask(task.Id, HelmOperationPhaseReady); err == nil {
		t.Fatal("expected a pending task to reject the terminal ready phase")
	}
	if err := StartHelmOperationTask(task.Id, HelmOperationPhaseLoading); err != nil {
		t.Fatalf("start task: %v", err)
	}
	if err := UpdateHelmOperationTaskPhase(task.Id, HelmOperationPhaseLoading); err == nil {
		t.Fatal("expected a running task to reject a repeated loading phase")
	}
	if err := UpdateHelmOperationTaskPhase(task.Id, HelmOperationPhaseReady); err == nil {
		t.Fatal("expected a running task to reject the terminal ready phase")
	}
	if err := UpdateHelmOperationTaskPhase(task.Id, HelmOperationPhaseInstalling); err != nil {
		t.Fatalf("advance task: %v", err)
	}
	if err := FinishHelmOperationTask(task.Id, true, ""); err != nil {
		t.Fatalf("finish task: %v", err)
	}

	stored, err := GetHelmOperationTask(task.Id)
	if err != nil {
		t.Fatalf("read task: %v", err)
	}
	if stored.Status != HelmOperationStatusSucceeded || stored.Phase != HelmOperationPhaseReady {
		t.Fatalf("unexpected terminal state: %s/%s", stored.Status, stored.Phase)
	}
	if stored.ActiveKey != nil {
		t.Fatalf("expected finished task to release its active key, got %q", *stored.ActiveKey)
	}
	if matches, err := HelmOperationTaskHasTerminalOutcomeContext(context.Background(), task.Id, true, ""); err != nil || !matches {
		t.Fatalf("verify successful terminal outcome: matches=%t err=%v", matches, err)
	}
	if matches, err := HelmOperationTaskHasTerminalOutcomeContext(context.Background(), task.Id, false, "failed"); err != nil || matches {
		t.Fatalf("mismatched terminal outcome must not be accepted: matches=%t err=%v", matches, err)
	}
	if err := FinishHelmOperationTask(task.Id, false, "should not overwrite completion"); !errors.Is(err, ErrHelmOperationAlreadyFinished) {
		t.Fatalf("expected a typed terminal-task error, got %v", err)
	}
	if _, err := CreateHelmOperationTask("admin", HelmOperationInstall, "demo", "default", "demo", "1.0.0"); err != nil {
		t.Fatalf("create replacement task after completion: %v", err)
	}
}

func TestHelmOperationActiveKeyEnforcesOneActiveTask(t *testing.T) {
	previousOrmer := ormer
	engine, err := xorm.NewEngine("sqlite", "file:helm-operation-active-key-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("create test engine: %v", err)
	}
	engine.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = engine.Close()
		ormer = previousOrmer
	})
	ormer = &Ormer{Engine: engine}

	if err := engine.Sync2(new(HelmOperationTask), new(HelmOperationLog)); err != nil {
		t.Fatalf("create task tables: %v", err)
	}
	first, err := CreateHelmOperationTask("admin", HelmOperationInstall, "demo", "default", "demo", "1.0.0")
	if err != nil {
		t.Fatalf("create first task: %v", err)
	}
	if first.ActiveKey == nil {
		t.Fatal("expected active task to have an arbitration key")
	}
	if _, err := CreateHelmOperationTask("other-admin", HelmOperationInstall, "demo", "default", "demo", "1.0.0"); !errors.Is(err, ErrHelmOperationAlreadyActive) {
		t.Fatalf("expected a typed active-task conflict, got %v", err)
	}
	duplicate := &HelmOperationTask{
		ActiveKey:   first.ActiveKey,
		Owner:       "admin",
		Operation:   HelmOperationInstall,
		ReleaseName: "demo",
		Namespace:   "default",
		ChartName:   "demo",
		Status:      HelmOperationStatusPending,
		Phase:       HelmOperationPhaseQueued,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if _, err := engine.Insert(duplicate); err == nil {
		t.Fatal("expected the database to reject a duplicate active key")
	}
}

func TestExpireStaleHelmOperationTask(t *testing.T) {
	previousOrmer := ormer
	engine, err := xorm.NewEngine("sqlite", "file:helm-operation-expiry-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("create test engine: %v", err)
	}
	engine.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = engine.Close()
		ormer = previousOrmer
	})
	ormer = &Ormer{Engine: engine}

	if err := engine.Sync2(new(HelmOperationTask), new(HelmOperationLog)); err != nil {
		t.Fatalf("create task tables: %v", err)
	}

	fresh, err := CreateHelmOperationTask("admin", HelmOperationInstall, "fresh", "default", "demo", "1.0.0")
	if err != nil {
		t.Fatalf("create fresh task: %v", err)
	}
	stale, err := CreateHelmOperationTask("admin", HelmOperationInstall, "stale", "default", "demo", "1.0.0")
	if err != nil {
		t.Fatalf("create stale task: %v", err)
	}
	old := time.Now().UTC().Add(-helmOperationStaleAfter - time.Minute)
	if _, err := engine.ID(stale.Id).Cols("updated_at").Update(&HelmOperationTask{UpdatedAt: old}); err != nil {
		t.Fatalf("age stale task: %v", err)
	}
	stale.UpdatedAt = old
	if IsHelmOperationTaskStale(fresh, time.Now().UTC()) {
		t.Fatal("fresh task was classified as stale")
	}
	if !IsHelmOperationTaskStale(stale, time.Now().UTC()) {
		t.Fatal("old active task was not classified as stale")
	}

	if err := ExpireStaleHelmOperationTask(fresh.Id); err != nil {
		t.Fatalf("check fresh task: %v", err)
	}
	if err := ExpireStaleHelmOperationTask(stale.Id); err != nil {
		t.Fatalf("expire stale task: %v", err)
	}
	fresh, _ = GetHelmOperationTask(fresh.Id)
	stale, _ = GetHelmOperationTask(stale.Id)
	if fresh.Status != HelmOperationStatusPending {
		t.Fatalf("fresh task was changed to %q", fresh.Status)
	}
	if stale.Status != HelmOperationStatusFailed || stale.Phase != HelmOperationPhaseFailed {
		t.Fatalf("stale task did not expire: %s/%s", stale.Status, stale.Phase)
	}
	if IsHelmOperationTaskStale(stale, time.Now().UTC()) {
		t.Fatal("terminal task must not be classified as stale")
	}
}

func TestHelmOperationRecorderFlushesLogsBeforeFinish(t *testing.T) {
	previousOrmer := ormer
	engine, err := xorm.NewEngine("sqlite", "file:helm-operation-recorder-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("create test engine: %v", err)
	}
	engine.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = engine.Close()
		ormer = previousOrmer
	})
	ormer = &Ormer{Engine: engine}

	if err := engine.Sync2(new(HelmOperationTask), new(HelmOperationLog)); err != nil {
		t.Fatalf("create task tables: %v", err)
	}
	task, err := CreateHelmOperationTask("admin", HelmOperationInstall, "recorded", "default", "demo", "1.0.0")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	recorder := NewHelmOperationRecorder(task.Id)
	if err := recorder.StartLoading(); err != nil {
		t.Fatalf("start task: %v", err)
	}
	if err := addHelmOperationLogs(task.Id, []*HelmOperationLog{{TaskId: task.Id + 1, Message: "wrong task"}}); err == nil {
		t.Fatal("expected a mismatched log task id to be rejected")
	}
	for _, line := range []string{"loading", "installing", "ERROR: failed"} {
		if err := recorder.RecordLog(line); err != nil {
			t.Fatalf("record log %q: %v", line, err)
		}
	}
	installErr := fmt.Errorf("failed")
	if err := recorder.Finish(installErr); err != nil {
		t.Fatalf("finish recorder: %v", err)
	}
	if err := recorder.Finish(nil); err != nil {
		t.Fatalf("finish recorder a second time: %v", err)
	}
	if err := recorder.RecordLog("late log"); err == nil {
		t.Fatal("expected a finished recorder to reject new logs")
	}

	logs, err := GetHelmOperationLogs(task.Id, 10)
	if err != nil {
		t.Fatalf("read logs: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 persisted logs, got %d", len(logs))
	}
	stored, err := GetHelmOperationTask(task.Id)
	if err != nil {
		t.Fatalf("read task: %v", err)
	}
	if stored.Status != HelmOperationStatusFailed || stored.ErrorMsg != installErr.Error() {
		t.Fatalf("unexpected terminal task: %s/%q", stored.Status, stored.ErrorMsg)
	}
}

func TestHelmOperationRecorderBoundsLogPersistence(t *testing.T) {
	recorder := NewHelmOperationRecorder(42)
	recorder.persistTimeout = 20 * time.Millisecond
	recorder.shutdownTimeout = 100 * time.Millisecond
	recorder.persistLogs = func(ctx context.Context, _ int64, _ []*HelmOperationLog) error {
		<-ctx.Done()
		return ctx.Err()
	}
	recorder.finishTask = func(_ context.Context, _ int64, success bool, errorMsg string) error {
		if !success || errorMsg != "" {
			t.Fatalf("log timeout must not change install outcome, success=%t error=%q", success, errorMsg)
		}
		return nil
	}

	if err := recorder.RecordLog("blocked"); err != nil {
		t.Fatalf("record log: %v", err)
	}
	started := time.Now()
	if err := recorder.Finish(nil); err != nil {
		t.Fatalf("log persistence must not replace a successful install outcome: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("finish remained blocked for %v", elapsed)
	}
	recorder.mu.Lock()
	batchErr := recorder.batchErr
	recorder.mu.Unlock()
	if batchErr == nil || !strings.Contains(batchErr.Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("expected an observable persistence timeout, got %v", batchErr)
	}
}

func TestGetHelmOperationLogsReturnsLatestEntriesInOrder(t *testing.T) {
	previousOrmer := ormer
	engine, err := xorm.NewEngine("sqlite", "file:helm-operation-log-window-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("create test engine: %v", err)
	}
	engine.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = engine.Close()
		ormer = previousOrmer
	})
	ormer = &Ormer{Engine: engine}
	if err := engine.Sync2(new(HelmOperationTask), new(HelmOperationLog)); err != nil {
		t.Fatalf("create task tables: %v", err)
	}
	task, err := CreateHelmOperationTask("admin", HelmOperationInstall, "logs", "default", "demo", "1.0.0")
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	entries := make([]*HelmOperationLog, 0, 5)
	for i := 1; i <= 5; i++ {
		entries = append(entries, &HelmOperationLog{TaskId: task.Id, Message: fmt.Sprintf("line %d", i)})
	}
	if err := addHelmOperationLogs(task.Id, entries); err != nil {
		t.Fatalf("persist logs: %v", err)
	}

	logs, err := GetHelmOperationLogs(task.Id, 2)
	if err != nil {
		t.Fatalf("get latest logs: %v", err)
	}
	if len(logs) != 2 || logs[0].Message != "line 4" || logs[1].Message != "line 5" {
		t.Fatalf("unexpected latest log window: %#v", logs)
	}
}

func TestHelmOperationRecorderRecoversPersistencePanic(t *testing.T) {
	recorder := NewHelmOperationRecorder(43)
	recorder.persistLogs = func(context.Context, int64, []*HelmOperationLog) error {
		panic("database driver panic")
	}
	recorder.finishTask = func(context.Context, int64, bool, string) error { return nil }

	if err := recorder.RecordLog("panic"); err != nil {
		t.Fatalf("record log: %v", err)
	}
	if err := recorder.Finish(nil); err != nil {
		t.Fatalf("log panic must not replace a successful install outcome: %v", err)
	}
	recorder.mu.Lock()
	batchErr := recorder.batchErr
	recorder.mu.Unlock()
	if batchErr == nil || !strings.Contains(batchErr.Error(), "database driver panic") {
		t.Fatalf("expected the persistence panic to remain observable, got %v", batchErr)
	}
}

func TestHelmOperationRecorderBoundsShutdownWhenPersistenceIgnoresContext(t *testing.T) {
	recorder := NewHelmOperationRecorder(46)
	recorder.shutdownTimeout = 20 * time.Millisecond
	blocked := make(chan struct{})
	recorder.persistLogs = func(context.Context, int64, []*HelmOperationLog) error {
		<-blocked
		return nil
	}
	recorder.finishTask = func(context.Context, int64, bool, string) error { return nil }
	if err := recorder.RecordLog("blocked driver"); err != nil {
		t.Fatalf("record log: %v", err)
	}
	started := time.Now()
	if err := recorder.Finish(nil); err != nil {
		t.Fatalf("finish task after bounded shutdown: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("shutdown remained blocked for %v", elapsed)
	}
	close(blocked)
}

func TestHelmOperationRecorderReturnsFinishRetryExhaustion(t *testing.T) {
	recorder := NewHelmOperationRecorder(47)
	recorder.finishRetryDelay = time.Millisecond
	var attempts atomic.Int32
	recorder.finishTask = func(context.Context, int64, bool, string) error {
		attempts.Add(1)
		return fmt.Errorf("database unavailable")
	}
	err := recorder.Finish(nil)
	if err == nil || !strings.Contains(err.Error(), "database unavailable") {
		t.Fatalf("expected terminal-state persistence error, got %v", err)
	}
	if got := attempts.Load(); got != helmOperationFinishAttempts {
		t.Fatalf("expected %d attempts, got %d", helmOperationFinishAttempts, got)
	}
}

func TestHelmOperationRecorderDoesNotRetryPermanentFinishError(t *testing.T) {
	recorder := NewHelmOperationRecorder(48)
	var attempts atomic.Int32
	recorder.finishTask = func(context.Context, int64, bool, string) error {
		attempts.Add(1)
		return ErrHelmOperationAlreadyFinished
	}
	recorder.terminalMatches = func(context.Context, int64, bool, string) (bool, error) {
		return false, nil
	}
	err := recorder.Finish(nil)
	if !errors.Is(err, ErrHelmOperationAlreadyFinished) {
		t.Fatalf("expected permanent finish error, got %v", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected one attempt for a permanent error, got %d", got)
	}
}

func TestHelmOperationRecorderAcceptsMatchingExistingTerminalState(t *testing.T) {
	recorder := NewHelmOperationRecorder(49)
	var attempts atomic.Int32
	recorder.finishTask = func(context.Context, int64, bool, string) error {
		attempts.Add(1)
		return ErrHelmOperationAlreadyFinished
	}
	recorder.terminalMatches = func(_ context.Context, id int64, success bool, errorMsg string) (bool, error) {
		return id == 49 && success && errorMsg == "", nil
	}
	if err := recorder.Finish(nil); err != nil {
		t.Fatalf("matching terminal state should be idempotent: %v", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("expected one idempotent finish attempt, got %d", got)
	}
}

func TestHelmOperationRecorderRetriesTerminalState(t *testing.T) {
	recorder := NewHelmOperationRecorder(44)
	recorder.persistLogs = func(context.Context, int64, []*HelmOperationLog) error { return nil }
	recorder.finishRetryDelay = time.Millisecond
	var attempts atomic.Int32
	recorder.finishTask = func(_ context.Context, _ int64, success bool, errorMsg string) error {
		if !success || errorMsg != "" {
			t.Fatalf("unexpected terminal state: success=%t error=%q", success, errorMsg)
		}
		if attempts.Add(1) < 3 {
			return fmt.Errorf("temporary database failure")
		}
		return nil
	}

	if err := recorder.Finish(nil); err != nil {
		t.Fatalf("finish after retry: %v", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("expected 3 finish attempts, got %d", got)
	}
}

func TestHelmOperationRecorderConcurrentWritersAndErrorLevel(t *testing.T) {
	recorder := NewHelmOperationRecorder(45)
	var mu sync.Mutex
	var persisted []*HelmOperationLog
	recorder.persistLogs = func(_ context.Context, _ int64, logs []*HelmOperationLog) error {
		mu.Lock()
		persisted = append(persisted, logs...)
		mu.Unlock()
		return nil
	}
	recorder.finishTask = func(context.Context, int64, bool, string) error { return nil }

	const writers = 40
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			prefix := "INFO:"
			if i == 0 {
				prefix = " error:"
			}
			if err := recorder.RecordLog(fmt.Sprintf("%s line %d", prefix, i)); err != nil {
				t.Errorf("record log %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	if err := recorder.Finish(nil); err != nil {
		t.Fatalf("finish recorder: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(persisted) != writers {
		t.Fatalf("expected %d persisted logs, got %d", writers, len(persisted))
	}
	foundError := false
	for _, entry := range persisted {
		if strings.HasPrefix(strings.TrimSpace(entry.Message), "error:") && entry.Level == HelmOperationLogLevelError {
			foundError = true
		}
	}
	if !foundError {
		t.Fatal("expected case-insensitive error prefix to use error level")
	}
}
