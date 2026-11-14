package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
)

func (r *Repository) ensureNativeUsageAccountingStateSchema(ctx context.Context) error {
	for _, column := range []string{
		`native_accounting_source_version INTEGER NOT NULL DEFAULT 0`,
		`native_accounting_required_sequence BIGINT NOT NULL DEFAULT 0`,
	} {
		if _, err := r.db.ExecContext(ctx, `ALTER TABLE usage_projection_scopes ADD COLUMN `+column); err != nil && !db.IsDuplicateColumnError(err) {
			return err
		}
	}
	_, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_usage_native_accounting_state ON usage_projection_scopes(native_accounting_source_version,scope_id)`)
	return err
}

// Source completion is separate from publication. Readers also require the
// active generation to replay this sequence and finish its publication work.
func (r *Repository) completeNativeUsageAccountingSourceTx(ctx context.Context, tx *sqlx.Tx, scope string) error {
	pending, err := r.nativeUsageAccountingRepairPendingTx(ctx, tx, scope)
	if err != nil || pending {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_scopes SET native_accounting_source_version=?,
		native_accounting_required_sequence=commit_sequence WHERE scope_id=?`), nativeUsageAccountingRepairVersion, scope)
	return err
}

// This runs in the accepted source transaction under the projection scope lock.
// A current native row cannot certify older rows in the same captured scope.
func (r *Repository) recordNativeUsageAccountingStateTx(ctx context.Context, tx *sqlx.Tx, scope string, sequence int64, before, after *indexedObservation) error {
	native := before != nil && before.Measurement.Source == usageSourceNative || after != nil && after.Measurement.Source == usageSourceNative
	if !native {
		return nil
	}
	pending, err := r.nativeUsageAccountingRepairPendingTx(ctx, tx, scope)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_scopes SET
		native_accounting_source_version=CASE WHEN ? THEN 0 ELSE native_accounting_source_version END,
		native_accounting_required_sequence=? WHERE scope_id=?`), pending, sequence, scope)
	return err
}
