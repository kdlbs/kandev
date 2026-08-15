package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestWorktreeBranchTemplateMigrationPreservesCustomValue(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-worktree-template-replay")

	custom := &models.Repository{
		ID:                     "repo-worktree-template-replay",
		WorkspaceID:            "ws-worktree-template-replay",
		Name:                   "custom-template",
		SourceType:             "local",
		WorktreeBranchPrefix:   "feature/",
		WorktreeBranchTemplate: "bugfix/{ticket}-{title}",
	}
	if err := repo.CreateRepository(ctx, custom); err != nil {
		t.Fatalf("create repository: %v", err)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}

	got, err := repo.GetRepository(ctx, custom.ID)
	if err != nil {
		t.Fatalf("get repository after replay: %v", err)
	}
	if got.WorktreeBranchTemplate != custom.WorktreeBranchTemplate {
		t.Fatalf("replayed template = %q, want %q", got.WorktreeBranchTemplate, custom.WorktreeBranchTemplate)
	}
}

func TestWorktreeBranchTemplateMigrationBackfillsLegacyPrefix(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-worktree-template-legacy")

	legacyRepositories := []struct {
		id           string
		prefix       string
		wantTemplate string
	}{
		{
			id:           "repo-worktree-template-fix",
			prefix:       "  fix/ ",
			wantTemplate: "fix/{title}-{suffix}",
		},
		{
			id:           "repo-worktree-template-default",
			prefix:       "  ",
			wantTemplate: "feature/{title}-{suffix}",
		},
	}
	for _, legacy := range legacyRepositories {
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID:                   legacy.id,
			WorkspaceID:          "ws-worktree-template-legacy",
			Name:                 legacy.id,
			SourceType:           "local",
			WorktreeBranchPrefix: legacy.prefix,
		}); err != nil {
			t.Fatalf("create legacy repository %q: %v", legacy.id, err)
		}
	}

	if _, err := repo.db.Exec(`ALTER TABLE repositories DROP COLUMN worktree_branch_template`); err != nil {
		t.Fatalf("remove template column from legacy schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay legacy migration: %v", err)
	}

	for _, legacy := range legacyRepositories {
		var got string
		if err := repo.db.QueryRow(`SELECT worktree_branch_template FROM repositories WHERE id = ?`, legacy.id).Scan(&got); err != nil {
			t.Fatalf("read migrated template %q: %v", legacy.id, err)
		}
		if got != legacy.wantTemplate {
			t.Errorf("migrated template %q = %q, want %q", legacy.id, got, legacy.wantTemplate)
		}
	}
}
