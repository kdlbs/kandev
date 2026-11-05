//go:build linux

package sandboxruntime

import (
	"context"
	"os"
	"testing"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/auth/authn"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSandboxRuntimeFailedSpawnReceiptReleasesAttempt(t *testing.T) {
	packagePath := os.Getenv("KANDEV_TEST_SANDBOX_PACKAGE")
	if packagePath == "" {
		t.Skip("requires dedicated sandbox package")
	}
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	providers, policies, selection := runtimeFixture(t, ctx, packagePath)
	installed, err := providers.ListSandboxProviders(ctx)
	require.NoError(t, err)
	require.Len(t, installed, 1)
	binding := installed[0].Binding
	work := t.TempDir()
	revision, err := policies.Publish(ctx, policy.Policy{ID: "work", Name: "Work", InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey, Grants: contract.Grants{Filesystem: []contract.FilesystemGrant{{Path: work, Writable: true}}}}, 0)
	require.NoError(t, err)
	_, err = policies.PutBinding(ctx, policy.Binding{Scope: policy.ScopeExecutor, TargetID: selection.ExecutorID, Executor: &policy.ExecutorPolicy{Mode: policy.ModeRequired, InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey, CeilingRevisionID: revision.ID, DefaultRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID}}}, 0)
	require.NoError(t, err)
	manager := NewManager(policies, providers, t.TempDir())
	prepared, err := manager.Prepare(ctx, PrepareRequest{Selection: selection, SessionID: "session", ExecutionID: "execution", Generation: 1, WorkDir: work, Arguments: []string{"/bin/true"}})
	require.NoError(t, err)
	// No process is spawned in this test; retain a cleanup route for a failed assertion.
	t.Cleanup(func() { prepared.startDispatched = false; _ = prepared.Close(); require.NoError(t, manager.Close()) })
	require.NoError(t, prepared.MarkStart(ctx))
	require.Error(t, prepared.Close(), "a missing start reply cannot prove no spawn")
	launch := prepared.Launch()
	receipt := &contract.StartReceipt{Version: 1, AttemptID: launch.AttemptID, Generation: launch.Generation + 1, PolicyDigest: launch.PolicyDigest, Outcome: contract.StartNotStarted}
	require.Error(t, prepared.ApplyStartReceipt(ctx, "agentctl", receipt))
	require.DirExists(t, launch.Root)
	receipt.Generation = launch.Generation
	require.NoError(t, prepared.ApplyStartReceipt(ctx, "agentctl", receipt))
	require.NoError(t, prepared.Close())
	require.NoDirExists(t, launch.Root)
	stored, err := policies.LatestSessionAttempt(context.Background(), "org", "session")
	require.NoError(t, err)
	require.Equal(t, policy.AttemptStopped, stored.State)
	require.Nil(t, stored.Owner, "a failed spawn must not fabricate a process identity")
	require.Error(t, prepared.ApplyStartReceipt(ctx, "agentctl", receipt), "closed attempt rejects further replies")
}
