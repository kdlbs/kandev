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

func TestSandboxUtilityOmitsTaskMCPWithoutChangingPolicy(t *testing.T) {
	packagePath := os.Getenv("KANDEV_TEST_SANDBOX_PACKAGE")
	if packagePath == "" {
		t.Skip("set KANDEV_TEST_SANDBOX_PACKAGE to a dedicated provider package")
	}
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", Role: authn.RoleAdmin, SessionID: "browser"})
	providers, policies, selection := runtimeFixture(t, ctx, packagePath)
	policies.SetTargetResolver(func(context.Context, string, string) (policy.TargetScope, error) {
		return policy.TargetScope{}, nil
	})
	selection.OrgID = ""
	selection.AgentProfileID = "profile"
	bindings, err := providers.ListSandboxProviders(ctx)
	require.NoError(t, err)
	binding := bindings[0].Binding
	revision, err := policies.Publish(ctx, policy.Policy{ID: "utility", Name: "Utility", InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey,
		Grants: contract.Grants{KandevMCP: true, Filesystem: []contract.FilesystemGrant{{Reference: contract.ReferenceWorkDir, Writable: true}, {Reference: contract.ReferenceAgentState, Writable: true}}}}, 0)
	require.NoError(t, err)
	_, err = policies.PutBinding(ctx, policy.Binding{Scope: policy.ScopeExecutor, TargetID: selection.ExecutorID, Executor: &policy.ExecutorPolicy{
		Mode: policy.ModeRequired, InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey, CeilingRevisionID: revision.ID,
		DefaultRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID}}}, 0)
	require.NoError(t, err)
	manager := NewManager(policies, providers, t.TempDir())
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	req := PrepareRequest{Selection: selection, ExecutionID: "utility", Generation: 1, WorkDir: t.TempDir(), Arguments: []string{"agent"}}
	prepared, err := manager.Prepare(ctx, req)
	require.NoError(t, err)
	require.Empty(t, prepared.Launch().MCPURL)
	require.False(t, prepared.policy.Grants.KandevMcp)
	attempt, err := policies.ReadAttempt(ctx, "", prepared.Launch().AttemptID)
	require.NoError(t, err)
	require.True(t, attempt.Snapshot.Resolved.Revision.Policy.Grants.KandevMCP)
	require.False(t, attempt.Snapshot.Grants.KandevMCP)
	require.NoError(t, prepared.Close())
	req.SessionID, req.ExecutionID = "session", "task-execution"
	_, err = manager.Prepare(ctx, req)
	require.ErrorIs(t, err, policy.ErrIneligible, "task sessions still require a scoped MCP forwarder")
}
