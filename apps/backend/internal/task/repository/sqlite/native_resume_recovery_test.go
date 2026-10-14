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

func TestNativeResumeCommitsRestoredRecoveryMetadata(t *testing.T) {
	testNativeResumeRestoredMetadata(t, newRepoForSessionTests(t))
}

func TestNativeResumeCommitsRestoredRecoveryMetadataPostgres(t *testing.T) {
	database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	testNativeResumeRestoredMetadata(t, repo)
}

// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4
func testNativeResumeRestoredMetadata(t *testing.T, repo *Repository) {
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, "native-resume", models.TaskSessionStateWaitingForInput)
	changed, err := repo.ResolveSessionRecoveryBlock(ctx, fixture.block.ID, "resume", time.Now().UTC())
	require.NoError(t, err)
	require.True(t, changed)
	session, err := repo.GetTaskSession(ctx, fixture.session.ID)
	require.NoError(t, err)
	recovery, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.True(t, exists)
	require.Equal(t, models.AgentDeliveryRecoveryRestored, recovery.Phase)
	require.Equal(t, fixture.recovery.Revision+1, recovery.Revision)
	require.Equal(t, fixture.recovery.AgentExecutionID, recovery.AgentExecutionID)
	require.Equal(t, fixture.recovery.SubmissionID, recovery.SubmissionID)
	_, errorRemains := session.Metadata[models.SessionMetaKeyLastAgentError]
	require.False(t, errorRemains)
	submission, err := repo.GetAgentDeliverySubmission(ctx, recovery.SubmissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, submission.State)
	verified, err := repo.HasResolvedNativeResumeRecovery(ctx, recovery)
	require.NoError(t, err)
	require.True(t, verified)
	changed, err = repo.ResolveSessionRecoveryBlock(ctx, fixture.block.ID, "resume", time.Now().UTC())
	require.NoError(t, err)
	require.False(t, changed)
	session, err = repo.GetTaskSession(ctx, fixture.session.ID)
	require.NoError(t, err)
	repeated, _ := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.Equal(t, recovery.Revision, repeated.Revision, "a retry must not manufacture a new recovery revision")
}

func TestNativeResumeResolutionRollsBackMetadata(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, "resume-rollback", models.TaskSessionStateWaitingForInput)
	_, err := repo.DB().ExecContext(ctx, `CREATE TRIGGER reject_resume_resolution BEFORE UPDATE ON session_recovery_blocks
		WHEN NEW.state = 'resolved' BEGIN SELECT RAISE(ABORT, 'resolution refused'); END`)
	require.NoError(t, err)
	changed, err := repo.ResolveSessionRecoveryBlock(ctx, fixture.block.ID, "resume", time.Now().UTC())
	require.Error(t, err)
	require.False(t, changed)
	session, err := repo.GetTaskSession(ctx, fixture.session.ID)
	require.NoError(t, err)
	recovery, _ := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.Equal(t, fixture.recovery.Revision, recovery.Revision)
	require.Equal(t, models.AgentDeliveryRecoveryUncertain, recovery.Phase)
	block, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, block.State)
}

func TestNativeResumePreservesNewerRecoveryAndErrors(t *testing.T) {
	for _, condition := range []string{"newer recovery", "unrelated error"} {
		t.Run(condition, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			fixture := newSilentRestoreFixture(t, repo, "resume-newer", models.TaskSessionStateWaitingForInput)
			session, err := repo.GetTaskSession(ctx, fixture.session.ID)
			require.NoError(t, err)
			if condition == "newer recovery" {
				recovery, _ := models.LoadAgentDeliveryRecovery(session.Metadata)
				recovery.SubmissionID = "new-submission"
				session.Metadata[models.SessionMetaKeyAgentDeliveryRecovery] = recovery
			} else {
				session.Metadata[models.SessionMetaKeyLastAgentError] = map[string]any{"code": "AGENT_FAILED", "details": "independent", "agent_execution_id": "new-execution"}
			}
			raw, err := json.Marshal(session.Metadata)
			require.NoError(t, err)
			_, err = repo.DB().ExecContext(ctx, `UPDATE task_sessions SET metadata = ? WHERE id = ?`, string(raw), session.ID)
			require.NoError(t, err)
			changed, err := repo.ResolveSessionRecoveryBlock(ctx, fixture.block.ID, "resume", time.Now().UTC())
			require.NoError(t, err)
			require.Equal(t, condition == "unrelated error", changed)
			current, err := repo.GetTaskSession(ctx, session.ID)
			require.NoError(t, err)
			if condition == "newer recovery" {
				recovery, _ := models.LoadAgentDeliveryRecovery(current.Metadata)
				require.Equal(t, "new-submission", recovery.SubmissionID)
				require.Equal(t, models.AgentDeliveryRecoveryUncertain, recovery.Phase)
			} else {
				require.Equal(t, session.Metadata[models.SessionMetaKeyLastAgentError], current.Metadata[models.SessionMetaKeyLastAgentError])
			}
		})
	}
}

