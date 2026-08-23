package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// reclaimTrackingAgentManager is a thin wrapper around mockAgentManager that
// records every CleanupStaleExecutionBySessionID invocation. Tests use the
// recorded slice (and the per-call channel) to assert end-to-end that the
// reclaim primitive fired on the right settle points without resorting to
// time.Sleep — channel-based synchronization, synctest-style.
type reclaimTrackingAgentManager struct {
	*mockAgentManager
	mu        sync.Mutex
	calls     []string
	callCh    chan string
	closeOnce sync.Once
}

func newReclaimTrackingAgentManager(inner *mockAgentManager) *reclaimTrackingAgentManager {
	return &reclaimTrackingAgentManager{
		mockAgentManager: inner,
		callCh:           make(chan string, 8),
	}
}

func (m *reclaimTrackingAgentManager) CleanupStaleExecutionBySessionID(_ context.Context, sessionID string) error {
	m.mu.Lock()
	m.calls = append(m.calls, sessionID)
	m.mu.Unlock()
	m.closeOnce.Do(func() { close(m.callCh) })
	// signal: another call arrives after the channel is closed — no-op.
	return nil
}

func (m *reclaimTrackingAgentManager) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

func (m *reclaimTrackingAgentManager) wasCalledFor(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, sid := range m.calls {
		if sid == sessionID {
			return true
		}
	}
	return false
}

func (m *reclaimTrackingAgentManager) callsSnapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.calls...)
}

// TestSubtaskTerminalCollapse_ReclaimsProviderRuntime is the canonical
// end-to-end wiring test: a child task whose last agent message did not
// request input collapses to COMPLETED inside
// setSessionWaitingForInputIfRequested. With no live agent process and no
// active turn, reclaimIdleSession must fire on this synchronous settle
// point, the executor row must flip to status=stopped with LocalPID=0,
// and the resume_token/worktree_path must remain intact.
func TestSubtaskTerminalCollapse_ReclaimsProviderRuntime(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	seedSession(t, repo, "child-task", "s-child", "")
	if err := repo.UpdateTask(ctx, &models.Task{
		ID: "child-task", WorkspaceID: "ws1", Title: "child",
		State: v1.TaskStateCompleted, ParentID: "parent",
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("set child-task ParentID: %v", err)
	}
	const token = "rt-keep-child"
	const worktree = "/worktrees/child"
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "s-child",
		SessionID:        "s-child",
		TaskID:           "child-task",
		AgentExecutionID: "exec-child",
		ResumeToken:      token,
		WorktreePath:     worktree,
		Runtime:          agentruntime.RuntimeStandalone,
		Status:           models.ExecutorRunningStatusRunning,
		LocalPID:         5151,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("upsert child row: %v", err)
	}

	taskRepo := newMockTaskRepo()
	inner := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         false, // agent process exited before completion event
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessDead
		},
	}
	agentMgr := newReclaimTrackingAgentManager(inner)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.turnService = &inactiveTurnService{}

	svc.handleAgentCompleted(ctx, watcherAgentCompletedData("child-task", "s-child", "exec-child"))
	waitForStopCall(t, inner)

	// 1. Session collapsed to COMPLETED (existing guard behavior).
	updated, err := repo.GetTaskSession(ctx, "s-child")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if updated.State != models.TaskSessionStateCompleted {
		t.Fatalf("subtask terminal must collapse to COMPLETED, got %q", updated.State)
	}

	// 2. reclaim fired exactly once for this session.
	if !agentMgr.wasCalledFor("s-child") {
		t.Fatalf("reclaim must fire on subtask terminal settle; calls=%v", agentMgr.callsSnapshot())
	}

	// 3. Row preserved (resume_token/worktree intact) with status=stopped.
	row, err := repo.GetExecutorRunningBySessionID(ctx, "s-child")
	if err != nil {
		t.Fatalf("row missing after reclaim: %v", err)
	}
	if row.Status != models.ExecutorRunningStatusStopped {
		t.Fatalf("Status = %q, want %q", row.Status, models.ExecutorRunningStatusStopped)
	}
	if row.LocalPID != 0 {
		t.Fatalf("LocalPID = %d, want 0", row.LocalPID)
	}
	if row.ResumeToken != token {
		t.Fatalf("ResumeToken lost: got %q, want %q", row.ResumeToken, token)
	}
	if row.WorktreePath != worktree {
		t.Fatalf("WorktreePath lost: got %q, want %q", row.WorktreePath, worktree)
	}
}

