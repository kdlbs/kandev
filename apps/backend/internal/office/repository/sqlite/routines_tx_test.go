package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// fullRoutine builds a Routine with every column a config sync write cares
// about set to a distinct non-zero value.
func fullRoutine(id, workspaceID, name string) *models.Routine {
	return &models.Routine{
		ID:                id,
		WorkspaceID:       workspaceID,
		Name:              name,
		Description:       "run nightly",
		TaskTemplate:      `{"title":"Nightly build"}`,
		Status:            "active",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
}

// TestCreateRoutineTx_CommitPersists verifies config sync can write a new
// routine inside a caller-owned transaction and have it visible once the
// transaction commits (AC-OFFICE-CONFIG-SYNC-003.14's per-entity atomicity).
func TestCreateRoutineTx_CommitPersists(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	want := fullRoutine("routine-tx-1", "ws-1", "Nightly Build")
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.CreateRoutineTx(ctx, tx, want); err != nil {
		t.Fatalf("CreateRoutineTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := repo.GetRoutine(ctx, "routine-tx-1")
	if err != nil {
		t.Fatalf("GetRoutine: %v", err)
	}
	if got.Name != want.Name || got.Description != want.Description ||
		got.TaskTemplate != want.TaskTemplate || got.ConcurrencyPolicy != want.ConcurrencyPolicy {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// CreateRoutineTx must apply the same defaulting as CreateRoutine.
	if got.CatchUpPolicy != "enqueue_missed_with_cap" || got.CatchUpMax != 25 {
		t.Errorf("catch-up defaults = (%q, %d), want (enqueue_missed_with_cap, 25)", got.CatchUpPolicy, got.CatchUpMax)
	}
}

// TestCreateRoutineTx_RollbackDiscards verifies a rolled-back transaction
// leaves no row behind.
func TestCreateRoutineTx_RollbackDiscards(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := fullRoutine("routine-tx-2", "ws-1", "Weekly Report")
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.CreateRoutineTx(ctx, tx, routine); err != nil {
		t.Fatalf("CreateRoutineTx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if _, err := repo.GetRoutine(ctx, "routine-tx-2"); err == nil {
		t.Fatal("GetRoutine() error = nil, want not-found after rollback")
	}
}

// TestUpdateRoutineConfigFieldsTx_CommitPersists verifies the
// transaction-scoped config-field writer used by reconcile's "Existing"
// apply case.
func TestUpdateRoutineConfigFieldsTx_CommitPersists(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := fullRoutine("routine-tx-3", "ws-1", "Cleanup")
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("CreateRoutine: %v", err)
	}

	fields := sqlite.RoutineConfigFields{
		Description:       "new description",
		TaskTemplate:      `{"title":"Updated"}`,
		ConcurrencyPolicy: models.RoutineConcurrencyPolicy("queue"),
	}
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.UpdateRoutineConfigFieldsTx(ctx, tx, "routine-tx-3", fields); err != nil {
		t.Fatalf("UpdateRoutineConfigFieldsTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	got, err := repo.GetRoutine(ctx, "routine-tx-3")
	if err != nil {
		t.Fatalf("GetRoutine: %v", err)
	}
	if got.Description != fields.Description || got.TaskTemplate != fields.TaskTemplate ||
		got.ConcurrencyPolicy != fields.ConcurrencyPolicy {
		t.Errorf("got %+v, want owned fields %+v", got, fields)
	}
	// status is not config-owned; it must survive untouched.
	if got.Status != routine.Status {
		t.Errorf("Status = %q, want unchanged %q", got.Status, routine.Status)
	}
}

// TestUpdateRoutineConfigFieldsTx_RollbackDiscards verifies a rolled-back
// config-field update leaves the prior row untouched.
func TestUpdateRoutineConfigFieldsTx_RollbackDiscards(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := fullRoutine("routine-tx-4", "ws-1", "Backups")
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("CreateRoutine: %v", err)
	}

	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	fields := sqlite.RoutineConfigFields{Description: "changed"}
	if err := repo.UpdateRoutineConfigFieldsTx(ctx, tx, "routine-tx-4", fields); err != nil {
		t.Fatalf("UpdateRoutineConfigFieldsTx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	got, err := repo.GetRoutine(ctx, "routine-tx-4")
	if err != nil {
		t.Fatalf("GetRoutine: %v", err)
	}
	if got.Description != routine.Description {
		t.Errorf("Description = %q, want unchanged %q after rollback", got.Description, routine.Description)
	}
}

// TestDeleteRoutineTx_CommitRemoves verifies the transaction-scoped delete
// used by reconcile's "Removed upstream" apply case.
func TestDeleteRoutineTx_CommitRemoves(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := fullRoutine("routine-tx-5", "ws-1", "Old Routine")
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("CreateRoutine: %v", err)
	}

	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.DeleteRoutineTx(ctx, tx, "routine-tx-5"); err != nil {
		t.Fatalf("DeleteRoutineTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if _, err := repo.GetRoutine(ctx, "routine-tx-5"); err == nil {
		t.Fatal("GetRoutine() error = nil, want not-found after delete commit")
	}
}

// TestDeleteRoutineTx_RollbackKeepsRow verifies a rolled-back delete leaves
// the row intact.
func TestDeleteRoutineTx_RollbackKeepsRow(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := fullRoutine("routine-tx-6", "ws-1", "Kept Routine")
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("CreateRoutine: %v", err)
	}

	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	if err := repo.DeleteRoutineTx(ctx, tx, "routine-tx-6"); err != nil {
		t.Fatalf("DeleteRoutineTx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if _, err := repo.GetRoutine(ctx, "routine-tx-6"); err != nil {
		t.Fatalf("GetRoutine() error = %v, want row to survive rollback", err)
	}
}
