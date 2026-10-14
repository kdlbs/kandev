package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type pendingRemoteRecoveryOwner struct {
	sessionID   string
	executionID string
}

type fakePendingRemoteRecoveryChecker struct {
	pending map[pendingRemoteRecoveryOwner]bool
	check   func(owner pendingRemoteRecoveryOwner, call int) bool
	calls   []pendingRemoteRecoveryOwner
}

func (c *fakePendingRemoteRecoveryChecker) HasPendingRemoteRecovery(sessionID, executionID string) bool {
	owner := pendingRemoteRecoveryOwner{sessionID: sessionID, executionID: executionID}
	c.calls = append(c.calls, owner)
	if c.check != nil {
		return c.check(owner, len(c.calls))
	}
	return c.pending[owner]
}

func setPendingRemoteRecoveryChecker(svc *Service, checker *fakePendingRemoteRecoveryChecker) {
	svc.SetPendingRemoteRecoveryChecker(checker.HasPendingRemoteRecovery)
}

func seedPendingRemoteExecutor(
	t *testing.T,
	repo *sqliterepo.Repository,
	sessionID, taskID, executionID string,
	runtime agentruntime.Runtime,
	updatedAt time.Time,
) {
	t.Helper()
	localPID, remotePID := 0, 1234
	if runtime == agentruntime.RuntimeStandalone {
		localPID, remotePID = remotePID, 0
	}
	if err := repo.UpsertExecutorRunning(context.Background(), &models.ExecutorRunning{
		ID: "executor-row-" + sessionID, SessionID: sessionID, TaskID: taskID,
		Runtime: runtime, ExecutorID: string(runtime), Status: models.ExecutorRunningStatusRunning,
		AgentExecutionID: executionID, LocalPID: localPID, PID: remotePID, CreatedAt: updatedAt, UpdatedAt: updatedAt,
	}); err != nil {
		t.Fatalf("seed executor row for %s: %v", sessionID, err)
	}
}

func seedStaleOpenTurn(t *testing.T, svc *Service, repo *sqliterepo.Repository, sessionID string, stale time.Time) string {
	t.Helper()
	ctx := context.Background()
	turn, err := svc.StartTurn(ctx, sessionID)
	if err != nil {
		t.Fatalf("StartTurn(%s): %v", sessionID, err)
	}
	if _, err := repo.DB().ExecContext(ctx,
		`UPDATE task_session_turns SET started_at = ?, created_at = ?, updated_at = ? WHERE id = ?`,
		stale, stale, stale, turn.ID); err != nil {
		t.Fatalf("age turn %s: %v", turn.ID, err)
	}
	if _, err := repo.DB().ExecContext(ctx,
		`UPDATE task_sessions SET updated_at = ? WHERE id = ?`, stale, sessionID); err != nil {
		t.Fatalf("age session %s: %v", sessionID, err)
	}
	return turn.ID
}

func newPendingRemoteOrphanFixture(t *testing.T, sessionID, executionID string) (*Service, *MockEventBus, *sqliterepo.Repository, string) {
	t.Helper()
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedOrphanSweepFixtures(t, repo)
	seedOrphanSweepTask(t, repo, "task-pending-remote", false)
	stale := time.Now().UTC().Add(-8 * 24 * time.Hour)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: "task-pending-remote", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", IsPrimary: true, UpdatedAt: stale, StartedAt: stale,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	turnID := seedStaleOpenTurn(t, svc, repo, sessionID, stale)
	seedPendingRemoteExecutor(t, repo, sessionID, "task-pending-remote", executionID, agentruntime.RuntimeSSH, stale)
	svc.SetExecutionLivenessChecker(&fakeExecutionLiveness{})
	return svc, eventBus, repo, turnID
}

