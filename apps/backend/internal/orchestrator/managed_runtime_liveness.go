package orchestrator

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

// managedAgentOperationActive returns true until the durable journal confirms
// the remote turn is terminal. Read errors fail closed so local timers cannot
// reclaim work whose provider state is unavailable.
func (s *Service) managedAgentOperationActive(ctx context.Context, sessionID string) bool {
	managed, ok := s.repo.(repository.ManagedAgentRepository)
	if !ok || sessionID == "" {
		return false
	}
	binding, err := managed.GetManagedAgentBindingBySession(ctx, sessionID)
	if errors.Is(err, repository.ErrManagedAgentBindingNotFound) {
		return false
	}
	if err != nil || binding == nil {
		return true
	}
	operation, err := managed.GetManagedAgentLatestOperation(ctx, binding.ID)
	if err != nil || operation == nil {
		return true
	}
	return models.ManagedAgentOperationActive(operation.State)
}
