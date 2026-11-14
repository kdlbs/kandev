package deliveryretention

import (
	"context"
	"errors"

	"go.uber.org/zap"
)

func (w *Worker) batchRowLimit(phase string) int {
	limit := w.opts.BatchRows
	if phase == "retention" {
		limit = min(limit, BackgroundBatchRows)
	}
	if reduced := w.batchRows[phase]; reduced > 0 {
		limit = min(limit, reduced)
	}
	return limit
}

func (w *Worker) recordBatchFailure(ctx context.Context, phase, reason string, err error) {
	if ctx.Err() != nil {
		return
	}
	previous := w.batchRowLimit(phase)
	// Retry the same protected frontier with less work after an expired lease.
	// The configured ceiling and repository transaction budgets still apply.
	if errors.Is(err, context.DeadlineExceeded) {
		if w.batchRows == nil {
			w.batchRows = make(map[string]int)
		}
		w.batchRows[phase] = max(1, previous/2)
	}
	if w.opts.Logger != nil {
		w.opts.Logger.Warn("agent_delivery.retention.batch_failed",
			zap.String("phase", phase), zap.String("reason", reason),
			zap.Int("candidate_rows", previous), zap.Int("next_candidate_rows", w.batchRowLimit(phase)),
			zap.Error(err))
	}
}