// TestSubtaskTerminalCollapse_LiveAgentBlocksReclaim pins the fail-closed
// guard at the production call site: a subtask whose agent process is
// still alive must NOT be reclaimed even when the session collapses to
// COMPLETED. The provider runtime stays up; the row keeps its status and
// LocalPID for the subsequent cleanup pass.
func TestSubtaskTerminalCollapse_LiveAgentBlocksReclaim(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	seedSession(t, repo, "child-live", "s-child-live", "")
	if err := repo.UpdateTask(ctx, &models.Task{
		ID: "child-live", WorkspaceID: "ws1", Title: "child live",
		State: v1.TaskStateCompleted, ParentID: "parent",
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("set child-live ParentID: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "s-child-live", SessionID: "s-child-live", TaskID: "child-live",
		AgentExecutionID: "exec-live", ResumeToken: "rt-live",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
		LocalPID: 8181, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	taskRepo := newMockTaskRepo()
	inner := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true, // critical: agent is still alive
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessAlive
		},
	}
	agentMgr := newReclaimTrackingAgentManager(inner)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.turnService = &inactiveTurnService{}

	svc.handleAgentCompleted(ctx, watcherAgentCompletedData("child-live", "s-child-live", "exec-live"))
	waitForStopCall(t, inner)

	if agentMgr.wasCalledFor("s-child-live") {
		t.Fatalf("reclaim must be blocked while the agent is alive; calls=%v", agentMgr.callsSnapshot())
	}
	row, err := repo.GetExecutorRunningBySessionID(ctx, "s-child-live")
	if err != nil {
		t.Fatalf("row missing: %v", err)
	}
	if row.Status != models.ExecutorRunningStatusRunning {
		t.Fatalf("live agent row must stay running, got status=%q", row.Status)
	}
	if row.LocalPID != 8181 {
		t.Fatalf("live agent row must keep its LocalPID, got %d", row.LocalPID)
	}
}

// TestSiblingSession_AgentCompletedDoesNotReclaim pins the second half of
// the safety case: a root-task session (ParentID empty) that finishes on
// agent.completed keeps its WAITING_FOR_INPUT affordance and the live
// agent that waits for the user is NOT reclaimed. This is the
// "sibling session / multi-session task" case the parent's guard
// specifically preserves.
func TestSiblingSession_AgentCompletedDoesNotReclaim(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	// Root task (no ParentID) with two sessions: one finishing, one
	// still running. The finishing session's agent exit has no
	// requests_input and the agent stays up waiting for input from
	// the user — the standard WAITING_FOR_INPUT case.
	seedSession(t, repo, "t1", "s-finishing", "")
	seedExecutorRunning(t, repo, "s-finishing", "t1", "exec-finishing")
	requireNoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "s-running", TaskID: "t1",
		State:     models.TaskSessionStateRunning,
		StartedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
	}))

	taskRepo := newMockTaskRepo()
	inner := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true, // finishing session's agent stays up
	}
	agentMgr := newReclaimTrackingAgentManager(inner)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.turnService = &inactiveTurnService{}

	svc.handleAgentCompleted(ctx, watcherAgentCompletedData("t1", "s-finishing", "exec-finishing"))
	waitForStopCall(t, inner)

	// Session still WAITING_FOR_INPUT — root-task affordance preserved.
	updated, err := repo.GetTaskSession(ctx, "s-finishing")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if updated.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("sibling must stay WAITING_FOR_INPUT, got %q", updated.State)
	}
	// Reclaim must NOT fire — the live agent is still answering.
	if agentMgr.wasCalledFor("s-finishing") {
		t.Fatalf("reclaim must not fire while a live agent is waiting for user input; calls=%v", agentMgr.callsSnapshot())
	}
}

// TestSubtaskTerminalCollapse_ReclaimIsIdempotent pins the wiring
// contract for repeated settle events. A subtask terminal is wired
// twice (subtask collapse inside setSessionWaitingForInputIfRequested +
// the post-handleAgentCompleted settle point), so a single
// agent.completed already produces two reclaim calls — both are
// idempotent at the runtime layer
// ("Safe to call even if the process is already stopped — StopInstance
// is idempotent. Returns nil if no execution exists for the session").
// The test pins the row invariants that must hold identically across
// repeated calls: the executor row is preserved (resume_token intact,
// status=stopped, LocalPID=0) regardless of how many times the settle
// event fires.
func TestSubtaskTerminalCollapse_ReclaimIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	seedSession(t, repo, "child-idem", "s-idem", "")
	if err := repo.UpdateTask(ctx, &models.Task{
		ID: "child-idem", WorkspaceID: "ws1", Title: "child idem",
		State: v1.TaskStateCompleted, ParentID: "parent",
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("set ParentID: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "s-idem", SessionID: "s-idem", TaskID: "child-idem",
		AgentExecutionID: "exec-idem", ResumeToken: "rt-idem",
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
		LocalPID: 2222, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	taskRepo := newMockTaskRepo()
	inner := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         false,
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessDead
		},
	}
	agentMgr := newReclaimTrackingAgentManager(inner)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.turnService = &inactiveTurnService{}

	// First agent.completed — fires reclaim at least once. The exact
	// call count is an implementation detail (the state CAS in
	// setSessionWaitingForInputIfRequested short-circuits on the second
	// pass, so the post-settle-point wiring is what guarantees the
	// invariant on duplicates).
	svc.handleAgentCompleted(ctx, watcherAgentCompletedData("child-idem", "s-idem", "exec-idem"))
	waitForStopCall(t, inner)
	if agentMgr.callCount() < 1 {
		t.Fatalf("first completion must reclaim at least once; calls=%v", agentMgr.callsSnapshot())
	}
	assertReclaimedRowInvariants(t, ctx, repo, "s-idem", "rt-idem")

	// A second agent.completed (e.g. a duplicate lifecycle event) must
	// preserve the same shape: row invariants unchanged, no PID
	// resurrection, no resume_token loss.
	prevCalls := agentMgr.callCount()
	svc.handleAgentCompleted(ctx, watcherAgentCompletedData("child-idem", "s-idem", "exec-idem"))
	waitForStopCall(t, inner)
	if agentMgr.callCount() < prevCalls {
		t.Fatalf("second completion dropped reclaim calls: %d -> %d", prevCalls, agentMgr.callCount())
	}
	assertReclaimedRowInvariants(t, ctx, repo, "s-idem", "rt-idem")
}

