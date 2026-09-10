package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// TestCreateProjectTx_CommitPersists verifies config sync can write a new
// project inside a caller-owned transaction and have it visible once the
// transaction commits (AC-OFFICE-CONFIG-SYNC-003.14's per-entity atomicity).
func TestCreateProjectTx_CommitPersists(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	want := fullProject("proj-tx-1", "ws-1", "Website")
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.CreateProjectTx(ctx, tx, want); err != nil {
		t.Fatalf("CreateProjectTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := repo.GetProject(ctx, "proj-tx-1")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	assertProjectEquals(t, got, want)
}

// TestCreateProjectTx_RollbackDiscards verifies a rolled-back transaction
// leaves no row behind.
func TestCreateProjectTx_RollbackDiscards(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	project := fullProject("proj-tx-2", "ws-1", "Mobile App")
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.CreateProjectTx(ctx, tx, project); err != nil {
		t.Fatalf("CreateProjectTx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if _, err := repo.GetProject(ctx, "proj-tx-2"); err == nil {
		t.Fatal("GetProject() error = nil, want not-found after rollback")
	}
}

// TestUpdateProjectConfigFieldsTx_CommitPersists verifies the
// transaction-scoped config-field writer used by reconcile's "Existing"
// apply case.
func TestUpdateProjectConfigFieldsTx_CommitPersists(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	project := fullProject("proj-tx-3", "ws-1", "Backend")
	if err := repo.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	fields := sqlite.ProjectConfigFields{
		Description:    "new description",
		Color:          "#00ff00",
		BudgetCents:    777,
		Repositories:   `["kdlbs/new"]`,
		ExecutorConfig: `{"type":"local_pc"}`,
	}
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.UpdateProjectConfigFieldsTx(ctx, tx, "proj-tx-3", fields); err != nil {
		t.Fatalf("UpdateProjectConfigFieldsTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := repo.GetProject(ctx, "proj-tx-3")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.Description != fields.Description || got.Color != fields.Color ||
		got.BudgetCents != fields.BudgetCents || got.Repositories != fields.Repositories ||
		got.ExecutorConfig != fields.ExecutorConfig {
		t.Errorf("got %+v, want owned fields %+v", got, fields)
	}
	// status and lead_agent_profile_id are not config-owned; they must
	// survive untouched.
	if got.Status != project.Status || got.LeadAgentProfileID != project.LeadAgentProfileID {
		t.Errorf("non-owned fields changed: status=%q lead=%q", got.Status, got.LeadAgentProfileID)
	}
}

// TestUpdateProjectConfigFieldsTx_RollbackDiscards verifies a rolled-back
// config-field update leaves the prior row untouched.
func TestUpdateProjectConfigFieldsTx_RollbackDiscards(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	project := fullProject("proj-tx-4", "ws-1", "Data Platform")
	if err := repo.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	fields := sqlite.ProjectConfigFields{Description: "changed"}
	if err := repo.UpdateProjectConfigFieldsTx(ctx, tx, "proj-tx-4", fields); err != nil {
		t.Fatalf("UpdateProjectConfigFieldsTx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	got, err := repo.GetProject(ctx, "proj-tx-4")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.Description != project.Description {
		t.Errorf("Description = %q, want unchanged %q after rollback", got.Description, project.Description)
	}
}

// TestDeleteProjectTx_CommitRemoves verifies the transaction-scoped delete
// used by reconcile's "Removed upstream" apply case.
func TestDeleteProjectTx_CommitRemoves(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	project := fullProject("proj-tx-5", "ws-1", "Legacy")
	if err := repo.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.DeleteProjectTx(ctx, tx, "proj-tx-5"); err != nil {
		t.Fatalf("DeleteProjectTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if _, err := repo.GetProject(ctx, "proj-tx-5"); err == nil {
		t.Fatal("GetProject() error = nil, want not-found after delete commit")
	}
}

// TestDeleteProjectTx_RollbackKeepsRow verifies a rolled-back delete leaves
// the row intact.
func TestDeleteProjectTx_RollbackKeepsRow(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	project := fullProject("proj-tx-6", "ws-1", "Archive")
	if err := repo.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.DeleteProjectTx(ctx, tx, "proj-tx-6"); err != nil {
		t.Fatalf("DeleteProjectTx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if _, err := repo.GetProject(ctx, "proj-tx-6"); err != nil {
		t.Fatalf("GetProject() error = %v, want row to survive rollback", err)
	}
}
