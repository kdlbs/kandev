package sqlite

import (
	"context"
	"errors"
	"testing"

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
}
