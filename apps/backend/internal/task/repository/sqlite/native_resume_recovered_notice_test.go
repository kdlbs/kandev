package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeRestoresRecoveredNoticeAfterExecutionLoss(t *testing.T) {
	testNativeResumeRestoresRecoveredNoticeAfterExecutionLoss(t, newRepoForSessionTests)
}

func TestPostgresNativeResumeRestoresRecoveredNoticeAfterExecutionLoss(t *testing.T) {
	testNativeResumeRestoresRecoveredNoticeAfterExecutionLoss(t, func(t *testing.T) *Repository {
		database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
		repo, err := NewWithDB(database, database, nil)
		require.NoError(t, err)
		return repo
	})
}

// A notice already marked recovered can outlive its execution when a later
// readiness failure reopens the session block. Explicit Resume on the current
// ready native execution must restore that notice instead of staying blocked.
func testNativeResumeRestoresRecoveredNoticeAfterExecutionLoss(t *testing.T, open func(*testing.T) *Repository) {
	for _, phase := range []string{models.AgentDeliveryRecoveryRecovered, models.AgentDeliveryRecoverySettled} {
		t.Run(phase, func(t *testing.T) {
			repo := open(t)
			ctx := context.Background()
			fixture := newSilentRestoreFixture(t, repo, "recovered-notice", models.TaskSessionStateWaitingForInput)
			submission, err := repo.GetAgentDeliverySubmission(ctx, fixture.recovery.SubmissionID)
			require.NoError(t, err)
			notice := fixture.recovery
			notice.Phase = phase
			raw, err := json.Marshal(map[string]any{
				"acp": map[string]any{"session_id": fixture.generation.NativeSessionID},
				models.SessionMetaKeyAgentDeliveryRecovery: notice,
			})
			require.NoError(t, err)
			_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE task_sessions SET metadata = ? WHERE id = ?`), string(raw), fixture.session.ID)
			require.NoError(t, err)
			_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE executors_running SET agent_execution_id = ? WHERE session_id = ?`), "current-execution", fixture.session.ID)
			require.NoError(t, err)
			proof := &models.NativeResumeDeliveryEvidence{SessionID: fixture.session.ID, IncarnationID: notice.IncarnationID,
				HarnessGeneration: 1, NativeSessionID: fixture.generation.NativeSessionID, StreamID: notice.StreamID,
				ExecutionID: "current-execution",
				Submissions: []models.NativeResumeSubmissionEvidence{{ID: submission.ID, HarnessGeneration: 1, PayloadHash: submission.PayloadHash, StreamID: notice.StreamID}}}

			changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, fixture.block.ID, proof, time.Now().UTC())
			require.NoError(t, err)
			restored := phase == models.AgentDeliveryRecoveryRecovered
			require.Equal(t, restored, changed, "explicit Resume must restore a recovered notice on the current native execution")
			block, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
			require.NoError(t, err)
			session, err := repo.GetTaskSession(ctx, fixture.session.ID)
			require.NoError(t, err)
			stored, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
			require.True(t, exists)
			if !restored {
				require.Equal(t, models.RecoveryBlockOpen, block.State, "a settled notice is not authenticated retirement evidence")
				require.Equal(t, phase, stored.Phase)
				return
			}
			require.Equal(t, models.RecoveryBlockResolved, block.State)
			require.Equal(t, models.AgentDeliveryRecoveryRestored, stored.Phase)
			require.Equal(t, notice.AgentExecutionID, stored.AgentExecutionID, "the notice keeps its historical owner")
			require.Equal(t, notice.SubmissionID, stored.SubmissionID)
			require.Greater(t, stored.Revision, notice.Revision)
			unchanged, err := repo.GetAgentDeliverySubmission(ctx, submission.ID)
			require.NoError(t, err)
			require.Equal(t, submission.State, unchanged.State)
			require.Equal(t, submission.PayloadHash, unchanged.PayloadHash)
		})
	}
}
