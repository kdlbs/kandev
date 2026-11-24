package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type failingPendingMoveQueuer struct {
	recordingMessageQueuer
	err error
}

func (f *failingPendingMoveQueuer) SetPendingMove(context.Context, string, *messagequeue.PendingMove) error {
	return f.err
}

func deferredMoveErrorCode(t *testing.T, setErr error) string {
	t.Helper()
	svc, repo, wfCtrl, wfRepo := newTestTaskServiceWithWorkflow(t)
	ctx := context.Background()
	seedRunningTask(t, repo, "ws-defer-busy", "wf-defer-busy", "task-defer-busy", "sess-defer-busy", "src-step-busy")
	now := time.Now().UTC()
	require.NoError(t, wfRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "src-step-busy", WorkflowID: "wf-defer-busy", Name: "Source", Position: 0, CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, wfRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
		ID: "dst-step-busy", WorkflowID: "wf-defer-busy", Name: "Dest", Position: 1, CreatedAt: now, UpdatedAt: now,
	}))
	h := &Handlers{
		taskSvc: svc, taskRepo: repo, workflowCtrl: wfCtrl,
		messageQueue: &failingPendingMoveQueuer{err: setErr},
		logger:       testLogger(t).WithFields(),
	}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id": "task-defer-busy", "workflow_id": "wf-defer-busy",
		"workflow_step_id": "dst-step-busy", "position": 0, "sender_session_id": "sess-defer-busy",
	})
	resp, err := h.handleMoveTask(ctx, msg)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, resp.Type)
	var payload ws.ErrorPayload
	require.NoError(t, resp.ParsePayload(&payload))
	return payload.Code
}

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.3
func TestMoveTaskDeferredAdmissionTimeoutIsRetryable(t *testing.T) {
	require.Equal(t, ws.ErrorCodeUnavailable, deferredMoveErrorCode(t, messagequeue.ErrSessionAdmissionTimeout))
	require.Equal(t, ws.ErrorCodeInternalError, deferredMoveErrorCode(t, errors.New("disk full")),
		"other persistence failures keep the internal error")
}
