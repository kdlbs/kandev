package runtime

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// BackgroundWorkController exposes background workload snapshot and action operations for an execution.
type BackgroundWorkController interface {
	GetBackgroundWorkSnapshot(ctx context.Context, executionID string) (*streams.BackgroundWorkSnapshot, error)
	PerformBackgroundWorkAction(ctx context.Context, executionID string, req streams.BackgroundWorkActionRequest) (*streams.BackgroundWorkActionResponse, error)
}
