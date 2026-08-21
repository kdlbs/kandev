package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// AC-29: automations export needs tx-accepting reads of workflows,
// repositories, and executor_profiles so all four store reads share one
// snapshot. These mirror GetWorkflow/GetRepository/GetExecutorProfile but
// accept the caller's *sqlx.Tx instead of opening their own, and report a
// three-outcome (value, found, err) result instead of a not-found error -
// AC-19 needs to tell "row missing" apart from "lookup failed" without
// string-matching an error.

func TestGetWorkflowTx_FoundAndMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-1")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Kanban"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	tx, err := repo.ro.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	workflow, found, err := repo.GetWorkflowTx(ctx, tx, "wf-1")
	if err != nil {
		t.Fatalf("GetWorkflowTx(wf-1): %v", err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if workflow == nil || workflow.ID != "wf-1" {
		t.Errorf("workflow = %+v, want ID wf-1", workflow)
	}

	missing, found, err := repo.GetWorkflowTx(ctx, tx, "does-not-exist")
	if err != nil {
		t.Fatalf("GetWorkflowTx(missing): %v", err)
	}
	if found {
		t.Error("found = true, want false")
	}
	if missing != nil {
		t.Errorf("workflow = %+v, want nil", missing)
	}
}

func TestGetRepositoryTx_FoundAndMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-1")
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-1", WorkspaceID: "ws-1", Name: "kandev"}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	tx, err := repo.ro.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	got, found, err := repo.GetRepositoryTx(ctx, tx, "repo-1")
	if err != nil {
		t.Fatalf("GetRepositoryTx(repo-1): %v", err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if got == nil || got.ID != "repo-1" {
		t.Errorf("repository = %+v, want ID repo-1", got)
	}

	missing, found, err := repo.GetRepositoryTx(ctx, tx, "does-not-exist")
	if err != nil {
		t.Fatalf("GetRepositoryTx(missing): %v", err)
	}
	if found {
		t.Error("found = true, want false")
	}
	if missing != nil {
		t.Errorf("repository = %+v, want nil", missing)
	}
}

func TestGetExecutorProfileTx_FoundAndMissing(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedExecutorForProfiles(t, repo, "exec-1")
	if err := repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{ID: "profile-1", ExecutorID: "exec-1", Name: "Worktree"}); err != nil {
		t.Fatalf("CreateExecutorProfile: %v", err)
	}

	tx, err := repo.ro.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	got, found, err := repo.GetExecutorProfileTx(ctx, tx, "profile-1")
	if err != nil {
		t.Fatalf("GetExecutorProfileTx(profile-1): %v", err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if got == nil || got.ID != "profile-1" {
		t.Errorf("profile = %+v, want ID profile-1", got)
	}

	missing, found, err := repo.GetExecutorProfileTx(ctx, tx, "does-not-exist")
	if err != nil {
		t.Fatalf("GetExecutorProfileTx(missing): %v", err)
	}
	if found {
		t.Error("found = true, want false")
	}
	if missing != nil {
		t.Errorf("profile = %+v, want nil", missing)
	}
}
