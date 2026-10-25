package sqlite

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresAssignArrivalPositionToleratesMissingWorkflowStepRow proves
// task creation does not require a workflow_steps row to already exist on
// Postgres: lockWorkflowStepForWrite's FOR UPDATE select has nothing to lock
// when the step is not a real row, and that must not fail the arrival.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresAssignArrivalPositionToleratesMissingWorkflowStepRow(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "arrival-no-step-ws", Name: "Arrival no-step workspace"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}

	const missingStep = "step-that-does-not-exist"
	first := &models.Task{ID: "arrival-no-step-1", WorkspaceID: "arrival-no-step-ws", WorkflowID: "wf", WorkflowStepID: missingStep, Title: "first"}
	if err := repo.CreateTask(ctx, first); err != nil {
		t.Fatalf("CreateTask into a step with no workflow_steps row: %v", err)
	}
	if first.Position != 0 {
		t.Fatalf("first arrival Position = %d, want 0", first.Position)
	}

	second := &models.Task{ID: "arrival-no-step-2", WorkspaceID: "arrival-no-step-ws", WorkflowID: "wf", WorkflowStepID: missingStep, Title: "second"}
	if err := repo.CreateTask(ctx, second); err != nil {
		t.Fatalf("CreateTask (second arrival) into a step with no workflow_steps row: %v", err)
	}
	if second.Position != 1 {
		t.Fatalf("second arrival Position = %d, want 1", second.Position)
	}
}

// TestPostgresAssignArrivalPositionSerializesConcurrentArrivals proves the
// FOR UPDATE lock actually does its job when the step row does exist:
// concurrent CreateTask calls into the same step must not race the
// max(position)+1 read against each other and produce duplicate positions.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresAssignArrivalPositionSerializesConcurrentArrivals(t *testing.T) {
	const (
		concurrency = 8
		stepID      = "arrival-concurrent-step"
	)
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), concurrency)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "arrival-concurrent-ws", Name: "Arrival concurrent workspace"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO workflow_steps (id, workflow_id, name, position)
		VALUES (?, ?, ?, ?)
	`), stepID, "arrival-concurrent-workflow", "Arrival concurrent step", 0); err != nil {
		t.Fatalf("seed workflow step: %v", err)
	}

	start := make(chan struct{})
	positions := make(chan int, concurrency)
	errs := make(chan error, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			task := &models.Task{
				ID: fmt.Sprintf("arrival-concurrent-task-%d", i), WorkspaceID: "arrival-concurrent-ws",
				WorkflowID: "arrival-concurrent-workflow", WorkflowStepID: stepID, Title: fmt.Sprintf("task %d", i),
			}
			if err := repo.CreateTask(ctx, task); err != nil {
				errs <- err
				return
			}
			positions <- task.Position
		}(i)
	}
	close(start)
	wg.Wait()
	close(positions)
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent CreateTask: %v", err)
	}
	got := make([]int, 0, concurrency)
	for p := range positions {
		got = append(got, p)
	}
	sort.Ints(got)
	want := make([]int, concurrency)
	for i := range want {
		want[i] = i
	}
	if len(got) != len(want) {
		t.Fatalf("got %d positions, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("positions = %v, want a permutation of %v (duplicate or gap: lock did not serialize)", got, want)
		}
	}
}
