package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/task/changes"
)

type runtimeTurnChangeOperationGuard struct {
	reader interface {
		TurnChangeCaptureOperation(executionID string, promptGeneration int64) (bool, bool)
	}
}

func (g runtimeTurnChangeOperationGuard) TurnChangeCaptureOperation(
	_ context.Context,
	executionID string,
	promptGeneration int64,
) changes.TurnChangeOperationState {
	if g.reader == nil {
		return changes.TurnChangeOperationUnknown
	}
	known, active := g.reader.TurnChangeCaptureOperation(executionID, promptGeneration)
	if !known {
		return changes.TurnChangeOperationInactive
	}
	if active {
		return changes.TurnChangeOperationActive
	}
	return changes.TurnChangeOperationInactive
}
