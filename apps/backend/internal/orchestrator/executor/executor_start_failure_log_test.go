package executor

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func newObservedExecutor(t *testing.T, level zapcore.Level) (*Executor, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(level)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	repo := newMockRepository()
	repo.sessions["session-123"] = &models.TaskSession{
		ID: "session-123", TaskID: "task-123", State: models.TaskSessionStateStarting,
	}
	repo.tasks["task-123"] = &models.Task{ID: "task-123", State: v1.TaskStateTODO}
	exec := NewExecutor(&mockAgentManager{}, repo, log, ExecutorConfig{
		ShellPrefs: &mockShellPrefs{},
	})
	exec.SetCapabilities(&mockCapabilities{})
	// Avoid side-effect callbacks changing the state under test.
	exec.SetOnAgentStartFailed(func(context.Context, string, string, string, error, bool) bool {
		return true
	})
	return exec, logs
}

func TestHandleAgentProcessStartFailure_LogsWarnForTeardown(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"context canceled", context.Canceled},
		{"terminal session", lifecycle.ErrSessionTerminal},
		{"wrapped terminal", errors.Join(errors.New("start failed"), lifecycle.ErrSessionTerminal)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec, logs := newObservedExecutor(t, zapcore.WarnLevel)
			exec.handleAgentProcessStartFailure(
				context.Background(),
				"task-123", "session-123", "exec-456",
				tt.err, true, false,
			)
			warns := logs.FilterMessage("agent process start aborted by session teardown").All()
			if len(warns) != 1 {
				t.Fatalf("warn entries = %d, want 1; all=%v", len(warns), logs.All())
			}
			if errs := logs.FilterLevelExact(zapcore.ErrorLevel).Len(); errs != 0 {
				t.Fatalf("error entries = %d, want 0", errs)
			}
		})
	}
}

func TestHandleAgentProcessStartFailure_LogsErrorForStartupDeadline(t *testing.T) {
	exec, logs := newObservedExecutor(t, zapcore.WarnLevel)
	exec.handleAgentProcessStartFailure(
		context.Background(),
		"task-123", "session-123", "exec-456",
		context.DeadlineExceeded, true, false,
	)
	errs := logs.FilterMessage("failed to start agent process").All()
	if len(errs) != 1 {
		t.Fatalf("error entries = %d, want 1; all=%v", len(errs), logs.All())
	}
	if got := errs[0].Level; got != zapcore.ErrorLevel {
		t.Fatalf("level = %v, want error", got)
	}
	if warns := logs.FilterMessage("agent process start aborted by session teardown").Len(); warns != 0 {
		t.Fatalf("warn teardown entries = %d, want 0", warns)
	}
}

func TestHandleAgentProcessStartFailure_RetriesTransientOwnershipRead(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-123"] = &models.TaskSession{
		ID: "session-123", TaskID: "task-123", State: models.TaskSessionStateStarting,
	}
	lookupCalls := 0
	repo.getTaskSessionFunc = func(_ context.Context, _ string) (*models.TaskSession, error) {
		lookupCalls++
		if lookupCalls == 2 {
			return nil, errors.New("transient session lookup failure")
		}
		return cloneMockTaskSession(repo.sessions["session-123"]), nil
	}
	var stopCalls int
	exec := newTestExecutor(t, &mockAgentManager{
		stopAgentFunc: func(context.Context, string, bool) error {
			stopCalls++
			return nil
		},
	}, repo)
	exec.SetOnSessionStateChange(func(
		ctx context.Context,
		_ string,
		sessionID string,
		state models.TaskSessionState,
		errorMessage string,
	) error {
		return repo.UpdateTaskSessionState(ctx, sessionID, state, errorMessage)
	})

	exec.handleAgentProcessStartFailure(
		context.Background(),
		"task-123",
		"session-123",
		"exec-456",
		errors.New("start failed"),
		false,
		false,
	)

	if lookupCalls < 4 {
		t.Fatalf("session lookup calls = %d, want initial check, retry, and commit reads", lookupCalls)
	}
	if got := repo.sessions["session-123"].State; got != models.TaskSessionStateFailed {
		t.Fatalf("session state = %s, want FAILED", got)
	}
	if stopCalls != 1 {
		t.Fatalf("StopAgent calls = %d, want 1", stopCalls)
	}
}
