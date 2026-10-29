package sandbox

import (
	"errors"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestSandboxPolicyTaskGuardSQLite(t *testing.T) { testTaskGuardStore(t, sqlitePolicyDB(t)) }
func TestSandboxPolicyTaskGuardPostgres(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	database.SetMaxOpenConns(4)
	testTaskGuardStore(t, database)
}
func testTaskGuardStore(t *testing.T, database *sqlx.DB) {
	t.Helper()
	s, err := NewService(database, true, nil)
	require.NoError(t, err)
	revision, err := s.Publish(operatorContext(t), policyFixture(t), 0)
	require.NoError(t, err)
	selected := &Resolved{Revision: revision, Ceiling: revision, ExecutorID: "executor", SelectionSource: ScopeExecutor, BindingVersion: 1}
	selection := SelectionContext{OrgID: "org", WorkspaceID: "workspace", TaskID: "task", ExecutorID: "executor"}
	results := make(chan error, 2)
	var writers sync.WaitGroup
	for range 2 {
		writers.Go(func() { results <- s.saveTaskGuard(t.Context(), selection, selected, 0) })
	}
	writers.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, ErrConflict):
			conflicts++
		default:
			t.Fatal(result)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	reloaded, err := NewService(database, false, nil)
	require.NoError(t, err)
	got, version, err := reloaded.readTaskGuard(t.Context(), "org", "task", "workspace")
	require.NoError(t, err)
	require.Equal(t, int64(1), version)
	require.Equal(t, revision.Digest, got.Revision.Digest)
	_, _, err = reloaded.readTaskGuard(t.Context(), "org", "task", "different-workspace")
	require.ErrorIs(t, err, ErrIneligible)
	absent, version, err := reloaded.readTaskGuard(t.Context(), "other-org", "task", "workspace")
	require.NoError(t, err)
	require.Nil(t, absent)
	require.Zero(t, version)
	// Disabling the feature and changing executor does not erase the captured limit.
	selection.ExecutorID = "unprotected"
	_, err = reloaded.AdmitTask(t.Context(), selection, "")
	require.ErrorIs(t, err, ErrIneligible)
}
