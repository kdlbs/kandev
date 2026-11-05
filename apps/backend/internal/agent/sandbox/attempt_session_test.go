package sandbox

import (
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSandboxSessionAttemptLookupAndOwnership(t *testing.T) {
	testSessionAttemptLookup(t, sqlitePolicyDB(t))
}
func TestSandboxSessionAttemptLookupAndOwnershipPostgres(t *testing.T) {
	testSessionAttemptLookup(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}
func testSessionAttemptLookup(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, original, _ := approvalStoreFixture(t, database)
	ctx := operatorContext(t)
	latest, err := service.LatestSessionAttempt(ctx, "org", "session")
	require.NoError(t, err)
	require.Equal(t, original.ID, latest.ID)
	_, err = service.LatestSessionAttempt(ctx, "foreign", "session")
	require.ErrorIs(t, err, ErrNotFound)
	request := CaptureRequest{Selection: SelectionContext{OrgID: "org", ExecutorID: "local"}, Resolved: original.Snapshot.Resolved, SessionID: "session", ExecutionID: "replacement", WorkDir: original.Snapshot.WorkDir, Launch: original.Snapshot.Launch.Clone(), Provider: original.Snapshot.Provider}
	request.Launch.AttemptID = "replacement"
	_, err = service.CaptureAttempt(ctx, request)
	require.ErrorIs(t, err, ErrConflict, "a second execution cannot claim a live session")
	_, err = service.TransitionAttempt(ctx, "org", original.ID, original.Version, AttemptStopped, nil)
	require.NoError(t, err)
	replacement, err := service.CaptureAttempt(ctx, request)
	require.NoError(t, err)
	latest, err = service.LatestSessionAttempt(ctx, "org", "session")
	require.NoError(t, err)
	require.Equal(t, replacement.ID, latest.ID)
	_, err = service.LatestSessionAttempt(ctx, "org", "")
	require.ErrorIs(t, err, ErrNotFound, "sessionless utility attempts are not a session history")
}

func TestSandboxSessionMigrationBackfillsExistingAttempts(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			database := sqlitePolicyDB(t)
			if backend == "postgres" {
				database = testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
			}
			service, original, _ := approvalStoreFixture(t, database)
			_, err := database.Exec(`DROP INDEX sandbox_attempts_active_session; DROP INDEX sandbox_attempts_session_history; ALTER TABLE sandbox_attempts DROP COLUMN session_id`)
			require.NoError(t, err)
			require.NoError(t, service.initAttemptSessions())
			latest, err := service.LatestSessionAttempt(t.Context(), "org", "session")
			require.NoError(t, err)
			require.Equal(t, original.ID, latest.ID)
			require.NoError(t, service.initAttemptSessions(), "migration is idempotent")
		})
	}
}
