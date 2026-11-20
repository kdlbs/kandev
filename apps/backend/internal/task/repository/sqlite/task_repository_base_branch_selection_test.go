package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

func insertBaseBranchSelectionRow(
	t *testing.T,
	db *sqlx.DB,
	id, taskID, repositoryID, baseBranch string,
	metadata map[string]interface{},
) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_repositories
			(id, task_id, repository_id, base_branch, checkout_branch, position, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), id, taskID, repositoryID, baseBranch, "", 0, mustJSON(t, metadata), now, now); err != nil {
		t.Fatalf("insert task repository %s: %v", id, err)
	}
}

func TestBaseBranchSelection_Persistence(t *testing.T) {
	repo, db := newTaskExternalIDTestRepo(t)
	ctx := context.Background()
	id := "task-repository-base-selection"
	now := time.Now().UTC()
	seedComparisonTargetAttachment(t, repo, "task-base-selection", "repository-base-selection", now)
	selection := models.BaseBranchSelection{
		Version: models.BaseBranchSelectionVersion,
		Branch:  "main",
		Source:  models.BaseBranchSelectionSourceRepositoryDefault,
	}
	insertBaseBranchSelectionRow(t, db, id, "task-base-selection", "repository-base-selection", "main",
		map[string]interface{}{
			models.BaseBranchSelectionMetadataKey: selection,
			"unrelated":                           "keep",
		})

	loaded, err := repo.GetTaskRepository(ctx, id)
	if err != nil {
		t.Fatalf("reload task repository: %v", err)
	}
	got, ok, err := models.LoadBaseBranchSelection(loaded.Metadata)
	if err != nil || !ok || got != selection {
		t.Fatalf("loaded selection = %#v, present=%v, err=%v", got, ok, err)
	}
	if loaded.Metadata["unrelated"] != "keep" {
		t.Fatalf("unrelated metadata changed: %#v", loaded.Metadata)
	}
	if policy := models.ResolveBaseBranchFallbackPolicy(loaded.Metadata, loaded.BaseBranch); !policy.PermitsRepositoryDefaultSubstitution() {
		t.Fatalf("round-tripped policy = %q, want repository default", policy)
	}

	t.Run("manual same-name selection becomes explicit", func(t *testing.T) {
		updated, changed, err := repo.UpdateTaskRepositoryBaseBranchAndClearComparisonTarget(ctx, id, "main", true)
		if err != nil {
			t.Fatalf("manual update: %v", err)
		}
		if !changed {
			t.Fatal("manual same-name selection reported no change")
		}
		requireStoredSelection(t, updated.Metadata, "main", models.BaseBranchSelectionSourceExplicit)
		if updated.Metadata["unrelated"] != "keep" {
			t.Fatalf("manual update dropped unrelated metadata: %#v", updated.Metadata)
		}
	})

	t.Run("system update rebinds a matching default binding", func(t *testing.T) {
		seedBaseBranchSelectionAttachment(t, repo, db, "task-repository-system-rebind", models.BaseBranchSelection{
			Version: models.BaseBranchSelectionVersion,
			Branch:  "main",
			Source:  models.BaseBranchSelectionSourceRepositoryDefault,
		})
		updated, changed, err := repo.UpdateTaskRepositoryBaseBranchAndClearComparisonTarget(
			ctx, "task-repository-system-rebind", "trunk", false,
		)
		if err != nil {
			t.Fatalf("system update: %v", err)
		}
		if !changed {
			t.Fatal("system fallback reported no change")
		}
		requireStoredSelection(t, updated.Metadata, "trunk", models.BaseBranchSelectionSourceRepositoryDefault)
	})

	t.Run("system update never grants a stale binding permission", func(t *testing.T) {
		seedBaseBranchSelectionAttachment(t, repo, db, "task-repository-system-stale", models.BaseBranchSelection{
			Version: models.BaseBranchSelectionVersion,
			Branch:  "release",
			Source:  models.BaseBranchSelectionSourceInherited,
		})
		updated, changed, err := repo.UpdateTaskRepositoryBaseBranchAndClearComparisonTarget(
			ctx, "task-repository-system-stale", "trunk", false,
		)
		if err != nil {
			t.Fatalf("system update: %v", err)
		}
		if !changed {
			t.Fatal("stale system update reported no change")
		}
		requireStoredSelection(t, updated.Metadata, "trunk", models.BaseBranchSelectionSourceExplicit)
	})

	t.Run("system update without provenance stays strict", func(t *testing.T) {
		seedBaseBranchSelectionAttachment(t, repo, db, "task-repository-system-legacy", models.BaseBranchSelection{})
		updated, changed, err := repo.UpdateTaskRepositoryBaseBranchAndClearComparisonTarget(
			ctx, "task-repository-system-legacy", "trunk", false,
		)
		if err != nil {
			t.Fatalf("system update: %v", err)
		}
		if !changed {
			t.Fatal("legacy system update reported no change")
		}
		requireStoredSelection(t, updated.Metadata, "trunk", models.BaseBranchSelectionSourceExplicit)
	})
}

func seedBaseBranchSelectionAttachment(
	t *testing.T,
	repo *Repository,
	db *sqlx.DB,
	id string,
	selection models.BaseBranchSelection,
) {
	t.Helper()
	taskID := "task-" + id
	repositoryID := "repository-" + id
	seedComparisonTargetAttachment(t, repo, taskID, repositoryID, time.Now().UTC())
	metadata := map[string]interface{}{}
	if selection.Version != 0 {
		metadata[models.BaseBranchSelectionMetadataKey] = selection
	}
	insertBaseBranchSelectionRow(t, db, id, taskID, repositoryID, "main", metadata)
}

func requireStoredSelection(
	t *testing.T, metadata map[string]interface{}, branch string, source models.BaseBranchSelectionSource,
) {
	t.Helper()
	selection, ok, err := models.LoadBaseBranchSelection(metadata)
	if err != nil {
		t.Fatalf("load base branch selection: %v", err)
	}
	if !ok {
		t.Fatalf("metadata is missing a base branch selection: %#v", metadata)
	}
	if selection.Branch != branch || selection.Source != source {
		t.Fatalf("selection = %#v, want branch %q source %q", selection, branch, source)
	}
}
