package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

type restartRecoveryBlockEnsurer interface {
	EnsureRestartRecoveryBlock(context.Context, *models.SessionRecoveryBlock) (*models.SessionRecoveryBlock, bool, error)
}

func TestRestartRecoveryBlockEnsureReusesIncident(t *testing.T) {
	testRestartRecoveryBlockEnsureReusesIncident(t, newRepoForSessionTests)
}

func TestPostgresRestartRecoveryBlockEnsureReusesIncident(t *testing.T) {
	testRestartRecoveryBlockEnsureReusesIncident(t, func(t *testing.T) *Repository {
		database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
		repo, err := NewWithDB(database, database, nil)
		require.NoError(t, err)
		return repo
	})
}

func TestRestartRecoveryBlockEnsureRefusesUnsafeInventory(t *testing.T) {
	testRestartRecoveryBlockEnsureRefusesUnsafeInventory(t, newRepoForSessionTests)
}

func TestPostgresRestartRecoveryBlockEnsureRefusesUnsafeInventory(t *testing.T) {
	testRestartRecoveryBlockEnsureRefusesUnsafeInventory(t, func(t *testing.T) *Repository {
		database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
		repo, err := NewWithDB(database, database, nil)
		require.NoError(t, err)
		return repo
	})
}

func TestPostgresRestartRecoveryBlockEnsureConcurrent(t *testing.T) {
	database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	fixture := newSilentRestoreFixture(t, repo, "ensure-concurrent", models.TaskSessionStateWaitingForInput)
	_, err = repo.DB().ExecContext(t.Context(), repo.db.Rebind(`DELETE FROM session_recovery_blocks WHERE id = ?`), fixture.block.ID)
	require.NoError(t, err)
	store, ok := any(repo).(restartRecoveryBlockEnsurer)
	require.True(t, ok)
	start := make(chan struct{})
	ready := make(chan struct{}, 2)
	results := make(chan restartRecoveryEnsureResult, 2)
	for range 2 {
		go func() {
			candidate := restartRecoveryBlockCandidate(fixture.session.ID, fixture.session.QueueIncarnationID, 1)
			ready <- struct{}{}
			<-start
			block, created, err := store.EnsureRestartRecoveryBlock(t.Context(), &candidate)
			results <- restartRecoveryEnsureResult{block: block, created: created, err: err}
		}()
	}
	<-ready
	<-ready
	close(start)
	first, second := <-results, <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.NotNil(t, first.block)
	require.NotNil(t, second.block)
	require.Equal(t, first.block.ID, second.block.ID)
	require.NotEqual(t, first.created, second.created)
	blocks, err := repo.ListOpenSessionRecoveryBlocks(t.Context(), fixture.session.ID, fixture.session.QueueIncarnationID, 1)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
}

func TestPostgresRestartRecoveryBlockEnsureWaitsForDeliveryFence(t *testing.T) {
	database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	fixture := newSilentRestoreFixture(t, repo, "ensure-delivery-race", models.TaskSessionStateWaitingForInput)
	_, err = repo.DB().ExecContext(t.Context(), repo.db.Rebind(`DELETE FROM session_recovery_blocks WHERE id = ?`), fixture.block.ID)
	require.NoError(t, err)
	writer, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = writer.Rollback() }()
	require.NoError(t, lockSessionTurnWrites(t.Context(), writer, repo.db.DriverName(), fixture.session.ID))
	require.NoError(t, lockAgentDeliveryWrites(t.Context(), writer, repo.db.DriverName(), []string{fixture.session.ID}, []string{fixture.recovery.StreamID}))
	require.NoError(t, repo.advanceRecoverySessionRevisionTx(t.Context(), writer, fixture.session.ID))
	deliveryBlock := fixture.block
	deliveryBlock.ID = "concurrent-delivery-fence"
	require.NoError(t, repo.upsertSessionRecoveryBlockTx(t.Context(), writer, &deliveryBlock))
	store, ok := any(repo).(restartRecoveryBlockEnsurer)
	require.True(t, ok)
	started := make(chan struct{})
	result := make(chan restartRecoveryEnsureResult, 1)
	go func() {
		candidate := restartRecoveryBlockCandidate(fixture.session.ID, fixture.session.QueueIncarnationID, 1)
		close(started)
		block, created, err := store.EnsureRestartRecoveryBlock(t.Context(), &candidate)
		result <- restartRecoveryEnsureResult{block: block, created: created, err: err}
	}()
	<-started
	require.Eventually(t, func() bool { return database.Stats().InUse == 2 }, time.Second, time.Millisecond,
		"ensure operation should hold its own connection while the delivery transaction owns the first")
	require.NoError(t, writer.Commit())
	got := <-result
	require.NoError(t, got.err)
	require.False(t, got.created)
	require.NotNil(t, got.block)
	require.Equal(t, deliveryBlock.ID, got.block.ID)
	blocks, err := repo.ListOpenSessionRecoveryBlocks(t.Context(), fixture.session.ID, fixture.session.QueueIncarnationID, 1)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
}

