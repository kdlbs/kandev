package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSessionRecoveryBlockIncidentUniquenessMigration(t *testing.T) {
	testSessionRecoveryBlockIncidentUniquenessMigration(t, "SQLite")
}

func TestPostgresSessionRecoveryBlockIncidentUniquenessMigration(t *testing.T) {
	testSessionRecoveryBlockIncidentUniquenessMigration(t, "Postgres")
}

func testSessionRecoveryBlockIncidentUniquenessMigration(t *testing.T, engine string) {
	t.Helper()
	repo := newRetentionTestRepo(t, engine)
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, "recovery-block-constraint-migration", models.TaskSessionStateWaitingForInput)

	_, err := repo.ResolveSessionRecoveryBlock(ctx, fixture.block.ID, "resume", time.Now().UTC())
	require.NoError(t, err)
	historical := models.SessionRecoveryBlock{
		ID: "legacy-resolved-generic-fence", SessionID: fixture.session.ID,
		IncarnationID: fixture.session.QueueIncarnationID, ExpectedGeneration: 1,
		Reason: deliveryUnresolvedWorkReason, State: models.RecoveryBlockOpen,
		ConsumerReference: reconstructionDeliveryConsumer,
	}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &historical))
	_, err = repo.ResolveSessionRecoveryBlock(ctx, historical.ID, "resume", time.Now().UTC())
	require.NoError(t, err)
	successor := models.AgentDeliverySubmission{
		ID: "migration-successor-prompt", SessionID: fixture.session.ID,
		IncarnationID: fixture.session.QueueIncarnationID, HarnessGeneration: 1,
		OwnerGeneration: 1, PayloadHash: "migration-successor-hash",
		Payload: []byte("successor prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	}
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &successor)
	require.NoError(t, err)

	installLegacyRecoveryBlockConstraint(t, repo)
	require.NoError(t, repo.runMigrations(ctx))
	require.NoError(t, repo.runMigrations(ctx))

	candidate := restartRecoveryBlockCandidate(fixture.session.ID, fixture.session.QueueIncarnationID, 1)
	candidate.ID = "migrated-successor-generic-fence"
	store, ok := any(repo).(restartRecoveryBlockEnsurer)
	require.True(t, ok)
	block, created, err := store.EnsureRestartRecoveryBlock(ctx, &candidate)
	require.NoError(t, err)
	require.True(t, created)
	require.NotNil(t, block)
	require.Equal(t, candidate.ID, block.ID)

	retained, err := repo.GetSessionRecoveryBlock(ctx, historical.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, retained.State)
	require.Equal(t, "resume", retained.AuthorizedAction)
	require.NotNil(t, retained.ResolvedAt)
	open, err := repo.ListOpenSessionRecoveryBlocks(ctx, fixture.session.ID, fixture.session.QueueIncarnationID, 1)
	require.NoError(t, err)
	require.Len(t, open, 1)
	require.Equal(t, candidate.ID, open[0].ID)
}

func installLegacyRecoveryBlockConstraint(t *testing.T, repo *Repository) {
	t.Helper()
	if repo.db.DriverName() != "sqlite3" {
		_, err := repo.db.Exec(`ALTER TABLE session_recovery_blocks ADD CONSTRAINT legacy_session_recovery_blocks_reason_key
			UNIQUE (session_id, incarnation_id, expected_generation, reason)`)
		require.NoError(t, err)
		return
	}

	_, err := repo.db.Exec(`ALTER TABLE session_recovery_blocks RENAME TO session_recovery_blocks_migration_source`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`CREATE TABLE session_recovery_blocks (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		incarnation_id TEXT NOT NULL,
		expected_generation BIGINT NOT NULL,
		reason TEXT NOT NULL,
		state TEXT NOT NULL,
		consumer_reference TEXT NOT NULL DEFAULT '',
		delivery_submission_id TEXT NOT NULL DEFAULT '',
		delivery_stream_id TEXT NOT NULL DEFAULT '',
		delivery_sequence BIGINT NOT NULL DEFAULT 0,
		delivery_turn_id TEXT NOT NULL DEFAULT '',
		delivery_outcome TEXT NOT NULL DEFAULT '',
		authorized_action TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		resolved_at TIMESTAMP,
		UNIQUE (session_id, incarnation_id, expected_generation, reason),
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	)`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`INSERT INTO session_recovery_blocks
		(id, session_id, incarnation_id, expected_generation, reason, state, consumer_reference,
		delivery_submission_id, delivery_stream_id, delivery_sequence, delivery_turn_id, delivery_outcome,
		authorized_action, created_at, updated_at, resolved_at)
		SELECT id, session_id, incarnation_id, expected_generation, reason, state, consumer_reference,
		delivery_submission_id, delivery_stream_id, delivery_sequence, delivery_turn_id, delivery_outcome,
		authorized_action, created_at, updated_at, resolved_at
		FROM session_recovery_blocks_migration_source`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`DROP TABLE session_recovery_blocks_migration_source`)
	require.NoError(t, err)
}
