package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/pause"
	"github.com/kandev/kandev/internal/office/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// fakeWorkspaceChecker satisfies pause.WorkspaceChecker with a fixed set
// of known workspace ids, mirroring pause/integration_test.go's fixture.
type fakeWorkspaceChecker struct{ known map[string]bool }

func (f *fakeWorkspaceChecker) GetWorkspace(_ context.Context, id string) (*taskmodels.Workspace, error) {
	if !f.known[id] {
		return nil, repoerrors.ErrWorkspaceNotFound
	}
	return &taskmodels.Workspace{ID: id}, nil
}

// noopTaskCanceller satisfies pause.TaskCanceller as a no-op: this
// regression is about deferred *assignment* replay, not the halt sweep's
// cancellation behaviour.
type noopTaskCanceller struct{}

func (noopTaskCanceller) CancelTaskExecution(_ context.Context, _ string, _ string, _ bool) error {
	return nil
}

// countQueuedRuns returns the number of rows in the runs table for the
// given agent + reason, regardless of status.
func countQueuedRuns(t *testing.T, svc *service.Service, agentProfileID, reason string) int {
	t.Helper()
	var n int
	if err := svc.RepoForTest().ReaderDB().QueryRowx(
		`SELECT COUNT(*) FROM runs WHERE agent_profile_id = ? AND reason = ?`,
		agentProfileID, reason,
	).Scan(&n); err != nil {
		t.Fatalf("count queued runs: %v", err)
	}
	return n
}

