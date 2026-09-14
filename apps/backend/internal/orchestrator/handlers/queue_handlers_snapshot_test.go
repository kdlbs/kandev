package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.5
// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.15
func TestQueueStatusRequestReturnsIdentityBoundPolicySnapshot(t *testing.T) {
	handlers, service := setupQueueHandlers(t)
	identity := messagequeue.QueueSessionIdentity{
		TaskID: "task", SessionID: "session", SessionIncarnationID: "memory:session",
	}
	_, err := service.SetSessionAutoMerge(context.Background(), identity, false)
	require.NoError(t, err)

	response, err := handlers.wsGetQueueStatus(context.Background(), createTestMessage(t, ws.ActionMessageQueueGet, map[string]interface{}{
		"task_id":                identity.TaskID,
		"session_id":             identity.SessionID,
		"session_incarnation_id": identity.SessionIncarnationID,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	var status messagequeue.QueueStatus
	require.NoError(t, json.Unmarshal(response.Payload, &status))
	require.Equal(t, identity.TaskID, status.TaskID)
	require.NotEmpty(t, status.StatusEpoch)
	require.NotEqual(t, identity.SessionIncarnationID, status.StatusEpoch)
	require.True(t, status.AutoMergeAvailable)
	require.NotNil(t, status.AutoMergeEnabled)
	require.False(t, *status.AutoMergeEnabled)
	require.Equal(t, messagequeue.AutoMergeSourceSession, status.AutoMergeSource)
}

type allowQueueIdentityAccess struct {
	allowQueueAccess
}

func (allowQueueIdentityAccess) AuthorizeTaskSessionIncarnationAccess(
	context.Context,
	string,
	string,
	string,
) error {
	return nil
}

type queueFullIdentityService struct {
	*messagequeue.Service
	snapshot         *messagequeue.QueueStatus
	snapshotErr      error
	legacyStatusRead bool
}

func (s *queueFullIdentityService) QueueMessageWithMetadataForSession(
	context.Context,
	messagequeue.QueueSessionIdentity,
	string,
	string,
	string,
	bool,
	[]messagequeue.MessageAttachment,
	map[string]interface{},
) (*messagequeue.QueuedMessage, error) {
	return nil, messagequeue.ErrQueueFull
}

func (s *queueFullIdentityService) AppendContentForSession(
	context.Context,
	messagequeue.QueueSessionIdentity,
	string,
	string,
	string,
	bool,
	[]messagequeue.MessageAttachment,
) (*messagequeue.QueuedMessage, bool, error) {
	return nil, false, messagequeue.ErrQueueFull
}

func (s *queueFullIdentityService) Snapshot(
	context.Context,
	messagequeue.QueueSessionIdentity,
) (*messagequeue.QueueStatus, error) {
	return s.snapshot, s.snapshotErr
}

func (s *queueFullIdentityService) GetStatus(
	context.Context,
	string,
) *messagequeue.QueueStatus {
	s.legacyStatusRead = true
	return &messagequeue.QueueStatus{Count: 99, Max: 99}
}

func TestQueueFullResponseUsesIdentityBoundSnapshot(t *testing.T) {
	testQueueFullResponses(t, func(t *testing.T, response *ws.Message, service *queueFullIdentityService) {
		require.Equal(t, ws.MessageTypeError, response.Type)
		errPayload := parseError(t, response)
		require.Equal(t, messagequeue.QueueFullErrorCode, errPayload.Code)
		require.EqualValues(t, 7, errPayload.Details[fieldQueueSize])
		require.EqualValues(t, 11, errPayload.Details[fieldMax])
		require.False(t, service.legacyStatusRead)
	}, &messagequeue.QueueStatus{Count: 7, Max: 11}, nil)
}

func TestQueueFullResponseRejectsStaleIdentityWithoutLegacyStatus(t *testing.T) {
	testQueueFullResponses(t, func(t *testing.T, response *ws.Message, service *queueFullIdentityService) {
		require.Equal(t, ws.MessageTypeError, response.Type)
		require.Equal(t, ws.ErrorCodeNotFound, parseError(t, response).Code)
		require.False(t, service.legacyStatusRead)
	}, nil, messagequeue.ErrSessionIdentityMismatch)
}

func testQueueFullResponses(
	t *testing.T,
	assertResponse func(*testing.T, *ws.Message, *queueFullIdentityService),
	snapshot *messagequeue.QueueStatus,
	snapshotErr error,
) {
	t.Helper()
	calls := map[string]func(*QueueHandlers, context.Context, *ws.Message) (*ws.Message, error){
		"queue":  (*QueueHandlers).wsQueueMessage,
		"append": (*QueueHandlers).wsAppendToQueue,
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			handlers, base := setupQueueHandlers(t)
			service := &queueFullIdentityService{
				Service: base, snapshot: snapshot, snapshotErr: snapshotErr,
			}
			handlers.queueService = service
			handlers.accessAuthorizer = allowQueueIdentityAccess{}
			response, err := call(handlers, context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
				fieldTaskID:             "task-1",
				fieldSessionID:          "session-1",
				fieldSessionIncarnation: "incarnation-1",
				"content":               "overflow",
			}))
			require.NoError(t, err)
			assertResponse(t, response, service)
		})
	}
}
