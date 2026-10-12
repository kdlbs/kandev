package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestRetryDeliveryRecognizesCommittedNativeResume(t *testing.T) {
	ctx := context.Background()
	service, repo, _, session, _ := newSilentRestoreEligibilityFixture(t, "native", "native-retry")
	block, err := repo.GetOpenSessionRecoveryBlock(ctx, session.ID, session.QueueIncarnationID, 1)
	require.NoError(t, err)
	changed, err := repo.ResolveSessionRecoveryBlock(ctx, block.ID, "resume", time.Now().UTC())
	require.NoError(t, err)
	require.True(t, changed)
	result, err := service.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryAttached, result.Outcome, result.Reason)
	current, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	recovery, _ := models.LoadAgentDeliveryRecovery(current.Metadata)
	recovery.UpdatedAt = recovery.UpdatedAt.Add(time.Second)
	current.Metadata[models.SessionMetaKeyAgentDeliveryRecovery] = recovery
	raw, err := json.Marshal(current.Metadata)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(ctx, `UPDATE task_sessions SET metadata = ? WHERE id = ?`, string(raw), current.ID)
	require.NoError(t, err)
	result, err = service.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryBlocked, result.Outcome, "an unrelated snapshot must not reuse an old resume resolution")
}
