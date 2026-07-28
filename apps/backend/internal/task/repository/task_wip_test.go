package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type workflowStepCapacityCreator interface {
	CreateTaskIfWorkflowStepHasCapacity(context.Context, *models.Task, string, int) error
}

func TestUpdateTaskIfWorkflowStepHasCapacity_ReturnsTypedWIPError(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	if err := repo.CreateTask(ctx, &models.Task{
		ID: "wip-existing", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "wip-step", Title: "Existing", State: v1.TaskStateCreated,
	}); err != nil {
		t.Fatalf("seed existing task: %v", err)
	}
	candidate := &models.Task{
		ID: "wip-candidate", WorkspaceID: "wip-workspace", WorkflowID: "wip-workflow",
		WorkflowStepID: "other-step", Title: "Candidate", State: v1.TaskStateCreated,
	}
	err := repo.UpdateTaskIfWorkflowStepHasCapacity(ctx, candidate, "wip-step", "wip-candidate", 1)
	if err == nil || !errors.Is(err, wfmodels.ErrWIPLimitExceeded) {
		t.Fatalf("error=%v, want typed WIP limit error", err)
	}
}

func TestCreateTaskIfWorkflowStepHasCapacity_Concurrent(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()

	creator, ok := any(repo).(workflowStepCapacityCreator)
	if !ok {
		t.Fatal("task repository does not implement atomic workflow-step capacity creation")
	}

	const (
		workerCount = 8
		stepID      = "wip-step"
	)
	ctx := context.Background()
	start := make(chan struct{})
	results := make(chan error, workerCount)
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results <- creator.CreateTaskIfWorkflowStepHasCapacity(ctx, &models.Task{
				ID:             fmt.Sprintf("wip-task-%d", index),
				WorkspaceID:    "wip-workspace",
				WorkflowID:     "wip-workflow",
				WorkflowStepID: stepID,
				Title:          fmt.Sprintf("Task %d", index),
				State:          v1.TaskStateCreated,
			}, stepID, 2)
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)

	created, rejected := 0, 0
	for err := range results {
		if err == nil {
			created++
			continue
		}
		if !strings.Contains(strings.ToLower(err.Error()), "wip limit exceeded") {
			t.Fatalf("unexpected create error: %v", err)
		}
		rejected++
	}
	if created != 2 || rejected != workerCount-2 {
		t.Fatalf("created=%d rejected=%d, want created=2 rejected=%d", created, rejected, workerCount-2)
	}

	occupants, err := repo.CountTasksByWorkflowStep(ctx, stepID)
	if err != nil {
		t.Fatalf("count occupants: %v", err)
	}
	if occupants != 2 {
		t.Fatalf("occupants=%d, want 2", occupants)
	}
}
