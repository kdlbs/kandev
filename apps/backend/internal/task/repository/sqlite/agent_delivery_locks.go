package sqlite

import (
	"context"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/db/dialect"
)

const (
	agentDeliverySessionLockNamespace = "agent-delivery-session:"
	agentDeliveryStreamLockNamespace  = "agent-delivery-stream:"
)

// lockAgentDeliveryWrites serializes delivery state after callers acquire any
// hierarchy or session-turn admission locks they already own.
func lockAgentDeliveryWrites(
	ctx context.Context,
	tx taskSessionExecutor,
	driverName string,
	sessionIDs, streamIDs []string,
) error {
	if !dialect.IsPostgres(driverName) {
		return nil
	}
	for _, sessionID := range uniqueSortedAgentDeliveryIDs(sessionIDs) {
		if err := lockAgentDeliveryKey(ctx, tx, agentDeliverySessionLockNamespace+sessionID); err != nil {
			return fmt.Errorf("lock agent delivery session %q: %w", sessionID, err)
		}
	}
	for _, streamID := range uniqueSortedAgentDeliveryIDs(streamIDs) {
		if err := lockAgentDeliveryKey(ctx, tx, agentDeliveryStreamLockNamespace+streamID); err != nil {
			return fmt.Errorf("lock agent delivery stream %q: %w", streamID, err)
		}
	}
	return nil
}

func lockAgentDeliveryKey(ctx context.Context, tx taskSessionExecutor, key string) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, key)
	return err
}

func uniqueSortedAgentDeliveryIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	ordered := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		ordered = append(ordered, value)
	}
	sort.Strings(ordered)
	return ordered
}
