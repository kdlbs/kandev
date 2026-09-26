package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func newRepoForSetTests(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "repository-set-test.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo
}

func seedSetRepository(t *testing.T, repo *Repository, workspaceID, id string) {
	t.Helper()
	err := repo.CreateRepository(context.Background(), &models.Repository{
		ID:          id,
		WorkspaceID: workspaceID,
		Name:        id,
		SourceType:  "local",
		LocalPath:   "/tmp/" + id,
	})
	if err != nil {
		t.Fatalf("seed repository %s: %v", id, err)
	}
}

// setFixture seeds one workspace with three repositories and returns a set
// carrying all three in a deliberate non-alphabetical order, so a test that
// asserts ordering cannot pass by accident.
func setFixture(t *testing.T, repo *Repository) *models.RepositorySet {
	t.Helper()
	seedWorkspace(t, repo, "ws-sets")
	seedSetRepository(t, repo, "ws-sets", "repo-web")
	seedSetRepository(t, repo, "ws-sets", "repo-gateway")
	seedSetRepository(t, repo, "ws-sets", "repo-orders")
	return &models.RepositorySet{
		WorkspaceID: "ws-sets",
		Name:        "Full-stack",
		Description: "web + gateway + orders",
		Items: []models.RepositorySetItem{
			{RepositoryID: "repo-web"},
			{RepositoryID: "repo-gateway"},
			{RepositoryID: "repo-orders"},
		},
	}
}

func repositoryIDsOf(set *models.RepositorySet) []string {
	ids := make([]string, 0, len(set.Items))
	for _, item := range set.Items {
		ids = append(ids, item.RepositoryID)
	}
	return ids
}

func assertRepositoryIDs(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("repository ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("repository ids = %v, want %v", got, want)
		}
	}
}

func TestCreateRepositorySetPersistsEveryFieldAndOrder(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)

	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}
	if set.ID == "" {
		t.Fatal("CreateRepositorySet left the id empty")
	}
	if set.CreatedAt.IsZero() || set.UpdatedAt.IsZero() {
		t.Fatal("CreateRepositorySet left a timestamp zero")
	}

	loaded, err := repo.GetRepositorySet(ctx, set.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet: %v", err)
	}
	if loaded.WorkspaceID != "ws-sets" || loaded.Name != "Full-stack" {
		t.Fatalf("loaded set = %+v", loaded)
	}
	if loaded.Description != "web + gateway + orders" {
		t.Fatalf("description = %q", loaded.Description)
	}
	assertRepositoryIDs(t, repositoryIDsOf(loaded), "repo-web", "repo-gateway", "repo-orders")
	for i, item := range loaded.Items {
		if item.Position != i {
			t.Fatalf("item %d position = %d", i, item.Position)
		}
		if item.ID == "" || item.RepositorySetID != set.ID {
			t.Fatalf("item %d = %+v", i, item)
		}
	}
}

func TestGetRepositorySetMissingReportsNotFound(t *testing.T) {
	repo := newRepoForSetTests(t)
	if _, err := repo.GetRepositorySet(context.Background(), "nope"); !errors.Is(err, repoerrors.ErrRepositorySetNotFound) {
		t.Fatalf("GetRepositorySet error = %v, want ErrRepositorySetNotFound", err)
	}
}

