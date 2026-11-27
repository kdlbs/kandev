package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

type workspaceHarnessGenerationReader interface {
	GetCurrentHarnessSessionGeneration(context.Context, string, string) (*models.HarnessSessionGeneration, error)
}

// Workspace-only recovery retains the same durable owner as a full agent
// launch, including when it recreates compute before a lazy native restore.
func (s *Service) workspaceDeliveryIdentity(ctx context.Context, session *models.TaskSession) (string, uint64, error) {
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	generation := int64(1)
	if reader, ok := s.sessions.(workspaceHarnessGenerationReader); ok {
		current, err := reader.GetCurrentHarnessSessionGeneration(ctx, session.ID, incarnationID)
		switch {
		case err == nil && current != nil && current.Generation > 0:
			generation = current.Generation
		case err == nil, errors.Is(err, models.ErrTaskSessionNotFound), errors.Is(err, sql.ErrNoRows):
		default:
			return "", 0, fmt.Errorf("load workspace delivery generation: %w", err)
		}
	}
	return incarnationID, uint64(generation), nil
}
