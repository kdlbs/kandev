package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func createSetPatchFixture(t *testing.T, repo *Repository) *models.RepositorySet {
	t.Helper()
	set := setFixture(t, repo)
	set.Items[0].BaseBranch = "original-web"
	set.Items[1].BaseBranch = "original-gateway"
	set.Items[2].BaseBranch = "original-orders"
	require.NoError(t, repo.CreateRepositorySet(context.Background(), set))
	stored, err := repo.GetRepositorySet(context.Background(), set.ID)
	require.NoError(t, err)
	return stored
}

func rawSetPatchItems(t *testing.T, repo *Repository, id string) []models.RepositorySetItem {
	t.Helper()
	rows, err := repo.db.QueryContext(context.Background(), repo.db.Rebind(`
		SELECT id, repository_set_id, repository_id, position, base_branch, created_at, updated_at
		FROM repository_set_items WHERE repository_set_id = ? ORDER BY position
	`), id)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	items := make([]models.RepositorySetItem, 0)
	for rows.Next() {
		var item models.RepositorySetItem
		require.NoError(t, rows.Scan(&item.ID, &item.RepositorySetID, &item.RepositoryID,
			&item.Position, &item.BaseBranch, &item.CreatedAt, &item.UpdatedAt))
		items = append(items, item)
	}
	require.NoError(t, rows.Err())
	return items
}

// @covers AC-WORKSPACES-REPOSITORY-SETS-001.9
func TestPatchRepositorySetPresence(t *testing.T) {
	checkSetPatchPresence(t, newRepoForSetTests(t))
}

func checkSetPatchPresence(t *testing.T, repo *Repository) {
	t.Helper()
	initial := createSetPatchFixture(t, repo)
	name, description, empty := "Touched name", "Touched description", ""
	cases := []struct {
		name                      string
		patch                     models.RepositorySetPatch
		wantName, wantDescription string
	}{
		{"description", models.RepositorySetPatch{Description: &description}, initial.Name, description},
		{"name", models.RepositorySetPatch{Name: &name}, name, description},
		{"clear", models.RepositorySetPatch{Description: &empty}, name, ""},
		{"no-op", models.RepositorySetPatch{}, name, ""},
		{"combined", models.RepositorySetPatch{Name: &initial.Name, Description: &description}, initial.Name, description},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before, err := repo.GetRepositorySet(context.Background(), initial.ID)
			require.NoError(t, err)
			require.NoError(t, repo.PatchRepositorySet(context.Background(), initial.ID, &tc.patch))
			stored, err := repo.GetRepositorySet(context.Background(), initial.ID)
			require.NoError(t, err)
			require.Equal(t, tc.wantName, stored.Name)
			require.Equal(t, tc.wantDescription, stored.Description)
			require.True(t, stored.UpdatedAt.After(before.UpdatedAt))
			require.Equal(t, initial.Items, rawSetPatchItems(t, repo, initial.ID))
		})
	}
	items := []models.RepositorySetItem{
		{RepositoryID: "repo-orders", BaseBranch: "release-orders"},
		{RepositoryID: "repo-web", BaseBranch: "release-web"},
	}
	require.NoError(t, repo.PatchRepositorySet(context.Background(), initial.ID, &models.RepositorySetPatch{Items: &items}))
	stored, err := repo.GetRepositorySet(context.Background(), initial.ID)
	require.NoError(t, err)
	require.Equal(t, initial.Name, stored.Name)
	require.Equal(t, description, stored.Description)
	for i, item := range items {
		require.NotEmpty(t, item.ID)
		require.Equal(t, initial.ID, item.RepositorySetID)
		require.Equal(t, item.ID, stored.Items[i].ID)
		require.WithinDuration(t, item.CreatedAt, stored.Items[i].CreatedAt, time.Microsecond)
		require.WithinDuration(t, item.UpdatedAt, stored.Items[i].UpdatedAt, time.Microsecond)
	}
	require.Equal(t, []string{"repo-orders", "repo-web"}, stored.RepositoryIDs())
	require.Equal(t, 0, items[0].Position)
	require.Equal(t, 1, items[1].Position)
	require.Equal(t, "release-orders", items[0].BaseBranch)
	require.Equal(t, "release-web", items[1].BaseBranch)
}

// @covers AC-WORKSPACES-REPOSITORY-SETS-001.8
func TestPatchRepositorySetRollsBack(t *testing.T) {
	checkSetPatchRollback(t, newRepoForSetTests(t))
}

func checkSetPatchRollback(t *testing.T, repo *Repository) {
	t.Helper()
	initial := createSetPatchFixture(t, repo)
	name, description := "Must roll back", "Must also roll back"
	items := []models.RepositorySetItem{
		{RepositoryID: "repo-orders", BaseBranch: "partial-insert"},
		{RepositoryID: "nonexistent", BaseBranch: "failure"},
	}
	err := repo.PatchRepositorySet(context.Background(), initial.ID,
		&models.RepositorySetPatch{Name: &name, Description: &description, Items: &items})
	require.Error(t, err)
	stored, err := repo.GetRepositorySet(context.Background(), initial.ID)
	require.NoError(t, err)
	require.Equal(t, initial, stored, "parent timestamp and all fields must roll back")
	require.Equal(t, initial.Items, rawSetPatchItems(t, repo, initial.ID))
}

func TestPatchRepositorySetDeleted(t *testing.T) {
	checkSetPatchDeleted(t, newRepoForSetTests(t))
}

func checkSetPatchDeleted(t *testing.T, repo *Repository) {
	t.Helper()
	initial := createSetPatchFixture(t, repo)
	deleted, err := repo.DeleteRepositorySet(context.Background(), initial.ID)
	require.NoError(t, err)
	require.True(t, deleted)
	name, description := "No resurrection", "No resurrection either"
	items := []models.RepositorySetItem{{RepositoryID: "repo-web", BaseBranch: "late-base"}}
	for _, patch := range []models.RepositorySetPatch{
		{}, {Name: &name}, {Description: &description}, {Items: &items},
		{Name: &name, Description: &description, Items: &items},
	} {
		require.ErrorIs(t, repo.PatchRepositorySet(context.Background(), initial.ID, &patch),
			repoerrors.ErrRepositorySetNotFound)
	}
	_, err = repo.GetRepositorySet(context.Background(), initial.ID)
	require.ErrorIs(t, err, repoerrors.ErrRepositorySetNotFound)
	require.Empty(t, rawSetPatchItems(t, repo, initial.ID))
}

func TestPatchRepositorySetNameConflict(t *testing.T) {
	checkSetPatchNameConflict(t, newRepoForSetTests(t))
}

func checkSetPatchNameConflict(t *testing.T, repo *Repository) {
	t.Helper()
	initial := createSetPatchFixture(t, repo)
	other := &models.RepositorySet{WorkspaceID: initial.WorkspaceID, Name: "Taken",
		Items: []models.RepositorySetItem{{RepositoryID: "repo-web", BaseBranch: "other-base"}}}
	require.NoError(t, repo.CreateRepositorySet(context.Background(), other))
	name, description := "tAKEN", "rejected"
	items := []models.RepositorySetItem{{RepositoryID: "repo-orders", BaseBranch: "rejected-base"}}
	require.Error(t, repo.PatchRepositorySet(context.Background(), initial.ID,
		&models.RepositorySetPatch{Name: &name, Description: &description, Items: &items}))
	stored, err := repo.GetRepositorySet(context.Background(), initial.ID)
	require.NoError(t, err)
	require.Equal(t, initial, stored)
	require.Equal(t, initial.Items, rawSetPatchItems(t, repo, initial.ID))
}