func TestService_OrphanedSessionReconciliationPreservesPendingRemoteOwner(t *testing.T) {
	const sessionID = "session-pending-remote"
	const executionID = "execution-pending-remote"
	svc, eventBus, repo, turnID := newPendingRemoteOrphanFixture(t, sessionID, executionID)
	checker := &fakePendingRemoteRecoveryChecker{
		pending: map[pendingRemoteRecoveryOwner]bool{{sessionID: sessionID, executionID: executionID}: true},
	}
	setPendingRemoteRecoveryChecker(svc, checker)

	svc.runOrphanedSessionReconciliation(context.Background())

	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want RUNNING while the exact remote owner is pending", session.State)
	}
	turn, err := svc.GetActiveTurn(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetActiveTurn: %v", err)
	}
	if turn == nil || turn.ID != turnID {
		t.Fatalf("active turn = %+v, want unfinished turn %s", turn, turnID)
	}
	if sessionRecoveredEventPublished(eventBus, sessionID) {
		t.Fatal("pending remote owner must not publish session recovery")
	}
	row, err := repo.GetExecutorRunningBySessionID(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetExecutorRunningBySessionID: %v", err)
	}
	if row.Status != models.ExecutorRunningStatusRunning || row.AgentExecutionID != executionID {
		t.Fatalf("executor row = %+v, want exact running remote owner preserved", row)
	}
	if len(checker.calls) == 0 || checker.calls[0] != (pendingRemoteRecoveryOwner{sessionID: sessionID, executionID: executionID}) {
		t.Fatalf("pending-owner checks = %+v, want exact session/execution pair", checker.calls)
	}
}

func TestService_OrphanedSessionReconciliationRechecksPendingOwnerBeforeRecoveryWrite(t *testing.T) {
	const sessionID = "session-pending-transition"
	const executionID = "execution-pending-transition"
	svc, eventBus, repo, turnID := newPendingRemoteOrphanFixture(t, sessionID, executionID)
	checker := &fakePendingRemoteRecoveryChecker{
		check: func(owner pendingRemoteRecoveryOwner, call int) bool {
			if owner != (pendingRemoteRecoveryOwner{sessionID: sessionID, executionID: executionID}) {
				t.Errorf("pending-owner check = %+v, want exact owner pair", owner)
			}
			return call >= 2
		},
	}
	setPendingRemoteRecoveryChecker(svc, checker)

	svc.runOrphanedSessionReconciliation(context.Background())

	if len(checker.calls) < 2 {
		t.Fatalf("pending-owner checks = %d, want initial classification and pre-write recheck", len(checker.calls))
	}
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want RUNNING when remote ownership appears before the write", session.State)
	}
	turn, err := svc.GetActiveTurn(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetActiveTurn: %v", err)
	}
	if turn == nil || turn.ID != turnID {
		t.Fatalf("active turn = %+v, want unfinished turn %s", turn, turnID)
	}
	if sessionRecoveredEventPublished(eventBus, sessionID) {
		t.Fatal("late pending remote owner must prevent the recovery write")
	}
}

func TestService_OrphanedSessionReconciliationRechecksLivenessAfterPendingOwnerRead(t *testing.T) {
	const sessionID = "session-owner-handoff"
	const executionID = "execution-owner-handoff"
	svc, eventBus, repo, turnID := newPendingRemoteOrphanFixture(t, sessionID, executionID)
	liveness := &fakeExecutionLiveness{live: map[string]bool{}}
	svc.SetExecutionLivenessChecker(liveness)
	checker := &fakePendingRemoteRecoveryChecker{
		check: func(owner pendingRemoteRecoveryOwner, call int) bool {
			if call == 2 {
				liveness.live[sessionID] = true
			}
			return false
		},
	}
	setPendingRemoteRecoveryChecker(svc, checker)

	svc.runOrphanedSessionReconciliation(context.Background())

	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want RUNNING after the pending owner becomes live", session.State)
	}
	turn, err := svc.GetActiveTurn(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetActiveTurn: %v", err)
	}
	if turn == nil || turn.ID != turnID {
		t.Fatalf("active turn = %+v, want unfinished turn %s", turn, turnID)
	}
	if sessionRecoveredEventPublished(eventBus, sessionID) {
		t.Fatal("owner handoff to a live execution must prevent recovery")
	}
}

func TestService_OrphanedSessionReconciliationAllowsDifferentExecutionIdentity(t *testing.T) {
	const sessionID = "session-pending-mismatch"
	const executionID = "current-execution"
	svc, eventBus, repo, _ := newPendingRemoteOrphanFixture(t, sessionID, executionID)
	checker := &fakePendingRemoteRecoveryChecker{
		pending: map[pendingRemoteRecoveryOwner]bool{{sessionID: sessionID, executionID: "older-execution"}: true},
	}
	setPendingRemoteRecoveryChecker(svc, checker)

	svc.runOrphanedSessionReconciliation(context.Background())

	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %q, want WAITING_FOR_INPUT for a different execution identity", session.State)
	}
	if !sessionRecoveredEventPublished(eventBus, sessionID) {
		t.Fatal("different execution identity must not block the stale session recovery")
	}
}

