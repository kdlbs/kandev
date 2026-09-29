package coordinator

import (
	"context"
	"fmt"
	"time"
)

// DeleteActivityBatch deletes up to limit rows created before cutoff, oldest
// first, in its own transaction, and reports how many it removed.
func (s *Store) DeleteActivityBatch(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM coordinator_activity WHERE id IN (
		SELECT id FROM coordinator_activity WHERE created_at < ? ORDER BY created_at, id LIMIT ?)`), cutoff.UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("delete coordinator activity batch: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete coordinator activity batch: %w", err)
	}
	return n, nil
}
