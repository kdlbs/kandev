//go:build linux

package sandboxruntime

import (
	"os"
	"testing"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/auth/authn"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestStoppedRecoveryRetriesOwnedRuntimeCleanup(t *testing.T) {
	packagePath := os.Getenv("KANDEV_TEST_SANDBOX_PACKAGE")
	if packagePath == "" {
		t.Skip("set KANDEV_TEST_SANDBOX_PACKAGE to a dedicated provider package")
	}
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	providers, policies, selection := runtimeFixture(t, ctx, packagePath)
	bindings, err := providers.ListSandboxProviders(ctx)
	require.NoError(t, err)
	binding := bindings[0].Binding
	revision, err := policies.Publish(ctx, policy.Policy{ID: "retry", Name: "Retry", InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey, Grants: contract.Grants{Filesystem: []contract.FilesystemGrant{{Reference: contract.ReferenceWorkDir, Writable: true}}}}, 0)
	require.NoError(t, err)
	_, err = policies.PutBinding(ctx, policy.Binding{Scope: policy.ScopeExecutor, TargetID: selection.ExecutorID, Executor: &policy.ExecutorPolicy{Mode: policy.ModeRequired, InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey, CeilingRevisionID: revision.ID, DefaultRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID}}}, 0)
	require.NoError(t, err)
	manager := NewManager(policies, providers, t.TempDir())
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	work := t.TempDir()
	request := PrepareRequest{Selection: selection, SessionID: "session", ExecutionID: "execution", Generation: 1, WorkDir: work, Arguments: []string{"agent"}}
	prepared, err := manager.Prepare(ctx, request)
	require.NoError(t, err)
	request.SessionID, request.ExecutionID = "sibling", "sibling-execution"
	sibling, err := manager.Prepare(ctx, request)
	require.NoError(t, err)
	// Reproduce a backend exit between recording stopped and removing runtime files.
	require.NoError(t, prepared.launch.Containment.Remove())
	attempt, err := policies.ReadAttempt(ctx, "org", prepared.launch.AttemptID)
	require.NoError(t, err)
	_, err = policies.TransitionAttempt(ctx, "org", attempt.ID, attempt.Version, policy.AttemptStopped, nil)
	require.NoError(t, err)
	restarted := NewManager(policies, providers, manager.base)
	t.Cleanup(func() { require.NoError(t, restarted.Close()) })
	_, err = restarted.ReconcileAttempt(ctx, "org", attempt.ID)
	require.NoError(t, err)
	require.NoDirExists(t, prepared.launch.Root)
	require.DirExists(t, sibling.launch.Root)
	require.DirExists(t, work)
	_, err = restarted.ReconcileAttempt(ctx, "org", attempt.ID)
	require.NoError(t, err, "a completed cleanup retry is idempotent")
	require.NoError(t, prepared.Close())
}