func TestService_ActiveSessionSweepDoesNotHealAroundPendingRemoteSibling(t *testing.T) {
	fixture := newSweepFixture(t, 5*time.Hour)
	ctx := context.Background()
	turnID := seedStaleOpenTurn(t, fixture.svc, fixture.rawRepo, "session-1", fixture.now.Add(-5*time.Hour))
	if err := fixture.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-local-orphan", TaskID: "task-1", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", UpdatedAt: fixture.now.Add(-5 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateTaskSession sibling: %v", err)
	}
	seedPendingRemoteExecutor(t, fixture.rawRepo, "session-1", "task-1", "remote-execution", agentruntime.RuntimeSSH, fixture.now.Add(-5*time.Hour))
	checker := &fakePendingRemoteRecoveryChecker{
		pending: map[pendingRemoteRecoveryOwner]bool{{sessionID: "session-1", executionID: "remote-execution"}: true},
	}
	setPendingRemoteRecoveryChecker(fixture.svc, checker)

	fixture.svc.runActiveSessionSweep(ctx, fixture.now)

	for _, sessionID := range []string{"session-1", "session-local-orphan"} {
		session, err := fixture.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			t.Fatalf("GetTaskSession(%s): %v", sessionID, err)
		}
		if session.State != models.TaskSessionStateRunning {
			t.Fatalf("session %s state = %q, want RUNNING while its task has a pending remote owner", sessionID, session.State)
		}
		if sessionRecoveredEventPublished(fixture.eventBus, sessionID) {
			t.Errorf("pending-owner task unexpectedly recovered session %s", sessionID)
		}
	}
	turn, err := fixture.svc.GetActiveTurn(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetActiveTurn: %v", err)
	}
	if turn == nil || turn.ID != turnID {
		t.Fatalf("pending owner's active turn = %+v, want unfinished turn %s", turn, turnID)
	}
}

func TestService_ActiveSessionSweepRechecksLivenessAfterPendingOwnerRead(t *testing.T) {
	fixture := newSweepFixture(t, 5*time.Hour)
	ctx := context.Background()
	turnID := seedStaleOpenTurn(t, fixture.svc, fixture.rawRepo, "session-1", fixture.now.Add(-5*time.Hour))
	seedPendingRemoteExecutor(t, fixture.rawRepo, "session-1", "task-1", "classification-execution", agentruntime.RuntimeSSH, fixture.now.Add(-5*time.Hour))
	checker := &fakePendingRemoteRecoveryChecker{
		check: func(_ pendingRemoteRecoveryOwner, call int) bool {
			if call == 1 {
				fixture.registry.liveSessions = []string{"session-1"}
			}
			return false
		},
	}
	setPendingRemoteRecoveryChecker(fixture.svc, checker)

	fixture.svc.runActiveSessionSweep(ctx, fixture.now)

	session, err := fixture.repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateRunning {
		t.Fatalf("session state = %q, want RUNNING after the owner becomes live", session.State)
	}
	turn, err := fixture.svc.GetActiveTurn(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetActiveTurn: %v", err)
	}
	if turn == nil || turn.ID != turnID {
		t.Fatalf("active turn = %+v, want unfinished turn %s", turn, turnID)
	}
	if stalled := findTaskStalledEvents(fixture.eventBus); len(stalled) != 0 {
		t.Fatalf("task.stalled events = %d, want 0 after owner becomes live", len(stalled))
	}
}

