package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresWorktreeBranchTemplateMigration(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedWorkspaceForWorktreeTemplateMigration(t, repo, "ws-pg-worktree-template")

	custom := &models.Repository{
		ID:                     "repo-pg-worktree-template-custom",
		WorkspaceID:            "ws-pg-worktree-template",
		Name:                   "custom-template",
		SourceType:             "local",
		WorktreeBranchPrefix:   "feature/",
		WorktreeBranchTemplate: "bugfix/{ticket}-{title}",
	}
	if err := repo.CreateRepository(ctx, custom); err != nil {
		t.Fatalf("create custom repository: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay custom template migration: %v", err)
	}
	assertPostgresWorktreeBranchTemplate(t, db, custom.ID, custom.WorktreeBranchTemplate)

	legacyRepositories := []struct {
		id           string
		prefix       string
		wantTemplate string
	}{
		{id: "repo-pg-worktree-template-fix", prefix: "  fix/ ", wantTemplate: "fix/{title}-{suffix}"},
		{id: "repo-pg-worktree-template-default", prefix: "  ", wantTemplate: "feature/{title}-{suffix}"},
	}
	for _, legacy := range legacyRepositories {
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID:                   legacy.id,
			WorkspaceID:          "ws-pg-worktree-template",
			Name:                 legacy.id,
			SourceType:           "local",
			WorktreeBranchPrefix: legacy.prefix,
		}); err != nil {
			t.Fatalf("create legacy repository %q: %v", legacy.id, err)
		}
	}

	if _, err := db.Exec(`ALTER TABLE repositories DROP COLUMN worktree_branch_template`); err != nil {
		t.Fatalf("remove template column from legacy schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("migrate legacy postgres schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay legacy postgres migration: %v", err)
	}

	for _, legacy := range legacyRepositories {
		assertPostgresWorktreeBranchTemplate(t, db, legacy.id, legacy.wantTemplate)
	}
}

func seedWorkspaceForWorktreeTemplateMigration(t *testing.T, repo *Repository, id string) {
	t.Helper()
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: id, Name: id}); err != nil {
		t.Fatalf("seed workspace %s: %v", id, err)
	}
}

func assertPostgresWorktreeBranchTemplate(t *testing.T, db interface {
	Get(dest interface{}, query string, args ...interface{}) error
}, id, want string) {
	t.Helper()
	var got string
	if err := db.Get(&got, `SELECT worktree_branch_template FROM repositories WHERE id = $1`, id); err != nil {
		t.Fatalf("read postgres template %q: %v", id, err)
	}
	if got != want {
		t.Fatalf("postgres template %q = %q, want %q", id, got, want)
	}
}
