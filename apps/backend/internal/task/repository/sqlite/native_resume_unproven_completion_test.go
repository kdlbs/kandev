package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeUnprovenSQLCompletion(t *testing.T) {
	testNativeResumeUnprovenSQLCompletion(t, newRepoForSessionTests)
}

func TestPostgresNativeResumeUnprovenSQLCompletion(t *testing.T) {
	testNativeResumeUnprovenSQLCompletion(t, func(t *testing.T) *Repository {
		database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
		repo, err := NewWithDB(database, database, nil)
		require.NoError(t, err)
		return repo
	})
}

type unprovenSQLFixture struct {
	*silentRestoreFixture
	submission *models.AgentDeliverySubmission
	proof      *models.NativeResumeDeliveryEvidence
}

// newUnprovenSQLFixture seeds the foreground-handoff mismatch and returns the
// complete authenticated proof the lifecycle would submit.
func newUnprovenSQLFixture(t *testing.T, repo *Repository) *unprovenSQLFixture {
	t.Helper()
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, "unproven", models.TaskSessionStateWaitingForInput)
	event := &models.AgentDeliveryEvent{
		SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		HarnessGeneration: 1, TaskID: fixture.task.ID, ExecutionID: fixture.recovery.AgentExecutionID,
		OwnerGeneration: 1, ExpectedStreamID: fixture.recovery.StreamID, StreamID: fixture.recovery.StreamID,
		Sequence: 1, EventType: "usage_update", Payload: []byte(`{}`),
	}
	_, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_submissions
		SET state = 'completed', outcome = 'completed', terminal_stream_id = '',
		    terminal_sequence = 0, terminal_turn_id = '', fully_settled_at = NULL, payload_pruned_at = NULL
		WHERE id = ?`), fixture.recovery.SubmissionID)
	require.NoError(t, err)
	row, err := repo.GetAgentDeliverySubmission(ctx, fixture.recovery.SubmissionID)
	require.NoError(t, err)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, fixture.recovery.StreamID)
	require.NoError(t, err)
	proof := &models.NativeResumeDeliveryEvidence{
		SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		HarnessGeneration: 1, NativeSessionID: fixture.generation.NativeSessionID,
		ExecutionID: fixture.recovery.AgentExecutionID, StreamID: fixture.recovery.StreamID,
		Cursor: cursor,
		Submissions: []models.NativeResumeSubmissionEvidence{{
			ID: row.ID, HarnessGeneration: 1, PayloadHash: row.PayloadHash,
			StreamID: fixture.recovery.StreamID, State: models.DeliverySubmissionCompleted,
			Outcome: string(models.DeliverySubmissionCompleted), UnprovenCompletion: true, Cursor: cursor,
		}},
	}
	return &unprovenSQLFixture{silentRestoreFixture: fixture, submission: row, proof: proof}
}

func projectUnprovenCursor(
	t *testing.T,
	repo *Repository,
	sessionID, incarnationID string,
	generation int64,
	streamID string,
) *models.AgentDeliveryCursor {
	t.Helper()
	ctx := context.Background()
	event := &models.AgentDeliveryEvent{
		SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: generation,
		StreamID: streamID, Sequence: 1, EventType: "usage_update", Payload: []byte(`{}`),
	}
	_, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, streamID)
	require.NoError(t, err)
	return cursor
}

type historicalCursorFixture struct {
	*silentRestoreFixture
	proof         *models.NativeResumeDeliveryEvidence
	oldStream     string
	currentStream string
}

// newHistoricalCursorFixture builds a current generation 2 recovery with an
// eligible generation 1 unproven stream. The generic fence and absent notice
// keep the test focused on the per-stream cursor proof.
func newHistoricalCursorFixture(t *testing.T, repo *Repository, suffix string) *historicalCursorFixture {
	t.Helper()
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, suffix, models.TaskSessionStateWaitingForInput)
	oldStream := fixture.recovery.StreamID
	oldCursor := projectUnprovenCursor(t, repo, fixture.session.ID, fixture.session.QueueIncarnationID, 1, oldStream)
	_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_submissions
		SET state = 'completed', outcome = 'completed', terminal_stream_id = '',
		    terminal_sequence = 0, terminal_turn_id = '', fully_settled_at = NULL, payload_pruned_at = NULL
		WHERE id = ?`), fixture.recovery.SubmissionID)
	require.NoError(t, err)
	now := time.Now().UTC()
	committed, err := repo.CommitHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		Generation: 2, NativeSessionID: "native-current", CreationReason: "test",
		CreatedAt: now, CommittedAt: now,
	}, 1)
	require.NoError(t, err)
	require.True(t, committed)
	_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE executors_running SET agent_execution_id = 'current-execution' WHERE session_id = ?`), fixture.session.ID)
	require.NoError(t, err)
	currentStream := "stream-current-" + suffix
	currentCursor := projectUnprovenCursor(t, repo, fixture.session.ID, fixture.session.QueueIncarnationID, 2, currentStream)
	submission, err := repo.GetAgentDeliverySubmission(ctx, fixture.recovery.SubmissionID)
	require.NoError(t, err)
	proof := &models.NativeResumeDeliveryEvidence{
		SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		HarnessGeneration: 2, NativeSessionID: "native-current",
		ExecutionID: "current-execution", StreamID: currentStream, Cursor: currentCursor,
		Submissions: []models.NativeResumeSubmissionEvidence{{
			ID: submission.ID, HarnessGeneration: 1, PayloadHash: submission.PayloadHash,
			StreamID: oldStream, State: models.DeliverySubmissionCompleted,
			Outcome: string(models.DeliverySubmissionCompleted), UnprovenCompletion: true, Cursor: oldCursor,
		}},
	}
	_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks
		SET reason = ?, delivery_submission_id = '', delivery_stream_id = '', expected_generation = 2
		WHERE id = ?`), deliveryUnresolvedWorkReason, fixture.block.ID)
	require.NoError(t, err)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, fixture.session.ID, models.SessionMetaKeyAgentDeliveryRecovery, nil))
	return &historicalCursorFixture{
		silentRestoreFixture: fixture, proof: proof,
		oldStream: oldStream, currentStream: currentStream,
	}
}

