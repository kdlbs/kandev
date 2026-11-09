package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type settledSessionStateFixture struct {
	taskID      string
	sessionID   string
	incarnation string
	executionID string
	streamID    string
	submission  string
	turnID      string
	request     models.AgentDeliverySessionStateRequest
}

func seedSettledSessionStateFixture(t *testing.T, repo *Repository, suffix string) settledSessionStateFixture {
	t.Helper()
	ctx := context.Background()
	fixture := settledSessionStateFixture{
		taskID:      "task-session-state-" + suffix,
		sessionID:   "session-session-state-" + suffix,
		incarnation: "incarnation-session-state-" + suffix,
		executionID: "execution-session-state-" + suffix,
		streamID:    "stream-session-state-" + suffix,
		submission:  "submission-session-state-" + suffix,
		turnID:      "turn-session-state-" + suffix,
	}
	now := time.Now().UTC()
	if err := repo.CreateTask(ctx, &models.Task{ID: fixture.taskID, Title: "Settled session state"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: fixture.sessionID, TaskID: fixture.taskID, QueueIncarnationID: fixture.incarnation,
		AgentExecutionID: fixture.executionID, State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: "running-session-state-" + suffix, SessionID: fixture.sessionID, TaskID: fixture.taskID,
		ExecutorID: "executor-session-state-" + suffix, AgentExecutionID: fixture.executionID,
		Status: "ready",
	}); err != nil {
		t.Fatalf("UpsertExecutorRunning: %v", err)
	}
	if err := repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: fixture.sessionID, IncarnationID: fixture.incarnation, Generation: 1,
		NativeSessionID: "native-session-state-" + suffix, CreationReason: "test",
	}); err != nil {
		t.Fatalf("CreateHarnessSessionGeneration: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: fixture.turnID, TaskID: fixture.taskID, TaskSessionID: fixture.sessionID,
		StartedAt: now, CompletedAt: &now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: fixture.submission, SessionID: fixture.sessionID, IncarnationID: fixture.incarnation,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash-" + suffix,
		Payload: []byte("prompt"), State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatalf("PrepareAgentDeliverySubmission: %v", err)
	}
	event := terminalDeliveryEvent(suffix, fixture.submission, fixture.streamID, 1, "complete")
	event.SessionID = fixture.sessionID
	event.IncarnationID = fixture.incarnation
	event.Payload = []byte(`{"type":"complete","data":{"stop_reason":"cancelled"},"turn_id":"` + fixture.turnID + `"}`)
	projectDeliveryEvent(t, repo, ctx, event)
	if settled, err := repo.SettleAgentDeliveryTerminal(
		ctx, event.StreamID, event.Sequence, models.DeliverySubmissionCancelled, now,
	); err != nil || !settled {
		t.Fatalf("SettleAgentDeliveryTerminal = %v, %v", settled, err)
	}
	fixture.request = models.AgentDeliverySessionStateRequest{
		SessionID: fixture.sessionID, TaskID: fixture.taskID, ExecutionID: fixture.executionID,
		IncarnationID: fixture.incarnation, HarnessGeneration: 1,
		StreamID: fixture.streamID, Sequence: event.Sequence,
		SubmissionID: fixture.submission, TurnID: fixture.turnID,
		Outcome: string(models.DeliverySubmissionCancelled),
	}
	return fixture
}

