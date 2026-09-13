package scheduler

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/service"
)

// newObservedTestScheduler wires a SchedulerService whose logger is
// backed by a zaptest/observer core, so tests can assert on the LEVEL a
// message was logged at instead of only whether an error was returned —
// the system design requires the typed paused error to log at DEBUG at
// the reactivity and approval-adapter call sites, while a genuine
// QueueRunCtx failure keeps its present level there.
func newObservedTestScheduler(t *testing.T, repo *officesqlite.Repository) (*SchedulerService, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("observer logger: %v", err)
	}
	svc := service.NewService(service.ServiceOptions{Repo: repo, Logger: log})
	return NewSchedulerService(repo, log, svc), logs
}

// TestReactivity_ApplyTaskMutation_BlockedByPause_LogsDebugNotError proves
// that ApplyTaskMutation's queue closure branches a typed
// shared.ErrWorkspacePaused to a DEBUG log, not the ERROR level it uses
// for every other QueueRunCtx failure (reactivity.go's queue closure).
func TestReactivity_ApplyTaskMutation_BlockedByPause_LogsDebugNotError(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss, logs := newObservedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ss.SetPauseGate(&fakeSchedulerPauseGate{active: []*models.WorkspacePause{{ID: "pause-1", WorkspaceID: "ws-1"}}})

	newStatus := statusTodo
	task := &TaskSnapshot{ID: "task-1", WorkspaceID: "ws-1", State: "BLOCKED", AssigneeAgentProfileID: "agent-1"}
	change := TaskMutation{NewStatus: &newStatus, ActorID: "user-1", ActorType: "user"}

	if _, err := ss.ApplyTaskMutation(context.Background(), task, change); err != nil {
		t.Fatalf("ApplyTaskMutation: %v", err)
	}

	if n := logs.FilterMessage("reactivity run skipped (workspace paused)").Len(); n != 1 {
		t.Fatalf("debug pause-skip log entries = %d, want 1", n)
	}
	if n := logs.FilterLevelExact(zapcore.DebugLevel).Len(); n != 1 {
		t.Fatalf("debug-level log entries = %d, want 1", n)
	}
	if n := logs.FilterLevelExact(zapcore.ErrorLevel).Len(); n != 0 {
		t.Fatalf("error-level log entries = %d, want 0 for a confirmed pause", n)
	}
}

// TestReactivity_ApplyTaskMutation_GenuineFailure_LogsErrorNotDebug is the
// sibling regression guard: a QueueRunCtx failure that is NOT a
// workspace pause (here, an unresolvable agent) must still log at ERROR,
// proving the DEBUG branch above is a narrow carve-out and not a general
// swallow of reactivity failures.
func TestReactivity_ApplyTaskMutation_GenuineFailure_LogsErrorNotDebug(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss, logs := newObservedTestScheduler(t, repo)
	// Deliberately no agent-1 row: guardAgentStatus fails with a plain
	// "agent not found" error before the pause gate is ever consulted.

	newStatus := statusTodo
	task := &TaskSnapshot{ID: "task-1", WorkspaceID: "ws-1", State: "BLOCKED", AssigneeAgentProfileID: "agent-1"}
	change := TaskMutation{NewStatus: &newStatus, ActorID: "user-1", ActorType: "user"}

	if _, err := ss.ApplyTaskMutation(context.Background(), task, change); err != nil {
		t.Fatalf("ApplyTaskMutation: %v", err)
	}

	if n := logs.FilterMessage("reactivity run failed").Len(); n != 1 {
		t.Fatalf("error-level reactivity-failed log entries = %d, want 1", n)
	}
	if n := logs.FilterLevelExact(zapcore.ErrorLevel).Len(); n != 1 {
		t.Fatalf("error-level log entries = %d, want 1", n)
	}
	if n := logs.FilterLevelExact(zapcore.DebugLevel).Len(); n != 0 {
		t.Fatalf("debug-level log entries = %d, want 0 for a non-pause failure", n)
	}
}

