package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// newManualStepMoveFixture seeds a running task on a source step and a
// destination step with the given on_enter actions.
func newManualStepMoveFixture(t *testing.T, suffix string, onEnter []workflowmodels.OnEnterAction) (*Handlers, *pendingMoveRecordingQueuer) {
	t.Helper()
	svc, repo, wfCtrl, wfRepo := newTestTaskServiceWithWorkflow(t)
	ctx := context.Background()
	seedRunningTask(t, repo, "ws-"+suffix, "wf-"+suffix, "task-"+suffix, "sess-"+suffix, "src-"+suffix)
	now := time.Now().UTC()
	require.NoError(t, wfRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "src-" + suffix, WorkflowID: "wf-" + suffix, Name: "Source", Position: 0, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, wfRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "dst-" + suffix, WorkflowID: "wf-" + suffix, Name: "Dest", Position: 1, CreatedAt: now, UpdatedAt: now,
		Events: workflowmodels.StepEvents{OnEnter: onEnter},
	}))
	queue := &pendingMoveRecordingQueuer{}
	return &Handlers{
		taskSvc:      svc,
		workflowCtrl: wfCtrl,
		messageQueue: queue,
		logger:       testLogger(t).WithFields(),
	}, queue
}

func moveWithPrompt(t *testing.T, h *Handlers, suffix string) *ws.Message {
	t.Helper()
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id":           "task-" + suffix,
		"workflow_id":       "wf-" + suffix,
		"workflow_step_id":  "dst-" + suffix,
		"position":          0,
		"prompt":            "continue the work",
		"sender_session_id": "sess-" + suffix,
	})
	resp, err := h.handleMoveTask(context.Background(), msg)
	require.NoError(t, err)
	return resp
}

// AC-TASKS-WORKFLOW-STAGE-MOVES-004.1
func TestHandleMoveTask_RejectsPromptForManualStep(t *testing.T) {
	h, queue := newManualStepMoveFixture(t, "manual", nil)

	resp := moveWithPrompt(t, h, "manual")

	assertWSError(t, resp, ws.ErrorCodeValidation)
	assert.Contains(t, string(resp.Payload), "does not start an agent on entry")
	assert.Empty(t, queue.pendingMoves, "a rejected hand-off must not record a pending move")
}

// AC-TASKS-WORKFLOW-STAGE-MOVES-004.2
func TestHandleMoveTask_AcceptsPromptForStepThatStartsRunOnEntry(t *testing.T) {
	h, queue := newManualStepMoveFixture(t, "run", []workflowmodels.OnEnterAction{{Type: workflowmodels.OnEnterQueueRun}})

	resp := moveWithPrompt(t, h, "run")

	assert.NotEqual(t, ws.MessageTypeError, resp.Type, "a step that starts a run on entry accepts entry options")
	require.Len(t, queue.pendingMoves, 1)
}

// AC-TASKS-WORKFLOW-STAGE-MOVES-004.3
func TestMoveTaskErrorMessage_SurfacesKnownValidationSentinel(t *testing.T) {
	assert.Equal(t,
		workflowmove.ErrEntryOptionsRequireStepChange.Error(),
		moveTaskErrorMessage(workflowmove.ErrEntryOptionsRequireStepChange),
	)
}
