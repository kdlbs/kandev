package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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

// @covers AC-TASKS-QUEUE-ADMISSION-001.1
func TestWsQueueMessagePreservesOrdinaryClientAdmissionIdentity(t *testing.T) {
	log := logger.Default()
	queue := messagequeue.NewServiceMemory(log)
	handlers := NewQueueHandlers(
		queue,
		&mockEventBus{},
		log,
		nil,
		allowQueueIdentityAccess{},
		nil,
	)

	response, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"task_id":                "task-1",
		"session_id":             "session-1",
		"session_incarnation_id": "memory:session-1",
		"client_queue_id":        "ordinary-admission-1",
		"content":                "send once",
	}))

	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	var queued messagequeue.QueuedMessage
	require.NoError(t, json.Unmarshal(response.Payload, &queued))
	require.Equal(t, "ordinary-admission-1", queued.ID)
}

// @covers AC-TASKS-QUEUE-ADMISSION-001.2
// @covers AC-TASKS-QUEUE-ADMISSION-001.3
func TestWsQueueMessageReplaysOrdinaryAdmissionWithoutDuplicatingContent(t *testing.T) {
	log := logger.Default()
	queue := messagequeue.NewServiceMemory(log)
	handlers := NewQueueHandlers(queue, &mockEventBus{}, log, nil, allowQueueIdentityAccess{}, nil)
	payload := map[string]interface{}{
		"task_id":                "task-1",
		"session_id":             "session-1",
		"session_incarnation_id": "memory:session-1",
		"client_queue_id":        "ordinary-admission-2",
		"content":                "send once",
	}

	first, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, first.Type)
	second, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, second.Type)
	entries, _, err := queue.SnapshotSession(context.Background(), "session-1")
	require.NoError(t, err)
	require.Len(t, entries, 1)

	payload["content"] = "changed"
	conflict, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeError, conflict.Type)
	require.Equal(t, "queue_id_conflict", parseError(t, conflict).Code)
}

// @covers AC-TASKS-QUEUE-ADMISSION-001.2
func TestWsQueueMessageReplaysBeforeMutableReferenceValidation(t *testing.T) {
	reference := v1.EntityReference{
		Version:  v1.EntityReferenceVersion,
		Ref:      entityrefs.CanonicalRef("kandev", "task", "workspace-1", "task-2"),
		Provider: "kandev", Kind: "task", ID: "task-2",
		Title: "Referenced task", URL: "/t/task-2", Scope: "workspace-1",
	}
	validator := &fakeReferenceSubmissionValidator{result: []v1.EntityReference{reference}}
	queue := messagequeue.NewServiceMemory(logger.Default())
	handlers := NewQueueHandlers(queue, &mockEventBus{}, logger.Default(), nil, allowQueueIdentityAccess{}, nil, validator)
	handlers.Start(context.Background())
	t.Cleanup(handlers.Stop)
	payload := map[string]interface{}{
		"task_id":                "task-1",
		"session_id":             "session-1",
		"session_incarnation_id": "memory:session-1",
		"client_queue_id":        "reference-replay-1",
		"content":                "send once",
		"entity_references":      []v1.EntityReference{reference},
	}

	first, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, first.Type)
	require.Equal(t, 1, validator.calls)

	validator.err = entityrefs.ErrUnauthorizedReference
	second, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, payload))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, second.Type)
	require.Equal(t, 1, validator.calls, "exact replay must not reauthorize mutable references")
	entries, _, err := queue.SnapshotSession(context.Background(), "session-1")
	require.NoError(t, err)
	require.Len(t, entries, 1)
}
