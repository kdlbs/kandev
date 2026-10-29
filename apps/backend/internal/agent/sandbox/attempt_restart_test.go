package sandbox

import (
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSandboxRestartRetainsCapturedPolicyAndProvider(t *testing.T) {
	testFrozenRestart(t, sqlitePolicyDB(t))
}
func TestSandboxRestartRetainsCapturedPolicyAndProviderPostgres(t *testing.T) {
	testFrozenRestart(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}
func testFrozenRestart(t *testing.T, database *sqlx.DB) {
	t.Helper()
	service, original, _ := approvalStoreFixture(t, database)
	ctx := operatorContext(t)
	selection := SelectionContext{OrgID: "org", ExecutorID: "local"}
	_, err := service.ResolveRestart(ctx, selection, original.Snapshot.SessionID, original.Snapshot.WorkDir, original.ID)
	require.Error(t, err, "a live attempt cannot be restarted")
	_, err = service.TransitionAttempt(ctx, "org", original.ID, original.Version, AttemptStopped, nil)
	require.NoError(t, err)
	changed := original.Snapshot.Resolved.Revision.Policy
	changed.Name = "New default"
	latest, err := service.Publish(ctx, changed, original.Snapshot.Resolved.Revision.Number)
	require.NoError(t, err)
	binding, err := service.GetBinding(ctx, ScopeExecutor, "local")
	require.NoError(t, err)
	binding.Executor.DefaultRevisionID = latest.ID
	binding.Executor.EligibleRevisionIDs = append(binding.Executor.EligibleRevisionIDs, latest.ID)
	_, err = service.PutBinding(ctx, *binding, binding.Version)
	require.NoError(t, err)
	previous, err := service.ResolveRestart(ctx, selection, original.Snapshot.SessionID, original.Snapshot.WorkDir, original.ID)
	require.NoError(t, err)
	require.Equal(t, original.Snapshot.Resolved.Revision.ID, previous.Snapshot.Resolved.Revision.ID)
	request := CaptureRequest{RestartOf: original.ID, Selection: selection, Resolved: previous.Snapshot.Resolved, SessionID: original.Snapshot.SessionID, ExecutionID: original.Snapshot.ExecutionID, WorkDir: original.Snapshot.WorkDir, Provider: original.Snapshot.Provider, Launch: original.Snapshot.Launch.Clone()}
	request.Launch.AttemptID = "restart"
	request.Launch.Generation++
	changedRequest := request
	changedRequest.Provider.Version = "other-version"
	_, err = service.CaptureAttempt(ctx, changedRequest)
	require.Error(t, err)
	restarted, err := service.CaptureAttempt(ctx, request)
	require.NoError(t, err)
	require.Equal(t, original.ID, restarted.Snapshot.RestartOf)
	require.Equal(t, original.Snapshot.Resolved.Revision.ID, restarted.Snapshot.Resolved.Revision.ID)
	_, err = service.ResolveRestart(ctx, selection, "foreign-session", original.Snapshot.WorkDir, original.ID)
	require.Error(t, err)
	require.NoError(t, service.Revoke(ctx, original.Snapshot.Resolved.Revision.ID, original.Snapshot.Resolved.Revision.Digest))
	_, err = service.ResolveRestart(ctx, selection, original.Snapshot.SessionID, original.Snapshot.WorkDir, original.ID)
	require.Error(t, err)
}
