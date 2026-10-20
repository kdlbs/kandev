package sqlite

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func ownershipJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(data, &fields))
	owner, _ := fields["owner_user_id"].(string)
	return owner
}

// @covers AC-EXECUTORS-OWNERSHIP-001.1, AC-EXECUTORS-OWNERSHIP-001.3
func TestExecutorOwnershipMigrationAndPersistence(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := t.Context()
	var executor models.Executor
	require.NoError(t, json.Unmarshal([]byte(`{"id":"private-executor","name":"Private","type":"ssh","status":"active","owner_user_id":"alice"}`), &executor))
	require.NoError(t, repo.CreateExecutor(ctx, &executor))
	got, err := repo.GetExecutor(ctx, executor.ID)
	require.NoError(t, err)
	require.Equal(t, "alice", ownershipJSON(t, got), "executor ownership must round trip")

	var profile models.ExecutorProfile
	require.NoError(t, json.Unmarshal([]byte(`{"id":"private-profile","executor_id":"private-executor","name":"Private","owner_user_id":"alice"}`), &profile))
	require.NoError(t, repo.CreateExecutorProfile(ctx, &profile))
	loaded, err := repo.GetExecutorProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.Equal(t, "alice", ownershipJSON(t, loaded))
	profiles, err := repo.ListAllExecutorProfiles(ctx)
	require.NoError(t, err)
	owners := map[string]string{}
	for _, item := range profiles {
		owners[item.ID] = ownershipJSON(t, item)
	}
	require.Equal(t, "alice", owners[profile.ID])
	executors, err := repo.ListExecutors(ctx)
	require.NoError(t, err)
	for _, item := range executors {
		owners[item.ID] = ownershipJSON(t, item)
	}
	require.Equal(t, "alice", owners[executor.ID])

	// Replay on the same database must retain private ownership.
	require.NoError(t, repo.runMigrations(ctx))
	loaded, err = repo.GetExecutorProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.Equal(t, "alice", ownershipJSON(t, loaded))

	seedExecutorForProfiles(t, repo, "legacy-shared")
	var owner *string
	require.NoError(t, repo.db.GetContext(ctx, &owner, `SELECT owner_user_id FROM executors WHERE id = 'legacy-shared'`))
	require.Nil(t, owner, "unowned resources must remain SQL NULL")
}

func TestExecutorOwnershipOrdinaryWritesPreserveOwner(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := t.Context()
	seedExecutorForProfiles(t, repo, "shared-parent")
	var profile models.ExecutorProfile
	require.NoError(t, json.Unmarshal([]byte(`{"id":"owned","executor_id":"shared-parent","name":"Owned","owner_user_id":"alice"}`), &profile))
	require.NoError(t, repo.CreateExecutorProfile(ctx, &profile))
	// A legacy snapshot omitting ownership must not clear an existing owner.
	legacy := profile
	require.NoError(t, json.Unmarshal([]byte(`{"owner_user_id":""}`), &legacy))
	require.NoError(t, repo.UpdateExecutorProfile(ctx, &legacy))
	loaded, err := repo.GetExecutorProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.Equal(t, "alice", ownershipJSON(t, loaded))
	require.NoError(t, repo.UpdateExecutorProfileWithScriptIntent(ctx, &legacy, models.ExecutorProfileScriptIntent{}))
	loaded, err = repo.GetExecutorProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.Equal(t, "alice", ownershipJSON(t, loaded))
}

// @covers AC-EXECUTORS-OWNERSHIP-002.3
func TestExecutorOwnershipTransferRejectsStaleSaveAndIncompatibleParent(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := t.Context()
	seedExecutorForProfiles(t, repo, "transfer-parent")
	profile := &models.ExecutorProfile{ID: "transfer-profile", ExecutorID: "transfer-parent", Name: "Original", OwnerUserID: "alice"}
	require.NoError(t, repo.CreateExecutorProfile(ctx, profile))
	stale := *profile
	bob := "bob"
	profile.OwnershipIntent = &models.ExecutorOwnershipIntent{ExpectedOwnerUserID: "alice", OwnerUserID: &bob}
	require.NoError(t, repo.UpdateExecutorProfile(ctx, profile))
	stale.Name = "Unauthorized stale edit"
	stale.OwnershipIntent = &models.ExecutorOwnershipIntent{ExpectedOwnerUserID: "alice"}
	require.ErrorIs(t, repo.UpdateExecutorProfileWithScriptIntent(ctx, &stale, models.ExecutorProfileScriptIntent{}), models.ErrExecutorOwnershipChanged)
	stored, err := repo.GetExecutorProfile(ctx, profile.ID)
	require.NoError(t, err)
	require.Equal(t, "bob", stored.OwnerUserID)
	require.Equal(t, "Original", stored.Name)
	parent, err := repo.GetExecutor(ctx, "transfer-parent")
	require.NoError(t, err)
	alice := "alice"
	parent.OwnershipIntent = &models.ExecutorOwnershipIntent{OwnerUserID: &alice}
	require.ErrorIs(t, repo.UpdateExecutor(ctx, parent), models.ErrExecutorOwnershipInvalid)
	parent.OwnershipIntent = &models.ExecutorOwnershipIntent{OwnerUserID: &bob}
	require.NoError(t, repo.UpdateExecutor(ctx, parent))
	require.ErrorIs(t, repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{ID: "foreign-child", ExecutorID: parent.ID, Name: "Foreign", OwnerUserID: "alice"}), models.ErrExecutorOwnershipInvalid)
}
