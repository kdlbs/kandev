package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/events"
	bus "github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// publishCommittedContextCompaction publishes the count/reporting metadata that
// canonical projection already committed. It never writes a second authority:
// a lost notification is repaired by boot hydration or a later metadata read.
func (s *Service) publishCommittedContextCompaction(
	ctx context.Context,
	taskID, sessionID string,
	canonical bool,
	data interface{},
) {
	if s.eventBus == nil || !canonical || sessionID == "" {
		return
	}
	raw, ok := data.(map[string]interface{})
	if !ok || raw == nil {
		return
	}
	metadata := map[string]interface{}{}
	if count, ok := numericMetadataValue(raw[models.SessionMetaKeyContextCompactionCount]); ok {
		metadata[models.SessionMetaKeyContextCompactionCount] = count
	}
	if reporting, ok := raw[models.SessionMetaKeyContextCompactionReporting]; ok && reporting != nil {
		metadata[models.SessionMetaKeyContextCompactionReporting] = reporting
	}
	if len(metadata) == 0 {
		return
	}
	_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
		events.TaskSessionStateChanged,
		"orchestrator",
		map[string]interface{}{
			"task_id":    taskID,
			"session_id": sessionID,
			"metadata":   metadata,
		},
	))
}

func numericMetadataValue(value interface{}) (interface{}, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		return int64(typed), true
	default:
		return nil, false
	}
}
