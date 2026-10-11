package mcp

import (
	"context"
	"encoding/json"
	"testing"

	backendmcp "github.com/kandev/kandev/internal/mcp/handlers"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type queueRecoveryIntegrationLauncher struct {
	backendmcp.SessionLauncher
	queue *messagequeue.Service
}

func (l *queueRecoveryIntegrationLauncher) GetMessageQueue() *messagequeue.Service { return l.queue }

func TestQueueRecoveryAuthenticatedTransportPreservesSource(t *testing.T) {
	_, taskSvc, repo, _ := newSelfPlacementJourney(t)
	const taskID, callerID, childID, targetID = "parent-self-placement", "parent-caller", "child-self-placement", "session-self-placement"
	ctx := context.Background()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: callerID, TaskID: taskID, State: models.TaskSessionStateRunning}))
	queueRepo := messagequeue.NewMemoryRepositoryWithAuthority(func(ctx context.Context, taskID, sessionID string) (messagequeue.QueueSessionIdentity, error) {
		session, err := repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			return messagequeue.QueueSessionIdentity{}, err
		}
		return messagequeue.QueueSessionIdentity{TaskID: session.TaskID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID}, nil
	})
	queue := messagequeue.NewService(queueRepo, 0, newTestLogger(t))
	_, err := queue.QueueMessage(ctx, targetID, childID, "retained private body", "", messagequeue.QueuedByAgent, false, nil)
	require.NoError(t, err)
	target, err := repo.GetTaskSession(ctx, targetID)
	require.NoError(t, err)
	identity := messagequeue.QueueSessionIdentity{TaskID: childID, SessionID: targetID, SessionIncarnationID: target.QueueIncarnationID}
	before, err := queue.RecoveryRead(ctx, identity)
	require.NoError(t, err)
	log := newTestLogger(t)
	handlers := backendmcp.NewHandlers(taskSvc, nil, nil, nil, nil, repo, repo, nil, nil, nil, &queueRecoveryIntegrationLauncher{queue: queue}, nil, log)
	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)
	backend := selfPlacementScopedBackend{client: NewDispatcherBackendClient(dispatcher, log), resolver: mcpscope.NewResolver(repo, nil, func() bool { return false }, log), taskID: taskID, sessionID: callerID}
	server := New(backend, callerID, taskID, 10005, log, "", false, ModeTask)
	inventory := callTool(t, server, "get_child_queue_inventory_kandev", map[string]interface{}{"task_id": childID})
	require.False(t, inventory.IsError, firstText(t, inventory))
	require.NotContains(t, firstText(t, inventory), "retained private body")
	var decoded struct {
		Sessions []struct {
			Manifest messagequeue.QueueRecoveryManifest `json:"manifest"`
		} `json:"sessions"`
	}
	require.NoError(t, json.Unmarshal([]byte(firstText(t, inventory)), &decoded))
	require.Len(t, decoded.Sessions, 1)
	result := callTool(t, server, "get_child_session_message_queue_kandev", map[string]interface{}{"task_id": childID, "session_id": targetID, "expected_snapshot_sha256": decoded.Sessions[0].Manifest.SnapshotSHA256})
	require.False(t, result.IsError, firstText(t, result))
	require.Contains(t, firstText(t, result), "retained private body")
	after, err := queue.RecoveryRead(ctx, identity)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
