package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// blockingLifecycleRepo blocks GetTask until release is closed, letting a
// test hold reconcileTaskLifecycleTokens open long enough to observe the
// stuck-sweep warning.
type blockingLifecycleRepo struct {
	sessionExecutorStore
	release <-chan struct{}
}

func (r *blockingLifecycleRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	<-r.release
	return r.sessionExecutorStore.GetTask(ctx, id)
}

func (r *blockingLifecycleRepo) ListTasksWithMetadataKey(ctx context.Context, key string) ([]*models.Task, error) {
	return r.sessionExecutorStore.(interface {
		ListTasksWithMetadataKey(context.Context, string) ([]*models.Task, error)
	}).ListTasksWithMetadataKey(ctx, key)
}

func observedTestLogger(t *testing.T) (*commonlogger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := commonlogger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observed logger: %v", err)
	}
	return log, logs
}

// observedTestLoggerWatching is observedTestLogger plus a channel that closes
// the first time a log entry with the given message is emitted, so a test can
// synchronize on the log itself instead of polling with time.Sleep.
func observedTestLoggerWatching(t *testing.T, message string) (*commonlogger.Logger, *observer.ObservedLogs, <-chan struct{}) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)

	seen := make(chan struct{})
	var once sync.Once
	hooked := zapcore.RegisterHooks(core, func(entry zapcore.Entry) error {
		if entry.Message == message {
			once.Do(func() { close(seen) })
		}
		return nil
	})

	log, err := commonlogger.NewFromZap(zap.New(hooked))
	if err != nil {
		t.Fatalf("create observed logger: %v", err)
	}
	return log, logs, seen
}

// TestReconcileTaskLifecycleTokensLogsStartAndFinish is the regression test
// for docs/plans/startup-listener-before-recovery/task-03-ordering-guard.md:
// a long startup sweep must be diagnosable from logs (task count, elapsed
// time) rather than requiring a goroutine dump.
//
// Expected pre-fix failure: reconcileTaskLifecycleTokens logs neither
// message today, so both FilterMessage results are empty.
func TestReconcileTaskLifecycleTokensLogsStartAndFinish(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "sweep-log-task", "sweep-log-session", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "sweep-log-task", models.MetaKeyQueuePromotionPending, true); err != nil {
		t.Fatalf("seed promotion token: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	log, logs := observedTestLogger(t)
	svc.logger = log

	svc.reconcileTaskLifecycleTokens(ctx)

	starts := logs.FilterMessage("startup lifecycle sweep starting").All()
	if len(starts) != 1 {
		t.Fatalf("start logs = %#v, want exactly one", starts)
	}
	if got := fmt.Sprint(starts[0].ContextMap()["task_count"]); got != "1" {
		t.Fatalf("start log task_count = %v, want 1", starts[0].ContextMap()["task_count"])
	}

	finishes := logs.FilterMessage("startup lifecycle sweep finished").All()
	if len(finishes) != 1 {
		t.Fatalf("finish logs = %#v, want exactly one", finishes)
	}
	if got := fmt.Sprint(finishes[0].ContextMap()["task_count"]); got != "1" {
		t.Fatalf("finish log task_count = %v, want 1", finishes[0].ContextMap()["task_count"])
	}
	if _, ok := finishes[0].ContextMap()["elapsed"]; !ok {
		t.Fatalf("finish log missing elapsed field: %#v", finishes[0].ContextMap())
	}
}

// TestReconcileTaskLifecycleTokensWarnsWhenStuck is the regression test for
// the "still recovering N tasks" log-only signal: a sweep still running
// after lifecycleSweepStuckWarningInterval must warn instead of staying
// silent, and must never cancel the in-flight recovery (context.WithoutCancel
// in task_operations.go means cancellation would not work anyway).
//
// Expected pre-fix failure: lifecycleSweepStuckWarningInterval does not
// exist yet, so this fails to compile.
func TestReconcileTaskLifecycleTokensWarnsWhenStuck(t *testing.T) {
	prevInterval := lifecycleSweepStuckWarningInterval
	lifecycleSweepStuckWarningInterval = 20 * time.Millisecond
	t.Cleanup(func() { lifecycleSweepStuckWarningInterval = prevInterval })

	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "stuck-task", "stuck-session", "step1")
	if err := repo.SetTaskMetadataKey(ctx, "stuck-task", models.MetaKeyQueuePromotionPending, true); err != nil {
		t.Fatalf("seed promotion token: %v", err)
	}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	log, _, warned := observedTestLoggerWatching(t, "startup lifecycle sweep still running")
	svc.logger = log

	release := make(chan struct{})
	svc.repo = &blockingLifecycleRepo{sessionExecutorStore: repo, release: release}

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.reconcileTaskLifecycleTokens(ctx)
	}()

	select {
	case <-warned:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("stuck-sweep warning never logged")
	}

	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sweep did not finish after release")
	}
}
