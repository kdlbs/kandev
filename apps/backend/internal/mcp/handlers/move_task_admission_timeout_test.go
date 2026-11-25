package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type admissionTimeoutMoveQueuer struct {
	pendingMoveFailingQueuer
	pendingMoveAttempts int
}

func (q *admissionTimeoutMoveQueuer) SetPendingMove(
	_ context.Context,
	_ string,
	_ *messagequeue.PendingMove,
) error {
	q.pendingMoveAttempts++
	if q.pendingErr != nil {
		return q.pendingErr
	}
	return messagequeue.ErrSessionAdmissionTimeout
}

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.7
func TestMoveTaskDeferredAdmissionTimeoutIsRetryable(t *testing.T) {
	for _, test := range []struct {
		name          string
		pendingErr    error
		wantCode      string
		wantRetryHint bool
	}{
		{name: "admission timeout", wantCode: ws.ErrorCodeUnavailable, wantRetryHint: true},
		{name: "other persistence failure", pendingErr: errors.New("disk full"), wantCode: ws.ErrorCodeInternalError},
	} {
		t.Run(test.name, func(t *testing.T) {
			taskService, repo, workflowCtrl, workflowRepo := newTestTaskServiceWithWorkflow(t)
			ctx := context.Background()
			seedRunningTask(t, repo,
				"ws-admission-timeout", "wf-admission-timeout", "task-admission-timeout",
				"session-admission-timeout", "step-current",
			)
			now := time.Now().UTC()
			require.NoError(t, workflowRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
				ID: "step-current", WorkflowID: "wf-admission-timeout", Name: "Source", Position: 0,
				CreatedAt: now, UpdatedAt: now,
			}))
			require.NoError(t, workflowRepo.CreateStep(ctx, &workflowmodels.WorkflowStep{
				ID: "step-target", WorkflowID: "wf-admission-timeout", Name: "Target", Position: 1,
				CreatedAt: now, UpdatedAt: now,
			}))
			queue := &admissionTimeoutMoveQueuer{
				pendingMoveFailingQueuer: pendingMoveFailingQueuer{pendingErr: test.pendingErr},
			}
			h := &Handlers{
				taskSvc: taskService, taskRepo: repo, workflowCtrl: workflowCtrl,
				messageQueue: queue, logger: testLogger(t).WithFields(),
			}
			msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
				"task_id": "task-admission-timeout", "workflow_id": "wf-admission-timeout",
				"workflow_step_id": "step-target", "position": 0,
				"sender_session_id": "session-admission-timeout",
			})

			response, err := h.handleMoveTask(context.Background(), msg)
			require.NoError(t, err)
			assertWSError(t, response, test.wantCode)
			require.Equal(t, 1, queue.pendingMoveAttempts)
			if test.wantRetryHint {
				var errorPayload ws.ErrorPayload
				require.NoError(t, json.Unmarshal(response.Payload, &errorPayload))
				require.Contains(t, errorPayload.Message, "retry")
			}
		})
	}
}
