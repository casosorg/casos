package object

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/beego/beego/logs"
)

const (
	helmOperationLogBatchSize      = 50
	helmOperationLogFlushInterval  = 200 * time.Millisecond
	helmOperationLogEnqueueTimeout = 2 * time.Second
	helmOperationLogPersistTimeout = HelmOperationPersistenceTimeout
	helmOperationRecorderShutdown  = 3*helmOperationLogPersistTimeout + time.Second
	helmOperationRecorderStopGrace = 100 * time.Millisecond
	helmOperationFinishTimeout     = HelmOperationPersistenceTimeout
	helmOperationFinishAttempts    = 3
	helmOperationFinishRetryDelay  = 100 * time.Millisecond
	helmOperationHeartbeatInterval = time.Minute
	helmOperationCancelPollInterval = time.Second
)

type HelmOperationRecorder struct {
	taskID int64
	queue  chan *HelmOperationLog
	done   chan struct{}

	mu        sync.Mutex
	closed    bool
	batchErr  error
	writers   sync.WaitGroup
	finish    sync.Once
	finishErr error

	persistLogs       func(context.Context, int64, []*HelmOperationLog) error
	finishTask        func(context.Context, int64, bool, string) error
	terminalMatches   func(context.Context, int64, bool, string) (bool, error)
	touchTask         func(context.Context, int64) error
	persistTimeout    time.Duration
	heartbeatInterval time.Duration
	shutdownTimeout   time.Duration
	finishTimeout     time.Duration
	finishRetryDelay  time.Duration
	persistCtx        context.Context
	cancelPersist     context.CancelFunc
	cancelCh          chan struct{}
	cancelPollOnce    sync.Once
}

func NewHelmOperationRecorder(taskID int64) *HelmOperationRecorder {
	persistCtx, cancelPersist := context.WithCancel(context.Background())
	recorder := &HelmOperationRecorder{
		taskID:            taskID,
		queue:             make(chan *HelmOperationLog, helmOperationLogBatchSize*2),
		done:              make(chan struct{}),
		cancelCh:          make(chan struct{}),
		persistLogs:       addHelmOperationLogsContext,
		finishTask:        FinishHelmOperationTaskContext,
		terminalMatches:   HelmOperationTaskHasTerminalOutcomeContext,
		touchTask:         TouchHelmOperationTaskContext,
		persistTimeout:    helmOperationLogPersistTimeout,
		heartbeatInterval: helmOperationHeartbeatInterval,
		shutdownTimeout:   helmOperationRecorderShutdown,
		finishTimeout:     helmOperationFinishTimeout,
		finishRetryDelay:  helmOperationFinishRetryDelay,
		persistCtx:        persistCtx,
		cancelPersist:     cancelPersist,
	}
	go recorder.run()
	go recorder.pollCancel()
	return recorder
}

// Cancelled returns a channel closed when a cancel has been requested for
// the underlying task. The installer selects on it to abort the Helm run.
func (r *HelmOperationRecorder) Cancelled() <-chan struct{} {
	return r.cancelCh
}

func (r *HelmOperationRecorder) pollCancel() {
	ticker := time.NewTicker(helmOperationCancelPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.persistCtx.Done():
			return
		case <-ticker.C:
			cancelling, err := HelmOperationTaskIsCancelling(r.taskID)
			if err != nil {
				logs.Warning("poll Helm operation task %d cancel state: %v", r.taskID, err)
				continue
			}
			if cancelling {
				r.cancelPollOnce.Do(func() { close(r.cancelCh) })
				return
			}
		}
	}
}

func (r *HelmOperationRecorder) StartLoading() error {
	ctx, cancel := context.WithTimeout(context.Background(), r.persistTimeout)
	defer cancel()
	return StartHelmOperationTaskContext(ctx, r.taskID, HelmOperationPhaseLoading)
}

func (r *HelmOperationRecorder) MarkInstalling() error {
	ctx, cancel := context.WithTimeout(context.Background(), r.persistTimeout)
	defer cancel()
	return UpdateHelmOperationTaskPhaseContext(ctx, r.taskID, HelmOperationPhaseInstalling)
}

func (r *HelmOperationRecorder) RecordLog(line string) error {
	level := HelmOperationLogLevelInfo
	trimmed := strings.TrimSpace(line)
	if len(trimmed) >= len("ERROR:") && strings.EqualFold(trimmed[:len("ERROR:")], "ERROR:") {
		level = HelmOperationLogLevelError
	}
	entry := &HelmOperationLog{
		TaskId:    r.taskID,
		Level:     level,
		Message:   line,
		CreatedAt: time.Now().UTC(),
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return fmt.Errorf("Helm operation recorder is closed")
	}
	r.writers.Add(1)
	timer := time.NewTimer(helmOperationLogEnqueueTimeout)
	defer timer.Stop()
	select {
	case r.queue <- entry:
		r.writers.Done()
		r.mu.Unlock()
		return nil
	case <-timer.C:
		r.writers.Done()
		r.mu.Unlock()
		return fmt.Errorf("Helm operation log queue is full")
	}
}

