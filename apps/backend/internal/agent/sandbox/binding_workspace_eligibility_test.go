package sandbox

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-SANDBOX-002.1 AC-AGENTS-SANDBOX-004.5
func TestSandboxPolicyGlobalExecutorAllowsWorkspaceEligibleRevision(t *testing.T) {
	service, ceiling, read := policySelectionFixture(t)
	ctx := operatorContext(t)
	service.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })
	service.SetTargetResolver(func(_ context.Context, scope, _ string) (TargetScope, error) {
		workspace := ""
		if scope == ScopeTask {
			workspace = "workspace"
		}
		return TargetScope{OrgID: "org", WorkspaceID: workspace}, nil
	})
	policy := read.Policy
	policy.ID, policy.WorkspaceID = "workspace-read", "workspace"
	local, err := service.Publish(ctx, policy, 0)
	require.NoError(t, err)
	binding, err := service.GetBinding(ctx, ScopeExecutor, "executor")
	require.NoError(t, err)
	binding.Executor.EligibleRevisionIDs = append(binding.Executor.EligibleRevisionIDs, local.ID)
	saved, err := service.PutBinding(ctx, *binding, binding.Version)
	require.NoError(t, err)
	_, err = service.PutBinding(ctx, Binding{Scope: ScopeTask, TargetID: "task", WorkspaceID: "workspace", RevisionID: local.ID}, 0)
	require.NoError(t, err)
	resolved, err := service.Resolve(ctx, SelectionContext{OrgID: "org", WorkspaceID: "workspace", ExecutorID: "executor", TaskID: "task"})
	require.NoError(t, err)
	require.Equal(t, local.ID, resolved.Revision.ID)
	require.Equal(t, ceiling.ID, resolved.Ceiling.ID)
	_, err = service.Resolve(ctx, SelectionContext{OrgID: "org", WorkspaceID: "other", ExecutorID: "executor", TaskID: "task"})
	require.ErrorIs(t, err, ErrIneligible)
	eligible, err := service.previewEligiblePolicies(ctx, saved, "other")
	require.NoError(t, err)
	for _, revision := range eligible {
		require.NotEqual(t, local.ID, revision.ID)
	}
	for _, kind := range []string{"default", "ceiling", "inaccessible"} {
		t.Run(kind, func(t *testing.T) {
			candidate := *saved
			executor := *saved.Executor
			candidate.Executor = &executor
			switch kind {
			case "default":
				executor.DefaultRevisionID = local.ID
			case "ceiling":
				executor.CeilingRevisionID = local.ID
			case "inaccessible":
				service.SetWorkspaceAuthorizer(func(context.Context, string) error { return ErrAuthority })
				t.Cleanup(func() { service.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil }) })
			}
			_, err := service.PutBinding(ctx, candidate, saved.Version)
			require.Error(t, err)
			current, err := service.GetBinding(ctx, ScopeExecutor, "executor")
			require.NoError(t, err)
			require.Equal(t, saved.Version, current.Version)
		})
	}
}
