package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
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
	beforeResolution := len(eb.events)
	require.NoError(t, svc.resolveSessionRecoveryBlock(ctx, session.ID, "test"))
	require.Greater(t, len(eb.events), beforeResolution)
	require.Equal(t, events.TaskSessionStateChanged, eb.events[beforeResolution].subject)
	data = eb.events[beforeResolution].event.Data.(map[string]interface{})
	blocks, ok = data["session_recovery_blocks"].([]dto.SessionRecoveryBlockDTO)
	require.True(t, ok, "resolution must publish the block projection")
	require.NotNil(t, blocks)
	require.Empty(t, blocks)
	resolvedSession, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.True(t, resolvedSession.UpdatedAt.After(blockedSession.UpdatedAt), "resolution must fence older block snapshots")
}

func TestLifecycleLaunchGuardPublishesDeliveryRecoveryOwner(t *testing.T) {
	for _, reason := range []string{"unresolved_durable_work", "native_state_missing"} {
		t.Run(reason, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")
			session, err := repo.GetTaskSession(ctx, "s1")
			require.NoError(t, err)
			require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
				SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 1,
				NativeSessionID: "native", CreationReason: "initial"}))
			eb := &recordingEventBus{}
			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
			svc.eventBus = eb
			guard := &lifecycle.RestoreRequiredError{Decision: lifecycle.RestoreDecision{Reason: lifecycle.RestoreReason(reason)}}
			require.NoError(t, svc.recordSessionRecoveryBlock(ctx, session.ID, "interactive", guard))
			require.NotEmpty(t, eb.events)
			data := eb.events[len(eb.events)-1].event.Data.(map[string]interface{})
			blocks := data["session_recovery_blocks"].([]dto.SessionRecoveryBlockDTO)
			require.Len(t, blocks, 1)
			wantOwner := "interactive"
			if reason == "unresolved_durable_work" {
				wantOwner = "agent_delivery"
			}
			require.Equal(t, wantOwner, blocks[0].ConsumerReference, "the open chat chooses delivery recovery controls from their durable owner")
		})
	}
}
