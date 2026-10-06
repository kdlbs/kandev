package backendapp

import (
	"context"
	"sync/atomic"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type issueMutationTaskHook struct {
	repository.TaskRepository
	armed            atomic.Bool
	arrived          chan struct{}
	release          chan struct{}
	after            func(context.Context) error
	observationError error
	committed        bool
}

func (h *issueMutationTaskHook) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if h.committed && h.observationError != nil {
		return nil, h.observationError
	}
	task, err := h.TaskRepository.GetTask(ctx, id)
	if err != nil || !h.armed.Swap(false) {
		return task, err
	}
	close(h.arrived)
	select {
	case <-h.release:
		return task, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (h *issueMutationTaskHook) UpdateTaskGitHubIssue(ctx context.Context, id string, link *models.TaskGitHubIssueLink) (*models.Task, error) {
	task, err := h.TaskRepository.UpdateTaskGitHubIssue(ctx, id, link)
	if err != nil {
		return task, err
	}
	h.committed = true
	if h.after != nil {
		if err := h.after(ctx); err != nil {
			return nil, err
		}
	}
	return task, nil
}
