package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type movingStepCompleteEventBus struct {
	repo     *sqliterepo.Repository
	taskID   string
	targetID string
}

// Publish moves the task synchronously so the response projects the committed route.
func (b *movingStepCompleteEventBus) Publish(ctx context.Context, _ string, _ *bus.Event) error {
	task, err := b.repo.GetTask(ctx, b.taskID)
	if err != nil {
		return err
	}
	task.WorkflowStepID = b.targetID
	return b.repo.UpdateTask(ctx, task)
}

type failingSnapshotRepo struct {
	*sqliterepo.Repository
}

// GetTaskCompletionSnapshot forces the projection to handle a failed snapshot read.
func (r *failingSnapshotRepo) GetTaskCompletionSnapshot(ctx context.Context, taskID string) (models.TaskCompletionSnapshot, error) {
	return models.TaskCompletionSnapshot{}, errors.New("snapshot unavailable")
}

type moveBeforeSnapshotRepo struct {
	*sqliterepo.Repository
	taskID   string
	targetID string
	moved    bool
}

// GetTaskCompletionSnapshot moves the task before delegating to the real reader.
func (r *moveBeforeSnapshotRepo) GetTaskCompletionSnapshot(ctx context.Context, taskID string) (models.TaskCompletionSnapshot, error) {
	if !r.moved {
		r.moved = true
		task, err := r.GetTask(ctx, r.taskID)
		if err != nil {
			return models.TaskCompletionSnapshot{}, err
		}
		task.WorkflowStepID = r.targetID
		if err := r.UpdateTask(ctx, task); err != nil {
			return models.TaskCompletionSnapshot{}, err
		}
	}
	return r.Repository.GetTaskCompletionSnapshot(ctx, taskID)
}

func projectStepCompleteNativeTurn(t *testing.T, repo *sqliterepo.Repository, sessionID, streamID, text string) {
	t.Helper()
	_, err := repo.DB().Exec(`UPDATE task_sessions SET queue_incarnation_id='incarnation' WHERE id=?`, sessionID)
	require.NoError(t, err)
	agent := streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: text, CanonicalMessageID: "message-" + streamID}
	raw, err := json.Marshal(agent)
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{
		SessionID: sessionID, IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: streamID, Sequence: 1, EventType: agent.Type, Payload: raw,
	}
	_, err = repo.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
	require.NoError(t, err)
	effect := &models.AgentDeliveryEffect{EffectKey: "event-" + streamID, StreamID: streamID, Sequence: 1, EffectType: "agent_delivery.event"}
	_, err = repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), event, effect)
	require.NoError(t, err)
}

func sendStepComplete(t *testing.T, h *Handlers, taskID, sessionID, summary, handoff string) map[string]interface{} {
	t.Helper()
	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id": taskID, "session_id": sessionID, "summary": summary, "handoff": handoff,
	})
	response, err := h.handleStepComplete(t.Context(), msg)
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	return payload
}

// TestHandleStepComplete_NewerNativeTurnReplacesPendingSignal covers the source-specific completion contract.
func TestHandleStepComplete_NewerNativeTurnReplacesPendingSignal(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task", "session", "step-a", models.TaskSessionStateRunning)
	createStepCompleteRecoveryTurn(t, repo, "turn-1", "task", "session")
	recorder := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, recorder)

	first := sendStepComplete(t, h, "task", "session", "first summary", "old review handoff")
	require.Equal(t, true, first["accepted"])
	firstSignalID := first["signal_id"].(string)
	require.NotEmpty(t, firstSignalID)

	require.NoError(t, repo.CompleteTurn(t.Context(), "turn-1"))
	projectStepCompleteNativeTurn(t, repo, "session", "correction", "later native output")

	second := sendStepComplete(t, h, "task", "session", "corrected summary", "latest review handoff")
	require.Equal(t, true, second["accepted"], "a later current turn must replace the earlier pending signal: %v", second)
	require.NotEqual(t, firstSignalID, second["signal_id"])
	require.Equal(t, "agent-delivery-turn-correction-1", second["source"].(map[string]interface{})["turn_id"])
	require.Len(t, recorder.events, 2, "replacement must publish its own wakeup")

	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	pending, ok := models.LoadPendingStepSignal(session.Metadata)
	require.True(t, ok)
	require.Equal(t, "latest review handoff", pending.Handoff)
	require.Equal(t, "agent-delivery-turn-correction-1", pending.SourceTurnID)
	receipt, ok := models.LoadStepCompletionReceipt(session.Metadata)
	require.True(t, ok)
	require.Equal(t, pending.SignalID, receipt.SignalID)
	require.Equal(t, models.StepCompletionOutcomePending, receipt.Outcome)
}

