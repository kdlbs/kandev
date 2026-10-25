package messagequeue

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSQLitePendingMoveOriginRoundtrip(t *testing.T) {
	verifyPendingMoveOriginRoundtrip(t, newTestSQLiteRepo(t))
}

func TestPostgresPendingMoveOriginRoundtrip(t *testing.T) {
	verifyPendingMoveOriginRoundtrip(t, newTestPostgresRepo(t))
}

func verifyPendingMoveOriginRoundtrip(t *testing.T, repo Repository) {
	t.Helper()
	ctx := context.Background()
	identity := QueueSessionIdentity{TaskID: "origin-task", SessionID: "origin-session", SessionIncarnationID: "origin-incarnation"}
	seedQueueSessionIdentity(t, repo, identity)
	move := &PendingMove{
		SessionIncarnationID: identity.SessionIncarnationID, MoveID: "origin-move", TaskID: "origin-task", WorkflowID: "origin-workflow",
		WorkflowStepID: "target-step", FromStepID: "source-step", FromTransitionID: 1 << 40,
		QueuedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.SetPendingMove(ctx, "origin-session", move))
	move.FromTransitionID = 1<<40 + 1
	require.NoError(t, repo.SetPendingMove(ctx, "origin-session", move))
	got, err := repo.GetPendingMove(ctx, "origin-session")
	require.NoError(t, err)
	require.Equal(t, "source-step", got.FromStepID)
	require.Equal(t, int64(1<<40+1), got.FromTransitionID)
	snapshot, err := repo.Snapshot(ctx, identity)
	require.NoError(t, err)
	require.Equal(t, int64(1<<40+1), snapshot.PendingMove.FromTransitionID)
	listed, err := repo.ListPendingMoves(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "source-step", listed[0].Move.FromStepID)
	require.Equal(t, int64(1<<40+1), listed[0].Move.FromTransitionID)
	taken, err := repo.TakePendingMove(ctx, "origin-session")
	require.NoError(t, err)
	require.Equal(t, int64(1<<40+1), taken.FromTransitionID)
	require.NoError(t, repo.ReplaceSession(ctx, "origin-session", snapshot.Entries, snapshot.PendingMove))
	restored, err := repo.GetPendingMove(ctx, "origin-session")
	require.NoError(t, err)
	require.Equal(t, "source-step", restored.FromStepID)
	require.Equal(t, int64(1<<40+1), restored.FromTransitionID)
	removed, err := repo.DeletePendingMoveIfMatch(ctx, PendingMoveRecord{SessionID: "origin-session", Move: *restored}, "")
	require.NoError(t, err)
	require.True(t, removed)
	got, err = repo.GetPendingMove(ctx, "origin-session")
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestSQLitePendingMoveOriginUpgradeAndReplay(t *testing.T) {
	verifyPendingMoveOriginUpgrade(t, newTestSQLiteRepo(t).(*sqliteRepository))
}

func TestPostgresPendingMoveOriginUpgradeAndReplay(t *testing.T) {
	verifyPendingMoveOriginUpgrade(t, newTestPostgresRepo(t).(*sqliteRepository))
}

func verifyPendingMoveOriginUpgrade(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := context.Background()
	_, err := repo.db.Exec(`DROP TABLE pending_moves`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`CREATE TABLE pending_moves (
		id TEXT PRIMARY KEY, session_id TEXT NOT NULL UNIQUE,
		move_id TEXT NOT NULL DEFAULT '', session_incarnation_id TEXT NOT NULL DEFAULT '',
		task_id TEXT NOT NULL DEFAULT '', workflow_id TEXT NOT NULL DEFAULT '',
		workflow_step_id TEXT NOT NULL DEFAULT '', step_position INTEGER NOT NULL DEFAULT 0,
		queued_at TIMESTAMP NOT NULL, actor TEXT NOT NULL DEFAULT '',
		sender_session_id TEXT NOT NULL DEFAULT '', entry_options_json TEXT NOT NULL DEFAULT '')`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`INSERT INTO pending_moves (id, session_id, move_id, task_id, workflow_step_id, queued_at)
		VALUES ('legacy-id', 'legacy-session', 'legacy-move', 'legacy-task', 'target-step', CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	for range 2 {
		require.NoError(t, repo.initSchema())
		legacy, err := repo.GetPendingMove(ctx, "legacy-session")
		require.NoError(t, err)
		require.Equal(t, "legacy-move", legacy.MoveID)
		require.Empty(t, legacy.FromStepID)
		require.Zero(t, legacy.FromTransitionID)
	}
	legacy, err := repo.GetPendingMove(ctx, "legacy-session")
	require.NoError(t, err)
	legacy.FromStepID = "source-step"
	legacy.FromTransitionID = 1<<40 + 2
	require.NoError(t, repo.SetPendingMove(ctx, "legacy-session", legacy))
	require.NoError(t, repo.initSchema())
	taken, err := repo.TakePendingMove(ctx, "legacy-session")
	require.NoError(t, err)
	require.Equal(t, "source-step", taken.FromStepID)
	require.Equal(t, int64(1<<40+2), taken.FromTransitionID)
}
