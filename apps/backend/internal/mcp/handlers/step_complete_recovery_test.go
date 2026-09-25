package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TestHandleStepComplete_StaleTurnRecoveryGuidance covers AC-003.1, AC-003.2,
// and AC-003.5: a stale turn remains harmless in every non-terminal session
// state and explains the fresh-turn recovery path.
func TestHandleStepComplete_StaleTurnRecoveryGuidance(t *testing.T) {
	cases := []struct {
		name  string
		state models.TaskSessionState
	}{
		{name: "running", state: models.TaskSessionStateRunning},
		{name: "waiting for input", state: models.TaskSessionStateWaitingForInput},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			svc, repo := newTestTaskService(t)
			seedStepCompleteTarget(t, repo, "task-stale-recovery", "session-stale-recovery", "step-review", tc.state)
			createStepCompleteRecoveryTurn(t, repo, "turn-review", "task-stale-recovery", "session-stale-recovery")

			task, err := repo.GetTask(ctx, "task-stale-recovery")
			require.NoError(t, err)
			task.WorkflowStepID = "step-work"
			require.NoError(t, repo.UpdateTask(ctx, task))

			bus := &mcpRecordingEventBus{}
			h := newStepCompleteHandler(t, svc, repo, bus)
			msg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
				"task_id":    "task-stale-recovery",
				"session_id": "session-stale-recovery",
				"summary":    "late reviewer signal",
			})

			for attempt := 0; attempt < 2; attempt++ {
				resp, callErr := h.handleStepComplete(ctx, msg)
				require.NoError(t, callErr)
				require.NotNil(t, resp)

				var payload ws.ErrorPayload
				require.NoError(t, json.Unmarshal(resp.Payload, &payload))
				assert.Equal(t, ws.ErrorCodeValidation, payload.Code)
				assert.Contains(t, payload.Message, "workflow step changed before signal was recorded")
				assert.Contains(t, payload.Message, "turn started in step step-review")
				assert.Contains(t, payload.Message, "current step is step-work")
				assert.Contains(t, payload.Message, "No signal was recorded")
				assert.Contains(t, payload.Message, "Retrying in this turn cannot recover")
				assert.Contains(t, payload.Message, "End this turn and ask the user to resume")
				assert.Contains(t, payload.Message, "Do not move the task solely to bypass this error")
			}

			assert.Empty(t, bus.events, "stale calls must not publish a completion event")
			session, err := repo.GetTaskSession(ctx, "session-stale-recovery")
			require.NoError(t, err)
			_, hasSignal := models.LoadPendingStepSignal(session.Metadata)
			assert.False(t, hasSignal, "stale calls must not persist a completion signal")
		})
	}
}

// TestHandleStepComplete_FreshTurnAfterStepChange covers AC-003.3: a new
// turn stamped with the current step can complete normally after an older
// turn was rejected.
func TestHandleStepComplete_FreshTurnAfterStepChange(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	seedStepCompleteTarget(t, repo, "task-fresh-recovery", "session-fresh-recovery", "step-review", models.TaskSessionStateRunning)
	createStepCompleteRecoveryTurn(t, repo, "turn-review", "task-fresh-recovery", "session-fresh-recovery")

	task, err := repo.GetTask(ctx, "task-fresh-recovery")
	require.NoError(t, err)
	task.WorkflowStepID = "step-work"
	require.NoError(t, repo.UpdateTask(ctx, task))

	bus := &mcpRecordingEventBus{}
	h := newStepCompleteHandler(t, svc, repo, bus)
	staleMsg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-fresh-recovery",
		"session_id": "session-fresh-recovery",
		"summary":    "late reviewer signal",
	})
	staleResp, err := h.handleStepComplete(ctx, staleMsg)
	require.NoError(t, err)
	assertWSError(t, staleResp, ws.ErrorCodeValidation)

	createStepCompleteRecoveryTurn(t, repo, "turn-work", "task-fresh-recovery", "session-fresh-recovery")
	freshMsg := makeWSMessage(t, ws.ActionMCPStepComplete, map[string]interface{}{
		"task_id":    "task-fresh-recovery",
		"session_id": "session-fresh-recovery",
		"summary":    "work step finished",
	})
	freshResp, err := h.handleStepComplete(ctx, freshMsg)
	require.NoError(t, err)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(freshResp.Payload, &payload))
	assert.Equal(t, true, payload["accepted"])
	assert.Equal(t, "step-work", payload["step_id"])
	require.Len(t, bus.events, 1, "the fresh current-step turn must publish one event")

	session, err := repo.GetTaskSession(ctx, "session-fresh-recovery")
	require.NoError(t, err)
	signal, ok := models.LoadPendingStepSignal(session.Metadata)
	require.True(t, ok, "the fresh turn must persist its completion signal")
	assert.Equal(t, "step-work", signal.StepID)
}

func createStepCompleteRecoveryTurn(
	t *testing.T,
	repo *sqliterepo.Repository,
	turnID, taskID, sessionID string,
) {
	t.Helper()
	now := time.Now().UTC()
	turn := &models.Turn{
		ID:            turnID,
		TaskSessionID: sessionID,
		TaskID:        taskID,
		StartedAt:     now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	stamped, err := repo.CreateTurnWithStepStamp(context.Background(), turn)
	require.NoError(t, err)
	require.True(t, stamped, "turn must carry its launch step stamp")
}
