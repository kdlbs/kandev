package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type recordingQueueAdmissionReadinessChecker struct {
	calls    int
	identity messagequeue.QueueSessionIdentity
}

func (c *recordingQueueAdmissionReadinessChecker) DrainQueuedMessage(context.Context, string) (bool, error) {
	return false, nil
}

func (c *recordingQueueAdmissionReadinessChecker) CheckQueueAdmissionReadiness(
	_ context.Context,
	identity messagequeue.QueueSessionIdentity,
) {
	c.calls++
	c.identity = identity
}

// @covers AC-TASKS-RESUME-PROMPT-QUEUE-001.3
// @covers AC-TASKS-RESUME-PROMPT-QUEUE-001.4
func TestWsQueueMessageChecksReadinessAfterSuccessfulAdmission(t *testing.T) {
	log := logger.Default()
	queue := messagequeue.NewServiceMemory(log)
	checker := &recordingQueueAdmissionReadinessChecker{}
	handlers := NewQueueHandlers(
		queue,
		&mockEventBus{},
		log,
		checker,
		allowQueueIdentityAccess{},
		nil,
	)

	response, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"task_id":                "task-1",
		"session_id":             "session-1",
		"session_incarnation_id": "memory:session-1",
		"content":                "resume me",
	}))

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, 1, checker.calls)
	require.Equal(t, messagequeue.QueueSessionIdentity{
		TaskID:               "task-1",
		SessionID:            "session-1",
		SessionIncarnationID: "memory:session-1",
	}, checker.identity)
}