func (f *historicalCursorFixture) refreshProofCursor(t *testing.T, repo *Repository) {
	t.Helper()
	cursor, err := repo.GetAgentDeliveryCursor(t.Context(), f.oldStream)
	require.NoError(t, err)
	f.proof.Submissions[0].Cursor = cursor
}

func testNativeResumeUnprovenSQLCompletionHistoricalCursor(t *testing.T, repo *Repository, suffix string) {
	t.Helper()
	ctx := context.Background()
	fixture := newHistoricalCursorFixture(t, repo, suffix+"-gap")
	_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_cursors SET received_sequence = 2 WHERE stream_id = ?`), fixture.oldStream)
	require.NoError(t, err)
	changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, fixture.block.ID, fixture.proof, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, changed, "an unprojected historical receipt must block resolution")

	_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_cursors SET projected_sequence = 2 WHERE stream_id = ?`), fixture.oldStream)
	require.NoError(t, err)
	fixture.refreshProofCursor(t, repo)
	changed, err = repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, fixture.block.ID, fixture.proof, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, changed, "complete projection admits a fresh proof")

	missing := newHistoricalCursorFixture(t, repo, suffix+"-missing")
	_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`DELETE FROM agent_delivery_cursors WHERE stream_id = ?`), missing.oldStream)
	require.NoError(t, err)
	changed, err = repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, missing.block.ID, missing.proof, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, changed, "a missing historical cursor must block resolution")

	highWater := newHistoricalCursorFixture(t, repo, suffix+"-high-water")
	_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_cursors SET remote_high_water = 2 WHERE stream_id = ?`), highWater.oldStream)
	require.NoError(t, err)
	changed, err = repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, highWater.block.ID, highWater.proof, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, changed, "an unreceived historical high-water mark must block resolution")
	_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_cursors SET received_sequence = 2, projected_sequence = 2 WHERE stream_id = ?`), highWater.oldStream)
	require.NoError(t, err)
	highWater.refreshProofCursor(t, repo)
	changed, err = repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, highWater.block.ID, highWater.proof, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, changed, "a fresh proof admits the received high-water mark")
}

func TestNativeResumeUnprovenCompletionInboxQueryPlan(t *testing.T) {
	repo := newRepoForSessionTests(t)
	rows, err := repo.db.Query(`EXPLAIN QUERY PLAN SELECT 1 FROM agent_delivery_inbox i WHERE i.submission_id = ? AND i.terminal <> 0`, "submission")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var details string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		details += detail
	}
	require.NoError(t, rows.Err())
	require.Contains(t, details, "idx_agent_delivery_inbox_submission_terminal",
		"terminal absence lookup must use the dedicated sparse index: %s", details)
}

