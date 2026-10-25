package sqlite_test

import (
	"context"
	"testing"
)

func TestGetWorkspaceBudgetDefault_NoRowReturnsNotFound(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	limitSubcents, found, err := repo.GetWorkspaceBudgetDefault(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceBudgetDefault: %v", err)
	}
	if found {
		t.Errorf("found = true, want false: no row has been written for this workspace")
	}
	if limitSubcents != 0 {
		t.Errorf("limitSubcents = %d, want 0 when not found", limitSubcents)
	}
}

func TestSetWorkspaceBudgetDefault_ThenGetReturnsWrittenValue(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.SetWorkspaceBudgetDefault(ctx, "ws-1", 750_000); err != nil {
		t.Fatalf("SetWorkspaceBudgetDefault: %v", err)
	}

	limitSubcents, found, err := repo.GetWorkspaceBudgetDefault(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceBudgetDefault: %v", err)
	}
	if !found {
		t.Error("found = false, want true after a write")
	}
	if limitSubcents != 750_000 {
		t.Errorf("limitSubcents = %d, want 750000", limitSubcents)
	}
}

// TestSetWorkspaceBudgetDefault_SecondWriteWins pins AC-OFFICE-BUDGET-003.10:
// the upsert is a single statement, so the last write in commit order is the
// effective value, and no intermediate state leaves the workspace with no
// effective row.
func TestSetWorkspaceBudgetDefault_SecondWriteWins(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.SetWorkspaceBudgetDefault(ctx, "ws-1", 100_000); err != nil {
		t.Fatalf("first SetWorkspaceBudgetDefault: %v", err)
	}
	if err := repo.SetWorkspaceBudgetDefault(ctx, "ws-1", 200_000); err != nil {
		t.Fatalf("second SetWorkspaceBudgetDefault: %v", err)
	}

	limitSubcents, found, err := repo.GetWorkspaceBudgetDefault(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceBudgetDefault: %v", err)
	}
	if !found || limitSubcents != 200_000 {
		t.Errorf("limitSubcents = %d, found = %v, want 200000, true", limitSubcents, found)
	}
}

func TestSetWorkspaceBudgetDefault_ScopedPerWorkspace(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.SetWorkspaceBudgetDefault(ctx, "ws-1", 100_000); err != nil {
		t.Fatalf("SetWorkspaceBudgetDefault ws-1: %v", err)
	}

	_, found, err := repo.GetWorkspaceBudgetDefault(ctx, "ws-2")
	if err != nil {
		t.Fatalf("GetWorkspaceBudgetDefault ws-2: %v", err)
	}
	if found {
		t.Error("found = true for ws-2, want false: a write to ws-1 must not leak")
	}
}
