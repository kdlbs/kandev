package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// QAR-001: the final wrapper is part of the response bound.
func TestChildQueueRecoveryFinalResponseBound(t *testing.T) {
	h, repo, parent, child, session, ctx := queueRecoveryFixture(t)
	session, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	identity := messagequeue.QueueSessionIdentity{TaskID: child.ID, SessionID: session.ID, SessionIncarnationID: session.QueueIncarnationID}
	queueRepo := messagequeue.NewMemoryRepositoryWithAuthority(func(context.Context, string, string) (messagequeue.QueueSessionIdentity, error) { return identity, nil })
	_, err = queueRepo.ResolveSessionIdentity(ctx, child.ID, session.ID)
	require.NoError(t, err)
	queue := messagequeue.NewService(queueRepo, 0, testLogger(t))
	entries := make([]messagequeue.QueuedMessage, 1024)
	for i := range entries {
		entries[i] = messagequeue.QueuedMessage{ID: fmt.Sprintf("%04d-", i) + strings.Repeat("x", 1600), TaskID: child.ID, SessionID: session.ID, Position: int64(i)}
		require.NoError(t, queueRepo.RestoreForSession(ctx, identity, &entries[i], 0))
	}
	manifests, err := queue.RecoveryInventory(ctx, []messagequeue.QueueSessionIdentity{identity}, parent.WorkspaceID)
	require.NoError(t, err)
	encoded, err := json.Marshal(manifests)
	require.NoError(t, err)
	// Replacement preserves every row's order; only one immutable source ID grows.
	padding := messagequeue.MaxRecoveryResponseBytes - len(encoded) - 1
	require.Positive(t, padding)
	entries[1023].ID += strings.Repeat("z", padding)
	require.NoError(t, queueRepo.ReplaceSession(ctx, session.ID, entries, nil))
	manifests, err = queue.RecoveryInventory(ctx, []messagequeue.QueueSessionIdentity{identity}, parent.WorkspaceID)
	require.NoError(t, err)
	encoded, err = json.Marshal(manifests)
	require.NoError(t, err)
	require.Equal(t, messagequeue.MaxRecoveryResponseBytes-1, len(encoded))
	h.sessionLauncher = &fakeOrchestrator{queue: queue}
	response, err := h.handleChildQueueInventory(ctx, makeWSMessage(t, ws.ActionMCPGetChildQueueInventory, map[string]interface{}{"task_id": child.ID}))
	require.NoError(t, err)
	assertWSError(t, response, ws.ErrorCodeConflict)
	require.LessOrEqual(t, len(response.Payload), messagequeue.MaxRecoveryResponseBytes)
}
