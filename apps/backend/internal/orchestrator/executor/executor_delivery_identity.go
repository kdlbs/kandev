package executor

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

type harnessSessionGenerationReader interface {
	GetCurrentHarnessSessionGeneration(context.Context, string, string) (*models.HarnessSessionGeneration, error)
}

// applyDeliveryIdentity resolves the durable stream owner before an executor
// launch. A process replacement keeps the same generation and stream; an
// explicit context continuation advances the generation before its new native
// conversation can publish events.
func (e *Executor) applyDeliveryIdentity(
	ctx context.Context,
	req *LaunchAgentRequest,
	session *models.TaskSession,
) error {
	if req == nil || session == nil {
		return nil
	}
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	current, generation, err := e.loadDeliveryGeneration(ctx, session.ID, incarnationID)
	if err != nil {
		return err
	}
	if current != nil && current.OriginalWorkspace != "" {
		req.OriginalWorkspacePath = current.OriginalWorkspace
	} else if req.OriginalWorkspacePath == "" {
		req.OriginalWorkspacePath = session.WorkspacePath
	}
	if req.ForceContextContinuation && current != nil {
		generation++
	}
	req.DeliveryIncarnationID = incarnationID
	req.DeliveryHarnessGeneration = uint64(generation)
	req.DeliveryStreamID = fmt.Sprintf("%s:g%d", incarnationID, generation)
	return nil
}

func (e *Executor) loadDeliveryGeneration(
	ctx context.Context,
	sessionID, incarnationID string,
) (*models.HarnessSessionGeneration, int64, error) {
	generation := int64(0)
	var current *models.HarnessSessionGeneration
	reader, ok := e.repo.(harnessSessionGenerationReader)
	if !ok {
		return nil, 1, nil
	}
	loaded, err := reader.GetCurrentHarnessSessionGeneration(ctx, sessionID, incarnationID)
	switch {
	case err == nil && loaded != nil:
		current = loaded
		generation = loaded.Generation
	case err == nil:
	case errors.Is(err, models.ErrTaskSessionNotFound):
	default:
		return nil, 0, fmt.Errorf("load current harness generation for delivery: %w", err)
	}
	if generation <= 0 {
		generation = 1
	}
	return current, generation, nil
}
