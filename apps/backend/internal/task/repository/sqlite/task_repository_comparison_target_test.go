package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func comparisonTargetForRepositoryTest() models.ComparisonTarget {
	return models.ComparisonTarget{
		Version:      models.ComparisonTargetVersion,
		Provider:     models.ComparisonTargetProviderGitHub,
		Kind:         models.ComparisonTargetKindPullRequest,
		Number:       1154,
		HeadBranch:   "feature/cursor-cost",
		TargetBranch: "main",
		HeadRepository: models.ComparisonTargetRepository{
			Host:       "github.com",
			Path:       "contributor/widget",
			ProviderID: "head-42",
			RemoteURL:  "https://github.com/contributor/widget.git",
		},
		TargetRepository: models.ComparisonTargetRepository{
			Host:       "github.com",
			Path:       "upstream/widget",
			ProviderID: "base-99",
			RemoteURL:  "https://github.com/upstream/widget.git",
		},
	}
}

func TestTaskRepositoryComparisonTargetMutationsAreAtomic(t *testing.T) {
	repo, db := newTaskExternalIDTestRepo(t)
	ctx := context.Background()
	id := "task-repository-comparison-target"
	now := time.Now().UTC()
	seedComparisonTargetAttachment(t, repo, "task-comparison-target", "repository-comparison-target", now)
	metadata := map[string]interface{}{"unrelated": "keep"}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_repositories
			(id, task_id, repository_id, base_branch, checkout_branch, position, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), id, "task-comparison-target", "repository-comparison-target", "main", "feature/cursor-cost", 0, mustJSON(t, metadata), now, now); err != nil {
		t.Fatalf("insert task repository: %v", err)
	}

	target := comparisonTargetForRepositoryTest()
	updated, changed, err := repo.UpdateTaskRepositoryComparisonTarget(ctx, id, &target, nil)
	if err != nil {
		t.Fatalf("set comparison target: %v", err)
	}
	if !changed || updated == nil {
		t.Fatalf("set comparison target changed=%v row=%#v", changed, updated)
	}
	loaded, err := repo.GetTaskRepository(ctx, id)
	if err != nil {
		t.Fatalf("reload task repository: %v", err)
	}
	got, ok, err := models.LoadComparisonTarget(loaded.Metadata)
	if err != nil || !ok || !got.Equal(target) {
		t.Fatalf("loaded comparison target = %#v, present=%v, err=%v", got, ok, err)
	}
	if loaded.Metadata["unrelated"] != "keep" {
		t.Fatalf("unrelated metadata changed: %#v", loaded.Metadata)
	}

	other := target
	other.Number++
	if _, changed, err := repo.UpdateTaskRepositoryComparisonTarget(ctx, id, nil, &other); err != nil || changed {
		t.Fatalf("source-aware removal = changed=%v err=%v, want no-op", changed, err)
	}
	if _, changed, err := repo.UpdateTaskRepositoryComparisonTarget(ctx, id, nil, &target); err != nil || !changed {
		t.Fatalf("owned removal = changed=%v err=%v, want change", changed, err)
	}
	loaded, err = repo.GetTaskRepository(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := models.LoadComparisonTarget(loaded.Metadata); err != nil || ok {
		t.Fatalf("comparison target remains after owned removal: present=%v err=%v", ok, err)
	}
}

func TestTaskRepositoryManualBaseBranchClearsTargetEvenWhenBranchIsUnchanged(t *testing.T) {
	repo, db := newTaskExternalIDTestRepo(t)
	ctx := context.Background()
	id := "task-repository-manual-comparison"
	now := time.Now().UTC()
	seedComparisonTargetAttachment(t, repo, "task-manual-comparison", "repository-manual-comparison", now)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_repositories
			(id, task_id, repository_id, base_branch, checkout_branch, position, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), id, "task-manual-comparison", "repository-manual-comparison", "main", "feature/cursor-cost", 0, `{}`, now, now); err != nil {
		t.Fatalf("insert task repository: %v", err)
	}
	target := comparisonTargetForRepositoryTest()
	if _, changed, err := repo.UpdateTaskRepositoryComparisonTarget(ctx, id, &target, nil); err != nil || !changed {
		t.Fatalf("seed comparison target: changed=%v err=%v", changed, err)
	}

	updated, changed, err := repo.UpdateTaskRepositoryBaseBranchAndClearComparisonTarget(ctx, id, "main")
	if err != nil {
		t.Fatalf("manual same-branch update: %v", err)
	}
	if !changed || updated.BaseBranch != "main" {
		t.Fatalf("manual same-branch update = changed=%v row=%#v", changed, updated)
	}
	if _, ok, err := models.LoadComparisonTarget(updated.Metadata); err != nil || ok {
		t.Fatalf("comparison target remains after manual update: present=%v err=%v", ok, err)
	}
	if updated.Metadata["unrelated"] != nil {
		t.Fatalf("unexpected metadata in empty row: %#v", updated.Metadata)
	}
	if _, changed, err := repo.UpdateTaskRepositoryBaseBranchAndClearComparisonTarget(ctx, id, "main"); err != nil || changed {
		t.Fatalf("repeated manual same-branch update = changed=%v err=%v, want no-op", changed, err)
	}
}

func mustJSON(t *testing.T, value interface{}) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return string(data)
}

func seedComparisonTargetAttachment(t *testing.T, repo *Repository, taskID, repositoryID string, now time.Time) {
	t.Helper()
	workspaceID := "workspace-" + taskID
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`), workspaceID, workspaceID, now, now); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO repositories (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
	`), repositoryID, workspaceID, repositoryID, now, now); err != nil {
		t.Fatalf("insert repository: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
	`), taskID, workspaceID, taskID, now, now); err != nil {
		t.Fatalf("insert task: %v", err)
	}
}
