package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLUGINS-MANAGED-COORDINATION-003.3
func TestManagedConversationPauseBlocksQueueDrain(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "managed-task", "managed-session", "step1")

	task, err := repo.GetTask(ctx, "managed-task")
	require.NoError(t, err)
	task.IsEphemeral = true
	task.Metadata = map[string]interface{}{
		models.MetaKeyManagedRetained:             true,
		models.MetaKeyManagedConversationPaused:   true,
		models.MetaKeyManagedConversationDetached: false,
	}
	require.NoError(t, repo.UpdateTask(ctx, task))

	session, err := repo.GetTaskSession(ctx, "managed-session")
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	require.NoError(t, service.messageQueue.SetAutoRun(ctx, session.ID, true))
	_, err = service.messageQueue.QueueMessageWithMetadata(ctx, session.ID, task.ID, "retained human input", "", messagequeue.QueuedByServer, false, nil, map[string]interface{}{
		messagequeue.MetadataManagedInput:   true,
		messagequeue.MetadataManagedInputID: "managed-input-one",
	})
	require.NoError(t, err)

	dispatched, err := service.DrainQueuedMessage(ctx, session.ID)
	require.NoError(t, err)
	require.False(t, dispatched, "paused managed conversation must retain accepted input without starting a turn")
	status := service.messageQueue.GetStatus(ctx, session.ID)
	require.Equal(t, 1, status.Count)
	require.True(t, status.AutoRun)

}
