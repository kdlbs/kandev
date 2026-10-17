package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// MaxSpendTaskIDs bounds the ids one SumUsageForTasks call accepts.
const MaxSpendTaskIDs = 500

// SumUsageForTasks sums the priced cost of the ledger rows of taskIDs whose
// occurred_at lies in [from, to), and reports whether any such row is
// unpriced. An empty id list sums nothing. A SUM that overflows int64 is an
// error, so an unrepresentable total can never read as a small one.
func (r *Repository) SumUsageForTasks(ctx context.Context, taskIDs []string, from, to time.Time) (models.UsageSum, error) {
	if len(taskIDs) == 0 {
		return models.UsageSum{}, nil
	}
	if len(taskIDs) > MaxSpendTaskIDs {
		return models.UsageSum{}, fmt.Errorf("sum usage for tasks: %d ids exceed the limit of %d", len(taskIDs), MaxSpendTaskIDs)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(taskIDs)), ",")
	query := r.ro.Rebind(`
		SELECT
			COALESCE(SUM(CASE WHEN cost_source <> ? THEN cost_subcents ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN cost_source = ? THEN 1 ELSE 0 END), 0)
		  FROM task_usage_events
		 WHERE task_id IN (` + placeholders + `)
		   AND occurred_at >= ? AND occurred_at < ?`)
	args := make([]interface{}, 0, len(taskIDs)+4)
	args = append(args, costSourceUnpriced, costSourceUnpriced)
	for _, id := range taskIDs {
		args = append(args, id)
	}
	args = append(args, from.UTC(), to.UTC())

	var sum models.UsageSum
	var unpriced int64
	if err := r.ro.QueryRowxContext(ctx, query, args...).Scan(&sum.CostSubcents, &unpriced); err != nil {
		return models.UsageSum{}, fmt.Errorf("sum usage for tasks: %w", err)
	}
	sum.HasUnpriced = unpriced > 0
	return sum, nil
}

// SumUsageForTurn sums the priced cost of the ledger rows recorded for one
// turn of one session with occurred_at no later than notAfter.
func (r *Repository) SumUsageForTurn(ctx context.Context, sessionID, turnID string, notAfter time.Time) (int64, error) {
	query := r.ro.Rebind(`
		SELECT COALESCE(SUM(cost_subcents), 0)
		  FROM task_usage_events
		 WHERE session_id = ? AND turn_id = ? AND cost_source <> ? AND occurred_at <= ?`)
	var total int64
	if err := r.ro.QueryRowxContext(ctx, query, sessionID, turnID, costSourceUnpriced, notAfter.UTC()).Scan(&total); err != nil {
		return 0, fmt.Errorf("sum usage for turn: %w", err)
	}
	return total, nil
}
