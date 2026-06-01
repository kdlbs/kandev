package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestErrorsAreClassifiable verifies that the repository wraps not-found
// errors with the ErrTaskNotFound sentinel so HTTP / orchestrator callers
// can classify via errors.Is.
func TestErrorsAreClassifiable(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)

	t.Run("GetTask returns sentinel-wrapped error for missing row", func(t *testing.T) {
		_, err := repo.GetTask(ctx, "task-does-not-exist")
		if err == nil {
			t.Fatal("expected GetTask to fail for missing id")
		}
		if !errors.Is(err, ErrTaskNotFound) {
			t.Errorf("GetTask error not classifiable as ErrTaskNotFound: %v", err)
		}
	})

	t.Run("DeleteTask returns sentinel-wrapped error for missing row", func(t *testing.T) {
		err := repo.DeleteTask(ctx, "task-does-not-exist")
		if err == nil {
			t.Fatal("expected DeleteTask to fail for missing id")
		}
		if !errors.Is(err, ErrTaskNotFound) {
			t.Errorf("DeleteTask error not classifiable as ErrTaskNotFound: %v", err)
		}
	})

	t.Run("UpdateTaskState returns sentinel-wrapped error for missing row", func(t *testing.T) {
		err := repo.UpdateTaskState(ctx, "task-does-not-exist", v1.TaskStateInProgress)
		if err == nil {
			t.Fatal("expected UpdateTaskState to fail for missing id")
		}
		if !errors.Is(err, ErrTaskNotFound) {
			t.Errorf("UpdateTaskState error not classifiable as ErrTaskNotFound: %v", err)
		}
	})

	t.Run("GetPrimarySessionByTaskID returns ErrNoPrimarySession for task with no session", func(t *testing.T) {
		// Create a task without a session so the no-rows path is exercised.
		taskID := "task-no-session"
		err := repo.CreateTask(ctx, &models.Task{
			ID:          taskID,
			WorkspaceID: "ws-err-test",
			WorkflowID:  "wf-err-test",
			Title:       "Sessionless task",
			State:       v1.TaskStateCreated,
		})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		_, err = repo.GetPrimarySessionByTaskID(ctx, taskID)
		if err == nil {
			t.Fatal("expected GetPrimarySessionByTaskID to fail for task with no session")
		}
		if !errors.Is(err, ErrNoPrimarySession) {
			t.Errorf("GetPrimarySessionByTaskID error not classifiable as ErrNoPrimarySession: %v", err)
		}
	})
}
