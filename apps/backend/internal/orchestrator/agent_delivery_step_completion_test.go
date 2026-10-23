package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/stretchr/testify/require"
)

func TestDeliveryFallbackStepCompletionStaleEntry(t *testing.T) {
	for _, enginePath := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "engine"}[enginePath], func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "task", "session", "step1")
			_, _, oldEntry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
			require.NoError(t, err)
			signal := models.PendingStepCompletionSignal{StepID: "step1", WorkflowStepTransitionID: &oldEntry, Source: models.StepCompletionSourceAgent, Summary: "old", SignaledAt: time.Now().UTC()}
			task, err := repo.GetTask(t.Context(), "task")
			require.NoError(t, err)
			for _, step := range []string{"step2", "step1"} {
				task.WorkflowStepID = step
				require.NoError(t, repo.UpdateTask(t.Context(), task))
			}
			require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "session", models.SessionMetaKeyPendingStepCompletion, signal))
			getter := newMockStepGetter()
			getter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", AutoAdvanceRequiresSignal: true, Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step2"}}}}}
			getter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Position: 1}
			svc := createTestService(repo, getter, newMockTaskRepo())
			if enginePath {
				svc.initWorkflowEngine()
			}
			session, err := repo.GetTaskSession(t.Context(), "session")
			require.NoError(t, err)
			task, err = repo.GetTask(t.Context(), "task")
			require.NoError(t, err)
			var advanced bool
			if enginePath {
				advanced = svc.processOnTurnCompleteViaEngine(t.Context(), task.ID, session)
			} else {
				advanced = svc.processOnTurnComplete(t.Context(), task, session)
			}
			require.False(t, advanced, "earlier-entry signal must not advance a revisited step")
			task, err = repo.GetTask(t.Context(), "task")
			require.NoError(t, err)
			require.Equal(t, "step1", task.WorkflowStepID)
		})
	}
}

func TestDeliveryFallbackStepCompletionCleanup(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task", "session", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	raw := map[string]interface{}{"step_id": "step1", "workflow_step_transition_id": float64(1), "summary": "observed"}
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "session", models.SessionMetaKeyPendingStepCompletion, raw))
	observed, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	svc.clearConsumedStepSignal(t.Context(), observed)
	stored, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	_, pending := models.LoadPendingStepSignal(stored.Metadata)
	require.False(t, pending, "conditional cleanup must compare JSON structurally")
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "session", models.SessionMetaKeyPendingStepCompletion, raw))
	observed, err = repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	raw["workflow_step_transition_id"] = float64(2)
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "session", models.SessionMetaKeyPendingStepCompletion, raw))
	svc.clearConsumedStepSignal(t.Context(), observed)
	stored, err = repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	signal, pending := models.LoadPendingStepSignal(stored.Metadata)
	require.True(t, pending)
	require.EqualValues(t, 2, *signal.WorkflowStepTransitionID)
}

type completionMoveRaceRepo struct {
	*sqliterepo.Repository
	beforeMove func()
}

func (r *completionMoveRaceRepo) UpdateTaskWithWorkflowStepAdmission(ctx context.Context, task *models.Task, from, to string, limit int) (bool, error) {
	r.beforeMove()
	return r.Repository.UpdateTaskWithWorkflowStepAdmission(ctx, task, from, to, limit)
}

func (r *completionMoveRaceRepo) UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx context.Context, task *models.Task, from, to string, limit int) (bool, error) {
	r.beforeMove()
	return r.Repository.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, task, from, to, limit)
}

func TestDeliveryFallbackStepCompletionMoveRace(t *testing.T) {
	for _, path := range []string{"legacy", "legacy_successor", "engine"} {
		t.Run(path, func(t *testing.T) {
			enginePath := path == "engine"
			repo := setupTestRepo(t)
			seedSession(t, repo, "task", "session", "step1")
			_, _, entry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
			require.NoError(t, err)
			_, _, err = repo.ClaimStepCompletionSignal(t.Context(), "task", "session", models.PendingStepCompletionSignal{StepID: "step1", WorkflowStepTransitionID: &entry, Summary: "done"})
			require.NoError(t, err)
			getter := newMockStepGetter()
			getter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", AutoAdvanceRequiresSignal: true, Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step2"}}}}}
			getter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Position: 1}
			svc := createTestService(repo, getter, newMockTaskRepo())
			raced := false
			svc.repo = &completionMoveRaceRepo{Repository: repo, beforeMove: func() {
				require.False(t, raced)
				raced = true
				task, err := repo.GetTask(t.Context(), "task")
				require.NoError(t, err)
				for _, step := range []string{"step2", "step1"} {
					task.WorkflowStepID = step
					require.NoError(t, repo.UpdateTask(t.Context(), task))
				}
				if path == "legacy_successor" {
					_, _, err = repo.ClaimStepCompletionSignal(t.Context(), "task", "session", models.PendingStepCompletionSignal{StepID: "step1", WorkflowStepTransitionID: signalEntryForSession(t, repo, "session"), Summary: "successor"})
					require.NoError(t, err)
				}
			}}
			if enginePath {
				svc.initWorkflowEngine()
			}
			session, err := repo.GetTaskSession(t.Context(), "session")
			require.NoError(t, err)
			task, err := repo.GetTask(t.Context(), "task")
			require.NoError(t, err)
			if enginePath {
				svc.processOnTurnCompleteViaEngine(t.Context(), task.ID, session)
			} else {
				svc.processOnTurnComplete(t.Context(), task, session)
			}
			require.True(t, raced)
			task, err = repo.GetTask(t.Context(), "task")
			require.NoError(t, err)
			require.Equal(t, "step1", task.WorkflowStepID, "a move after eligibility must invalidate completion at commit")
			if !enginePath {
				settled, err := repo.GetTaskSession(t.Context(), "session")
				require.NoError(t, err)
				require.Equal(t, models.TaskSessionStateWaitingForInput, settled.State)
				signal, pending := models.LoadPendingStepSignal(settled.Metadata)
				require.Equal(t, path == "legacy_successor", pending, "rejected completion must clear only its observed signal")
				if pending {
					require.Equal(t, "successor", signal.Summary)
				}
			}
		})
	}
}

