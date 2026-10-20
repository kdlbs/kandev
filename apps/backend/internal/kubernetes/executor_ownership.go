package kubernetes

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
)

type executorConfigurationAccess interface {
	AuthorizeExecutorConfiguration(context.Context, string, string) error
}

func (h *Handler) executorForRead(ctx context.Context, executorID string) (*models.Executor, error) {
	checker, wired := h.access.(executorConfigurationAccess)
	if wired {
		if err := checker.AuthorizeExecutorConfiguration(ctx, executorID, ""); err != nil {
			return nil, models.ErrExecutorNotFound
		}
	}
	executor, err := h.repo.GetExecutor(ctx, executorID)
	if err != nil {
		return nil, err
	}
	if executor != nil && executor.OwnerUserID != "" && !wired {
		return nil, models.ErrExecutorNotFound
	}
	return executor, nil
}