func deliverySessionStateOf(t *testing.T, repo *Repository, sessionID string) models.TaskSessionState {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	if err != nil || session == nil {
		t.Fatalf("GetTaskSession: %+v, %v", session, err)
	}
	return session.State
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
func TestSQLiteDeliverySessionStateConvergesFromExactSettledTerminal(t *testing.T) {
	repo := newRepoForSessionTests(t)
	fixture := seedSettledSessionStateFixture(t, repo, "converge")
	result, err := repo.CorrectSettledAgentDeliverySessionState(context.Background(), fixture.request)
	if err != nil || result == nil || result.Outcome != models.AgentDeliverySessionStateChanged {
		t.Fatalf("correction = %+v, %v", result, err)
	}
	if result.OldState != models.TaskSessionStateRunning || result.NewState != models.TaskSessionStateWaitingForInput {
		t.Fatalf("correction states = %s -> %s", result.OldState, result.NewState)
	}
	if got := deliverySessionStateOf(t, repo, fixture.sessionID); got != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %s, want WAITING_FOR_INPUT", got)
	}
	repeated, err := repo.CorrectSettledAgentDeliverySessionState(context.Background(), fixture.request)
	if err != nil || repeated.Outcome != models.AgentDeliverySessionStateAlreadyConsistent {
		t.Fatalf("repeated correction = %+v, %v", repeated, err)
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
func TestSQLiteDeliverySessionStateCorrectionIsRetryableAfterWriteFailure(t *testing.T) {
	repo := newRepoForSessionTests(t)
	fixture := seedSettledSessionStateFixture(t, repo, "retry")
	if _, err := repo.db.Exec(`
		CREATE TRIGGER fail_session_state_write BEFORE UPDATE OF state ON task_sessions
		WHEN NEW.state = 'WAITING_FOR_INPUT'
		BEGIN SELECT RAISE(ABORT, 'injected state write failure'); END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	if _, err := repo.CorrectSettledAgentDeliverySessionState(context.Background(), fixture.request); err == nil {
		t.Fatal("injected state write failure was swallowed")
	}
	if got := deliverySessionStateOf(t, repo, fixture.sessionID); got != models.TaskSessionStateRunning {
		t.Fatalf("failed correction committed state %s", got)
	}
	if _, err := repo.db.Exec(`DROP TRIGGER fail_session_state_write`); err != nil {
		t.Fatalf("drop failure trigger: %v", err)
	}
	result, err := repo.CorrectSettledAgentDeliverySessionState(context.Background(), fixture.request)
	if err != nil || result.Outcome != models.AgentDeliverySessionStateChanged {
		t.Fatalf("retry correction = %+v, %v", result, err)
	}
	if got := deliverySessionStateOf(t, repo, fixture.sessionID); got != models.TaskSessionStateWaitingForInput {
		t.Fatalf("retried session state = %s", got)
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
func TestSQLiteDeliverySessionStateRejectsLiveSuccessor(t *testing.T) {
	repo := newRepoForSessionTests(t)
	fixture := seedSettledSessionStateFixture(t, repo, "successor")
	if _, err := repo.PrepareAgentDeliverySubmission(context.Background(), &models.AgentDeliverySubmission{
		ID: "submission-session-state-successor-live", SessionID: fixture.sessionID,
		IncarnationID: fixture.incarnation, HarnessGeneration: 1, OwnerGeneration: 1,
		PayloadHash: "hash-successor-live", Payload: []byte("next prompt"),
		State: models.DeliverySubmissionPrepared,
	}); err != nil {
		t.Fatalf("prepare successor: %v", err)
	}
	result, err := repo.CorrectSettledAgentDeliverySessionState(context.Background(), fixture.request)
	if err != nil || result.Outcome != models.AgentDeliverySessionStateSuperseded {
		t.Fatalf("successor correction = %+v, %v", result, err)
	}
	if got := deliverySessionStateOf(t, repo, fixture.sessionID); got != models.TaskSessionStateRunning {
		t.Fatalf("successor session state = %s, want RUNNING", got)
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
func TestSQLiteDeliverySessionStateRejectsMissingEvidence(t *testing.T) {
	repo := newRepoForSessionTests(t)
	fixture := seedSettledSessionStateFixture(t, repo, "evidence")
	if _, err := repo.db.Exec(repo.db.Rebind(
		`UPDATE agent_delivery_cursors SET projected_sequence = 0 WHERE stream_id = ?`,
	), fixture.streamID); err != nil {
		t.Fatalf("damage projection: %v", err)
	}
	result, err := repo.CorrectSettledAgentDeliverySessionState(context.Background(), fixture.request)
	if err != nil || result.Outcome != models.AgentDeliverySessionStateUnavailable {
		t.Fatalf("missing projection correction = %+v, %v", result, err)
	}
	if got := deliverySessionStateOf(t, repo, fixture.sessionID); got != models.TaskSessionStateRunning {
		t.Fatalf("missing projection committed state %s", got)
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
func TestSQLiteDeliverySessionStateRejectsArchivedTask(t *testing.T) {
	repo := newRepoForSessionTests(t)
	fixture := seedSettledSessionStateFixture(t, repo, "archived")
	if err := repo.ArchiveTask(context.Background(), fixture.taskID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	result, err := repo.CorrectSettledAgentDeliverySessionState(context.Background(), fixture.request)
	if err != nil || result.Outcome != models.AgentDeliverySessionStateSuperseded {
		t.Fatalf("archived correction = %+v, %v", result, err)
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
func TestSQLiteDeliverySessionStateLoadsCurrentSettledTerminal(t *testing.T) {
	repo := newRepoForSessionTests(t)
	fixture := seedSettledSessionStateFixture(t, repo, "load")
	if _, err := repo.db.Exec(repo.db.Rebind(`
		UPDATE agent_delivery_submissions
		SET fully_settled_at = ?, terminal_stream_id = ?, terminal_sequence = 1, terminal_turn_id = ?
		WHERE id = ?`), time.Now().UTC(), fixture.streamID, fixture.turnID, fixture.submission); err != nil {
		t.Fatalf("record settled terminal binding: %v", err)
	}
	loaded, found, err := repo.LoadCurrentSettledAgentDeliveryTerminal(context.Background(), fixture.sessionID)
	if err != nil || !found {
		t.Fatalf("LoadCurrentSettledAgentDeliveryTerminal = %+v, %v, %v", loaded, found, err)
	}
	if loaded.SubmissionID != fixture.submission || loaded.StreamID != fixture.streamID ||
		loaded.TurnID != fixture.turnID || loaded.Outcome != string(models.DeliverySubmissionCancelled) {
		t.Fatalf("loaded terminal = %+v", loaded)
	}
	if loaded.ExecutionID != fixture.executionID || loaded.TaskID != fixture.taskID {
		t.Fatalf("loaded owner = %+v", loaded)
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
func TestSQLiteDeliverySessionStateRejectsUnattemptedSuccessorTurn(t *testing.T) {
	repo := newRepoForSessionTests(t)
	fixture := seedSettledSessionStateFixture(t, repo, "reserved-successor")
	now := time.Now().UTC()
	if err := repo.CreateTurn(context.Background(), &models.Turn{
		ID: "turn-session-state-reserved-successor-next", TaskID: fixture.taskID,
		TaskSessionID: fixture.sessionID, StartedAt: now, CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]interface{}{models.TurnMetaKeyPromptDispatchPending: true},
	}); err != nil {
		t.Fatalf("create reserved successor turn: %v", err)
	}
	if _, err := repo.GetActiveTurnBySessionID(context.Background(), fixture.sessionID); err == nil {
		t.Fatal("test setup: an unattempted reservation must stay outside active-turn authority")
	}
	result, err := repo.CorrectSettledAgentDeliverySessionState(context.Background(), fixture.request)
	if err != nil || result.Outcome != models.AgentDeliverySessionStateSuperseded {
		t.Fatalf("reserved successor correction = %+v, %v", result, err)
	}
	if got := deliverySessionStateOf(t, repo, fixture.sessionID); got != models.TaskSessionStateRunning {
		t.Fatalf("reserved successor session state = %s, want RUNNING", got)
	}
	open, err := repo.GetTurn(context.Background(), "turn-session-state-reserved-successor-next")
	if err != nil || open == nil || open.CompletedAt != nil {
		t.Fatalf("reserved successor turn = %+v, %v", open, err)
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
func TestSQLiteDeliverySessionStateListsCompletedCandidates(t *testing.T) {
	repo := newRepoForSessionTests(t)
	fixture := seedSettledSessionStateFixture(t, repo, "candidates")
	if _, err := repo.db.Exec(repo.db.Rebind(`
		UPDATE agent_delivery_submissions
		SET fully_settled_at = ?, terminal_stream_id = ?, terminal_sequence = 1, terminal_turn_id = ?
		WHERE id = ?`), time.Now().UTC(), fixture.streamID, fixture.turnID, fixture.submission); err != nil {
		t.Fatalf("record settled terminal binding: %v", err)
	}
	candidates, err := repo.ListSettledAgentDeliverySessionStateCandidates(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %d, want one", len(candidates))
	}
	candidate := candidates[0]
	if candidate.SessionID != fixture.sessionID || candidate.SubmissionID != fixture.submission ||
		candidate.TurnID != fixture.turnID || candidate.ExecutionID != fixture.executionID ||
		candidate.Outcome != string(models.DeliverySubmissionCancelled) {
		t.Fatalf("candidate = %+v", candidate)
	}
}
