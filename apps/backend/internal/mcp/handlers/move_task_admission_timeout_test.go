package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
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
	return messagequeue.ErrSessionAdmissionTimeout
}

// @covers AC-TASKS-BOUNDED-QUEUE-ADMISSION-001.7
func TestMoveTaskDeferredAdmissionTimeoutIsRetryable(t *testing.T) {
	taskService, repo := newTestTaskService(t)
	seedRunningTask(t, repo,
		"ws-admission-timeout", "wf-admission-timeout", "task-admission-timeout",
		"session-admission-timeout", "step-current",
	)
	queue := &admissionTimeoutMoveQueuer{}
	h := &Handlers{taskSvc: taskService, messageQueue: queue, logger: testLogger(t).WithFields()}
	msg := makeWSMessage(t, ws.ActionMCPMoveTask, map[string]interface{}{
		"task_id":          "task-admission-timeout",
		"workflow_id":      "wf-admission-timeout",
		"workflow_step_id": "step-target",
	})

	response, err := h.handleMoveTask(context.Background(), msg)
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeUnavailable)
	var errorPayload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &errorPayload))
	require.Contains(t, errorPayload.Message, "retry")
	require.Equal(t, 1, queue.pendingMoveAttempts)
}
