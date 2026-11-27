package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// AC-TASKS-WORKFLOW-STAGE-MOVES-001.1, 001.2: a deferred move records the step
// and ledger entry it was requested from, so turn-end replay can tell a later
// move apart from no move.
func TestDeferMoveTask_RecordsSourceStepAndTransition(t *testing.T) {
	svc, repo, wfCtrl, wfRepo := newTestTaskServiceWithWorkflow(t)
	ctx := context.Background()
	seedRunningTask(t, repo, "ws-defer-src", "wf-defer-src", "task-defer-src", "sess-defer-src", "src-step-src")

	now := time.Now().UTC()
	require.NoError(t, wfRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "src-step-src", WorkflowID: "wf-defer-src", Name: "Source", Position: 0, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, wfRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "dst-step-src", WorkflowID: "wf-defer-src", Name: "Dest", Position: 1, CreatedAt: now, UpdatedAt: now,
	}))
	sourceTransitionID, err := repo.GetLatestTaskStepTransitionID(ctx, "task-defer-src")
	require.NoError(t, err)
	require.Positive(t, sourceTransitionID)

	queue := &pendingMoveRecordingQueuer{}
	h := &Handlers{
		taskSvc:      svc,
		taskRepo:     repo,
		workflowCtrl: wfCtrl,
		messageQueue: queue,
		logger:       testLogger(t).WithFields(),
	}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id":           "task-defer-src",
		"workflow_id":       "wf-defer-src",
		"workflow_step_id":  "dst-step-src",
		"position":          0,
		"sender_session_id": "sess-defer-src",
	})

	resp, err := h.handleMoveTask(ctx, msg)
	require.NoError(t, err)
	require.NotEqual(t, ws.MessageTypeError, resp.Type, "valid deferred move must succeed")
	require.Len(t, queue.pendingMoves, 1)

	// Read the recorded move through its persisted JSON shape.
	encoded, err := json.Marshal(queue.pendingMoves[0])
	require.NoError(t, err)
	var stored map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &stored))
	assert.Equal(t, "src-step-src", stored["from_step_id"])
	assert.EqualValues(t, sourceTransitionID, stored["from_transition_id"])
}
