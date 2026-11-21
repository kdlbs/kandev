package sqlite

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageHistorySourceGuardErasureResumesBoundedPages(t *testing.T) {
	runSourceGuardErasurePages(t, false)
}

func TestUsageHistoryPostgresSourceGuardErasureResumesBoundedPages(t *testing.T) {
	runSourceGuardErasurePages(t, true)
}

func runSourceGuardErasurePages(t *testing.T, postgres bool) {
	repo, _ := currentTargetDatabaseFixture(t, postgres)
	seedUnrelatedCurrentTargets(t, repo, 451)
	scope := usageScopeID("ws-1", "org-a")
	var before []retainedTarget
	require.NoError(t, repo.db.Select(&before, repo.db.Rebind(`SELECT * FROM usage_collection_bindings WHERE scope_id = ? ORDER BY sequence`), scope))
	require.Greater(t, len(before), 400)
	_, err := repo.db.Exec(`DELETE FROM usage_collection_source_guards`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`DELETE FROM usage_collection_guard_workspaces`)
	require.NoError(t, err)
	require.NoError(t, repo.BeginOrganizationUsageErasure(t.Context(), "org-a"))
	done, err := repo.eraseOrganizationUsageBatch(t.Context(), "org-a", 200)
	require.NoError(t, err)
	require.False(t, done)
	assertSourceGuardErasureCursor(t, repo, scope, before[199].Sequence)
	var guards int
	require.NoError(t, repo.db.Get(&guards, `SELECT COUNT(*) FROM usage_collection_source_guards`))
	removeFault := failSourceGuardInsert(t, repo, usageSourceGuardHash("usage-transcript", before[299].ExternalSessionID))
	_, err = repo.eraseOrganizationUsageBatch(t.Context(), "org-a", 200)
	require.ErrorContains(t, err, "source guard fault")
	assertSourceGuardErasureCursor(t, repo, scope, before[199].Sequence)
	var after, bindings int
	require.NoError(t, repo.db.Get(&after, `SELECT COUNT(*) FROM usage_collection_source_guards`))
	require.Equal(t, guards, after, "guard bits and their durable checkpoint roll back together")
	require.NoError(t, repo.db.Get(&bindings, repo.db.Rebind(`SELECT COUNT(*) FROM usage_collection_bindings WHERE scope_id = ?`), scope))
	require.Equal(t, len(before), bindings, "a failed guard page cannot purge its source evidence")
	removeFault()
	restarted, err := NewWithDB(repo.db, repo.ro)
	require.NoError(t, err)
	assertSourceGuardErasureCursor(t, restarted, scope, before[199].Sequence)
	require.NoError(t, restarted.EraseOrganizationUsage(t.Context(), "org-a"))
	for _, row := range before {
		var erased int
		require.NoError(t, repo.db.Get(&erased, repo.db.Rebind(`SELECT erased FROM usage_collection_source_guards WHERE source_hash = ?`), usageSourceGuardHash("usage-transcript", row.ExternalSessionID)))
		require.Equal(t, 1, erased)
	}
	for _, table := range []string{"usage_collection_bindings", "usage_collection_current_targets", "usage_collection_guard_erasure"} {
		require.NoError(t, repo.db.Get(&after, repo.db.Rebind(`SELECT COUNT(*) FROM `+table+` WHERE scope_id = ?`), scope))
		require.Zero(t, after, table)
	}
	other := currentTargetForTest(t, restarted, "collector-a", "ws-other", currentUsageTargetSelector{SessionID: "archive-ws-other-session-0000"})
	require.Equal(t, "eligible", other.SourceScopeStatus)
}

func assertSourceGuardErasureCursor(t *testing.T, repo *Repository, scope string, expected int64) {
	t.Helper()
	var last int64
	var complete int
	require.NoError(t, repo.db.QueryRowx(repo.db.Rebind(`SELECT last_sequence,complete FROM usage_collection_guard_erasure WHERE scope_id = ?`), scope).Scan(&last, &complete))
	require.Equal(t, expected, last)
	require.Zero(t, complete)
}

func failSourceGuardInsert(t *testing.T, repo *Repository, source string) func() {
	t.Helper()
	if repo.db.DriverName() == usageDriverPostgres {
		_, err := repo.db.Exec(`CREATE FUNCTION usage_guard_fail_fn() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
			IF NEW.source_hash = '` + source + `' THEN RAISE EXCEPTION 'source guard fault'; END IF; RETURN NEW; END $$`)
		require.NoError(t, err)
		_, err = repo.db.Exec(`CREATE TRIGGER usage_guard_fail BEFORE INSERT ON usage_collection_source_guards FOR EACH ROW EXECUTE FUNCTION usage_guard_fail_fn()`)
		require.NoError(t, err)
		return func() {
			_, err := repo.db.Exec(`DROP TRIGGER usage_guard_fail ON usage_collection_source_guards`)
			require.NoError(t, err)
			_, err = repo.db.Exec(`DROP FUNCTION usage_guard_fail_fn()`)
			require.NoError(t, err)
		}
	}
	_, err := repo.db.Exec(`CREATE TRIGGER usage_guard_fail BEFORE INSERT ON usage_collection_source_guards
		WHEN NEW.source_hash = '` + source + `' BEGIN SELECT RAISE(ABORT, 'source guard fault'); END`)
	require.NoError(t, err)
	return func() {
		_, err := repo.db.Exec(`DROP TRIGGER usage_guard_fail`)
		require.NoError(t, err)
	}
}