// TestApprovalAdapter_QueueApprovalRuns_BlockedByPause_LogsDebugNotWarn
// proves the same DEBUG-vs-present-level branch at the second shared call
// site: DashboardApprovalAdapter.QueueApprovalRuns logs a confirmed pause
// at DEBUG rather than the WARN level it uses for every other failure.
func TestApprovalAdapter_QueueApprovalRuns_BlockedByPause_LogsDebugNotWarn(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss, logs := newObservedTestScheduler(t, repo)
	createChildrenCompletedAgent(t, repo, "agent-1")
	ss.SetPauseGate(&fakeSchedulerPauseGate{active: []*models.WorkspacePause{{ID: "pause-1", WorkspaceID: "ws-1"}}})
	adapter := NewDashboardApprovalAdapter(ss)

	err := adapter.QueueApprovalRuns(context.Background(), []dashboard.ApprovalRun{
		{AgentID: "agent-1", Reason: RunReasonTaskReviewRequested, TaskID: "task-1", WorkspaceID: "ws-1"},
	})
	if err != nil {
		t.Fatalf("QueueApprovalRuns: %v", err)
	}

	// Scope to the adapter's own "approval run ..." messages rather than
	// every WARN in the log: service.NewService itself logs an unrelated
	// startup WARN (missing TaskStarter) on the same shared logger.
	approvalLogs := logs.FilterMessageSnippet("approval run")
	if n := approvalLogs.Len(); n != 1 {
		t.Fatalf("approval-run log entries = %d, want 1", n)
	}
	if n := approvalLogs.FilterMessageSnippet("approval run skipped (workspace paused)").FilterLevelExact(zapcore.DebugLevel).Len(); n != 1 {
		t.Fatalf("debug pause-skip log entries = %d, want 1", n)
	}
	if n := approvalLogs.FilterLevelExact(zapcore.WarnLevel).Len(); n != 0 {
		t.Fatalf("warn-level approval-run log entries = %d, want 0 for a confirmed pause", n)
	}
}

// TestApprovalAdapter_QueueApprovalRuns_GenuineFailure_LogsWarnNotDebug is
// the approval-adapter sibling of the reactivity regression guard: a
// non-pause QueueRunCtx failure must still log at WARN.
func TestApprovalAdapter_QueueApprovalRuns_GenuineFailure_LogsWarnNotDebug(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss, logs := newObservedTestScheduler(t, repo)
	// Deliberately no agent-1 row: guardAgentStatus fails before the
	// pause gate is consulted.
	adapter := NewDashboardApprovalAdapter(ss)

	err := adapter.QueueApprovalRuns(context.Background(), []dashboard.ApprovalRun{
		{AgentID: "agent-1", Reason: RunReasonTaskReviewRequested, TaskID: "task-1", WorkspaceID: "ws-1"},
	})
	if err != nil {
		t.Fatalf("QueueApprovalRuns: %v", err)
	}

	// Scope to the adapter's own "approval run ..." messages rather than
	// every WARN in the log: service.NewService itself logs an unrelated
	// startup WARN (missing TaskStarter) on the same shared logger.
	approvalLogs := logs.FilterMessageSnippet("approval run")
	if n := approvalLogs.Len(); n != 1 {
		t.Fatalf("approval-run log entries = %d, want 1", n)
	}
	if n := approvalLogs.FilterMessageSnippet("approval run failed").FilterLevelExact(zapcore.WarnLevel).Len(); n != 1 {
		t.Fatalf("warn-level approval-failed log entries = %d, want 1", n)
	}
	if n := approvalLogs.FilterLevelExact(zapcore.DebugLevel).Len(); n != 0 {
		t.Fatalf("debug-level approval-run log entries = %d, want 0 for a non-pause failure", n)
	}
}
