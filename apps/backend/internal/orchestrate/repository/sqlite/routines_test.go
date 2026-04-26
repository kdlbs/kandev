package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

func TestRoutine_CRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              "Daily Check",
		Description:       "Run daily checks",
		TaskTemplate:      `{"title":"Daily check"}`,
		Status:            "active",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "Daily Check" {
		t.Errorf("name = %q, want %q", got.Name, "Daily Check")
	}

	routines, err := repo.ListRoutines(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(routines) != 1 {
		t.Fatalf("list count = %d, want 1", len(routines))
	}

	routine.Name = "Updated Routine"
	if err := repo.UpdateRoutine(ctx, routine); err != nil {
		t.Fatalf("update: %v", err)
	}

	if err := repo.DeleteRoutine(ctx, routine.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestRoutineRun_Create(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              "Runner",
		TaskTemplate:      "{}",
		Status:            "active",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	run := &models.RoutineRun{
		RoutineID:      routine.ID,
		Source:         "manual",
		Status:         "received",
		TriggerPayload: "{}",
	}
	if err := repo.CreateRoutineRun(ctx, run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if run.ID == "" {
		t.Fatal("expected run ID to be set")
	}
}