func TestService_ActiveSessionSweepRechecksTaskLivenessBeforeSiblingWrite(t *testing.T) {
	fixture := newSweepFixture(t, 5*time.Hour)
	ctx := context.Background()
	turnID := seedStaleOpenTurn(t, fixture.svc, fixture.rawRepo, "session-1", fixture.now.Add(-5*time.Hour))
	if err := fixture.rawRepo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "dead-local-row", SessionID: "session-1", TaskID: "task-1",
		Runtime: agentruntime.RuntimeStandalone, ExecutorID: "standalone",
		Status: models.ExecutorRunningStatusRunning, AgentExecutionID: "dead-local-execution",
		CreatedAt: fixture.now.Add(-5 * time.Hour), UpdatedAt: fixture.now.Add(-5 * time.Hour),
	}); err != nil {
		t.Fatalf("seed dead local executor row: %v", err)
	}
	const siblingID = "session-live-sibling"
	if err := fixture.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: siblingID, TaskID: "task-1", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", UpdatedAt: fixture.now.Add(-5 * time.Hour),
	}); err != nil {
		t.Fatalf("CreateTaskSession live sibling: %v", err)
	}
	seedPendingRemoteExecutor(t, fixture.rawRepo, siblingID, "task-1", "sibling-execution", agentruntime.RuntimeSSH, fixture.now.Add(-5*time.Hour))
	ownerCalls := make(map[string]int)
	checker := &fakePendingRemoteRecoveryChecker{
		check: func(owner pendingRemoteRecoveryOwner, _ int) bool {
			ownerCalls[owner.sessionID]++
			if owner.sessionID == siblingID && ownerCalls[siblingID] == 2 {
				fixture.registry.liveSessions = []string{siblingID}
			}
			return false
		},
	}
	setPendingRemoteRecoveryChecker(fixture.svc, checker)

	fixture.svc.runActiveSessionSweep(ctx, fixture.now)

	candidate, err := fixture.repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession candidate: %v", err)
	}
	if candidate.State != models.TaskSessionStateRunning {
		t.Fatalf("dead-local candidate state = %q, want RUNNING when its sibling becomes live", candidate.State)
	}
	turn, err := fixture.svc.GetActiveTurn(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetActiveTurn candidate: %v", err)
	}
	if turn == nil || turn.ID != turnID {
		t.Fatalf("candidate active turn = %+v, want unfinished turn %s", turn, turnID)
	}
	sibling, err := fixture.repo.GetTaskSession(ctx, siblingID)
	if err != nil {
		t.Fatalf("GetTaskSession sibling: %v", err)
	}
	if sibling.State != models.TaskSessionStateRunning {
		t.Fatalf("live sibling state = %q, want RUNNING", sibling.State)
	}
	if sessionRecoveredEventPublished(fixture.eventBus, "session-1") {
		t.Fatal("a live sibling must prevent the dead-local candidate recovery write")
	}
}

func TestService_ActiveSessionSweepDoesNotRetrySettlementForPendingRemoteOwner(t *testing.T) {
	fixture := newSweepFixture(t, 5*time.Hour)
	ctx := context.Background()
	turn, err := fixture.svc.StartTurn(ctx, "session-1")
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	old := fixture.now.Add(-5 * time.Hour)
	if err := fixture.svc.messages.CreateMessage(ctx, &models.Message{
		ID: "pending-remote-tool-message", TaskSessionID: "session-1", TaskID: "task-1", TurnID: turn.ID,
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolExecute, Content: "tool output",
		Metadata:  map[string]interface{}{"tool_call_id": "pending-remote-call", "status": "pending"},
		CreatedAt: old, UpdatedAt: old,
	}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if _, err := fixture.rawRepo.DB().ExecContext(ctx,
		`UPDATE task_session_turns SET started_at = ?, created_at = ?, updated_at = ? WHERE id = ?`, old, old, old, turn.ID); err != nil {
		t.Fatalf("age turn: %v", err)
	}
	if _, err := fixture.rawRepo.DB().ExecContext(ctx,
		`UPDATE task_sessions SET updated_at = ? WHERE id = ?`, old, "session-1"); err != nil {
		t.Fatalf("age session: %v", err)
	}
	seedPendingRemoteExecutor(t, fixture.rawRepo, "session-1", "task-1", "settlement-execution", agentruntime.RuntimeSSH, old)
	fixture.svc.turns = &failOnceToolSettlementTurns{TurnRepository: fixture.svc.turns, fail: true}
	checker := &fakePendingRemoteRecoveryChecker{pending: map[pendingRemoteRecoveryOwner]bool{}}
	setPendingRemoteRecoveryChecker(fixture.svc, checker)

	fixture.svc.runActiveSessionSweep(ctx, fixture.now)
	failed, err := fixture.repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession after first recovery: %v", err)
	}
	if _, pending := models.LoadInterruptedRecoverySettlement(failed.Metadata); !pending {
		t.Fatalf("session metadata after first recovery = %#v, want settlement retry marker", failed.Metadata)
	}
	if err := fixture.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-retry-sibling", TaskID: "task-1", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", UpdatedAt: fixture.now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("CreateTaskSession retry sibling: %v", err)
	}
	checker.pending[pendingRemoteRecoveryOwner{sessionID: "session-1", executionID: "settlement-execution"}] = true

	fixture.svc.runActiveSessionSweep(ctx, fixture.now.Add(time.Minute))

	retried, err := fixture.repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession while remote owner pending: %v", err)
	}
	if _, pending := models.LoadInterruptedRecoverySettlement(retried.Metadata); !pending {
		t.Fatalf("session metadata while remote owner pending = %#v, want settlement marker retained", retried.Metadata)
	}
	message, err := fixture.svc.messages.GetMessage(ctx, "pending-remote-tool-message")
	if err != nil {
		t.Fatalf("GetMessage while remote owner pending: %v", err)
	}
	if got := message.Metadata["status"]; got != "pending" {
		t.Fatalf("tool status while remote owner pending = %v, want pending", got)
	}

	checker.pending[pendingRemoteRecoveryOwner{sessionID: "session-1", executionID: "settlement-execution"}] = false
	fixture.svc.runActiveSessionSweep(ctx, fixture.now.Add(2*time.Minute))
	settled, err := fixture.repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession after owner clears: %v", err)
	}
	if _, pending := models.LoadInterruptedRecoverySettlement(settled.Metadata); pending {
		t.Fatalf("session metadata after owner clears = %#v, want settlement marker cleared", settled.Metadata)
	}
	message, err = fixture.svc.messages.GetMessage(ctx, "pending-remote-tool-message")
	if err != nil {
		t.Fatalf("GetMessage after owner clears: %v", err)
	}
	if got := message.Metadata["status"]; got != "complete" {
		t.Fatalf("tool status after owner clears = %v, want complete", got)
	}
}

