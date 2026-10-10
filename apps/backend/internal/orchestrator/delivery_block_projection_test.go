package orchestrator

import (
	"context"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDeliveryBlockPublicationWithoutPriorError(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	session, err := repo.GetTaskSession(ctx, "s1")
	require.NoError(t, err)
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 1,
		NativeSessionID: "native", CreationReason: "initial",
	}))
	eb := &recordingEventBus{}
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eb
	require.ErrorIs(t, svc.deliveryRecoveryError(ctx, session.ID, "unresolved_durable_work"), ErrSessionRecoveryRequired)
	require.NotEmpty(t, eb.events, "a saved delivery block must reach the open chat immediately")
	data := eb.events[len(eb.events)-1].event.Data.(map[string]interface{})
	blocks, ok := data["session_recovery_blocks"].([]dto.SessionRecoveryBlockDTO)
	require.True(t, ok)
	require.Len(t, blocks, 1)
	blockedSession, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.True(t, blockedSession.UpdatedAt.After(session.UpdatedAt), "block creation must advance the websocket snapshot fence")
	require.Equal(t, "unresolved_durable_work", blocks[0].Reason)
	_, err = repo.ResolveSessionRecoveryBlock(ctx, blocks[0].ID, "test", time.Now().UTC())
	require.NoError(t, err)
	svc.publishAgentDeliveryRecoveryState(ctx, session.ID)
	data = eb.events[len(eb.events)-1].event.Data.(map[string]interface{})
	require.Empty(t, data["session_recovery_blocks"])
	resolvedSession, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.True(t, resolvedSession.UpdatedAt.After(blockedSession.UpdatedAt), "resolution must fence older block snapshots")
}
