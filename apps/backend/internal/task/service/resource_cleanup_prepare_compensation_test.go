package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type failingCleanupSnapshotRepository struct {
	repository.TaskResourceCleanupRepository
	err error
}

func (r *failingCleanupSnapshotRepository) UpdateTaskResourceCleanupSnapshot(
	context.Context,
	string,
	string,
) error {
	return r.err
}

func TestPrepareTaskResourceCleanupCancelsBarrierAfterSnapshotFailure(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	seedCleanupTaskAndSession(t, repo, "task-snapshot-failure", "session-snapshot-failure")
	snapshotErr := errors.New("snapshot persistence unavailable")
	taskSvc.resourceCleanups = &failingCleanupSnapshotRepository{
		TaskResourceCleanupRepository: taskSvc.resourceCleanups,
		err:                           snapshotErr,
	}

	err := taskSvc.PrepareTaskResourceCleanup(
		context.Background(),
		"task-snapshot-failure",
		models.TaskResourceCleanupTriggerCascadeArchive,
		"cascade-archive:operation",
		false,
	)
	if !errors.Is(err, snapshotErr) {
		t.Fatalf("PrepareTaskResourceCleanup error = %v, want snapshot error", err)
	}
	job, getErr := repo.GetTaskResourceCleanupJobByOperationID(context.Background(), "cascade-archive:operation")
	if getErr != nil {
		t.Fatalf("GetTaskResourceCleanupJobByOperationID: %v", getErr)
	}
	if job == nil || job.State != models.TaskResourceCleanupStateCancelled {
		t.Fatalf("cleanup job = %#v, want cancelled prepared barrier", job)
	}
}