// TestHandleStepComplete_SameTurnKeepsFirstPayload covers the source-specific completion contract.
func TestHandleStepComplete_SameTurnKeepsFirstPayload(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task", "session", "step-a", models.TaskSessionStateRunning)
	createStepCompleteRecoveryTurn(t, repo, "turn-1", "task", "session")
	recorder := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, recorder)

	first := sendStepComplete(t, h, "task", "session", "first", "first handoff")
	require.Equal(t, true, first["accepted"])
	second := sendStepComplete(t, h, "task", "session", "changed", "changed handoff")
	require.Equal(t, false, second["accepted"])
	require.Equal(t, "already_signaled", second["reason"])
	require.Equal(t, first["signal_id"], second["signal_id"])
	require.Equal(t, "turn-1", second["source"].(map[string]interface{})["turn_id"])
	require.Equal(t, models.StepCompletionOutcomePending, second["outcome"].(map[string]interface{})["status"])
	require.Len(t, recorder.events, 1, "a same-turn duplicate must not publish another event")

	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	pending, ok := models.LoadPendingStepSignal(session.Metadata)
	require.True(t, ok)
	require.Equal(t, "first handoff", pending.Handoff)
}

// TestHandleStepComplete_NativeVerificationKeepsHandoff covers the source-specific completion contract.
func TestHandleStepComplete_NativeVerificationKeepsHandoff(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task", "session", "step-a", models.TaskSessionStateRunning)
	createStepCompleteRecoveryTurn(t, repo, "turn-1", "task", "session")
	h := newStepCompleteHandler(t, svc, repo, &mcpRecordingEventBus{})

	accepted := sendStepComplete(t, h, "task", "session", "done", "keep me")
	require.Equal(t, true, accepted["accepted"])
	signalID := accepted["signal_id"].(string)

	require.NoError(t, repo.CompleteTurn(t.Context(), "turn-1"))
	projectStepCompleteNativeTurn(t, repo, "session", "verify", "verification output")

	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	pending, ok := models.LoadPendingStepSignal(session.Metadata)
	require.True(t, ok)
	require.Equal(t, signalID, pending.SignalID)
	require.Equal(t, "keep me", pending.Handoff)
	require.Equal(t, "turn-1", pending.SourceTurnID,
		"native verification without another completion call must not replace the accepted source")
}

// TestHandleStepComplete_DuplicateOutcome covers the source-specific completion contract.
func TestHandleStepComplete_DuplicateOutcome(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task", "session", "step-a", models.TaskSessionStateRunning)
	createStepCompleteRecoveryTurn(t, repo, "turn-1", "task", "session")
	h := newStepCompleteHandler(t, svc, repo, &mcpRecordingEventBus{})

	first := sendStepComplete(t, h, "task", "session", "done", "handoff")
	require.Equal(t, true, first["accepted"])
	require.NotEmpty(t, first["step_id"])
	require.NotEmpty(t, first["signaled_at"])

	duplicate := sendStepComplete(t, h, "task", "session", "done again", "")
	require.Equal(t, false, duplicate["accepted"])
	require.Equal(t, "already_signaled", duplicate["reason"])
	require.Equal(t, first["signal_id"], duplicate["signal_id"])
	require.Equal(t, "step-a", duplicate["step_id"])
	require.Equal(t, first["signaled_at"], duplicate["signaled_at"])
	source := duplicate["source"].(map[string]interface{})
	require.Equal(t, "turn-1", source["turn_id"])
	require.Equal(t, true, source["turn_known"])
	require.NotNil(t, source["entry_id"])
	require.Equal(t, models.StepCompletionOutcomePending, duplicate["outcome"].(map[string]interface{})["status"])
	current := duplicate["current"].(map[string]interface{})
	require.Equal(t, "step-a", current["step_id"])
	require.Equal(t, string(v1.TaskStateInProgress), current["state"])
	_, hasTransition := duplicate["transition"]
	require.False(t, hasTransition, "a pending duplicate must not claim a transition")
}

