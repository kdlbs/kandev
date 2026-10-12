package dto

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type recoveryBlockProjectionReader struct {
	generation *models.HarnessSessionGeneration
	blocks     []*models.SessionRecoveryBlock
	listOwner  [3]any
}

func (r *recoveryBlockProjectionReader) GetCurrentHarnessSessionGeneration(
	_ context.Context,
	sessionID, incarnationID string,
) (*models.HarnessSessionGeneration, error) {
	r.listOwner = [3]any{sessionID, incarnationID, int64(0)}
	return r.generation, nil
}

func (r *recoveryBlockProjectionReader) ListOpenSessionRecoveryBlocks(
	_ context.Context,
	sessionID, incarnationID string,
	generation int64,
) ([]*models.SessionRecoveryBlock, error) {
	r.listOwner = [3]any{sessionID, incarnationID, generation}
	return r.blocks, nil
}

func TestEnrichSessionRecoveryBlocksProjectsOnlyOpenCurrentGeneration(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)
	session := &models.TaskSession{ID: "session-projection", QueueIncarnationID: "incarnation-projection"}
	reader := &recoveryBlockProjectionReader{
		generation: &models.HarnessSessionGeneration{SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 7},
		blocks: []*models.SessionRecoveryBlock{
			{ID: "delivery-block", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
				ExpectedGeneration: 7, Reason: "unresolved_durable_work", State: models.RecoveryBlockOpen,
				ConsumerReference: "agent_delivery", DeliveryStreamID: "stream-1", UpdatedAt: now},
			{ID: "resolved-block", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
				ExpectedGeneration: 7, Reason: "old", State: models.RecoveryBlockResolved},
		},
	}
	dto := FromTaskSession(session)
	require.NoError(t, EnrichSessionRecoveryBlocks(ctx, &dto, session, reader))
	require.Equal(t, [3]any{session.ID, session.QueueIncarnationID, int64(7)}, reader.listOwner)
	require.Equal(t, []SessionRecoveryBlockDTO{{
		ID: "delivery-block", IncarnationID: session.QueueIncarnationID, ExpectedGeneration: 7,
		Reason: "unresolved_durable_work", ConsumerReference: "agent_delivery",
		DeliveryStreamID: "stream-1", UpdatedAt: now,
	}}, dto.SessionRecoveryBlocks)
}

func TestEnrichSessionRecoveryBlocksClearsWhenGenerationIsMissing(t *testing.T) {
	reader := &recoveryBlockProjectionReader{blocks: []*models.SessionRecoveryBlock{{ID: "old"}}}
	dto := FromTaskSession(&models.TaskSession{ID: "session-no-generation"})
	require.NoError(t, EnrichSessionRecoveryBlocks(context.Background(), &dto, &models.TaskSession{ID: "session-no-generation"}, reader))
	require.Empty(t, dto.SessionRecoveryBlocks)
	require.Equal(t, [3]any{"session-no-generation", "session-no-generation", int64(0)}, reader.listOwner)
}
