package object

import (
	"context"
	"testing"
	"time"

	_ "modernc.org/sqlite"
	"xorm.io/xorm"
)

func newHelmOperationTestEngine(t *testing.T) *xorm.Engine {
	t.Helper()
	engine, err := xorm.NewEngine("sqlite", "file:helm_operation_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open test sqlite engine: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	engine.SetMaxOpenConns(1)
	if err := engine.Sync2(new(HelmOperationTask), new(HelmOperationLog)); err != nil {
		t.Fatalf("sync test schema: %v", err)
	}
	return engine
}

func insertTestHelmOperationTask(t *testing.T, status, phase string) int64 {
	t.Helper()
	task := &HelmOperationTask{
		Operation: "install", ReleaseName: "demo", Namespace: "default",
		ChartName: "demo-chart", Status: status, Phase: phase,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if _, err := ormer.Engine.Insert(task); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	return task.Id
}

func TestHelmOperationCancelFlow(t *testing.T) {
	prevOrmer := ormer
	ormer = &Ormer{Engine: newHelmOperationTestEngine(t)}
	defer func() { ormer = prevOrmer }()
	ctx := context.Background()
	id := insertTestHelmOperationTask(t, HelmOperationStatusPending, HelmOperationPhaseQueued)

	if err := StartHelmOperationTaskContext(ctx, id, HelmOperationPhaseLoading); err != nil {
		t.Fatalf("start task: %v", err)
	}
	if err := RequestHelmOperationCancel(id); err != nil {
		t.Fatalf("request cancel: %v", err)
	}
	if err := UpdateHelmOperationTaskPhaseContext(ctx, id, HelmOperationPhaseInstalling); err != nil {
		t.Fatalf("mark installing during cancelling should not error: %v", err)
	}
	if err := TouchHelmOperationTaskContext(ctx, id); err != nil {
		t.Fatalf("touch during cancelling: %v", err)
	}
	if err := FinishHelmOperationTaskContext(ctx, id, false, "install cancelled by user"); err != nil {
		t.Fatalf("finish cancelled task: %v", err)
	}
	final := &HelmOperationTask{Id: id}
	if found, err := ormer.Engine.Get(final); err != nil || !found {
		t.Fatalf("get task: found=%v err=%v", found, err)
	}
	if final.Status != HelmOperationStatusCancelled || final.Phase != HelmOperationPhaseFailed || final.ErrorMsg != "install cancelled by user" {
		t.Fatalf("unexpected terminal state: status=%q phase=%q error=%q", final.Status, final.Phase, final.ErrorMsg)
	}
	matched, err := HelmOperationTaskHasTerminalOutcomeContext(ctx, id, false, "install cancelled by user")
	if err != nil || !matched {
		t.Fatalf("terminal outcome should match cancelled task: matched=%v err=%v", matched, err)
	}
}

func TestHelmOperationLogsDuringCancelling(t *testing.T) {
	prevOrmer := ormer
	ormer = &Ormer{Engine: newHelmOperationTestEngine(t)}
	defer func() { ormer = prevOrmer }()
	ctx := context.Background()
	id := insertTestHelmOperationTask(t, HelmOperationStatusPending, HelmOperationPhaseQueued)
	if err := StartHelmOperationTaskContext(ctx, id, HelmOperationPhaseLoading); err != nil {
		t.Fatalf("start task: %v", err)
	}
	if err := RequestHelmOperationCancel(id); err != nil {
		t.Fatalf("request cancel: %v", err)
	}
	entries := []*HelmOperationLog{{
		TaskId: id, Level: HelmOperationLogLevelInfo,
		Message: "still logging during cancelling", CreatedAt: time.Now().UTC(),
	}}
	if err := addHelmOperationLogsContext(ctx, id, entries); err != nil {
		t.Fatalf("persist logs during cancelling: %v", err)
	}
	logs, err := GetHelmOperationLogs(id, 10)
	if err != nil || len(logs) != 1 || logs[0].Message != "still logging during cancelling" {
		t.Fatalf("unexpected logs: len=%d err=%v", len(logs), err)
	}
}

func TestHelmOperationRecorderCancelPoll(t *testing.T) {
	prevOrmer := ormer
	ormer = &Ormer{Engine: newHelmOperationTestEngine(t)}
	defer func() { ormer = prevOrmer }()
	id := insertTestHelmOperationTask(t, HelmOperationStatusPending, HelmOperationPhaseQueued)
	rec := NewHelmOperationRecorder(id)
	defer func() {
		_ = rec.Finish(nil)
	}()
	if err := rec.StartLoading(); err != nil {
		t.Fatalf("start loading: %v", err)
	}
	if err := RequestHelmOperationCancel(id); err != nil {
		t.Fatalf("request cancel: %v", err)
	}
	select {
	case <-rec.Cancelled():
	case <-time.After(5 * time.Second):
		t.Fatal("cancel channel not closed within 5s")
	}
}
