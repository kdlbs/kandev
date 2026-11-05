//go:build linux

package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func testSandboxAvailabilityPolicySchema(t *testing.T, ctx context.Context, policies *sandbox.Service) {
	t.Helper()
	before, err := policies.GetBinding(ctx, sandbox.ScopeExecutor, "local")
	require.NoError(t, err)
	source, err := policies.GetRevision(ctx, before.Executor.DefaultRevisionID)
	require.NoError(t, err)
	policy := source.Policy
	policy.ID, policy.Grants = "unsupported-v2", contract.Grants{}
	policy.PolicyV2 = &contract.PolicyV2{SchemaVersion: 2, StaticGrants: contract.PermissionsV2{
		Filesystem: []contract.FilesystemGrant{{Reference: contract.ReferenceWorkDir, Writable: true}},
	}}
	revision, err := policies.Publish(ctx, policy, 0)
	require.NoError(t, err)
	changed, executor := *before, *before.Executor
	executor.DefaultRevisionID, executor.CeilingRevisionID, executor.EligibleRevisionIDs = revision.ID, revision.ID, []string{revision.ID}
	changed.Executor = &executor
	saved, err := policies.PutBinding(ctx, changed, before.Version)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, restoreErr := policies.PutBinding(ctx, *before, saved.Version)
		require.NoError(t, restoreErr)
	})
	availability, err := policies.CheckAvailability(ctx, sandbox.PreviewContext{ExecutorID: "local"})
	require.NoError(t, err)
	require.False(t, availability.Available, "major 2 does not establish support for policy schema 2")
	require.Equal(t, "sandbox_runtime_unavailable", availability.Blocker)
}
