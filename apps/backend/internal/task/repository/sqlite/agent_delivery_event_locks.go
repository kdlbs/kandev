package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// lockAgentDeliveryEventWrites acquires only locks. Authority reads remain in
// a following statement so they observe commits made by a previous lock owner.
func lockAgentDeliveryEventWrites(ctx context.Context, tx taskSessionExecutor, driverName string, sessionIDs, streamIDs []string) error {
	return lockAgentDeliveryEventWritesMode(ctx, tx, driverName, sessionIDs, streamIDs, false)
}

func lockAgentDeliveryEventWritesMode(ctx context.Context, tx taskSessionExecutor, driverName string, sessionIDs, streamIDs []string, sharedSessions bool) error {
	if !dialect.IsPostgres(driverName) {
		return nil
	}
	sessions := uniqueSortedAgentDeliveryIDs(sessionIDs)
	keys := make([]string, 0, len(sessions)*2+len(streamIDs))
	for _, sessionID := range sessions {
		keys = append(keys, sessionTurnLockNamespace+sessionID)
	}
	for _, sessionID := range sessions {
		keys = append(keys, agentDeliverySessionLockNamespace+sessionID)
	}
	for _, streamID := range uniqueSortedAgentDeliveryIDs(streamIDs) {
		keys = append(keys, agentDeliveryStreamLockNamespace+streamID)
	}
	if len(keys) == 0 {
		return nil
	}
	statements := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for index, key := range keys {
		dependency := ""
		if index > 0 {
			dependency = fmt.Sprintf(" FROM delivery_lock_%d", index-1)
		}
		lockFunction := "pg_advisory_xact_lock"
		if sharedSessions && index >= len(sessions) && index < len(sessions)*2 {
			lockFunction = "pg_advisory_xact_lock_shared"
		}
		statements = append(statements, fmt.Sprintf(
			"delivery_lock_%d AS MATERIALIZED (SELECT %s(hashtextextended($%d, 0))%s)",
			index, lockFunction, index+1, dependency))
		args = append(args, key)
	}
	query := "WITH " + strings.Join(statements, ", ") + fmt.Sprintf(" SELECT 1 FROM delivery_lock_%d", len(keys)-1)
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("lock delivery event writes: %w", err)
	}
	return nil
}

// Shared projection cannot create dependencies for another stream or settle a
// terminal. Flexible effects retain the exclusive session barrier.
func sharedCanonicalDeliverySession(events []*models.AgentDeliveryEvent, effects []*models.AgentDeliveryEffect) bool {
	for index, event := range events {
		effect := effects[index]
		if event.Terminal || effect == nil || effect.EffectType != "agent_delivery.event" ||
			effect.EffectKey != fmt.Sprintf("agent_delivery.event:%s:%d", event.StreamID, event.Sequence) ||
			effect.StreamID != event.StreamID || effect.Sequence != event.Sequence ||
			effect.State != models.DeliveryEffectCompleted || effect.CompletedAt == nil {
			return false
		}
		if !genericDeliveryEffectOwnershipMatches(event, effect) {
			return false
		}
	}
	return true
}

func genericDeliveryEffectOwnershipMatches(event *models.AgentDeliveryEvent, effect *models.AgentDeliveryEffect) bool {
	return (effect.SessionID == "" || effect.SessionID == event.SessionID) &&
		(effect.IncarnationID == "" || effect.IncarnationID == event.IncarnationID) &&
		(effect.HarnessGeneration == 0 || effect.HarnessGeneration == event.HarnessGeneration) &&
		(effect.SubmissionID == "" || effect.SubmissionID == event.SubmissionID)
}

// Session modes are decided before acquisition, so a prompt candidate never
// upgrades an already shared barrier. Stream barriers remain exclusive.
func lockAgentDeliveryRetentionWrites(ctx context.Context, tx taskSessionExecutor, driverName string, sessionIDs, streamIDs []string, prompts []agentDeliveryPromptRetentionCandidate) error {
	if !dialect.IsPostgres(driverName) {
		return nil
	}
	exclusive := make(map[string]bool, len(prompts))
	for _, prompt := range prompts {
		exclusive[prompt.sessionID] = true
	}
	for _, sessionID := range uniqueSortedAgentDeliveryIDs(sessionIDs) {
		key := agentDeliverySessionLockNamespace + sessionID
		if exclusive[sessionID] {
			if err := lockAgentDeliveryKey(ctx, tx, key); err != nil {
				return err
			}
		} else if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock_shared(hashtextextended($1, 0))`, key); err != nil {
			return err
		}
	}
	for _, streamID := range uniqueSortedAgentDeliveryIDs(streamIDs) {
		if err := lockAgentDeliveryKey(ctx, tx, agentDeliveryStreamLockNamespace+streamID); err != nil {
			return err
		}
	}
	return nil
}
