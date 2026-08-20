package sqlite

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresUpdateTaskWithWorkflowStepAdmissionIfAtStep_ConcurrentRace is
// the AC-46/48 PostgreSQL counterpart to
// TestPostgresUpdateTaskWithWorkflowStepAdmission_ConcurrentLastSlot: it
// exercises the same workspace-row lock the CAS variant reuses, proving that
// with several real connections racing the same expectedStepID
// precondition, exactly one attempt observes a match and applies.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresUpdateTaskWithWorkflowStepAdmissionIfAtStep_ConcurrentRace(t *testing.T) {
	const (
		concurrency = 8
		sourceStep  = "postgres-cas-source"
		targetStep  = "postgres-cas-target"
	)
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), concurrency)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "postgres-cas-workspace", Name: "Postgres CAS workspace"}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	for i, stepID := range []string{sourceStep, targetStep} {
		if _, err := db.Exec(db.Rebind(`
			INSERT INTO workflow_steps (id, workflow_id, name, position)
			VALUES (?, ?, ?, ?)
		`), stepID, "postgres-cas-workflow", stepID, i); err != nil {
			t.Fatalf("seed workflow step %s: %v", stepID, err)
		}
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "postgres-cas-task", WorkspaceID: "postgres-cas-workspace", WorkflowID: "postgres-cas-workflow",
		WorkflowStepID: sourceStep, Title: "CAS race candidate", WIPAdmitted: true,
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	start := make(chan struct{})
	results := make(chan struct {
		applied bool
		err     error
	}, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			own, err := repo.GetTask(ctx, "postgres-cas-task")
			if err != nil {
				results <- struct {
					applied bool
					err     error
				}{false, err}
				return
			}
			applied, err := repo.UpdateTaskWithWorkflowStepAdmissionIfAtStep(ctx, own, sourceStep, targetStep, 0)
			results <- struct {
				applied bool
				err     error
			}{applied, err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	appliedCount := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent CAS attempt: %v", result.err)
		}
		if result.applied {
			appliedCount++
		}
	}
	if appliedCount != 1 {
		t.Fatalf("expected exactly 1 concurrent CAS attempt to apply, got %d", appliedCount)
	}
	final, err := repo.GetTask(ctx, "postgres-cas-task")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if final.WorkflowStepID != targetStep {
		t.Fatalf("expected the task to end at %s, got %q", targetStep, final.WorkflowStepID)
	}
}