func (r *HelmOperationRecorder) Finish(installErr error) error {
	r.finish.Do(func() {
		r.mu.Lock()
		r.closed = true
		r.writers.Wait()
		close(r.queue)
		r.mu.Unlock()
		shutdownTimer := time.NewTimer(r.shutdownTimeout)
		select {
		case <-r.done:
			shutdownTimer.Stop()
			r.cancelPersist()
		case <-shutdownTimer.C:
			shutdownTimer.Stop()
			r.setBatchErr(fmt.Errorf("timed out waiting for Helm operation log persistence"))
			r.cancelPersist()
			stopTimer := time.NewTimer(helmOperationRecorderStopGrace)
			select {
			case <-r.done:
				stopTimer.Stop()
			case <-stopTimer.C:
				stopTimer.Stop()
				// A database driver that ignores context cancellation may leave its
				// call running; task completion remains bounded and the timeout is logged.
				logs.Warning("Helm operation task %d log persistence did not stop after cancellation", r.taskID)
			}
		}

		r.mu.Lock()
		batchErr := r.batchErr
		r.mu.Unlock()
		// Log durability is observable but does not change the outcome of a Helm
		// install that already completed in the cluster.
		success := installErr == nil
		errorMsg := ""
		if installErr != nil {
			errorMsg = installErr.Error()
		}
		finishErr := r.finishTaskWithRetry(success, errorMsg)
		if batchErr != nil {
			logs.Warning("persist Helm operation task %d logs: %v", r.taskID, batchErr)
			if finishErr != nil {
				r.finishErr = fmt.Errorf("persist Helm operation logs: %v; finish task: %w", batchErr, finishErr)
				return
			}
		}
		r.finishErr = finishErr
	})
	return r.finishErr
}

func (r *HelmOperationRecorder) finishTaskWithRetry(success bool, errorMsg string) error {
	// The install intentionally outlives the browser request, so terminal-state
	// persistence uses its own bounded deadline instead of the request context.
	var err error
	for attempt := 0; attempt < helmOperationFinishAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), r.finishTimeout)
		err = r.finishTask(ctx, r.taskID, success, errorMsg)
		if err == nil {
			cancel()
			return nil
		}
		if errors.Is(err, ErrHelmOperationAlreadyFinished) {
			cancel()
			matchCtx, matchCancel := context.WithTimeout(context.Background(), r.finishTimeout)
			matches, matchErr := r.terminalMatches(matchCtx, r.taskID, success, errorMsg)
			matchCancel()
			if matchErr != nil {
				return fmt.Errorf("verify existing Helm operation terminal state: %w", matchErr)
			}
			if matches {
				return nil
			}
			return err
		}
		cancel()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if attempt+1 < helmOperationFinishAttempts {
			time.Sleep(r.finishRetryDelay)
		}
	}
	return err
}

func (r *HelmOperationRecorder) setBatchErr(err error) {
	if err == nil {
		return
	}
	r.mu.Lock()
	if r.batchErr == nil {
		r.batchErr = err
	}
	r.mu.Unlock()
}

func (r *HelmOperationRecorder) run() {
	defer func() {
		if recovered := recover(); recovered != nil {
			stack := debug.Stack()
			logs.Error("persist Helm operation task %d logs panic: %v\n%s", r.taskID, recovered, stack)
			r.setBatchErr(fmt.Errorf("persist Helm operation logs panic: %v", recovered))
		}
		close(r.done)
	}()
	ticker := time.NewTicker(helmOperationLogFlushInterval)
	defer ticker.Stop()
	heartbeatTicker := time.NewTicker(r.heartbeatInterval)
	defer heartbeatTicker.Stop()
	batch := make([]*HelmOperationLog, 0, helmOperationLogBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		logs := append([]*HelmOperationLog(nil), batch...)
		ctx, cancel := context.WithTimeout(r.persistCtx, r.persistTimeout)
		err := r.persistLogs(ctx, r.taskID, logs)
		cancel()
		if err != nil {
			r.setBatchErr(err)
		}
		batch = make([]*HelmOperationLog, 0, helmOperationLogBatchSize)
	}

	for {
		select {
		case entry, ok := <-r.queue:
			if !ok {
				flush()
				return
			}
			batch = append(batch, entry)
			if len(batch) >= helmOperationLogBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-heartbeatTicker.C:
			ctx, cancel := context.WithTimeout(r.persistCtx, r.persistTimeout)
			err := r.touchTask(ctx, r.taskID)
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				logs.Warning("refresh Helm operation task %d heartbeat: %v", r.taskID, err)
			}
		}
	}
}
