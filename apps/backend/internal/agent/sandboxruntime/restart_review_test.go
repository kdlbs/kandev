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

func TestSandboxRuntimeCompilesExplicitReviewedRestart(t *testing.T) {
	packagePath := os.Getenv("KANDEV_TEST_SANDBOX_PACKAGE")
	if packagePath == "" {
		t.Skip("set KANDEV_TEST_SANDBOX_PACKAGE to a dedicated provider package")
	}
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	providers, policies, selection := runtimeFixture(t, ctx, packagePath)
	policies.SetSessionAuthorizer(func(context.Context, string) error { return nil })
	listed, err := providers.ListSandboxProviders(ctx)
	require.NoError(t, err)
	binding := listed[0].Binding
	revision, err := policies.Publish(ctx, policy.Policy{ID: "work", Name: "Captured", InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey, Grants: contract.Grants{Filesystem: []contract.FilesystemGrant{{Reference: contract.ReferenceWorkDir, Writable: true}}}}, 0)
	require.NoError(t, err)
	saved, err := policies.PutBinding(ctx, policy.Binding{Scope: policy.ScopeExecutor, TargetID: selection.ExecutorID, Executor: &policy.ExecutorPolicy{Mode: policy.ModeRequired, InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey, CeilingRevisionID: revision.ID, DefaultRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID}}}, 0)
	require.NoError(t, err)
	manager := NewManager(policies, providers, t.TempDir())
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	request := PrepareRequest{Selection: selection, SessionID: "session", ExecutionID: "execution", Generation: 1, WorkDir: t.TempDir(), Arguments: []string{"agent"}}
	first, err := manager.Prepare(ctx, request)
	require.NoError(t, err)
	require.NoError(t, first.Close())
	changed := revision.Policy
	changed.Name = "Reviewed"
	current, err := policies.Publish(ctx, changed, revision.Number)
	require.NoError(t, err)
	saved.Executor.DefaultRevisionID = current.ID
	saved.Executor.EligibleRevisionIDs = append(saved.Executor.EligibleRevisionIDs, current.ID)
	_, err = policies.PutBinding(ctx, *saved, saved.Version)
	require.NoError(t, err)
	preview, err := policies.PreviewSelection(ctx, policy.PreviewContext{ExecutorID: selection.ExecutorID})
	require.NoError(t, err)
	request.Selection.RestartReview = &contract.RestartPolicyReview{AttemptID: first.Launch().AttemptID, SelectionDigest: preview.SelectionDigest}
	request.RestartOf = first.Launch().AttemptID
	request.Generation++
	reviewed, err := manager.Prepare(ctx, request)
	require.NoError(t, err, "the provider must compile the explicitly reviewed revision")
	require.Equal(t, current.Digest, reviewed.Launch().PolicyDigest)
	recorded, err := policies.ReadAttempt(ctx, "org", reviewed.Launch().AttemptID)
	require.NoError(t, err)
	require.NotNil(t, recorded.Snapshot.RestartReview)
	require.Equal(t, "operator", recorded.Snapshot.RestartReview.OperatorID)
}
