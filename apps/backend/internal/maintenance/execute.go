package maintenance

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ExecutionResult reports how many rows were actually deleted per
// retention category during a --execute run. Row counts only - never IDs
// or content.
type ExecutionResult struct {
	DeletedGitSnapshots    int
	DeletedPlanRevisions   int
	DeletedMessagePayloads int
}

// TotalDeleted sums every category's deleted row count.
func (e ExecutionResult) TotalDeleted() int {
	return e.DeletedGitSnapshots + e.DeletedPlanRevisions + e.DeletedMessagePayloads
}

// executeDeleteChunkSize bounds each IN clause below SQLite's default
// SQLITE_MAX_VARIABLE_NUMBER (999 in most builds), leaving headroom.
const executeDeleteChunkSize = 400

// execute deletes exactly the rows identified by set inside a single
// transaction: either every configured retention category is applied, or
// none is (on any error the transaction rolls back and the database is left
// unchanged). Both task_session_git_snapshots and task_plan_revisions have
// no inbound foreign-key references from any other table (confirmed at
// design time), and task_message_payloads is only ever referenced by
// task_session_messages.payload_digest - so no FK cascade side effects are
// possible here beyond what set's callers (Analyze) already excluded.
//
// Deletes are idempotent by construction: applying execute a second time
// with the same (now empty, since the prior run already deleted them) ID
// sets is a correct no-op.
func execute(ctx context.Context, writer *sql.DB, set candidateSet) (ExecutionResult, error) {
	tx, err := writer.BeginTx(ctx, nil)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("begin retention transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	deletedSnapshots, err := deleteByColumn(ctx, tx, "task_session_git_snapshots", "id", set.gitSnapshotIDs)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("delete duplicate git snapshots: %w", err)
	}
	deletedRevisions, err := deleteByColumn(ctx, tx, "task_plan_revisions", "id", set.planRevisionIDs)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("delete obsolete plan revisions: %w", err)
	}
	deletedPayloads, err := deleteByColumn(ctx, tx, "task_message_payloads", "digest", set.payloadDigests)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("delete orphaned message payloads: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return ExecutionResult{}, fmt.Errorf("commit retention transaction: %w", err)
	}
	committed = true

	return ExecutionResult{
		DeletedGitSnapshots:    deletedSnapshots,
		DeletedPlanRevisions:   deletedRevisions,
		DeletedMessagePayloads: deletedPayloads,
	}, nil
}

// deleteByColumn deletes every row in table whose column value is in keys,
// batching the IN clause to stay under SQLite's bound-parameter limit.
// table and column are always compile-time literals supplied by execute
// above, never derived from external input.
func deleteByColumn(ctx context.Context, tx *sql.Tx, table, column string, keys []string) (int, error) {
	total := 0
	for start := 0; start < len(keys); start += executeDeleteChunkSize {
		end := start + executeDeleteChunkSize
		if end > len(keys) {
			end = len(keys)
		}
		batch := keys[start:end]
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(batch)), ",")
		query := fmt.Sprintf(`DELETE FROM %s WHERE %s IN (%s)`, table, column, placeholders) //nolint:gosec // table/column are literal call-site constants, not user input
		args := make([]interface{}, len(batch))
		for i, key := range batch {
			args[i] = key
		}
		res, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return 0, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return 0, err
		}
		total += int(affected)
	}
	return total, nil
}
