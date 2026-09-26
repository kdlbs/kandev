package workflowsync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// denyingAuthorizer asserts the exact workspace ID reaches the authorizer,
// then denies access — simulating a caller who does not own the workspace.
func denyingAuthorizer(t *testing.T, wantWorkspaceID string) func(context.Context, string) error {
	t.Helper()
	return func(_ context.Context, workspaceID string) error {
		if workspaceID != wantWorkspaceID {
			t.Fatalf("authorizer workspace = %q, want %q", workspaceID, wantWorkspaceID)
		}
		return repoerrors.ErrWorkspaceNotFound
	}
}

func TestGetConfigForWorkspace_DeniesForeignWorkspace(t *testing.T) {
	svc, _ := setupTestService(t, github.NewMockClient())
	configureWorkspace(t, svc, "victim")
	svc.SetWorkspaceAuthorizer(denyingAuthorizer(t, "victim"))

	cfg, err := svc.GetConfigForWorkspace(context.Background(), "victim")
	assert.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)
	assert.Nil(t, cfg)
}

func TestSetConfigForWorkspace_DeniesForeignWorkspaceAndLeavesConfigUnchanged(t *testing.T) {
	svc, _ := setupTestService(t, github.NewMockClient())
	configureWorkspace(t, svc, "victim")
	before, err := svc.store.GetConfigForWorkspace(context.Background(), "victim")
	require.NoError(t, err)
	require.NotNil(t, before)

	svc.SetWorkspaceAuthorizer(denyingAuthorizer(t, "victim"))
	_, err = svc.SetConfigForWorkspace(context.Background(), "victim", &SetConfigRequest{
		RepoOwner: "attacker",
		RepoName:  "evil",
	})
	assert.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)

	after, err := svc.store.GetConfigForWorkspace(context.Background(), "victim")
	require.NoError(t, err)
	require.NotNil(t, after)
	assert.Equal(t, before.RepoOwner, after.RepoOwner)
	assert.Equal(t, before.RepoName, after.RepoName)
}

func TestDeleteConfigForWorkspace_DeniesForeignWorkspaceAndLeavesConfigInPlace(t *testing.T) {
	svc, applier := setupTestService(t, github.NewMockClient())
	configureWorkspace(t, svc, "victim")

	svc.SetWorkspaceAuthorizer(denyingAuthorizer(t, "victim"))
	err := svc.DeleteConfigForWorkspace(context.Background(), "victim")
	assert.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)

	cfg, err := svc.store.GetConfigForWorkspace(context.Background(), "victim")
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Empty(t, applier.released, "denied delete must not release synced workflows")
}

func TestSyncWorkspace_DeniesForeignWorkspaceAndNeverApplies(t *testing.T) {
	svc, applier := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, "victim")

	svc.SetWorkspaceAuthorizer(denyingAuthorizer(t, "victim"))
	_, err := svc.SyncWorkspace(context.Background(), "victim")
	assert.ErrorIs(t, err, repoerrors.ErrWorkspaceNotFound)
	assert.Zero(t, applier.callCount(), "denied sync must never reach the applier")
}

// The internal periodic poller (SyncDueConfigs) and any caller that never
// wires an authorizer must keep working exactly as before this feature —
// authorization is opt-in scoping, not a default-deny gate.
func TestServiceMethods_SucceedWhenNoAuthorizerWired(t *testing.T) {
	svc, _ := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, "ws-1")

	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.NotNil(t, cfg)

	_, err = svc.SyncWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)

	err = svc.DeleteConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
}

func TestServiceMethods_SucceedWhenAuthorizerAllows(t *testing.T) {
	svc, _ := setupTestService(t, seededMockClient())
	configureWorkspace(t, svc, "ws-1")
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })

	cfg, err := svc.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.NotNil(t, cfg)

	_, err = svc.SetConfigForWorkspace(context.Background(), "ws-1", &SetConfigRequest{
		RepoOwner: "acme", RepoName: "flows",
	})
	require.NoError(t, err)

	_, err = svc.SyncWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)

	require.NoError(t, svc.DeleteConfigForWorkspace(context.Background(), "ws-1"))
}
