package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	turnStatusKindDetached = "turn_ended_detached"
	turnStatusKindPaused   = "paused_offline_budget"

	cancelCauseOfflineBudget = "offline_budget"
)

// recordTurnStatusNotice writes at most one status message for a stamped
// terminal event. A budget cancel writes only the paused record, at the time of
// the expiry decision; any other terminal that committed while detached writes
// the detached record at its commit time. The message ID is derived from the
// terminal's delivery identity, so a replayed terminal cannot add a second one.
func (s *Service) recordTurnStatusNotice(ctx context.Context, data watcher.AgentEventData) {
	if s.messageCreator == nil || data.DeliveryStreamID == "" || data.DeliverySequence == 0 {
		return
	}
	var kind, content string
	var at *time.Time
	switch {
	case data.CancelCause == cancelCauseOfflineBudget:
		kind, at = turnStatusKindPaused, data.CancelDecidedAt
		content = "The turn was paused because Kandev stayed unreachable beyond the offline budget."
	case data.DetachedAtCommit:
		kind, at = turnStatusKindDetached, data.CommittedAt
		content = "The turn ended while Kandev was unreachable."
	default:
		return
	}
	occurred := time.Now().UTC()
	if at != nil && !at.IsZero() {
		occurred = at.UTC()
	}
	episode := fmt.Sprintf("%s:%d", data.DeliveryStreamID, data.DeliverySequence)
	metadata := map[string]interface{}{
		detailsKeyKind: kind,
		"episode":      episode,
		"occurred_at":  occurred.Format(time.RFC3339),
	}
	messageID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("turn_status_record:"+episode)).String()
	if err := s.messageCreator.CreateSessionMessageIdempotent(
		ctx, messageID, data.TaskID, content, data.SessionID,
		string(v1.MessageTypeStatus), data.TurnID, metadata, false,
	); err != nil {
		s.logger.Warn("failed to record turn status notice",
			zap.String("session_id", data.SessionID), zap.String("episode", episode), zap.Error(err))
	}
}