func TestDeliveryFallbackStepCompletionAdvancesOnce(t *testing.T) {
	for _, path := range []string{"legacy", "engine", "waiting"} {
		t.Run(path, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "task", "session", "step1", "incarnation")
			predecessor := &models.Turn{ID: "previous", TaskID: "task", TaskSessionID: "session"}
			_, err := repo.CreateTurnWithStepStamp(t.Context(), predecessor)
			require.NoError(t, err)
			require.NoError(t, repo.CompleteTurn(t.Context(), predecessor.ID))
			raw, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "self wake", CanonicalMessageID: "message"})
			require.NoError(t, err)
			event := &models.AgentDeliveryEvent{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream", Sequence: 1, EventType: streams.EventTypeMessageChunk, Payload: raw}
			_, err = repo.ReceiveAgentDeliveryEvent(t.Context(), event, 1)
			require.NoError(t, err)
			effect := &models.AgentDeliveryEffect{EffectKey: "wake", StreamID: "stream", Sequence: 1, EffectType: "agent_delivery.event"}
			_, err = repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), event, effect)
			require.NoError(t, err)
			turn, err := repo.GetTurn(t.Context(), "agent-delivery-turn-stream-1")
			require.NoError(t, err)
			entry, ok := models.WorkflowEntryIDFromAny(turn.Metadata[models.TurnMetaKeyWorkflowStepTransitionIDAtStart])
			require.True(t, ok)
			signal, claimed, err := repo.ClaimStepCompletionSignal(t.Context(), "task", "session", models.PendingStepCompletionSignal{StepID: models.StringFromAny(turn.Metadata[models.TurnMetaKeyWorkflowStepIDAtStart]), WorkflowStepTransitionID: &entry, Summary: "done", SignaledAt: time.Now().UTC()})
			require.NoError(t, err)
			require.True(t, claimed)
			getter := newMockStepGetter()
			getter.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1", AutoAdvanceRequiresSignal: true, Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step2"}}}}}
			getter.steps["step2"] = &wfmodels.WorkflowStep{ID: "step2", WorkflowID: "wf1", Position: 1}
			svc := createTestService(repo, getter, newMockTaskRepo())
			if path != "legacy" {
				svc.initWorkflowEngine()
			}
			if path == "waiting" {
				require.NoError(t, repo.UpdateTaskSessionState(t.Context(), "session", models.TaskSessionStateWaitingForInput, ""))
			}
			session, err := repo.GetTaskSession(t.Context(), "session")
			require.NoError(t, err)
			task, err := repo.GetTask(t.Context(), "task")
			require.NoError(t, err)
			switch path {
			case "legacy":
				require.True(t, svc.processOnTurnComplete(t.Context(), task, session))
			case "engine":
				require.True(t, svc.processOnTurnCompleteViaEngine(t.Context(), task.ID, session))
			case "waiting":
				svc.onStepCompletionSignaled(t.Context(), bus.NewEvent("workflow.step_completion_signaled", "test", map[string]interface{}{"task_id": "task", "session_id": "session", "step_id": "step1"}))
			}
			task, err = repo.GetTask(t.Context(), "task")
			require.NoError(t, err)
			require.Equal(t, "step2", task.WorkflowStepID)
			_, _, afterEntry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
			require.NoError(t, err)
			_, err = repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), event, effect)
			require.NoError(t, err)
			// Delayed delivery of the original signal is just a wakeup, not new authority.
			svc.onStepCompletionSignaled(t.Context(), bus.NewEvent("workflow.step_completion_signaled", "test", map[string]interface{}{"task_id": "task", "session_id": "session", "step_id": signal.StepID}))
			_, _, finalEntry, err := repo.GetTaskWorkflowStepEntry(t.Context(), "task")
			require.NoError(t, err)
			require.Equal(t, afterEntry, finalEntry)
		})
	}
}

func TestDeliveryFallbackStepCompletionWatchdogStaleEntry(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task", "session", "step1")
	oldEntry := int64(0)
	require.NoError(t, repo.SetSessionMetadataKey(t.Context(), "session", models.SessionMetaKeyPendingStepCompletion, models.PendingStepCompletionSignal{StepID: "step1", WorkflowStepTransitionID: &oldEntry, SignaledAt: time.Now().Add(-time.Hour)}))
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	_, _, candidate := svc.stuckSignalCandidate(t.Context(), session, time.Now())
	require.False(t, candidate, "a prior entry's signal must not authorize cancelling the current turn")
}
