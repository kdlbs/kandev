package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func createTestRoutine(t *testing.T, svc interface {
	CreateRoutine(ctx context.Context, r *models.Routine) error
}, name, policy string) *models.Routine {
	t.Helper()
	r := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              name,
		TaskTemplate:      `{"title":"{{name}} - {{date}}","description":"Run for {{date}}"}`,
		Status:            "active",
		ConcurrencyPolicy: policy,
		Variables:         `{"name":{"default":"Daily Check"}}`,
	}
	if err := svc.CreateRoutine(context.Background(), r); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	return r
}

func TestFireManual_Basic(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	routine := createTestRoutine(t, svc, "Manual Test", "always_create")
	run, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "Security Scan"})
	if err != nil {
		t.Fatalf("fire manual: %v", err)
	}
	if run.Status != "task_created" {
		t.Errorf("status = %q, want task_created", run.Status)
	}
	if run.Source != "manual" {
		t.Errorf("source = %q, want manual", run.Source)
	}
	if run.DispatchFingerprint == "" {
		t.Error("expected fingerprint to be set")
	}
}

func TestDispatch_SkipIfActive(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	routine := createTestRoutine(t, svc, "Skip Test", "skip_if_active")

	// First run should succeed.
	run1, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if run1.Status != "task_created" {
		t.Errorf("first run status = %q, want task_created", run1.Status)
	}

	// Second run with same fingerprint should be skipped.
	run2, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if run2.Status != "skipped" {
		t.Errorf("second run status = %q, want skipped", run2.Status)
	}
}

func TestDispatch_CoalesceIfActive(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	routine := createTestRoutine(t, svc, "Coalesce Test", "coalesce_if_active")

	run1, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if run1.Status != "task_created" {
		t.Errorf("first run status = %q, want task_created", run1.Status)
	}

	run2, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if run2.Status != "coalesced" {
		t.Errorf("second run status = %q, want coalesced", run2.Status)
	}
	if run2.CoalescedIntoRunID != run1.ID {
		t.Errorf("coalesced_into = %q, want %q", run2.CoalescedIntoRunID, run1.ID)
	}
}

func TestDispatch_AlwaysCreate(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	routine := createTestRoutine(t, svc, "Always Test", "always_create")

	run1, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	run2, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}

	if run1.Status != "task_created" || run2.Status != "task_created" {
		t.Errorf("both runs should be task_created, got %q and %q", run1.Status, run2.Status)
	}
}

func TestDispatch_DifferentVarsNotSkipped(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	routine := createTestRoutine(t, svc, "Diff Vars", "skip_if_active")

	run1, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "A"})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if run1.Status != "task_created" {
		t.Errorf("first run status = %q, want task_created", run1.Status)
	}

	// Different variable -> different fingerprint -> not skipped.
	run2, err := svc.FireManual(ctx, routine.ID, map[string]string{"name": "B"})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if run2.Status != "task_created" {
		t.Errorf("second run with different vars should be task_created, got %q", run2.Status)
	}
}

func TestListRoutineRuns(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	routine := createTestRoutine(t, svc, "Runs List", "always_create")
	for i := 0; i < 3; i++ {
		_, _ = svc.FireManual(ctx, routine.ID, nil)
	}

	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 3 {
		t.Errorf("expected 3 runs, got %d", len(runs))
	}
}