// TestBetaPausedAssignmentResume is the ISSUE-8 regression matrix: a task
// assignment made while a workspace is paused must be replayed once the
// workspace resumes, instead of being silently lost.
//
// Root cause: checkPauseGateForAgent correctly blocks queueTaskAssignedRun
// while paused (ErrWorkspacePaused), but nothing records that a deferred
// assignment occurred, and pause.Service.Resume does nothing but flip the
// pause record — there is no consumer of the blocked occurrence, so the
// assignee never gets a task_assigned run.
func TestBetaPausedAssignmentResume(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	pauseSvc := pause.NewService(
		svc.RepoForTest(),
		noopTaskCanceller{},
		&fakeWorkspaceChecker{known: map[string]bool{"ws-1": true}},
		logger.Default(),
	)
	svc.SetPauseGate(pauseSvc)
	pauseSvc.SetAssignmentReplayer(svc)

	svc.ExecSQL(t, `INSERT INTO workspaces (id) VALUES ('ws-1')`)
	createTestAgent(t, svc, "ws-1", "agent-1")
	svc.ExecSQL(t,
		`INSERT INTO tasks (id, workspace_id, project_id, assignment_generation) VALUES (?, ?, 'proj-1', 1)`,
		"task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "agent-1")

	if _, err := pauseSvc.Pause(ctx, "ws-1", "regression test", "user-1", "user"); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	gen := int64(1)
	if err := svc.QueueTaskAssignedRunForTest(ctx, "task-1", "agent-1", &gen); err != nil {
		t.Fatalf("QueueTaskAssignedRunForTest while paused: %v", err)
	}

	if got := countQueuedRuns(t, svc, "agent-1", service.RunReasonTaskAssigned); got != 0 {
		t.Fatalf("queued runs while paused = %d, want 0", got)
	}

	if _, err := pauseSvc.Resume(ctx, "ws-1", "resolved", "user-1", "user"); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	if got := countQueuedRuns(t, svc, "agent-1", service.RunReasonTaskAssigned); got != 1 {
		t.Fatalf("queued task_assigned runs after resume = %d, want 1 (the deferred assignment was not replayed)", got)
	}
}

// deferredAssignmentOutcome reads office_deferred_assignments.outcome for a
// task, or "" for a still-pending (or nonexistent) row.
func deferredAssignmentOutcome(t *testing.T, svc *service.Service, taskID string) string {
	t.Helper()
	var outcome string
	err := svc.RepoForTest().ReaderDB().QueryRowx(
		`SELECT outcome FROM office_deferred_assignments WHERE task_id = ?`, taskID,
	).Scan(&outcome)
	if err != nil {
		t.Fatalf("read deferred assignment outcome: %v", err)
	}
	return outcome
}

// TestPausedAssignmentReplay_RecoveryTickBackstop proves the recovery
// tick's ReplayPendingDeferredAssignments replays a deferred assignment
// even when pause.Service has no AssignmentReplayer wired — the scenario
// a Resume-hook failure or a backend restart between release and replay
// leaves behind (AC-OFFICE-PAUSE-REPLAY-001.5).
func TestPausedAssignmentReplay_RecoveryTickBackstop(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	pauseSvc := pause.NewService(
		svc.RepoForTest(),
		noopTaskCanceller{},
		&fakeWorkspaceChecker{known: map[string]bool{"ws-1": true}},
		logger.Default(),
	)
	svc.SetPauseGate(pauseSvc)
	// Deliberately no SetAssignmentReplayer: simulates the Resume hook
	// never having run.

	svc.ExecSQL(t, `INSERT INTO workspaces (id) VALUES ('ws-1')`)
	createTestAgent(t, svc, "ws-1", "agent-1")
	svc.ExecSQL(t,
		`INSERT INTO tasks (id, workspace_id, project_id, assignment_generation) VALUES (?, ?, 'proj-1', 1)`,
		"task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "agent-1")

	if _, err := pauseSvc.Pause(ctx, "ws-1", "regression test", "user-1", "user"); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	gen := int64(1)
	if err := svc.QueueTaskAssignedRunForTest(ctx, "task-1", "agent-1", &gen); err != nil {
		t.Fatalf("QueueTaskAssignedRunForTest while paused: %v", err)
	}
	if _, err := pauseSvc.Resume(ctx, "ws-1", "resolved", "user-1", "user"); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if got := countQueuedRuns(t, svc, "agent-1", service.RunReasonTaskAssigned); got != 0 {
		t.Fatalf("queued runs after resume with no replayer wired = %d, want 0", got)
	}

	if err := svc.ReplayPendingDeferredAssignments(ctx); err != nil {
		t.Fatalf("ReplayPendingDeferredAssignments: %v", err)
	}

	if got := countQueuedRuns(t, svc, "agent-1", service.RunReasonTaskAssigned); got != 1 {
		t.Fatalf("queued task_assigned runs after recovery-tick backstop = %d, want 1", got)
	}
	if outcome := deferredAssignmentOutcome(t, svc, "task-1"); outcome != "replayed" {
		t.Fatalf("deferred assignment outcome = %q, want \"replayed\"", outcome)
	}
}

// TestPausedAssignmentReplay_DropsOnReassignment proves a deferred
// assignment whose task was reassigned to a different runner before
// replay ran is dropped rather than replayed against the stale record
// (AC-OFFICE-PAUSE-REPLAY-001.4) — the run the reassignment is itself
// owed comes from the ordinary live assignment path, not this one.
func TestPausedAssignmentReplay_DropsOnReassignment(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	pauseSvc := pause.NewService(
		svc.RepoForTest(),
		noopTaskCanceller{},
		&fakeWorkspaceChecker{known: map[string]bool{"ws-1": true}},
		logger.Default(),
	)
	svc.SetPauseGate(pauseSvc)
	pauseSvc.SetAssignmentReplayer(svc)

	svc.ExecSQL(t, `INSERT INTO workspaces (id) VALUES ('ws-1')`)
	createTestAgent(t, svc, "ws-1", "agent-1")
	createTestAgent(t, svc, "ws-1", "agent-2")
	svc.ExecSQL(t,
		`INSERT INTO tasks (id, workspace_id, project_id, assignment_generation) VALUES (?, ?, 'proj-1', 1)`,
		"task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "agent-1")

	if _, err := pauseSvc.Pause(ctx, "ws-1", "regression test", "user-1", "user"); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	gen := int64(1)
	if err := svc.QueueTaskAssignedRunForTest(ctx, "task-1", "agent-1", &gen); err != nil {
		t.Fatalf("QueueTaskAssignedRunForTest while paused: %v", err)
	}

	// Reassign to agent-2 without going through the deferral-recording
	// path, simulating a write that lands between the deferral and
	// replay without itself being observed as a paused occurrence.
	svc.ExecSQL(t, `UPDATE tasks SET assignment_generation = 2 WHERE id = 'task-1'`)
	setTestTaskAssignee(t, svc, "task-1", "agent-2")

	if _, err := pauseSvc.Resume(ctx, "ws-1", "resolved", "user-1", "user"); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	if got := countQueuedRuns(t, svc, "agent-1", service.RunReasonTaskAssigned); got != 0 {
		t.Fatalf("queued runs for the stale assignee agent-1 = %d, want 0", got)
	}
	if outcome := deferredAssignmentOutcome(t, svc, "task-1"); outcome != "dropped" {
		t.Fatalf("deferred assignment outcome = %q, want \"dropped\"", outcome)
	}
}

// TestPausedAssignmentReplay_DropsOnArchive proves a deferred assignment
// whose task was archived before replay ran is dropped, never queuing a
// run for an archived task (AC-OFFICE-PAUSE-REPLAY-001.4).
func TestPausedAssignmentReplay_DropsOnArchive(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	pauseSvc := pause.NewService(
		svc.RepoForTest(),
		noopTaskCanceller{},
		&fakeWorkspaceChecker{known: map[string]bool{"ws-1": true}},
		logger.Default(),
	)
	svc.SetPauseGate(pauseSvc)
	pauseSvc.SetAssignmentReplayer(svc)

	svc.ExecSQL(t, `INSERT INTO workspaces (id) VALUES ('ws-1')`)
	createTestAgent(t, svc, "ws-1", "agent-1")
	svc.ExecSQL(t,
		`INSERT INTO tasks (id, workspace_id, project_id, assignment_generation) VALUES (?, ?, 'proj-1', 1)`,
		"task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "agent-1")

	if _, err := pauseSvc.Pause(ctx, "ws-1", "regression test", "user-1", "user"); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	gen := int64(1)
	if err := svc.QueueTaskAssignedRunForTest(ctx, "task-1", "agent-1", &gen); err != nil {
		t.Fatalf("QueueTaskAssignedRunForTest while paused: %v", err)
	}

	svc.ExecSQL(t, `UPDATE tasks SET archived_at = datetime('now') WHERE id = 'task-1'`)

	if _, err := pauseSvc.Resume(ctx, "ws-1", "resolved", "user-1", "user"); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	if got := countQueuedRuns(t, svc, "agent-1", service.RunReasonTaskAssigned); got != 0 {
		t.Fatalf("queued runs for an archived task = %d, want 0", got)
	}
	if outcome := deferredAssignmentOutcome(t, svc, "task-1"); outcome != "dropped" {
		t.Fatalf("deferred assignment outcome = %q, want \"dropped\"", outcome)
	}
}

// TestPausedAssignmentReplay_DropsOnUnassign proves a deferred assignment
// whose task was unassigned before replay ran is dropped rather than
// replayed against a runner the task no longer has
// (AC-OFFICE-PAUSE-REPLAY-001.4).
func TestPausedAssignmentReplay_DropsOnUnassign(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	pauseSvc := pause.NewService(
		svc.RepoForTest(),
		noopTaskCanceller{},
		&fakeWorkspaceChecker{known: map[string]bool{"ws-1": true}},
		logger.Default(),
	)
	svc.SetPauseGate(pauseSvc)
	pauseSvc.SetAssignmentReplayer(svc)

	svc.ExecSQL(t, `INSERT INTO workspaces (id) VALUES ('ws-1')`)
	createTestAgent(t, svc, "ws-1", "agent-1")
	svc.ExecSQL(t,
		`INSERT INTO tasks (id, workspace_id, project_id, assignment_generation) VALUES (?, ?, 'proj-1', 1)`,
		"task-1", "ws-1")
	setTestTaskAssignee(t, svc, "task-1", "agent-1")

	if _, err := pauseSvc.Pause(ctx, "ws-1", "regression test", "user-1", "user"); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	gen := int64(1)
	if err := svc.QueueTaskAssignedRunForTest(ctx, "task-1", "agent-1", &gen); err != nil {
		t.Fatalf("QueueTaskAssignedRunForTest while paused: %v", err)
	}

	setTestTaskAssignee(t, svc, "task-1", "")

	if _, err := pauseSvc.Resume(ctx, "ws-1", "resolved", "user-1", "user"); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	if got := countQueuedRuns(t, svc, "agent-1", service.RunReasonTaskAssigned); got != 0 {
		t.Fatalf("queued runs for an unassigned task = %d, want 0", got)
	}
	if outcome := deferredAssignmentOutcome(t, svc, "task-1"); outcome != "dropped" {
		t.Fatalf("deferred assignment outcome = %q, want \"dropped\"", outcome)
	}
}
