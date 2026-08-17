package sqlite

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresUpdateTaskWithWorkflowStepAdmission_ConcurrentLastSlot exercises
// the PostgreSQL row lock used by workflow-step admission. SQLite coverage is
// not sufficient here because the dialect-specific lock is what serializes two
// connections that both observe the final available slot.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresUpdateTaskWithWorkflowStepAdmission_ConcurrentLastSlot(t *testing.T) {
	const (
		concurrency = 8
		targetStep  = "postgres-wip-target"
	)
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), concurrency)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	// The creation/admission paths lock the workspace row before the
	// workflow-step locks; the tasks below reference it, so it must exist.
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "postgres-wip-workspace", Name: "Postgres WIP workspace"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO workflow_steps (id, workflow_id, name, position)
		VALUES (?, ?, ?, ?)
	`), targetStep, "postgres-wip-workflow", "Postgres WIP target", 0); err != nil {
		t.Fatalf("seed target workflow step: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "postgres-wip-occupant", WorkspaceID: "postgres-wip-workspace", WorkflowID: "postgres-wip-workflow",
		WorkflowStepID: targetStep, Title: "Occupant", WIPAdmitted: true,
	}); err != nil {
		t.Fatalf("seed occupant: %v", err)
	}

	candidates := make([]*models.Task, concurrency)
	for i := range candidates {
		candidates[i] = &models.Task{
			ID: fmt.Sprintf("postgres-wip-candidate-%d", i), WorkspaceID: "postgres-wip-workspace",
			WorkflowID: "postgres-wip-workflow", WorkflowStepID: "postgres-wip-source",
			Title: "Candidate", WIPAdmitted: true,
		}
		if err := repo.CreateTask(ctx, candidates[i]); err != nil {
			t.Fatalf("seed candidate %d: %v", i, err)
		}
	}

	start := make(chan struct{})
	results := make(chan struct {
		admitted bool
		err      error
	}, concurrency)
	var wg sync.WaitGroup
	for _, candidate := range candidates {
		wg.Add(1)
		go func(task *models.Task) {
			defer wg.Done()
			<-start
			admitted, err := repo.UpdateTaskWithWorkflowStepAdmission(ctx, task, targetStep, 2)
			results <- struct {
				admitted bool
				err      error
			}{admitted: admitted, err: err}
		}(candidate)
	}
	close(start)
	wg.Wait()
	close(results)

	admitted, queued := 0, 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent admission: %v", result.err)
		}
		if result.admitted {
			admitted++
		} else {
			queued++
		}
	}
	if admitted != 1 || queued != concurrency-1 {
		t.Fatalf("admitted=%d queued=%d, want admitted=1 queued=%d", admitted, queued, concurrency-1)
	}
	occupants, err := repo.CountAdmittedTasksByWorkflowStep(ctx, targetStep)
	if err != nil {
		t.Fatalf("count admitted occupants: %v", err)
	}
	if occupants != 2 {
		t.Fatalf("admitted occupants=%d, want 2", occupants)
	}
}
