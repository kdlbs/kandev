package sqlite

import (
	"context"
	"testing"

	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-004.2 AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006.1
func TestSQLiteRecoveryBlockRebuildRollback(t *testing.T) {
	for _, failure := range []struct {
		name   string
		op     int
		object string
	}{
		{"inventory", sqlite3.SQLITE_READ, "sqlite_master"},
		{"first_trigger_drop", sqlite3.SQLITE_DROP_TRIGGER, `a"attached`},
		{"second_trigger_drop", sqlite3.SQLITE_DROP_TRIGGER, "b_external"},
		{"replacement_creation", sqlite3.SQLITE_CREATE_TABLE, "session_recovery_blocks_migration"},
		{"row_copy", sqlite3.SQLITE_INSERT, "session_recovery_blocks_migration"},
		{"old_table_drop", sqlite3.SQLITE_DROP_TABLE, "session_recovery_blocks"},
		{"replacement_rename", sqlite3.SQLITE_ALTER_TABLE, "session_recovery_blocks_migration"},
		{"lookup_index", sqlite3.SQLITE_CREATE_INDEX, "idx_session_recovery_blocks_open"},
		{"unique_index", sqlite3.SQLITE_CREATE_INDEX, "idx_session_recovery_blocks_open_reason"},
		{"first_trigger_restore", sqlite3.SQLITE_CREATE_TRIGGER, `a"attached`},
		{"second_trigger_restore", sqlite3.SQLITE_CREATE_TRIGGER, "b_external"},
		{"commit", sqlite3.SQLITE_TRANSACTION, "COMMIT"},
	} {
		t.Run(failure.name, func(t *testing.T) {
			repo := recoveryBlockRebuildFixture(t)
			beforeSchema := recoveryBlockSchemaSnapshot(t, repo)
			beforeRows := recoveryBlockRowsSnapshot(t, repo)
			denied := false
			setRecoveryBlockAuthorizer(t, repo, func(op int, first, second, _ string) int {
				object := first
				if op == sqlite3.SQLITE_ALTER_TABLE {
					object = second
				}
				if op == failure.op && object == failure.object {
					denied = true
					return sqlite3.SQLITE_DENY
				}
				return sqlite3.SQLITE_OK
			})
			t.Cleanup(func() { setRecoveryBlockAuthorizer(t, repo, nil) })
			err := repo.rebuildSQLiteRecoveryBlocks(t.Context())
			setRecoveryBlockAuthorizer(t, repo, nil)
			require.True(t, denied, "must reach the selected database boundary")
			require.Error(t, err)
			require.Equal(t, beforeSchema, recoveryBlockSchemaSnapshot(t, repo))
			require.Equal(t, beforeRows, recoveryBlockRowsSnapshot(t, repo))
			require.NoError(t, repo.migrateSessionRecoveryBlockIncidentUniqueness(t.Context()))
			require.Equal(t, beforeRows, recoveryBlockRowsSnapshot(t, repo))
			assertRecoveryBlockTriggersWork(t, repo)
		})
	}
}

func recoveryBlockRebuildFixture(t *testing.T) *Repository {
	t.Helper()
	repo := newRepoForSessionTests(t)
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: "migration-task", Title: "Migration"}))
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "migration-session", TaskID: "migration-task"}))
	installLegacyRecoveryBlockConstraint(t, repo)
	_, err := repo.db.Exec(`INSERT INTO session_recovery_blocks
		(id, session_id, incarnation_id, expected_generation, reason, state, consumer_reference,
		 delivery_submission_id, delivery_stream_id, delivery_sequence, delivery_turn_id, delivery_outcome,
		 authorized_action, created_at, updated_at, resolved_at)
		VALUES ('open', 'migration-session', 'incarnation', 3, 'current', 'open', 'consumer',
		 'submission', 'stream', 19, 'turn', 'unknown', '',
		 '2026-10-01 01:02:03.123456+00:00', '2026-10-02 01:02:03.654321+00:00', NULL),
		 ('resolved', 'migration-session', 'incarnation', 3, 'history', 'resolved', 'historical-consumer',
		 'old-submission', 'old-stream', 7, 'old-turn', 'completed', 'resume',
		 '2026-09-01 01:02:03.123456+00:00', '2026-09-02 01:02:03.654321+00:00', '2026-09-02 01:02:03.654321+00:00');
		CREATE TABLE migration_events (id TEXT);
		CREATE TABLE recovery_audit (value TEXT);
		CREATE TRIGGER "a""attached" AFTER UPDATE ON session_recovery_blocks
		 BEGIN INSERT INTO recovery_audit VALUES ('attached'); END;
		CREATE TRIGGER b_external AFTER INSERT ON migration_events
		 BEGIN INSERT INTO recovery_audit SELECT id FROM SESSION_RECOVERY_BLOCKS; END;
		CREATE TRIGGER z_unrelated AFTER DELETE ON migration_events
		 BEGIN INSERT INTO recovery_audit VALUES ('unrelated'); END`)
	require.NoError(t, err)
	return repo
}

func recoveryBlockSchemaSnapshot(t *testing.T, repo *Repository) []string {
	t.Helper()
	var schema []string
	require.NoError(t, repo.db.Select(&schema, `SELECT type || ':' || name || ':' || tbl_name || ':' || COALESCE(sql, '')
		FROM sqlite_master ORDER BY type, name`))
	return schema
}

func recoveryBlockRowsSnapshot(t *testing.T, repo *Repository) []string {
	t.Helper()
	var rows []string
	require.NoError(t, repo.db.Select(&rows, `SELECT
		quote(id) || ',' || quote(session_id) || ',' || quote(incarnation_id) || ',' || quote(expected_generation) || ',' ||
		quote(reason) || ',' || quote(state) || ',' || quote(consumer_reference) || ',' || quote(delivery_submission_id) || ',' ||
		quote(delivery_stream_id) || ',' || quote(delivery_sequence) || ',' || quote(delivery_turn_id) || ',' || quote(delivery_outcome) || ',' ||
		quote(authorized_action) || ',' || quote(created_at) || ',' || quote(updated_at) || ',' || quote(resolved_at)
		FROM session_recovery_blocks ORDER BY id`))
	return rows
}

func assertRecoveryBlockTriggersWork(t *testing.T, repo *Repository) {
	t.Helper()
	_, err := repo.db.Exec(`UPDATE session_recovery_blocks SET consumer_reference = consumer_reference WHERE id = 'open';
		INSERT INTO migration_events VALUES ('event'); DELETE FROM migration_events`)
	require.NoError(t, err)
	var audit []string
	require.NoError(t, repo.db.Select(&audit, `SELECT value FROM recovery_audit ORDER BY value`))
	require.Equal(t, []string{"attached", "open", "resolved", "unrelated"}, audit)
}

func setRecoveryBlockAuthorizer(t *testing.T, repo *Repository, authorize func(int, string, string, string) int) {
	t.Helper()
	conn, err := repo.db.Conn(context.Background())
	require.NoError(t, err)
	defer func() { require.NoError(t, conn.Close()) }()
	require.NoError(t, conn.Raw(func(raw any) error {
		raw.(*sqlite3.SQLiteConn).RegisterAuthorizer(authorize)
		return nil
	}))
}
