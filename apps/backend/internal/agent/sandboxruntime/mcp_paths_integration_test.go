//go:build linux

package sandboxruntime

import (
	"context"
	"os"
	"testing"
	"time"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/auth/authn"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestPackagedRuntimeCapturesAndRetainsMCPPackagePaths(t *testing.T) {
	packagePath := os.Getenv("KANDEV_TEST_SANDBOX_PACKAGE")
	if packagePath == "" {
		t.Skip("set KANDEV_TEST_SANDBOX_PACKAGE to the dedicated provider package")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	providers, policies, selection := runtimeFixture(t, ctx, packagePath)
	available, err := providers.ListSandboxProviders(ctx)
	require.NoError(t, err)
	require.Len(t, available, 1)
	binding := available[0].Binding
	work := t.TempDir()
	revision, err := policies.Publish(ctx, policy.Policy{ID: "scoped-mcp", Name: "Scoped MCP", InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey,
		Grants: contract.Grants{KandevMCP: true, Filesystem: []contract.FilesystemGrant{{Path: work, Writable: true}}}}, 0)
	require.NoError(t, err)
	_, err = policies.PutBinding(ctx, policy.Binding{Scope: policy.ScopeExecutor, TargetID: selection.ExecutorID, Executor: &policy.ExecutorPolicy{
		Mode: policy.ModeRequired, InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey,
		CeilingRevisionID: revision.ID, DefaultRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID}}}, 0)
	require.NoError(t, err)
	manager := NewManager(policies, providers, t.TempDir())
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	forward, _ := runtimeMCPFixture(t)
	paths := []string{"/mcp", "/mcp/packages/rich-output"}
	request := PrepareRequest{MCPPaths: paths, MCPForward: forward, Selection: selection, SessionID: "approval-session",
		ExecutionID: "scoped-execution", Generation: 1, WorkDir: work, Arguments: []string{"unstarted-agent"}}
	prepared, err := manager.Prepare(ctx, request)
	require.NoError(t, err)
	require.Equal(t, paths, prepared.Launch().MCPPaths)
	stored, err := policies.ReadAttempt(ctx, "org", prepared.Launch().AttemptID)
	require.NoError(t, err)
	require.Equal(t, paths, stored.Snapshot.Launch.MCPPaths)
	require.NoError(t, prepared.Close())
	request.RestartOf = prepared.Launch().AttemptID
	request.ExecutionID = "restarted-scoped-execution"
	request.MCPPaths = []string{"/mcp", "/mcp/packages/configuration"}
	restarted, err := manager.Prepare(ctx, request)
	require.NoError(t, err)
	require.Equal(t, paths, restarted.Launch().MCPPaths, "restart must retain its captured route set")
	require.NoError(t, restarted.Close())
}