func TestCreateRepositorySetRejectsDuplicateNameInWorkspace(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	duplicate := &models.RepositorySet{
		WorkspaceID: "ws-sets",
		Name:        "Full-stack",
		Items:       []models.RepositorySetItem{{RepositoryID: "repo-web"}},
	}
	if err := repo.CreateRepositorySet(ctx, duplicate); err == nil {
		t.Fatal("CreateRepositorySet accepted a duplicate name")
	}

	sets, err := repo.ListRepositorySets(ctx, "ws-sets")
	if err != nil {
		t.Fatalf("ListRepositorySets: %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("rejected duplicate still wrote a row: %d sets", len(sets))
	}
}

func TestCreateRepositorySetAllowsSameNameInAnotherWorkspace(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	seedWorkspace(t, repo, "ws-other")
	seedSetRepository(t, repo, "ws-other", "repo-other")
	other := &models.RepositorySet{
		WorkspaceID: "ws-other",
		Name:        "Full-stack",
		Items:       []models.RepositorySetItem{{RepositoryID: "repo-other"}},
	}
	if err := repo.CreateRepositorySet(ctx, other); err != nil {
		t.Fatalf("CreateRepositorySet in second workspace: %v", err)
	}
}

func TestCreateRepositorySetRejectsRepeatedRepository(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-sets")
	seedSetRepository(t, repo, "ws-sets", "repo-web")

	set := &models.RepositorySet{
		WorkspaceID: "ws-sets",
		Name:        "Twice",
		Items: []models.RepositorySetItem{
			{RepositoryID: "repo-web"},
			{RepositoryID: "repo-web"},
		},
	}
	if err := repo.CreateRepositorySet(ctx, set); err == nil {
		t.Fatal("CreateRepositorySet accepted the same repository twice")
	}
	sets, err := repo.ListRepositorySets(ctx, "ws-sets")
	if err != nil {
		t.Fatalf("ListRepositorySets: %v", err)
	}
	if len(sets) != 0 {
		t.Fatalf("rejected set still wrote a row: %d sets", len(sets))
	}
}

func TestReplaceRepositorySetItemsRewritesOrderContiguously(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	// Reorder and drop one member in a single replace.
	members := []models.RepositorySetItem{
		{RepositoryID: "repo-orders"},
		{RepositoryID: "repo-web"},
	}
	if err := repo.UpdateRepositorySet(ctx, set, &members); err != nil {
		t.Fatalf("UpdateRepositorySet: %v", err)
	}

	loaded, err := repo.GetRepositorySet(ctx, set.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet: %v", err)
	}
	assertRepositoryIDs(t, repositoryIDsOf(loaded), "repo-orders", "repo-web")
	for i, item := range loaded.Items {
		if item.Position != i {
			t.Fatalf("after replace, item %d position = %d", i, item.Position)
		}
	}
}

func TestGetRepositorySetExcludesSoftDeletedRepository(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	// Bypass the delete path that also prunes memberships, so this asserts the
	// read filter on its own rather than the transaction's cleanup.
	_, err := repo.db.Exec(repo.db.Rebind(
		`UPDATE repositories SET deleted_at = CURRENT_TIMESTAMP WHERE id = ?`), "repo-gateway")
	if err != nil {
		t.Fatalf("soft-delete repository: %v", err)
	}

	loaded, err := repo.GetRepositorySet(ctx, set.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet: %v", err)
	}
	assertRepositoryIDs(t, repositoryIDsOf(loaded), "repo-web", "repo-orders")
}

func TestDeleteRepositoryPrunesSetMembership(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	if err := repo.DeleteRepository(ctx, "repo-gateway"); err != nil {
		t.Fatalf("DeleteRepository: %v", err)
	}

	var count int
	err := repo.db.Get(&count, repo.db.Rebind(
		`SELECT COUNT(*) FROM repository_set_items WHERE repository_id = ?`), "repo-gateway")
	if err != nil {
		t.Fatalf("count memberships: %v", err)
	}
	if count != 0 {
		t.Fatalf("membership rows survived repository deletion: %d", count)
	}

	loaded, err := repo.GetRepositorySet(ctx, set.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet after repository delete: %v", err)
	}
	assertRepositoryIDs(t, repositoryIDsOf(loaded), "repo-web", "repo-orders")
}

func TestDeleteRepositorySetRemovesItemsAndKeepsRepositories(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	deleted, err := repo.DeleteRepositorySet(ctx, set.ID)
	if err != nil {
		t.Fatalf("DeleteRepositorySet: %v", err)
	}
	if !deleted {
		t.Fatal("DeleteRepositorySet reported no deletion")
	}

	var items int
	if err := repo.db.Get(&items, repo.db.Rebind(
		`SELECT COUNT(*) FROM repository_set_items WHERE repository_set_id = ?`), set.ID); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if items != 0 {
		t.Fatalf("items survived set deletion: %d", items)
	}
	if _, err := repo.GetRepository(ctx, "repo-web"); err != nil {
		t.Fatalf("set deletion touched a repository: %v", err)
	}

	deleted, err = repo.DeleteRepositorySet(ctx, set.ID)
	if err != nil {
		t.Fatalf("second DeleteRepositorySet: %v", err)
	}
	if deleted {
		t.Fatal("DeleteRepositorySet reported a second deletion")
	}
}

func TestUpdateRepositorySetChangesNameAndDescription(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	set.Name = "Backend only"
	set.Description = "gateway + orders"
	if err := repo.UpdateRepositorySet(ctx, set, nil); err != nil {
		t.Fatalf("UpdateRepositorySet: %v", err)
	}

	loaded, err := repo.GetRepositorySet(ctx, set.ID)
	if err != nil {
		t.Fatalf("GetRepositorySet: %v", err)
	}
	if loaded.Name != "Backend only" || loaded.Description != "gateway + orders" {
		t.Fatalf("loaded set = %+v", loaded)
	}
	// An update that does not supply members must leave them untouched.
	assertRepositoryIDs(t, repositoryIDsOf(loaded), "repo-web", "repo-gateway", "repo-orders")
}

func TestGetRepositorySetByNameIsCaseInsensitiveWithinWorkspace(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	found, err := repo.GetRepositorySetByName(ctx, "ws-sets", "full-STACK")
	if err != nil {
		t.Fatalf("GetRepositorySetByName: %v", err)
	}
	if found == nil || found.ID != set.ID {
		t.Fatalf("GetRepositorySetByName = %+v, want set %s", found, set.ID)
	}

	missing, err := repo.GetRepositorySetByName(ctx, "ws-sets", "nothing")
	if err != nil {
		t.Fatalf("GetRepositorySetByName miss: %v", err)
	}
	if missing != nil {
		t.Fatalf("GetRepositorySetByName returned %+v for an unused name", missing)
	}
}

func TestListRepositorySetsScopesToWorkspaceAndResolvesItems(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}
	second := &models.RepositorySet{
		WorkspaceID: "ws-sets",
		Name:        "Backend",
		Items: []models.RepositorySetItem{
			{RepositoryID: "repo-gateway"},
			{RepositoryID: "repo-orders"},
		},
	}
	if err := repo.CreateRepositorySet(ctx, second); err != nil {
		t.Fatalf("CreateRepositorySet second: %v", err)
	}
	seedWorkspace(t, repo, "ws-other")
	seedSetRepository(t, repo, "ws-other", "repo-other")
	if err := repo.CreateRepositorySet(ctx, &models.RepositorySet{
		WorkspaceID: "ws-other",
		Name:        "Other",
		Items:       []models.RepositorySetItem{{RepositoryID: "repo-other"}},
	}); err != nil {
		t.Fatalf("CreateRepositorySet other workspace: %v", err)
	}

	sets, err := repo.ListRepositorySets(ctx, "ws-sets")
	if err != nil {
		t.Fatalf("ListRepositorySets: %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("ListRepositorySets returned %d sets, want 2", len(sets))
	}
	byName := map[string][]string{}
	for _, entry := range sets {
		byName[entry.Name] = repositoryIDsOf(entry)
	}
	assertRepositoryIDs(t, byName["Full-stack"], "repo-web", "repo-gateway", "repo-orders")
	assertRepositoryIDs(t, byName["Backend"], "repo-gateway", "repo-orders")
}

func TestDeleteWorkspaceCascadesRepositorySets(t *testing.T) {
	repo := newRepoForSetTests(t)
	ctx := context.Background()
	set := setFixture(t, repo)
	if err := repo.CreateRepositorySet(ctx, set); err != nil {
		t.Fatalf("CreateRepositorySet: %v", err)
	}

	if err := repo.DeleteWorkspace(ctx, "ws-sets"); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}

	var sets, items int
	if err := repo.db.Get(&sets, `SELECT COUNT(*) FROM repository_sets`); err != nil {
		t.Fatalf("count sets: %v", err)
	}
	if err := repo.db.Get(&items, `SELECT COUNT(*) FROM repository_set_items`); err != nil {
		t.Fatalf("count items: %v", err)
	}
	if sets != 0 || items != 0 {
		t.Fatalf("workspace deletion left %d sets and %d items", sets, items)
	}
}