func TestDeliveryRecoveryPartialNoticePreservesStreamBinding(t *testing.T) {
	repo := newRepoForSessionTests(t)
	fixture := newSilentRestoreFixture(t, repo, "partial", models.TaskSessionStateRunning)
	ctx := context.Background()
	block, err := repo.GetOpenSessionRecoveryBlock(ctx, fixture.session.ID, fixture.session.QueueIncarnationID, 1)
	require.NoError(t, err)
	stream := block.DeliveryStreamID
	require.NotEmpty(t, stream)
	block.DeliveryStreamID = ""
	block.UpdatedAt = time.Now().UTC()
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, block))
	stored, err := repo.GetOpenSessionRecoveryBlock(ctx, fixture.session.ID, fixture.session.QueueIncarnationID, 1)
	require.NoError(t, err)
	require.Equal(t, stream, stored.DeliveryStreamID)
}

func TestNativeResumeMissingDeliveryMetadataKeepsBlockOpen(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, "resume-absent", models.TaskSessionStateWaitingForInput)
	_, err := repo.DB().ExecContext(ctx, `UPDATE task_sessions SET metadata = '{}' WHERE id = ?`, fixture.session.ID)
	require.NoError(t, err)
	changed, err := repo.ResolveSessionRecoveryBlock(ctx, fixture.block.ID, "resume", time.Now().UTC())
	require.NoError(t, err)
	require.False(t, changed)
	block, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, block.State)
}

func TestNativeResumeLegacyRecoveryRequiresMatchingNativeEvidence(t *testing.T) {
	testNativeResumeLegacyEvidence(t, newRepoForSessionTests(t))
}

func TestNativeResumeLegacyRecoveryRequiresMatchingNativeEvidencePostgres(t *testing.T) {
	database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	testNativeResumeLegacyEvidence(t, repo)
}

func testNativeResumeLegacyEvidence(t *testing.T, repo *Repository) {
	ctx := context.Background()
	for _, condition := range []string{"matching", "wrong native", "newer generation", "wrong submission", "missing stream"} {
		t.Run(condition, func(t *testing.T) {
			fixture := newSilentRestoreFixture(t, repo, "legacy-"+condition, models.TaskSessionStateWaitingForInput)
			nativeID := fixture.generation.NativeSessionID
			if condition == "wrong native" {
				nativeID = "unrelated-native"
			}
			metadata := map[string]any{"acp": map[string]any{"session_id": nativeID}}
			raw, err := json.Marshal(metadata)
			require.NoError(t, err)
			_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE task_sessions SET metadata = ? WHERE id = ?`), string(raw), fixture.session.ID)
			require.NoError(t, err)
			if condition == "newer generation" {
				generation := fixture.generation
				generation.Generation++
				require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &generation))
			}
			if condition == "wrong submission" {
				_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_submissions SET harness_generation = 2 WHERE id = ?`), fixture.recovery.SubmissionID)
				require.NoError(t, err)
			}
			if condition == "missing stream" {
				_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET delivery_stream_id = '' WHERE id = ?`), fixture.block.ID)
				require.NoError(t, err)
			}
			changed, err := repo.ResolveSessionRecoveryBlock(ctx, fixture.block.ID, "resume", time.Now().UTC())
			require.NoError(t, err)
			require.Equal(t, condition == "matching", changed)
			block, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
			require.NoError(t, err)
			if condition == "matching" {
				require.Equal(t, models.RecoveryBlockResolved, block.State)
				require.Equal(t, "resume", block.AuthorizedAction)
			} else {
				require.Equal(t, models.RecoveryBlockOpen, block.State)
			}
			session, err := repo.GetTaskSession(ctx, fixture.session.ID)
			require.NoError(t, err)
			require.Equal(t, metadata, session.Metadata)
			submission, err := repo.GetAgentDeliverySubmission(ctx, fixture.recovery.SubmissionID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, submission.State)
		})
	}
}
