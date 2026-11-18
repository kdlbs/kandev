package orchestrator

import (
	"context"
	"errors"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

// BuiltinMCPDispatchBlocked keeps queued input intact while the provider is
// being replaced. Queue insertion has independent admission after retirement.
func (s *Service) BuiltinMCPDispatchBlocked(ctx context.Context, sessionID string) (bool, error) {
	reader, ok := s.repo.(interface {
		GetBuiltinMCPApplication(context.Context, string) (*models.BuiltinMCPApplication, error)
	})
	if !ok {
		return false, nil
	}
	app, err := reader.GetBuiltinMCPApplication(ctx, sessionID)
	if err != nil || app == nil {
		return false, err
	}
	switch app.Stage {
	case models.BuiltinMCPApplicationPreparing, models.BuiltinMCPApplicationRetiring, models.BuiltinMCPApplicationStarting, models.BuiltinMCPApplicationVerifying, models.BuiltinMCPApplicationAccepting:
		return true, nil
	case models.BuiltinMCPApplicationPending:
		return app.RetryOfOperationID != "", nil
	case models.BuiltinMCPApplicationFailed:
		running, err := s.repo.GetExecutorRunningBySessionID(ctx, sessionID)
		if errors.Is(err, models.ErrExecutorRunningNotFound) {
			return app.AttemptSource != "", nil
		}
		if err != nil {
			return false, err
		}
		value, present := running.Metadata[p.ProviderMCPEffectsClosedMetadataKey]
		if !present {
			return false, nil
		}
		closed, valid := value.(bool)
		return !valid || closed, nil
	}
	return false, nil
}

func (s *Service) builtinApplicationBlocksQueue(ctx context.Context, sessionID string) bool {
	blocked, err := s.BuiltinMCPDispatchBlocked(ctx, sessionID)
	return blocked || err != nil
}
