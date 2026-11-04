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

func TestPackagedProviderRefusesUnadvertisedPolicySchema(t *testing.T) {
	packagePath := os.Getenv("KANDEV_TEST_SANDBOX_PACKAGE")
	if packagePath == "" {
		t.Skip("set KANDEV_TEST_SANDBOX_PACKAGE to a provider that supports only policy schema 1")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "operator", OrgID: "org", Role: authn.RoleAdmin, SessionID: "browser"})
	providers, policies, selection := runtimeFixture(t, ctx, packagePath)
	available, err := providers.ListSandboxProviders(ctx)
	require.NoError(t, err)
	require.Len(t, available, 1)
	binding := available[0].Binding
	require.Equal(t, uint32(2), binding.ProtocolVersion())
	work := t.TempDir()
	revision, err := policies.Publish(ctx, policy.Policy{ID: "typed", Name: "Typed", InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey,
		PolicyV2: &contract.PolicyV2{SchemaVersion: contract.PolicySchemaV2,
			StaticGrants: contract.PermissionsV2{Filesystem: []contract.FilesystemGrant{{Path: work, Writable: true}}}}}, 0)
	require.NoError(t, err)
	_, err = policies.PutBinding(ctx, policy.Binding{Scope: policy.ScopeExecutor, TargetID: selection.ExecutorID, Executor: &policy.ExecutorPolicy{
		Mode: policy.ModeRequired, InstallationID: binding.InstallationID, ProviderKey: binding.ProviderKey,
		CeilingRevisionID: revision.ID, DefaultRevisionID: revision.ID, EligibleRevisionIDs: []string{revision.ID},
	}}, 0)
	require.NoError(t, err)
	manager := NewManager(policies, providers, t.TempDir())
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	prepared, err := manager.Prepare(ctx, PrepareRequest{Selection: selection, SessionID: "session", ExecutionID: "execution", Generation: 1, WorkDir: work, Arguments: []string{"unstarted-agent"}})
	require.ErrorIs(t, err, policy.ErrIneligible, "protocol major 2 does not imply support for policy schema 2")
	require.Nil(t, prepared)
}
