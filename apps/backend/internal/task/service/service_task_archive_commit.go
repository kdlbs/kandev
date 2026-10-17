package service

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"time"
)

func validateExactTaskArchive(ctx context.Context, task *models.Task) error {
	exact, ok := ctx.Value(exactTaskArchiveContextKey{}).(ExactTaskArchiveRequest)
	if !ok {
		return nil
	}
	expectedVersion, err := time.Parse(time.RFC3339Nano, exact.ExpectedResourceVersion)
	if err != nil || task.WorkspaceID != exact.WorkspaceID || !task.UpdatedAt.Equal(expectedVersion) {
		return repoerrors.ErrTaskVersionConflict
	}
	return nil
}

func (s *Service) commitTaskArchive(ctx context.Context, taskID string) error {
	exact, ok := ctx.Value(exactTaskArchiveContextKey{}).(ExactTaskArchiveRequest)
	if !ok {
		return s.tasks.ArchiveTask(ctx, taskID)
	}
	repository, supported := s.tasks.(taskrepo.ExactTaskArchiveRepository)
	if !supported {
		return errExactTaskUpdatesUnavailable
	}
	_, err := repository.ArchiveTaskExact(ctx, taskID, exact.WorkspaceID, exact.ExpectedResourceVersion, exact.OperationID, exact.PayloadDigest, exact.ClaimFence)
	return err
}
