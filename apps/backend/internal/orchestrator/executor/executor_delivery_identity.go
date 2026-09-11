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
	generation := int64(0)
	var current *models.HarnessSessionGeneration
	if reader, ok := e.repo.(harnessSessionGenerationReader); ok {
		loaded, err := reader.GetCurrentHarnessSessionGeneration(ctx, session.ID, incarnationID)
		switch {
		case err == nil && loaded != nil:
			current = loaded
			generation = loaded.Generation
		case err == nil:
		case errors.Is(err, models.ErrTaskSessionNotFound):
		default:
			return fmt.Errorf("load current harness generation for delivery: %w", err)
		}
	}
	if generation <= 0 {
		generation = 1
	}
	if req.ForceContextContinuation && current != nil {
		generation++
	}
	req.DeliveryIncarnationID = incarnationID
	req.DeliveryHarnessGeneration = uint64(generation)
	req.DeliveryStreamID = fmt.Sprintf("%s:g%d", incarnationID, generation)
	return nil
}