func TestPostgresNativeResumeUnprovenCompletionInboxQueryPlan(t *testing.T) {
	database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	ctx := t.Context()
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	for index := 0; index < 2000; index++ {
		_, err := tx.ExecContext(ctx, repo.db.Rebind(`INSERT INTO agent_delivery_inbox
			(stream_id, sequence, session_id, incarnation_id, harness_generation, submission_id, event_type, payload, terminal, received_at)
			VALUES (?, 1, 'session-plan', 'incarnation-plan', 1, ?, 'complete', ?, 1, CURRENT_TIMESTAMP)`),
			fmt.Sprintf("stream-plan-%d", index), fmt.Sprintf("unrelated-%d", index), []byte(`{}`))
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit())
	_, err = repo.db.Exec(`ANALYZE agent_delivery_inbox`)
	require.NoError(t, err)
	rows, err := repo.db.QueryxContext(ctx, repo.db.Rebind(`EXPLAIN (COSTS OFF)
		SELECT 1 FROM agent_delivery_inbox i WHERE i.submission_id = ? AND i.terminal <> 0`), "unrelated-1")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		plan = append(plan, line)
	}
	require.NoError(t, rows.Err())
	require.True(t, strings.Contains(strings.Join(plan, "\n"), "idx_agent_delivery_inbox_submission_terminal"),
		"postgres terminal absence lookup must use the dedicated sparse index: %v", plan)
}

func TestNativeResumeUnprovenSQLCompletionHistoricalCursor(t *testing.T) {
	testNativeResumeUnprovenSQLCompletionHistoricalCursor(t, newRepoForSessionTests(t), "sqlite")
}

func TestPostgresNativeResumeUnprovenSQLCompletionHistoricalCursor(t *testing.T) {
	database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	testNativeResumeUnprovenSQLCompletionHistoricalCursor(t, repo, "pg")
}

func countWaitingPostgresAdvisoryLocks(t *testing.T, repo *Repository) int {
	t.Helper()
	var count int
	err := repo.DB().QueryRow(`SELECT COUNT(*) FROM pg_locks
		WHERE NOT granted AND locktype = 'advisory'
		AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`).Scan(&count)
	require.NoError(t, err)
	return count
}

// TestPostgresNativeResumeUnprovenSQLCompletionConcurrentReceipt holds the
// resolution locks while the real receive path commits an unprojected receipt
// on the historical stream. Resolution must then reject the stale proof and
// accept only a rebuilt proof after complete projection.
func TestPostgresNativeResumeUnprovenSQLCompletionConcurrentReceipt(t *testing.T) {
	database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	ctx := t.Context()
	fixture := newHistoricalCursorFixture(t, repo, "pg-race")
	event := &models.AgentDeliveryEvent{
		SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		HarnessGeneration: 1, StreamID: fixture.oldStream, Sequence: 2,
		EventType: "usage_update", Payload: []byte(`{}`),
	}
	lockTx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = lockTx.Rollback() }()
	require.NoError(t, lockSessionTurnWrites(ctx, lockTx, repo.db.DriverName(), fixture.session.ID))
	require.NoError(t, lockAgentDeliveryWrites(ctx, lockTx, repo.db.DriverName(), []string{fixture.session.ID}, []string{fixture.proof.StreamID}))
	waitingBefore := countWaitingPostgresAdvisoryLocks(t, repo)
	receiveErr := make(chan error, 1)
	go func() {
		_, receiveFailure := repo.ReceiveAgentDeliveryEvent(ctx, event, 2)
		receiveErr <- receiveFailure
	}()
	require.Eventually(t, func() bool {
		return countWaitingPostgresAdvisoryLocks(t, repo) > waitingBefore
	}, 5*time.Second, 10*time.Millisecond, "the real receipt path must wait on the resolution lock")
	require.NoError(t, lockTx.Commit())
	require.NoError(t, <-receiveErr)

	changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, fixture.block.ID, fixture.proof, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, changed, "the committed unprojected receipt must invalidate the proof")

	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	fixture.refreshProofCursor(t, repo)
	changed, err = repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, fixture.block.ID, fixture.proof, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, changed, "a rebuilt proof admits complete projection")
}