// TestHandleStepComplete_ReceiptAfterApplied covers the source-specific completion contract.
func TestHandleStepComplete_ReceiptAfterApplied(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task", "session", "step-a", models.TaskSessionStateRunning)
	createStepCompleteRecoveryTurn(t, repo, "turn-1", "task", "session")
	recorder := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, recorder)

	first := sendStepComplete(t, h, "task", "session", "done", "handoff")
	require.Equal(t, true, first["accepted"])

	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	receipt, ok := models.LoadStepCompletionReceipt(session.Metadata)
	require.True(t, ok)
	receipt = receipt.WithTransition(models.StepCompletionTransition{
		LedgerID: 42, WorkflowID: "wf", StepID: "step-a",
	})
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "session", models.SessionMetaKeyLastStepCompletionReceipt, receipt))
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "session", models.SessionMetaKeyPendingStepCompletion, nil))

	duplicate := sendStepComplete(t, h, "task", "session", "retry", "")
	require.Equal(t, false, duplicate["accepted"])
	require.Equal(t, models.StepCompletionOutcomeTransitionApplied, duplicate["outcome"].(map[string]interface{})["status"])
	transition := duplicate["transition"].(map[string]interface{})
	require.EqualValues(t, 42, transition["ledger_id"])
	require.Equal(t, "step-a", transition["step_id"])
	require.Len(t, recorder.events, 1, "a committed receipt must not publish another transition")
}

// TestHandleStepComplete_UnconfiguredOutcome covers the source-specific completion contract.
func TestHandleStepComplete_UnconfiguredOutcome(t *testing.T) {
	svc, repo, workflowCtrl, workflowRepo := newTestTaskServiceWithWorkflow(t)
	seedStepCompleteTarget(t, repo, "task", "session", "step-a", models.TaskSessionStateRunning)
	createStepCompleteRecoveryTurn(t, repo, "turn-1", "task", "session")
	require.NoError(t, repo.CreateWorkflow(t.Context(), &models.Workflow{ID: "wf-step-complete", WorkspaceID: "ws-step-complete", Name: "Board"}))
	require.NoError(t, workflowRepo.CreateStep(t.Context(), &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf-step-complete", Name: "Build", Position: 1,
	}))
	h := newStepCompleteHandler(t, svc, repo, &mcpRecordingEventBus{})
	h.workflowCtrl = workflowCtrl

	payload := sendStepComplete(t, h, "task", "session", "done", "")
	require.Equal(t, true, payload["accepted"])
	require.Equal(t, false, payload["advances"])
	outcome := payload["outcome"].(map[string]interface{})
	require.Equal(t, models.StepCompletionOutcomeNotConfigured, outcome["status"])
	require.Equal(t, models.StepCompletionReasonStepNotSignalGated, outcome["reason"])

	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	receipt, ok := models.LoadStepCompletionReceipt(session.Metadata)
	require.True(t, ok)
	require.Equal(t, models.StepCompletionOutcomeNotConfigured, receipt.Outcome)
	require.Equal(t, models.StepCompletionReasonStepNotSignalGated, receipt.Reason)
}

