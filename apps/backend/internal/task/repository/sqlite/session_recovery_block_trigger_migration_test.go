package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionRecoveryBlockMigrationPreservesQuarantineTriggers(t *testing.T) {
	repo := newRetentionTestRepo(t, "SQLite")
	ctx := context.Background()
	installLegacyRecoveryBlockConstraint(t, repo)
	_, err := repo.db.Exec(`CREATE TABLE migration_trigger_source (id TEXT);
 CREATE TABLE migration_trigger_audit (block_count INTEGER);`)
	require.NoError(t, err)
	triggers := map[string]string{
		"automation_retry_legacy_quarantine_insert":    "AFTER INSERT",
		"automation_retry_legacy_quarantine_update":    "AFTER UPDATE",
		"automation_retry_operation_quarantine_insert": "BEFORE INSERT",
		"automation_retry_operation_quarantine_update": "BEFORE UPDATE",
	}
	original := make(map[string]string)
	for name, event := range triggers {
		statement := `CREATE TRIGGER ` + name + ` ` + event + ` ON migration_trigger_source BEGIN
   INSERT INTO migration_trigger_audit SELECT COUNT(*) FROM session_recovery_blocks; END`
		_, err = repo.db.Exec(statement)
		require.NoError(t, err)
		var saved string
		require.NoError(t, repo.db.Get(&saved, `SELECT sql FROM sqlite_master WHERE name = ?`, name))
		original[name] = saved
	}
	require.NoError(t, repo.runMigrations(ctx))
	require.NoError(t, repo.runMigrations(ctx))
	for name, expected := range original {
		var actual string
		require.NoError(t, repo.db.Get(&actual, `SELECT sql FROM sqlite_master WHERE name = ?`, name))
		require.Equal(t, expected, actual)
	}
	_, err = repo.db.Exec(`INSERT INTO migration_trigger_source VALUES ('one');
 UPDATE migration_trigger_source SET id = 'two'`)
	require.NoError(t, err)
	var fired int
	require.NoError(t, repo.db.Get(&fired, `SELECT COUNT(*) FROM migration_trigger_audit`))
	require.Equal(t, 4, fired)
}

func TestSessionRecoveryBlockMigrationRollsBackQuarantineTriggers(t *testing.T) {
	repo := newRetentionTestRepo(t, "SQLite")
	ctx := context.Background()
	installLegacyRecoveryBlockConstraint(t, repo)
	_, err := repo.db.Exec(`CREATE TABLE migration_trigger_source (id TEXT);
 CREATE TRIGGER automation_retry_legacy_quarantine_insert AFTER INSERT ON migration_trigger_source
 BEGIN SELECT COUNT(*) FROM session_recovery_blocks; END;
 CREATE VIEW migration_recovery_view AS SELECT * FROM session_recovery_blocks`)
	require.NoError(t, err)
	err = repo.migrateSessionRecoveryBlockIncidentUniqueness(ctx)
	require.ErrorContains(t, err, "migration_recovery_view")
	legacy, err := repo.sqliteHasLegacyRecoveryBlockIncidentConstraint(ctx)
	require.NoError(t, err)
	require.True(t, legacy)
	_, err = repo.db.Exec(`INSERT INTO migration_trigger_source VALUES ('after rollback')`)
	require.NoError(t, err)
	var retained int
	require.NoError(t, repo.db.Get(&retained, `SELECT COUNT(*) FROM sqlite_master
 WHERE type = 'trigger' AND name = 'automation_retry_legacy_quarantine_insert'`))
	require.Equal(t, 1, retained)
	_, err = repo.db.Exec(`DROP VIEW migration_recovery_view`)
	require.NoError(t, err)
	require.NoError(t, repo.migrateSessionRecoveryBlockIncidentUniqueness(ctx))
}