func testRestartRecoveryBlockEnsureReusesIncident(t *testing.T, open func(*testing.T) *Repository) {
	t.Run("reuse current delivery fence", func(t *testing.T) {
		repo := open(t)
		ctx := context.Background()
		fixture := newSilentRestoreFixture(t, repo, "ensure-reuse", models.TaskSessionStateWaitingForInput)
		original, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
		require.NoError(t, err)
		candidate := models.SessionRecoveryBlock{
			SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
			ExpectedGeneration: 1, Reason: "unresolved_durable_work", State: models.RecoveryBlockOpen,
			ConsumerReference: "agent_delivery",
		}
		store, ok := any(repo).(restartRecoveryBlockEnsurer)
		require.True(t, ok, "repository must support transactional restart fence reuse")
		block, created, err := store.EnsureRestartRecoveryBlock(ctx, &candidate)
		require.NoError(t, err)
		require.NotNil(t, block)
		require.False(t, created)
		require.Equal(t, *original, *block)
		blocks, err := repo.ListOpenSessionRecoveryBlocks(ctx, fixture.session.ID, fixture.session.QueueIncarnationID, 1)
		require.NoError(t, err)
		require.Len(t, blocks, 1)
	})

	t.Run("insert when no fence exists", func(t *testing.T) {
		repo := open(t)
		ctx := context.Background()
		fixture := newSilentRestoreFixture(t, repo, "ensure-insert", models.TaskSessionStateWaitingForInput)
		_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`DELETE FROM session_recovery_blocks WHERE id = ?`), fixture.block.ID)
		require.NoError(t, err)
		candidate := models.SessionRecoveryBlock{
			SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
			ExpectedGeneration: 1, Reason: "unresolved_durable_work", State: models.RecoveryBlockOpen,
			ConsumerReference: "agent_delivery",
		}
		store, ok := any(repo).(restartRecoveryBlockEnsurer)
		require.True(t, ok, "repository must support transactional restart fence creation")
		block, created, err := store.EnsureRestartRecoveryBlock(ctx, &candidate)
		require.NoError(t, err)
		require.NotNil(t, block)
		require.True(t, created)
		require.NotEmpty(t, block.ID)
		blocks, err := repo.ListOpenSessionRecoveryBlocks(ctx, fixture.session.ID, fixture.session.QueueIncarnationID, 1)
		require.NoError(t, err)
		require.Len(t, blocks, 1)
		require.Equal(t, block.ID, blocks[0].ID)
	})
}

