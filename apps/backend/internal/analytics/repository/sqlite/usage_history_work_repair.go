package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

func (r *Repository) repairUsageWorkBatch(ctx context.Context, limit int) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	scope, err := r.nextUsageRepairScopeTx(ctx, tx)
	if err != nil {
		return false, err
	}
	if scope == "" {
		return true, tx.Commit()
	}
	if _, _, err := r.lockUsageProjectionScopeTx(ctx, tx, scope); err != nil {
		return false, err
	}
	worked, err := r.repairUsageContextPageTx(ctx, tx, scope, limit)
	if err != nil {
		return false, err
	}
	if !worked {
		if _, err := r.repairUsageScopePageTx(ctx, tx, scope, limit); err != nil {
			return false, err
		}
	}
	next, err := r.nextUsageRepairScopeTx(ctx, tx)
	if err != nil {
		return false, err
	}
	return next == "", tx.Commit()
}

func (r *Repository) repairUsageWorkRowTx(ctx context.Context, tx *sqlx.Tx, ref selectionReference) (models.UsageMutation, error) {
	before, err := r.loadIndexedObservationTx(ctx, tx, ref.Table, ref.ID)
	if err != nil {
		return models.UsageMutation{}, err
	}
	if before.Metadata == "" {
		err = r.indexLegacyUsageTx(ctx, tx, before.Table, before.Measurement)
	} else {
		err = r.indexContractMetadataTx(ctx, tx, before)
	}
	if err != nil {
		return models.UsageMutation{}, err
	}
	after, err := r.loadIndexedObservationTx(ctx, tx, ref.Table, ref.ID)
	if err != nil {
		return models.UsageMutation{}, err
	}
	if before.WorkKey == "" {
		return r.recordUsageMutationTx(ctx, tx, "index", nil, &after)
	}
	return r.recordUsageMutationTx(ctx, tx, "index", &before, &after)
}

func (r *Repository) invalidateUsageWorkContextTx(ctx context.Context, tx *sqlx.Tx, target retainedTarget) error {
	if target.ExternalSessionID == "" || target.SourceScopeStatus != usageSourceEligible {
		return nil
	}
	var previous string
	err := tx.GetContext(ctx, &previous, r.db.Rebind(`SELECT external_session_id FROM usage_collection_bindings WHERE scope_id=? AND subject_id = ? ORDER BY sequence DESC LIMIT 1`), target.ScopeID, target.SubjectID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && previous == target.ExternalSessionID {
		return nil
	}
	return r.enqueueUsageSubjectRepairTx(ctx, tx, target.ScopeID, target.SubjectID, transcriptUsageWork(target.ScopeID, target.SubjectID, target.ExternalSessionID))
}

func (r *Repository) usageWorkVersionTx(ctx context.Context, tx *sqlx.Tx, scope string) (int64, error) {
	var version int64
	err := tx.GetContext(ctx, &version, r.db.Rebind(`SELECT work_version FROM usage_scopes WHERE scope_id = ?`), scope)
	return version, err
}
