package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestDeleteTaskEnvironmentMissingReturnsSentinel(t *testing.T) {
	repo := newRepoForHealTests(t)

	err := repo.DeleteTaskEnvironment(context.Background(), "missing-environment")

	if !errors.Is(err, ErrTaskEnvironmentNotFound) {
		t.Fatalf("DeleteTaskEnvironment error = %v, want ErrTaskEnvironmentNotFound", err)
	}
}

func TestGetTaskEnvironmentMissingReturnsSentinel(t *testing.T) {
	repo := newRepoForHealTests(t)

	_, err := repo.GetTaskEnvironment(context.Background(), "missing-environment")

	if !errors.Is(err, ErrTaskEnvironmentNotFound) {
		t.Fatalf("GetTaskEnvironment error = %v, want ErrTaskEnvironmentNotFound", err)
	}
}

func TestUpdateTaskEnvironmentMissingReturnsSentinel(t *testing.T) {
	repo := newRepoForHealTests(t)

	err := repo.UpdateTaskEnvironment(context.Background(), &models.TaskEnvironment{ID: "missing-environment"})

	if !errors.Is(err, ErrTaskEnvironmentNotFound) {
		t.Fatalf("UpdateTaskEnvironment error = %v, want ErrTaskEnvironmentNotFound", err)
	}
}

func TestTransferTaskEnvironmentMissingReturnsSentinel(t *testing.T) {
	repo := newRepoForHealTests(t)

	err := repo.TransferTaskEnvironmentToTask(context.Background(), "missing-environment", "task-1")

	if !errors.Is(err, ErrTaskEnvironmentNotFound) {
		t.Fatalf("TransferTaskEnvironmentToTask error = %v, want ErrTaskEnvironmentNotFound", err)
	}
}