func testRestartRecoveryBlockEnsureRefusesUnsafeInventory(t *testing.T, open func(*testing.T) *Repository) {
	for _, scenario := range []string{"multiple fences", "independent fence", "generation replaced", "resolved predecessor", "insert failure"} {
		t.Run(scenario, func(t *testing.T) {
			repo := open(t)
			ctx := context.Background()
			fixture := newSilentRestoreFixture(t, repo, "ensure-refuse-"+scenario, models.TaskSessionStateWaitingForInput)
			candidate := restartRecoveryBlockCandidate(fixture.session.ID, fixture.session.QueueIncarnationID, 1)
			switch scenario {
			case "multiple fences":
				companion := candidate
				companion.ID = "second-delivery-fence"
				companion.Reason = deliveryUnresolvedWorkReason
				require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &companion))
			case "independent fence":
				_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`DELETE FROM session_recovery_blocks WHERE id = ?`), fixture.block.ID)
				require.NoError(t, err)
				independent := models.SessionRecoveryBlock{
					ID: "independent-fence", SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
					ExpectedGeneration: 1, Reason: "office_restriction", State: models.RecoveryBlockOpen,
					ConsumerReference: "office",
				}
				require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &independent))
			case "generation replaced":
				generation := fixture.generation
				generation.Generation = 2
				generation.PredecessorGeneration = 1
				generation.CreationReason = models.SilentRestoreAction
				require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &generation))
			case "resolved predecessor":
				changed, err := repo.ResolveSessionRecoveryBlock(ctx, fixture.block.ID, "stop", fixture.block.UpdatedAt)
				require.NoError(t, err)
				require.True(t, changed)
				candidate.Reason = deliveryUnknownPromptReason
				candidate.DeliverySubmissionID = fixture.block.DeliverySubmissionID
				candidate.DeliveryStreamID = fixture.block.DeliveryStreamID
			case "insert failure":
				if repo.db.DriverName() != "sqlite3" {
					t.Skip("SQLite insert trigger injects the storage failure")
				}
				_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`DELETE FROM session_recovery_blocks WHERE id = ?`), fixture.block.ID)
				require.NoError(t, err)
				candidate.ID = "rejected-restart-fence"
				_, err = repo.DB().ExecContext(ctx, `CREATE TRIGGER reject_restart_fence BEFORE INSERT ON session_recovery_blocks
					WHEN NEW.id = 'rejected-restart-fence' BEGIN SELECT RAISE(ABORT, 'insert refused'); END`)
				require.NoError(t, err)
			}
			store, ok := any(repo).(restartRecoveryBlockEnsurer)
			require.True(t, ok)
			block, created, err := store.EnsureRestartRecoveryBlock(ctx, &candidate)
			if scenario == "insert failure" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Nil(t, block)
			require.False(t, created)
			blocks, err := repo.ListOpenSessionRecoveryBlocks(ctx, fixture.session.ID, fixture.session.QueueIncarnationID, 1)
			require.NoError(t, err)
			switch scenario {
			case "multiple fences":
				require.Len(t, blocks, 2)
			case "insert failure", "resolved predecessor":
				require.Empty(t, blocks)
			case "generation replaced":
				require.Len(t, blocks, 1)
				require.Equal(t, fixture.block.ID, blocks[0].ID)
			default:
				require.Len(t, blocks, 1)
				require.Equal(t, "office_restriction", blocks[0].Reason)
			}
			if scenario == "resolved predecessor" {
				previous, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
				require.NoError(t, err)
				require.Equal(t, models.RecoveryBlockResolved, previous.State)
				require.Equal(t, "stop", previous.AuthorizedAction)
			}
		})
	}
}

type restartRecoveryEnsureResult struct {
	block   *models.SessionRecoveryBlock
	created bool
	err     error
}

func restartRecoveryBlockCandidate(sessionID, incarnationID string, generation int64) models.SessionRecoveryBlock {
	return models.SessionRecoveryBlock{
		SessionID: sessionID, IncarnationID: incarnationID, ExpectedGeneration: generation,
		Reason: deliveryUnresolvedWorkReason, State: models.RecoveryBlockOpen,
		ConsumerReference: reconstructionDeliveryConsumer,
	}
}
