package dto

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type recoveryBlockProjectionReader struct {
	generation    *models.HarnessSessionGeneration
	blocks        []*models.SessionRecoveryBlock
	listOwner     [3]any
	generationErr error
	blocksErr     error
}

func (r *recoveryBlockProjectionReader) GetCurrentHarnessSessionGeneration(
	_ context.Context,
	sessionID, incarnationID string,
) (*models.HarnessSessionGeneration, error) {
	r.listOwner = [3]any{sessionID, incarnationID, int64(0)}
	return r.generation, r.generationErr
}

func (r *recoveryBlockProjectionReader) ListOpenSessionRecoveryBlocks(
	_ context.Context,
	sessionID, incarnationID string,
	generation int64,
) ([]*models.SessionRecoveryBlock, error) {
	r.listOwner = [3]any{sessionID, incarnationID, generation}
	return r.blocks, r.blocksErr
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

// @covers AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.22
func TestEnrichSessionRecoveryBlocksTypedMissingGeneration(t *testing.T) {
	for _, err := range []error{models.ErrTaskSessionNotFound, fmt.Errorf("generation: %w", models.ErrTaskSessionNotFound)} {
		t.Run(err.Error(), func(t *testing.T) {
			reader := &recoveryBlockProjectionReader{generationErr: err, blocksErr: errors.New("must not list blocks")}
			projection := TaskSessionDTO{SessionRecoveryBlocks: []SessionRecoveryBlockDTO{{ID: "stale"}}}
			require.NoError(t, EnrichSessionRecoveryBlocks(context.Background(), &projection, &models.TaskSession{ID: "fresh"}, reader))
			require.NotNil(t, projection.SessionRecoveryBlocks)
			require.Empty(t, projection.SessionRecoveryBlocks)
		})
	}
}

// @covers AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.23
func TestEnrichSessionRecoveryBlocksPreservesReadFailures(t *testing.T) {
	readErr := errors.New("database unavailable")
	for _, tc := range []struct {
		name          string
		generationErr error
		blocksErr     error
		want          error
	}{
		{name: "generation failure", generationErr: readErr, want: readErr},
		{name: "block failure", blocksErr: readErr, want: readErr},
		{name: "block not found", blocksErr: models.ErrTaskSessionNotFound, want: models.ErrTaskSessionNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &recoveryBlockProjectionReader{
				generation:    &models.HarnessSessionGeneration{Generation: 1},
				generationErr: tc.generationErr, blocksErr: tc.blocksErr,
			}
			projection := TaskSessionDTO{}
			require.ErrorIs(t, EnrichSessionRecoveryBlocks(context.Background(), &projection, &models.TaskSession{ID: "session"}, reader), tc.want)
		})
	}
}
