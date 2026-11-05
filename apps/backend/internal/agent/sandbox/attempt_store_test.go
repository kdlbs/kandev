package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestSandboxAttemptsSQLite(t *testing.T) { testAttemptPersistence(t, sqlitePolicyDB(t)) }
func TestSandboxAttemptsPostgres(t *testing.T) {
	testAttemptPersistence(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func testAttemptPersistence(t *testing.T, database *sqlx.DB) {
	t.Helper()
	svc, err := NewService(database, true, nil)
	require.NoError(t, err)
	ctx := operatorContext(t)
	svc.SetTargetResolver(func(context.Context, string, string) (TargetScope, error) { return TargetScope{OrgID: "org"}, nil })
	policy := policyFixture(t)
	revision, err := svc.Publish(ctx, policy, 0)
	require.NoError(t, err)
	_, err = svc.PutBinding(ctx, Binding{Scope: ScopeExecutor, TargetID: "local", Executor: &ExecutorPolicy{Mode: ModeRequired, InstallationID: policy.InstallationID, ProviderKey: policy.ProviderKey, CeilingRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID}, DefaultRevisionID: revision.ID}}, 0)
	require.NoError(t, err)
	selection := SelectionContext{OrgID: "org", ExecutorID: "local"}
	resolved, err := svc.Resolve(ctx, selection)
	require.NoError(t, err)
	content := []byte("fixture")
	digest := sha256.Sum256(content)
	runtimeDigest := hex.EncodeToString(digest[:])
	files, err := contract.MaterializeRuntime(ctx, t.TempDir(), content, runtimeDigest, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, files.Close()) })
	launch := &contract.Launch{Version: 1, AttemptID: "attempt", Generation: 1, PolicyDigest: revision.Digest, PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable}, Digests: files.Digests}
	request := CaptureRequest{Selection: selection, Resolved: resolved, SessionID: "session", ExecutionID: "execution", WorkDir: policy.Grants.Filesystem[0].Path, Launch: launch, Provider: AttemptProvider{PluginID: "plugin", InstallationID: policy.InstallationID, Key: policy.ProviderKey, Version: "1.0.0", PackageDigest: launch.PackageDigest, RuntimeVersion: "1.0.0", RuntimeDigest: runtimeDigest}}
	changes := 0
	svc.SetAttemptChangeNotifier(func() { changes++ })
	attempt, err := svc.CaptureAttempt(ctx, request)
	require.NoError(t, err)
	require.Equal(t, 1, changes, "committed capture must wake the observer")
	require.Equal(t, AttemptPrepared, attempt.State)
	require.EqualValues(t, 1, attempt.Version)
	_, err = svc.CaptureAttempt(ctx, request)
	require.ErrorIs(t, err, ErrConflict)
	require.Equal(t, 1, changes, "rejected capture must not wake the observer")
	// Changes to a caller's captured data and later publications cannot rewrite the stored attempt.
	launch.Prefix[0] = "changed"
	resolved.Revision.Policy.Name = "changed"
	policy.Name = "Next default"
	latest, err := svc.Publish(ctx, policy, 1)
	require.NoError(t, err)
	reloaded, err := NewService(database, true, nil)
	require.NoError(t, err)
	retained, err := reloaded.ReadAttempt(ctx, "org", "attempt")
	require.NoError(t, err)
	require.Equal(t, revision.Digest, retained.Snapshot.Resolved.Revision.Digest)
	require.Equal(t, "Work", retained.Snapshot.Resolved.Revision.Policy.Name)
	require.Equal(t, filepath.Join(files.Root, "runtime"), retained.Snapshot.Launch.Prefix[0])
	_, err = reloaded.ReadAttempt(ctx, "foreign", "attempt")
	require.ErrorIs(t, err, ErrNotFound)

	_, err = svc.PutBinding(ctx, Binding{Scope: ScopeExecutor, TargetID: "local", Executor: &ExecutorPolicy{Mode: ModeRequired, InstallationID: policy.InstallationID, ProviderKey: policy.ProviderKey, CeilingRevisionID: latest.ID, EligibleRevisionIDs: []string{latest.ID}, DefaultRevisionID: latest.ID}}, 1)
	require.NoError(t, err)
	request.Launch = retained.Snapshot.Launch.Clone()
	request.Launch.AttemptID = "stale-selection"
	request.Launch.Generation = 2
	_, err = svc.CaptureAttempt(ctx, request)
	require.ErrorIs(t, err, ErrConflict)
	transitions := 0
	reloaded.SetAttemptChangeNotifier(func() { transitions++ })
	starting, err := reloaded.TransitionAttempt(ctx, "org", "attempt", 1, AttemptStarting, nil)
	require.NoError(t, err)
	_, err = reloaded.TransitionAttempt(ctx, "org", "attempt", 1, AttemptFailed, nil)
	require.ErrorIs(t, err, ErrConflict)
	_, err = reloaded.TransitionAttempt(ctx, "org", "attempt", starting.Version, AttemptRunning, nil)
	require.Error(t, err, "running requires an owned process identity")
	owner := &AttemptOwner{SupervisorPID: 123, SupervisorBirth: "boot:123:456", AgentctlIdentity: "runtime-owner"}
	running, err := reloaded.TransitionAttempt(ctx, "org", "attempt", starting.Version, AttemptRunning, owner)
	require.NoError(t, err)

	replaced := *owner
	replaced.SupervisorBirth = "different-process"
	_, err = reloaded.TransitionAttempt(ctx, "org", "attempt", running.Version, AttemptRunning, &replaced)
	require.Error(t, err)
	stopped, err := reloaded.TransitionAttempt(ctx, "org", "attempt", running.Version, AttemptStopped, owner)
	require.NoError(t, err)
	_, err = reloaded.TransitionAttempt(ctx, "org", "attempt", stopped.Version, AttemptRunning, owner)
	require.Error(t, err, "late process events cannot revive a stopped attempt")
	require.Equal(t, 3, transitions, "only committed start, bind, and stop transitions wake the observer")
}