func TestService_ActiveSessionSweepDoesNotRetrySettlementAfterRemoteOwnerBecomesLive(t *testing.T) {
	fixture := newSweepFixture(t, 5*time.Hour)
	ctx := context.Background()
	turn, err := fixture.svc.StartTurn(ctx, "session-1")
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	old := fixture.now.Add(-5 * time.Hour)
	if err := fixture.svc.messages.CreateMessage(ctx, &models.Message{
		ID: "handoff-tool-message", TaskSessionID: "session-1", TaskID: "task-1", TurnID: turn.ID,
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeToolExecute, Content: "tool output",
		Metadata:  map[string]interface{}{"tool_call_id": "handoff-tool-call", "status": "pending"},
		CreatedAt: old, UpdatedAt: old,
	}); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if _, err := fixture.rawRepo.DB().ExecContext(ctx,
		`UPDATE task_session_turns SET started_at = ?, created_at = ?, updated_at = ? WHERE id = ?`, old, old, old, turn.ID); err != nil {
		t.Fatalf("age turn: %v", err)
	}
	if _, err := fixture.rawRepo.DB().ExecContext(ctx,
		`UPDATE task_sessions SET updated_at = ? WHERE id = ?`, old, "session-1"); err != nil {
		t.Fatalf("age session: %v", err)
	}
	seedPendingRemoteExecutor(t, fixture.rawRepo, "session-1", "task-1", "handoff-execution", agentruntime.RuntimeSSH, old)
	fixture.svc.turns = &failOnceToolSettlementTurns{TurnRepository: fixture.svc.turns, fail: true}
	checker := &fakePendingRemoteRecoveryChecker{pending: map[pendingRemoteRecoveryOwner]bool{}}
	setPendingRemoteRecoveryChecker(fixture.svc, checker)

	fixture.svc.runActiveSessionSweep(ctx, fixture.now)
	failed, err := fixture.repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession after first recovery: %v", err)
	}
	if _, pending := models.LoadInterruptedRecoverySettlement(failed.Metadata); !pending {
		t.Fatalf("session metadata after first recovery = %#v, want settlement retry marker", failed.Metadata)
	}
	if err := fixture.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "handoff-retry-sibling", TaskID: "task-1", State: models.TaskSessionStateRunning,
		AgentProfileID: "agent-1", UpdatedAt: fixture.now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("CreateTaskSession retry sibling: %v", err)
	}

	owner := pendingRemoteRecoveryOwner{sessionID: "session-1", executionID: "handoff-execution"}
	ownerChecks := 0
	checker.check = func(checked pendingRemoteRecoveryOwner, _ int) bool {
		if checked != owner {
			t.Errorf("pending-owner check = %+v, want exact owner %+v", checked, owner)
		}
		ownerChecks++
		if ownerChecks == 2 {
			// The first check is the sweep's pending-owner snapshot. The second
			// is settlement retry, after the live-session snapshot was captured.
			fixture.registry.liveSessions = []string{"session-1"}
		}
		return false
	}
	checker.calls = nil
	fixture.svc.runActiveSessionSweep(ctx, fixture.now.Add(time.Minute))

	if ownerChecks < 2 {
		t.Fatalf("pending-owner checks for session-1 = %d, want snapshot and settlement-retry checks", ownerChecks)
	}
	whileLive, err := fixture.repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession after owner becomes live: %v", err)
	}
	if _, pending := models.LoadInterruptedRecoverySettlement(whileLive.Metadata); !pending {
		t.Fatalf("session metadata after owner becomes live = %#v, want settlement marker retained", whileLive.Metadata)
	}
	message, err := fixture.svc.messages.GetMessage(ctx, "handoff-tool-message")
	if err != nil {
		t.Fatalf("GetMessage after owner becomes live: %v", err)
	}
	if got := message.Metadata["status"]; got != "pending" {
		t.Fatalf("tool status after owner becomes live = %v, want pending", got)
	}

	fixture.registry.liveSessions = nil
	checker.check = nil
	checker.pending[owner] = false
	fixture.svc.runActiveSessionSweep(ctx, fixture.now.Add(2*time.Minute))
	settled, err := fixture.repo.GetTaskSession(ctx, "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession after owner clears: %v", err)
	}
	if _, pending := models.LoadInterruptedRecoverySettlement(settled.Metadata); pending {
		t.Fatalf("session metadata after owner clears = %#v, want settlement marker cleared", settled.Metadata)
	}
	message, err = fixture.svc.messages.GetMessage(ctx, "handoff-tool-message")
	if err != nil {
		t.Fatalf("GetMessage after owner clears: %v", err)
	}
	if got := message.Metadata["status"]; got != "complete" {
		t.Fatalf("tool status after owner clears = %v, want complete", got)
	}
}

