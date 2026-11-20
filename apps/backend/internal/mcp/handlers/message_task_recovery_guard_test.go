package handlers

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A peer message to a session the recovery guard refuses to start stays queued
// on that session instead of being rolled back.
func TestPeerMessage_RecoveryGuardRefusalKeepsMessageQueued(t *testing.T) {
	ctx := context.Background()
	svc, repo := newTestTaskService(t)
	sender, target, session := seedTaskWithSession(t, svc, repo, models.TaskSessionStateCreated)
	h, orch := newMessageTaskHandler(t, svc, repo)
	orch.startCreatedErr = fmt.Errorf("resume: %w", &orchestrator.SessionRecoveryGuardError{
		SessionID: session.ID, Retryable: true,
	})
	require.NoError(t, orch.queue.SetAutoRun(ctx, session.ID, false))

	result, err := h.dispatchPreparedTaskMessage(
		ctx, target.ID, session, "peer follow-up", map[string]interface{}{"sender_task_id": sender.ID}, false,
	)

	require.NoError(t, err)
	assert.Equal(t, taskMessageStatusQueued, result.status)
	status := orch.queue.GetStatus(ctx, session.ID)
	require.Equal(t, 1, status.Count)
	assert.Equal(t, "peer follow-up", status.Entries[0].Content)
}
