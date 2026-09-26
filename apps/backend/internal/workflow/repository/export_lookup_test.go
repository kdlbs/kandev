package repository

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

// AC-29: the automations YAML export opens one read transaction spanning
// several stores. GetStepTx is GetStep's counterpart that accepts the
// caller's *sqlx.Tx instead of opening its own, and reports a three-outcome
// (value, found, err) result so a missing step is distinguishable from a
// lookup failure without string-matching an error.

func TestGetStepTx_FoundAndMissing(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	step := &models.WorkflowStep{WorkflowID: "wf-test", Name: "In Progress", Position: 0}
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("CreateStep: %v", err)
	}

	tx, err := repo.ro.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	defer func() { _ = tx.Rollback() }()

	got, found, err := repo.GetStepTx(ctx, tx, step.ID)
	if err != nil {
		t.Fatalf("GetStepTx(%s): %v", step.ID, err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if got == nil || got.ID != step.ID {
		t.Errorf("step = %+v, want ID %s", got, step.ID)
	}

	missing, found, err := repo.GetStepTx(ctx, tx, "does-not-exist")
	if err != nil {
		t.Fatalf("GetStepTx(missing): %v", err)
	}
	if found {
		t.Error("found = true, want false")
	}
	if missing != nil {
		t.Errorf("step = %+v, want nil", missing)
	}
}