func TestService_ActiveSessionSweepFailsClosedWhenPendingOwnerSnapshotFails(t *testing.T) {
	fixture := newSweepFixture(t, 5*time.Hour)
	fixture.svc.executors = failGetExecutorRunningRepository{
		ExecutorRepository: fixture.rawRepo,
		err:                errors.New("executor inventory unavailable"),
	}
	checkerCalls := 0
	fixture.svc.SetPendingRemoteRecoveryChecker(func(string, string) bool {
		checkerCalls++
		return false
	})

	fixture.svc.runActiveSessionSweep(context.Background(), fixture.now)

	session, err := fixture.repo.GetTaskSession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateRunning || models.HasInterruptedRecoveryPending(session.Metadata) {
		t.Fatalf("session after failed owner snapshot = %+v, want unchanged RUNNING session", session)
	}
	if checkerCalls != 0 {
		t.Fatalf("pending checker calls = %d, want 0 without an exact persisted execution identity", checkerCalls)
	}
	if events := findTaskStalledEvents(fixture.eventBus); len(events) != 0 {
		t.Fatalf("task.stalled events = %d, want 0 when executor-owner evidence cannot be read", len(events))
	}
}

func TestService_ActiveSessionSweepStillHealsDeadLocalExecution(t *testing.T) {
	fixture := newSweepFixture(t, 5*time.Hour)
	seedPendingRemoteExecutor(
		t, fixture.rawRepo, "session-1", "task-1", "dead-local-execution", agentruntime.RuntimeStandalone, fixture.now.Add(-5*time.Hour),
	)
	checker := &fakePendingRemoteRecoveryChecker{pending: map[pendingRemoteRecoveryOwner]bool{}}
	setPendingRemoteRecoveryChecker(fixture.svc, checker)

	fixture.svc.runActiveSessionSweep(context.Background(), fixture.now)

	session, err := fixture.repo.GetTaskSession(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("dead local session state = %q, want WAITING_FOR_INPUT", session.State)
	}
	if len(checker.calls) == 0 || checker.calls[0] != (pendingRemoteRecoveryOwner{sessionID: "session-1", executionID: "dead-local-execution"}) {
		t.Fatalf("pending-owner checks = %+v, want exact dead-local row pair", checker.calls)
	}
}