func testNativeResumeUnprovenSQLCompletion(t *testing.T, open func(*testing.T) *Repository) {
	t.Run("resolves matching historical completion", func(t *testing.T) {
		repo := open(t)
		fixture := newUnprovenSQLFixture(t, repo)
		changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), fixture.block.ID, fixture.proof, time.Now().UTC())
		require.NoError(t, err)
		require.True(t, changed)
		block, err := repo.GetSessionRecoveryBlock(t.Context(), fixture.block.ID)
		require.NoError(t, err)
		require.Equal(t, models.RecoveryBlockResolved, block.State)
		after, err := repo.GetAgentDeliverySubmission(t.Context(), fixture.submission.ID)
		require.NoError(t, err)
		require.Equal(t, fixture.submission, after, "historical SQL completion must remain immutable")
		verified, err := repo.VerifyResolvedSessionRecoveryBlockWithNativeEvidence(t.Context(), fixture.block.ID, fixture.proof)
		require.NoError(t, err)
		require.True(t, verified)
		changed, err = repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), fixture.block.ID, fixture.proof, time.Now().UTC())
		require.NoError(t, err)
		require.False(t, changed, "a resolved fence must not be released twice")
	})
	t.Run("rejects stale and conflicting proof", func(t *testing.T) {
		mutations := map[string]func(context.Context, *Repository) error{
			"terminal stream": func(ctx context.Context, repo *Repository) error {
				_, err := repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET terminal_stream_id = 'stream-silent-unproven' WHERE id = 'submission-silent-unproven'`)
				return err
			},
			"terminal sequence": func(ctx context.Context, repo *Repository) error {
				_, err := repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET terminal_sequence = 1 WHERE id = 'submission-silent-unproven'`)
				return err
			},
			"terminal turn": func(ctx context.Context, repo *Repository) error {
				_, err := repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET terminal_turn_id = 'turn' WHERE id = 'submission-silent-unproven'`)
				return err
			},
			"settled marker": func(ctx context.Context, repo *Repository) error {
				_, err := repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET fully_settled_at = CURRENT_TIMESTAMP WHERE id = 'submission-silent-unproven'`)
				return err
			},
			"pruned marker": func(ctx context.Context, repo *Repository) error {
				_, err := repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET payload_pruned_at = CURRENT_TIMESTAMP WHERE id = 'submission-silent-unproven'`)
				return err
			},
			"failed label": func(ctx context.Context, repo *Repository) error {
				_, err := repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET state = 'failed', outcome = 'failed' WHERE id = 'submission-silent-unproven'`)
				return err
			},
			"foreign hash": func(ctx context.Context, repo *Repository) error {
				_, err := repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET payload_hash = 'foreign' WHERE id = 'submission-silent-unproven'`)
				return err
			},
			"foreign incarnation": func(ctx context.Context, repo *Repository) error {
				_, err := repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET incarnation_id = 'foreign' WHERE id = 'submission-silent-unproven'`)
				return err
			},
			"foreign generation": func(ctx context.Context, repo *Repository) error {
				_, err := repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET harness_generation = 2 WHERE id = 'submission-silent-unproven'`)
				return err
			},
			"projection gap": func(ctx context.Context, repo *Repository) error {
				_, err := repo.DB().ExecContext(ctx, `UPDATE agent_delivery_cursors SET projected_sequence = 0 WHERE stream_id = 'stream-silent-unproven'`)
				return err
			},
			"invalid retired": func(ctx context.Context, repo *Repository) error {
				_, err := repo.DB().ExecContext(ctx, `UPDATE agent_delivery_cursors SET retired_sequence = 2 WHERE stream_id = 'stream-silent-unproven'`)
				return err
			},
			"terminal inbox": func(ctx context.Context, repo *Repository) error {
				_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`INSERT INTO agent_delivery_inbox
					(stream_id, sequence, session_id, incarnation_id, harness_generation, submission_id, event_type, payload, terminal, received_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`),
					"stream-silent-unproven", 99, "session-silent-unproven", "session-silent-unproven", 1,
					"submission-silent-unproven", "complete", []byte(`{}`), 1)
				return err
			},
			"terminal effect": func(ctx context.Context, repo *Repository) error {
				_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`INSERT INTO agent_delivery_effects
					(effect_key, stream_id, sequence, effect_type, state, created_at, session_id, incarnation_id, harness_generation, submission_id, outcome)
					VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?, ?, ?, ?)`),
					"foreign-terminal", "stream-silent-unproven", 99, models.DeliveryTerminalEffectType,
					models.DeliveryEffectPending, "session-silent-unproven", "session-silent-unproven", 1,
					"submission-silent-unproven", "completed")
				return err
			},
		}
		for scenario, mutation := range mutations {
			t.Run(scenario, func(t *testing.T) {
				repo := open(t)
				fixture := newUnprovenSQLFixture(t, repo)
				require.NoError(t, mutation(t.Context(), repo))
				changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), fixture.block.ID, fixture.proof, time.Now().UTC())
				require.NoError(t, err)
				require.False(t, changed)
				block, err := repo.GetSessionRecoveryBlock(t.Context(), fixture.block.ID)
				require.NoError(t, err)
				require.Equal(t, models.RecoveryBlockOpen, block.State)
			})
		}
	})
	t.Run("rejects terminal after proof construction", func(t *testing.T) {
		repo := open(t)
		fixture := newUnprovenSQLFixture(t, repo)
		_, err := repo.DB().ExecContext(t.Context(), `UPDATE agent_delivery_submissions SET state = 'completed', outcome = 'completed', terminal_stream_id = 'stream-silent-unproven', terminal_sequence = 1 WHERE id = 'submission-silent-unproven'`)
		require.NoError(t, err)
		changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), fixture.block.ID, fixture.proof, time.Now().UTC())
		require.NoError(t, err)
		require.False(t, changed)
	})
}
