package service

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type agentActivityReader interface {
	GetLastAgentActivityForTurn(context.Context, string, string) (time.Time, error)
}

// projectRunningNotices derives resolution metadata without modifying persisted
// notices. Whole-turn evidence keeps paginated snapshots consistent with live events.
func (s *Service) projectRunningNotices(ctx context.Context, messages []*models.Message) error {
	reader, ok := s.messages.(agentActivityReader)
	if !ok {
		return nil
	}
	type turnKey struct{ sessionID, turnID string }
	activityByTurn := make(map[turnKey]time.Time)
	for _, message := range messages {
		if message.Type != models.MessageTypeStatus || message.TurnID == "" ||
			message.Metadata["action_visibility"] != "running" {
			continue
		}
		key := turnKey{message.TaskSessionID, message.TurnID}
		activity, found := activityByTurn[key]
		if !found {
			var err error
			activity, err = reader.GetLastAgentActivityForTurn(ctx, key.sessionID, key.turnID)
			if err != nil {
				return err
			}
			activityByTurn[key] = activity
		}
		if activity.After(message.CreatedAt) {
			message.Metadata["running_notice_resolved"] = true
		}
	}
	return nil
}
