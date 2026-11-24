package orchestrator

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	bus "github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
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
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		s.logger.Warn("cannot publish committed compaction metadata without the authoritative session",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	eventData := map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"metadata":   metadata,
	}
	if !session.UpdatedAt.IsZero() {
		eventData["updated_at"] = session.UpdatedAt.Format(time.RFC3339Nano)
	}
	_ = s.eventBus.Publish(ctx, events.TaskSessionStateChanged, bus.NewEvent(
		events.TaskSessionStateChanged,
		"orchestrator",
		eventData,
	))
}

func compactionReportingFromMetadata(metadata map[string]interface{}) *streams.ContextCompactionReporting {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata[models.SessionMetaKeyContextCompactionReporting]
	if !ok || raw == nil {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var reporting streams.ContextCompactionReporting
	if json.Unmarshal(encoded, &reporting) != nil {
		return nil
	}
	return streams.NormalizeReporting(&reporting)
}

func compactionReportingUnavailablePartial(reporting *streams.ContextCompactionReporting) bool {
	return reporting != nil &&
		reporting.Availability == streams.ContextCompactionAvailabilityUnavailable &&
		reporting.Coverage == streams.ContextCompactionCoveragePartial
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
