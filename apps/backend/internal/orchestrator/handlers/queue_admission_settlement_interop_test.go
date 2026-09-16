package handlers

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// recordingQueuedWorkAdmitter records settlements routed through the
// completion-settlement guard and forwards the admission itself.
type recordingQueuedWorkAdmitter struct {
	mu              sync.Mutex
	settledTaskIDs  []string
	settledSessions []string
	forwardErr      error
}

func (a *recordingQueuedWorkAdmitter) AdmitQueuedUserWork(
	ctx context.Context,
	taskID, sessionID string,
	admit func(context.Context) (*messagequeue.QueuedMessage, error),
) (*messagequeue.QueuedMessage, error) {
	a.mu.Lock()
	a.settledTaskIDs = append(a.settledTaskIDs, taskID)
	a.settledSessions = append(a.settledSessions, sessionID)
	a.mu.Unlock()
	queued, err := admit(ctx)
	if err != nil {
		return nil, err
	}
	if a.forwardErr != nil {
		return queued, a.forwardErr
	}
	return queued, nil
}

// TestWsQueueMessageRoutesAnonymousWorkThroughSettlementAdmitter proves the
// merged admission contract: an anonymous-identity ordinary message routes
// through the completion-settlement admitter (the exact-turn reopen guard),
// while an identified admission (client_queue_id) uses the idempotent
// identified path without invoking the settlement admitter.
func TestWsQueueMessageRoutesAnonymousWorkThroughSettlementAdmitter(t *testing.T) {
	log := logger.Default()
	queue := messagequeue.NewServiceMemory(log)
	admitter := &recordingQueuedWorkAdmitter{}
	handlers := NewQueueHandlers(queue, &mockEventBus{}, log, nil, allowQueueIdentityAccess{}, nil)
	handlers.queuedWorkAdmitter = admitter

	anonymous, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"task_id":                "task-1",
		"session_id":             "session-1",
		"session_incarnation_id": "memory:session-1",
		"content":                "anonymous ordinary work",
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, anonymous.Type)

	admitter.mu.Lock()
	taskIDs := append([]string(nil), admitter.settledTaskIDs...)
	admitter.mu.Unlock()
	require.Equal(t, []string{"task-1"}, taskIDs, "anonymous ordinary work must be guarded by the settlement admitter")

	identified, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"task_id":         "task-1",
		"session_id":      "session-1",
		"client_queue_id": "identified-1",
		"content":         "identified work",
	}))
	require.NoError(t, err, "identified admission must succeed when the identity gate is unavailable but access allows it")
	require.NotEqual(t, "", identified.Type)

	admitter.mu.Lock()
	taskIDsAfter := append([]string(nil), admitter.settledTaskIDs...)
	admitter.mu.Unlock()
	require.Len(t, taskIDsAfter, 1, "identified admissions must bypass the settlement admitter and keep their idempotent path")
}

// TestWsQueueMessageSettlementAdmitterFailureSurfacesError pins the
// interoperability rule that a settlement reopen failure after an otherwise
// successful admission is returned to the caller rather than being swallowed
// by the switch ordering added with upstream queue-admission reliability.
func TestWsQueueMessageSettlementAdmitterFailureSurfacesError(t *testing.T) {
	log := logger.Default()
	queue := messagequeue.NewServiceMemory(log)
	admitter := &recordingQueuedWorkAdmitter{forwardErr: context.Canceled}
	handlers := NewQueueHandlers(queue, &mockEventBus{}, log, nil, allowQueueIdentityAccess{}, nil)
	handlers.queuedWorkAdmitter = admitter

	response, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"task_id":                "task-1",
		"session_id":             "session-1",
		"session_incarnation_id": "memory:session-1",
		"content":                "work guarded by settlement",
	}))
	require.NoError(t, err)
	requireNotSuccessPayload(t, response)
}

func requireNotSuccessPayload(t *testing.T, response *ws.Message) {
	t.Helper()
	require.Equal(t, ws.MessageTypeError, response.Type, "settlement reopen failure must surface as an error response, got: %s", string(response.Payload))
}
