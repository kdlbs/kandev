package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

func TestPreparedCleanupCanTransferOnlyItsOwnEnvironment(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "cleanup-transfer")
	for _, id := range []string{"owner", "borrower"} {
		if err := repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "cleanup-transfer", Title: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "environment", TaskID: "owner", Status: models.TaskEnvironmentStatusReady}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, &models.TaskResourceCleanupJob{
		ID: "cleanup", OperationID: "delete:owner", TaskID: "owner", Trigger: models.TaskResourceCleanupTriggerDelete, State: models.TaskResourceCleanupStatePrepared,
	}); err != nil {
		t.Fatal(err)
	}
	for _, job := range []recoveryclaim.TaskCleanupJob{
		{ID: "other", TaskID: "owner"}, {ID: "cleanup", TaskID: "borrower"}, {ID: "cleanup", TaskID: "owner", Attempts: 1},
	} {
		if err := repo.TransferTaskEnvironmentOwnership(recoveryclaim.WithTaskCleanupJob(ctx, job), "environment", "owner", 1, "borrower"); err == nil {
			t.Fatalf("transfer accepted unrelated or stale cleanup: %+v", job)
		}
	}
	admitted := recoveryclaim.WithTaskCleanupJob(ctx, recoveryclaim.TaskCleanupJob{ID: "cleanup", TaskID: "owner"})
	if err := repo.TransferTaskEnvironmentOwnership(admitted, "environment", "owner", 1, "borrower"); err != nil {
		t.Fatalf("transfer under prepared cleanup: %v", err)
	}
	env, err := repo.GetTaskEnvironment(ctx, "environment")
	if err != nil || env.TaskID != "borrower" || env.OwnershipGeneration != 2 {
		t.Fatalf("transferred environment = %+v, %v", env, err)
	}
}
