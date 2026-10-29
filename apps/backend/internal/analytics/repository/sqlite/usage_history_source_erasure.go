package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
)

func (r *Repository) preserveOrganizationUsageSourcesBatchTx(ctx context.Context, tx *sqlx.Tx, organization string, limit int) (bool, error) {
	var scope string
	err := tx.GetContext(ctx, &scope, r.db.Rebind(`SELECT s.scope_id FROM usage_scopes s
		LEFT JOIN usage_collection_guard_erasure p ON p.scope_id=s.scope_id
		WHERE s.organization_id = ? AND COALESCE(p.complete,0) = 0 ORDER BY s.scope_id LIMIT 1`), organization)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	_, err = r.advanceErasedUsageSourceScopeTx(ctx, tx, scope, limit)
	return false, err
}

func (r *Repository) advanceErasedUsageSourceScopeTx(ctx context.Context, tx *sqlx.Tx, scope string, limit int) (bool, error) {
	var last int64
	var complete int
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT last_sequence,complete FROM usage_collection_guard_erasure WHERE scope_id = ?`), scope).Scan(&last, &complete)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if complete != 0 {
		return true, nil
	}
	next, done, err := r.preserveErasedUsageSourcePageTx(ctx, tx, scope, last, limit)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_collection_guard_erasure (scope_id,last_sequence,complete) VALUES (?, ?, ?)
		ON CONFLICT(scope_id) DO UPDATE SET
		last_sequence=CASE WHEN excluded.last_sequence > usage_collection_guard_erasure.last_sequence THEN excluded.last_sequence ELSE usage_collection_guard_erasure.last_sequence END,
		complete=CASE WHEN usage_collection_guard_erasure.complete=1 OR excluded.complete=1 THEN 1 ELSE 0 END`), scope, next, boolInt(done))
	return done, err
}

func (r *Repository) preserveErasedUsageSourceScopeTx(ctx context.Context, tx *sqlx.Tx, scope string) error {
	for {
		done, err := r.advanceErasedUsageSourceScopeTx(ctx, tx, scope, usageHistoryBatchLimit)
		if err != nil || done {
			return err
		}
	}
}
