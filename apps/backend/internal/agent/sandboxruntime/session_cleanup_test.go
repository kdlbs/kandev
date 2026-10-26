//go:build linux

package sandboxruntime

import (
	"os"
	"path/filepath"
	"testing"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/processcontainment"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSandboxSessionStateCleanupRequiresStoppedAttempts(t *testing.T) {
	packagePath := os.Getenv("KANDEV_TEST_SANDBOX_PACKAGE")
	if packagePath == "" {
		t.Skip("set KANDEV_TEST_SANDBOX_PACKAGE to a dedicated provider package")
	}
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	providers, policies, selection := runtimeFixture(t, ctx, packagePath)
	selection.TaskID, selection.AgentProfileID = "task", "profile"
	bindings, err := providers.ListSandboxProviders(ctx)
	require.NoError(t, err)
	binding := bindings[0].Binding
	revision, err := policies.Publish(ctx, policy.Policy{ID: "session", Name: "Session", InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey,
		Grants: contract.Grants{Filesystem: []contract.FilesystemGrant{{Reference: contract.ReferenceWorkDir, Writable: true}, {Reference: contract.ReferenceAgentState, Writable: true}}}}, 0)
	require.NoError(t, err)
	_, err = policies.PutBinding(ctx, policy.Binding{Scope: policy.ScopeExecutor, TargetID: selection.ExecutorID, Executor: &policy.ExecutorPolicy{
		Mode: policy.ModeRequired, InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey, CeilingRevisionID: revision.ID,
		DefaultRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID}}}, 0)
	require.NoError(t, err)
	manager := NewManager(policies, providers, t.TempDir())
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	req := PrepareRequest{Selection: selection, SessionID: "session", ExecutionID: "execution", Generation: 1, WorkDir: t.TempDir(), Arguments: []string{"agent"}}
	prepared, err := manager.Prepare(ctx, req)
	require.NoError(t, err)
	home := prepared.Launch().ChildHome
	file := filepath.Join(home, "history")
	require.NoError(t, os.WriteFile(file, []byte("retained"), 0600))
	require.Error(t, manager.CleanupSessionState(ctx, "task", "session"))
	require.FileExists(t, file)
	require.NoError(t, prepared.Close())
	req.SessionID, req.ExecutionID = "sibling", "sibling-execution"
	sibling, err := manager.Prepare(ctx, req)
	require.NoError(t, err)
	require.Error(t, manager.CleanupSessionState(ctx, "another-task", "session"))
	require.FileExists(t, file)
	require.NoError(t, manager.CleanupSessionState(ctx, "task", "session"))
	require.NoDirExists(t, filepath.Dir(home))
	require.DirExists(t, sibling.Launch().ChildHome)
	require.DirExists(t, req.WorkDir)
	require.NoError(t, manager.CleanupSessionState(ctx, "task", "session"))
}

func TestSandboxSessionCleanupRejectsForeignStateScope(t *testing.T) {
	manager := &Manager{attempts: map[string]*Prepared{}}
	scope := contract.AgentStateScope{OrgID: "org", WorkspaceID: "workspace", SessionID: "session", AgentProfileID: "profile", InstallationID: "installation", ProviderKey: "provider"}
	attempt := &policy.Attempt{ID: "attempt", State: policy.AttemptStopped, Snapshot: policy.AttemptSnapshot{
		OrgID: scope.OrgID, WorkspaceID: scope.WorkspaceID, SessionID: scope.SessionID, AgentProfileID: scope.AgentProfileID,
		Provider:   policy.AttemptProvider{InstallationID: scope.InstallationID, Key: scope.ProviderKey},
		AgentState: &contract.AgentStateSnapshot{Scope: scope}, Launch: &contract.Launch{Containment: &processcontainment.Identity{}},
	}}
	_, err := manager.stoppedSessionState(t.Context(), attempt)
	require.NoError(t, err)
	for _, change := range []func(*contract.AgentStateScope){
		func(s *contract.AgentStateScope) { s.OrgID = "other" }, func(s *contract.AgentStateScope) { s.WorkspaceID = "other" },
		func(s *contract.AgentStateScope) { s.SessionID = "other" }, func(s *contract.AgentStateScope) { s.AgentProfileID = "other" },
		func(s *contract.AgentStateScope) { s.InstallationID = "other" }, func(s *contract.AgentStateScope) { s.ProviderKey = "other" },
	} {
		foreign := *attempt.Snapshot.AgentState
		change(&foreign.Scope)
		attempt.Snapshot.AgentState = &foreign
		_, err := manager.stoppedSessionState(t.Context(), attempt)
		require.ErrorIs(t, err, policy.ErrConflict)
		attempt.Snapshot.AgentState = &contract.AgentStateSnapshot{Scope: scope}
	}
}
