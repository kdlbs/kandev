package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

type usageContextRepair struct {
	Scope      string `db:"scope_id"`
	Subject    string `db:"subject_id"`
	AfterTable string `db:"work_repair_after_table"`
	AfterID    string `db:"work_repair_after_id"`
}

func (r *Repository) enqueueUsageSubjectRepairTx(ctx context.Context, tx *sqlx.Tx, scope, subject, newWork string) error {
	needed := false
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		var id string
		err := tx.GetContext(ctx, &id, r.db.Rebind(`SELECT id FROM `+table+` WHERE scope_id=? AND subject_id=? AND work_status='unproven' AND source<>'native' LIMIT 1`), scope, subject)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		needed = needed || id != ""
	}
	if !needed {
		return nil
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_subjects SET work_repair_pending=1,work_repair_after_table='',work_repair_after_id='',work_repair_new_work=? WHERE scope_id=? AND subject_id=?`), newWork, scope, subject)
	return err
}

func (r *Repository) repairUsageContextPageTx(ctx context.Context, tx *sqlx.Tx, scope string, limit int) (bool, error) {
	var state usageContextRepair
	err := tx.GetContext(ctx, &state, r.db.Rebind(`SELECT scope_id,subject_id,work_repair_after_table,work_repair_after_id FROM usage_subjects WHERE work_repair_pending=1 AND scope_id=? ORDER BY subject_id LIMIT 1`), scope)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	table := state.AfterTable
	if table == "" {
		table = usageBucketsTable
	}
	var refs []selectionReference
	query := `SELECT id,'` + table + `' AS source_table,selection_coverage FROM ` + table + ` WHERE scope_id=? AND subject_id=? AND work_status='unproven' AND source<>'native' AND id>? ORDER BY id LIMIT ?`
	if err := tx.SelectContext(ctx, &refs, r.db.Rebind(query), scope, state.Subject, state.AfterID, limit); err != nil {
		return true, err
	}
	var changes []models.UsageMutation
	for _, ref := range refs {
		change, err := r.repairUsageWorkRowTx(ctx, tx, ref)
		if err != nil {
			return true, err
		}
		changes = append(changes, change)
		state.AfterID = ref.ID
	}
	if err := r.groupUsageProjectionMutationsTx(ctx, tx, changes); err != nil {
		return true, err
	}
	pending := 1
	if len(refs) < limit {
		if table == usageBucketsTable {
			table, state.AfterID = usageMeasurementsTable, ""
		} else {
			pending = 0
		}
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_subjects SET work_repair_pending=?,work_repair_after_table=?,work_repair_after_id=? WHERE scope_id=? AND subject_id=?`), pending, table, state.AfterID, scope, state.Subject)
	return true, err
}

func (r *Repository) usageContextPublicationBlockedTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork) (bool, error) {
	var blocked bool
	err := tx.GetContext(ctx, &blocked, r.db.Rebind(`SELECT EXISTS(SELECT 1 FROM usage_subjects s JOIN usage_projection_work w ON w.scope_id=s.scope_id
 WHERE s.scope_id=? AND s.work_repair_pending=1 AND w.generation=? AND w.publish_group=?
 AND (w.subject_id=s.subject_id OR w.work_key=s.work_repair_new_work))`), work.Scope, work.Generation, work.Group)
	return blocked, err
}