// TestHandleStepComplete_CurrentSnapshotAfterPublishMove covers the source-specific completion contract.
func TestHandleStepComplete_CurrentSnapshotAfterPublishMove(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task", "session", "step-a", models.TaskSessionStateRunning)
	_, err := repo.DB().Exec(`UPDATE task_sessions SET is_primary=1 WHERE id='session'`)
	require.NoError(t, err)
	createStepCompleteRecoveryTurn(t, repo, "turn-1", "task", "session")
	_, _, beforeEntry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
	require.NoError(t, err)

	h := newStepCompleteHandler(t, svc, repo, &mcpRecordingEventBus{})
	h.eventBus = &movingStepCompleteEventBus{repo: repo, taskID: "task", targetID: "step-b"}

	payload := sendStepComplete(t, h, "task", "session", "done", "handoff")
	require.Equal(t, true, payload["accepted"])
	require.Equal(t, "step-a", payload["step_id"], "top-level step_id remains the source step")
	current := payload["current"].(map[string]interface{})
	require.Equal(t, "step-b", current["step_id"], "current must reflect the publication-time move")
	require.Equal(t, "session", current["primary_session_id"])
	require.Greater(t, current["entry_id"].(float64), float64(beforeEntry))
}

// TestHandleStepComplete_CurrentSnapshotSurvivesProjectionMove covers the source-specific completion contract.
func TestHandleStepComplete_CurrentSnapshotSurvivesProjectionMove(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task", "session", "step-a", models.TaskSessionStateRunning)
	_, err := repo.DB().Exec(`UPDATE task_sessions SET is_primary=1 WHERE id='session'`)
	require.NoError(t, err)
	createStepCompleteRecoveryTurn(t, repo, "turn-1", "task", "session")
	_, _, beforeEntry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
	require.NoError(t, err)

	repoWithMove := &moveBeforeSnapshotRepo{Repository: repo, taskID: "task", targetID: "step-c"}
	h := &Handlers{
		taskSvc: svc, sessionRepo: repoWithMove,
		eventBus: &mcpRecordingEventBus{}, logger: testLogger(t).WithFields(),
	}

	payload := sendStepComplete(t, h, "task", "session", "done", "handoff")
	require.Equal(t, true, payload["accepted"])
	current := payload["current"].(map[string]interface{})
	require.Equal(t, "step-c", current["step_id"])
	require.Greater(t, current["entry_id"].(float64), float64(beforeEntry))
	require.Equal(t, "session", current["primary_session_id"])
}

// TestHandleStepComplete_SnapshotFailureOmitsCurrent covers the source-specific completion contract.
func TestHandleStepComplete_SnapshotFailureOmitsCurrent(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task", "session", "step-a", models.TaskSessionStateRunning)
	createStepCompleteRecoveryTurn(t, repo, "turn-1", "task", "session")
	h := &Handlers{
		taskSvc: svc, sessionRepo: &failingSnapshotRepo{Repository: repo},
		eventBus: &mcpRecordingEventBus{}, logger: testLogger(t).WithFields(),
	}

	payload := sendStepComplete(t, h, "task", "session", "done", "handoff")
	require.Equal(t, true, payload["accepted"])
	_, hasCurrent := payload["current"]
	require.False(t, hasCurrent, "a failed snapshot must omit current rather than return a stale row")
}

// TestHandleStepComplete_TerminalSessionRejectsBeforeStaleStep covers the source-specific completion contract.
func TestHandleStepComplete_TerminalSessionRejectsBeforeStaleStep(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task", "session", "step-a", models.TaskSessionStateCompleted)
	createStepCompleteRecoveryTurn(t, repo, "turn-1", "task", "session")
	task, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	task.WorkflowStepID = "step-b"
	require.NoError(t, repo.UpdateTask(t.Context(), task))
	h := newStepCompleteHandler(t, svc, repo, &mcpRecordingEventBus{})

	payload := sendStepComplete(t, h, "task", "session", "retry", "")
	require.Equal(t, ws.ErrorCodeValidation, payload["code"])
	require.Contains(t, payload["message"], "terminal session")
}

// TestHandleStepComplete_TerminalSessionLaunchErrorIsValidation covers the source-specific completion contract.
func TestHandleStepComplete_TerminalSessionLaunchErrorIsValidation(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task", "session", "step-a", models.TaskSessionStateCompleted)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTurn(t.Context(), &models.Turn{
		ID: "unstamped-turn", TaskSessionID: "session", TaskID: "task", StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}))
	h := newStepCompleteHandler(t, svc, repo, &mcpRecordingEventBus{})

	payload := sendStepComplete(t, h, "task", "session", "retry", "")
	require.Equal(t, ws.ErrorCodeValidation, payload["code"])
	require.Contains(t, payload["message"], "terminal session")
}

