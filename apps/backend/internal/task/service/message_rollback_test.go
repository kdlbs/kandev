package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestRestoreTaskMessageRollback_RejectionReturnsPersistedTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx,
		sessionID,
		models.TaskSessionStateWaitingForInput,
		"",
	))

	before, err := repo.GetTask(ctx, "task-123")
	require.NoError(t, err)
	returned, updated, err := svc.RestoreTaskMessageRollback(
		ctx,
		before.ID,
		sessionID,
		models.TaskSessionStateRunning,
		v1.TaskStateReview,
		"restored-step",
	)
	require.NoError(t, err)
	require.False(t, updated)
	require.Equal(t, before.State, returned.State)
	require.Equal(t, before.WorkflowStepID, returned.WorkflowStepID)
	require.Equal(t, before.UpdatedAt, returned.UpdatedAt)

	persisted, err := repo.GetTask(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, before.State, persisted.State)
	require.Equal(t, before.WorkflowStepID, persisted.WorkflowStepID)
	require.Empty(t, eventBus.GetPublishedEvents())
}

// TestRestoreTaskMessageRollback_AttributesCausalSession proves the fix for
// Review round 3's must-fix #2: the MCP message-dispatch rollback path
// (handlers.go's handleMessageTask -> dispatchTaskMessage ->
// RestoreTaskMessageRollback) has a genuinely causal sender session
// (req.SenderSessionID) known several call-frames up, but
// RestoreTaskMessageRollback unconditionally hardcoded actor_kind=system
// with no ID, discarding it. RestoreTaskMessageRollback must now prefer an
// attribution already preset on ctx — the way the sqlite repository's
// hardcodedTriggerAttribution already does for genesis/detach — over its
// own ActorSystem default.
func TestRestoreTaskMessageRollback_AttributesCausalSession(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx,
		sessionID,
		models.TaskSessionStateRunning,
		"",
	))
	// session_id has a foreign key to task_sessions, so the causal sender
	// session must be a real row, not a bare string literal.
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sender-sess-1", TaskID: "task-123", State: models.TaskSessionStateRunning,
	}))

	senderCtx := steptelemetry.WithAttribution(ctx, steptelemetry.Attribution{
		ActorKind: steptelemetry.ActorAgent,
		ActorID:   "sender-sess-1",
		SessionID: "sender-sess-1",
	})
	_, updated, err := svc.RestoreTaskMessageRollback(
		senderCtx,
		"task-123",
		sessionID,
		models.TaskSessionStateRunning,
		v1.TaskStateReview,
		"restored-step",
	)
	require.NoError(t, err)
	require.True(t, updated)

	trigger, actorKind, _, gotSessionID := lastLedgerAttribution(t, repo, "task-123")
	require.Equal(t, string(steptelemetry.TriggerUnarchiveRestore), trigger)
	require.Equal(t, string(steptelemetry.ActorAgent), actorKind, "the causal sender session must be attributed, not defaulted to system")
	require.NotNil(t, gotSessionID)
	require.Equal(t, "sender-sess-1", *gotSessionID)
}

// TestRestoreTaskMessageRollback_NoPresetFallsBackToSystem covers the no-
// preset case: a bare ctx (no sender session attribution set by a caller)
// still resolves to the existing ActorSystem default, never a guessed
// identity.
func TestRestoreTaskMessageRollback_NoPresetFallsBackToSystem(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	require.NoError(t, repo.UpdateTaskSessionState(
		ctx,
		sessionID,
		models.TaskSessionStateRunning,
		"",
	))

	_, updated, err := svc.RestoreTaskMessageRollback(
		ctx,
		"task-123",
		sessionID,
		models.TaskSessionStateRunning,
		v1.TaskStateReview,
		"restored-step",
	)
	require.NoError(t, err)
	require.True(t, updated)

	trigger, actorKind, _, gotSessionID := lastLedgerAttribution(t, repo, "task-123")
	require.Equal(t, string(steptelemetry.TriggerUnarchiveRestore), trigger)
	require.Equal(t, string(steptelemetry.ActorSystem), actorKind)
	require.Nil(t, gotSessionID)
}
