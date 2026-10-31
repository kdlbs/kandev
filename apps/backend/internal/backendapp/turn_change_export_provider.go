package backendapp

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/changes"
)

// runtimeTurnChangeExportProviderAdapter gives the task-owned content
// processor ordered access to the retained executor client. It never creates
// or relaunches an execution.
type runtimeTurnChangeExportProviderAdapter struct {
	manager interface {
		AcquireTurnChangeExporter(sessionID string) (agentruntime.TurnChangeCheckpointClient, func(), bool)
		TurnChangeRestoreInProgress(sessionID string) bool
	}
}

func (p runtimeTurnChangeExportProviderAdapter) AcquireTurnChangeExporter(
	_ context.Context,
	sessionID string,
) (changes.CheckpointClient, func(), bool) {
	if p.manager == nil {
		return nil, nil, false
	}
	return p.manager.AcquireTurnChangeExporter(sessionID)
}

func (p runtimeTurnChangeExportProviderAdapter) TurnChangeRestoreInProgress(sessionID string) bool {
	if p.manager == nil {
		return false
	}
	return p.manager.TurnChangeRestoreInProgress(sessionID)
}
