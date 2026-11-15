package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type turnErrorQueueAccess struct {
	allowQueueIdentityAccess
	stamp     string
	taskID    string
	sessionID string
	err       error
}

func (a *turnErrorQueueAccess) AcknowledgeTaskTurnError(
	_ context.Context,
	taskID, sessionID, stamp string,
) error {
	a.taskID, a.sessionID, a.stamp = taskID, sessionID, stamp
	return a.err
}

func TestQueueAdmissionAcknowledgesOnlyNewObservedTurnError(t *testing.T) {
	log := logger.Default()
	queue := messagequeue.NewServiceMemory(log)
	access := &turnErrorQueueAccess{}
	handlers := NewQueueHandlers(queue, &mockEventBus{}, log, nil, access, nil)

	response, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"task_id":                   "task-1",
		"session_id":                "session-1",
		"session_incarnation_id":    "memory:session-1",
		"content":                   "queued follow-up",
		"observed_turn_error_stamp": "failure-1",
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, "failure-1", access.stamp)
	require.Equal(t, "task-1", access.taskID)
	require.Equal(t, "session-1", access.sessionID)
}

func TestQueueAdmissionReportsAcknowledgementFailure(t *testing.T) {
	log := logger.Default()
	queue := messagequeue.NewServiceMemory(log)
	access := &turnErrorQueueAccess{err: errors.New("metadata write failed")}
	handlers := NewQueueHandlers(queue, &mockEventBus{}, log, nil, access, nil)

	response, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"task_id":                   "task-1",
		"session_id":                "session-1",
		"session_incarnation_id":    "memory:session-1",
		"content":                   "queued follow-up",
		"observed_turn_error_stamp": "failure-1",
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	var payload struct {
		TurnErrorAcknowledgementFailed bool `json:"turn_error_acknowledgement_failed"`
	}
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.True(t, payload.TurnErrorAcknowledgementFailed)
}

func TestQueueMaintenanceCannotAcknowledgeWithoutObservation(t *testing.T) {
	log := logger.Default()
	queue := messagequeue.NewServiceMemory(log)
	access := &turnErrorQueueAccess{}
	handlers := NewQueueHandlers(queue, &mockEventBus{}, log, nil, access, nil)

	_, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"task_id":                "task-1",
		"session_id":             "session-1",
		"session_incarnation_id": "memory:session-1",
		"content":                "no observation",
	}))
	require.NoError(t, err)
	require.Empty(t, access.stamp)
}
