package backendapp

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/review"
)

// ReviewSourceMetadata prefers task metadata, with session metadata as a fallback.
func (c reviewTaskContext) ReviewSourceMetadata(ctx context.Context, taskID, sessionID string) (map[string]any, error) {
	if c.tasks == nil {
		return nil, nil
	}
	task, err := c.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("%w: read task review source: %v", review.ErrWorkspaceUnavailable, err)
	}
	if task != nil && task.Metadata[review.SourceMetadataKey] != nil {
		return task.Metadata, nil
	}
	sessions, ok := c.tasks.(reviewSessionGetter)
	if !ok || sessionID == "" {
		return nil, nil
	}
	session, err := sessions.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: read session review source: %v", review.ErrWorkspaceUnavailable, err)
	}
	if session != nil {
		return session.Metadata, nil
	}
	return nil, nil
}