// TestHandleStepComplete_ReceiptAfterDestinationEntry covers the source-specific completion contract.
func TestHandleStepComplete_ReceiptAfterDestinationEntry(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task", "session", "step-a", models.TaskSessionStateWaitingForInput)
	createStepCompleteRecoveryTurn(t, repo, "turn-1", "task", "session")
	recorder := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, recorder)

	first := sendStepComplete(t, h, "task", "session", "done", "handoff")
	require.Equal(t, true, first["accepted"])

	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	receipt, ok := models.LoadStepCompletionReceipt(session.Metadata)
	require.True(t, ok)
	receipt = receipt.WithTransition(models.StepCompletionTransition{LedgerID: 51, WorkflowID: "wf", StepID: "step-b"})
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "session", models.SessionMetaKeyLastStepCompletionReceipt, receipt))
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "session", models.SessionMetaKeyPendingStepCompletion, nil))
	task, err := repo.GetTask(t.Context(), "task")
	require.NoError(t, err)
	task.WorkflowStepID = "step-b"
	require.NoError(t, repo.UpdateTask(t.Context(), task))

	duplicate := sendStepComplete(t, h, "task", "session", "retry after restart", "")
	require.Equal(t, false, duplicate["accepted"], "a retry from the recorded source must not create a new signal: %v", duplicate)
	require.Equal(t, models.StepCompletionOutcomeTransitionApplied, duplicate["outcome"].(map[string]interface{})["status"])
	require.EqualValues(t, 51, duplicate["transition"].(map[string]interface{})["ledger_id"])
	require.Equal(t, "step-b", duplicate["current"].(map[string]interface{})["step_id"])
	require.Len(t, recorder.events, 1)
}

// TestHandleStepComplete_ReceiptDeniedToForeignCaller covers the source-specific completion contract.
func TestHandleStepComplete_ReceiptDeniedToForeignCaller(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-a", "session-a", "step-a", models.TaskSessionStateRunning)
	createStepCompleteRecoveryTurn(t, repo, "turn-a", "task-a", "session-a")
	h := newStepCompleteHandler(t, svc, repo, &mcpRecordingEventBus{})
	first := sendStepComplete(t, h, "task-a", "session-a", "done", "handoff")
	require.Equal(t, true, first["accepted"])

	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
		ID: "task-b", WorkspaceID: "ws-step-complete", WorkflowStepID: "step-a", Title: "Foreign", State: v1.TaskStateInProgress,
	}))
	msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id": "task-b", "session_id": "session-a", "summary": "steal",
	})
	response, err := h.handleStepComplete(t.Context(), msg)
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeValidation)
	require.NotContains(t, string(response.Payload), first["signal_id"].(string))
}

// TestHandleStepComplete_LegacyOutcomeUnknown covers the source-specific completion contract.
func TestHandleStepComplete_LegacyOutcomeUnknown(t *testing.T) {
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task", "session", "step-a", models.TaskSessionStateRunning)
	_, _, entry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
	require.NoError(t, err)
	legacy := models.PendingStepCompletionSignal{
		StepID: "step-a", WorkflowStepTransitionID: &entry, Summary: "legacy", SignaledAt: time.Now().UTC(),
	}
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "session", models.SessionMetaKeyPendingStepCompletion, legacy))
	h := newStepCompleteHandler(t, svc, repo, &mcpRecordingEventBus{})

	duplicate := sendStepComplete(t, h, "task", "session", "retry", "")
	require.Equal(t, false, duplicate["accepted"])
	require.Empty(t, duplicate["signal_id"])
	require.Equal(t, models.StepCompletionOutcomeUnproven, duplicate["outcome"].(map[string]interface{})["status"])
	require.False(t, duplicate["source"].(map[string]interface{})["turn_known"].(bool))
	_, hasTransition := duplicate["transition"]
	require.False(t, hasTransition, "a legacy signal must not invent transition proof")
}