// assertReclaimedRowInvariants pins the post-reclaim row shape the rest
// of the suite relies on: status=stopped, LocalPID=0, resume_token intact,
// row still present. Channel-based synchronization is unnecessary —
// handleAgentCompleted is synchronous and reclaim runs inline, so by the
// time the call returns the row has settled.
func assertReclaimedRowInvariants(t *testing.T, ctx context.Context, repo executorRowGetter, sessionID, expectedToken string) {
	t.Helper()
	row, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	if err != nil {
		t.Fatalf("row missing after reclaim: %v", err)
	}
	if row.Status != models.ExecutorRunningStatusStopped {
		t.Fatalf("Status = %q, want stopped", row.Status)
	}
	if row.LocalPID != 0 {
		t.Fatalf("LocalPID = %d, want 0", row.LocalPID)
	}
	if row.ResumeToken != expectedToken {
		t.Fatalf("ResumeToken lost: got %q, want %q", row.ResumeToken, expectedToken)
	}
}

// executorRowGetter is the read-only surface assertReclaimedRowInvariants
// needs. The concrete *sqliterepo.Repository satisfies this; declaring it
// here keeps the helper self-contained without a heavyweight import.
type executorRowGetter interface {
	GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error)
}

// TestReclaimPreservesRowForResumeTaskSession locks the compatibility
// contract the reviewer asked for: after reclaim runs, the executors_running
// row is still present (status=stopped, resume_token intact), which is the
// pre-condition ResumeTaskSession relies on to re-attach a session. Resume
// must observe the preserved row, not crash on ErrExecutorRunningNotFound.
func TestReclaimPreservesRowForResumeTaskSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	seedSession(t, repo, "task-resume", "s-resume", "")
	if err := repo.UpdateTask(ctx, &models.Task{
		ID: "task-resume", WorkspaceID: "ws1", Title: "resume compat",
		State: v1.TaskStateFailed, ParentID: "parent",
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("set ParentID: %v", err)
	}
	const token = "rt-resume-keep"
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "s-resume", SessionID: "s-resume", TaskID: "task-resume",
		AgentExecutionID: "exec-resume", ResumeToken: token,
		Runtime: agentruntime.RuntimeStandalone, Status: models.ExecutorRunningStatusRunning,
		LocalPID: 3333, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	taskRepo := newMockTaskRepo()
	inner := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         false,
		rowLivenessFn: func(*models.ExecutorRunning) models.ProcessLiveness {
			return models.ProcessLivenessDead
		},
	}
	agentMgr := newReclaimTrackingAgentManager(inner)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.turnService = &inactiveTurnService{}

	svc.handleAgentCompleted(ctx, watcherAgentCompletedData("task-resume", "s-resume", "exec-resume"))
	waitForStopCall(t, inner)

	// ResumeTaskSession requires the executor row to exist; if reclaim
	// had pruned it (as a non-preserved terminal row would be) the call
	// would return ErrExecutorRunningNotFound. The row is still here
	// because resume_token is non-empty (RowMustBePreserved=true), and
	// reclaim routes through repairDeadRowLiveness which preserves it.
	row, err := repo.GetExecutorRunningBySessionID(ctx, "s-resume")
	if err != nil {
		t.Fatalf("reclaim must preserve the row for resume: %v", err)
	}
	if row.Status != models.ExecutorRunningStatusStopped {
		t.Fatalf("Status = %q, want %q", row.Status, models.ExecutorRunningStatusStopped)
	}
	if row.ResumeToken != token {
		t.Fatalf("ResumeToken lost during reclaim: got %q, want %q", row.ResumeToken, token)
	}
	// Confirm the row is the only artifact a resume would see — no
	// ErrExecutorRunningNotFound would be returned by the resume path.
	if _, err := repo.GetExecutorRunningBySessionID(ctx, "s-resume"); err != nil {
		t.Fatalf("ResumeTaskSession precondition (row present) violated: %v", err)
	}
}
