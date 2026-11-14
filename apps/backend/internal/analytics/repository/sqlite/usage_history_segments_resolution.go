package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"

	"github.com/jmoiron/sqlx"
)

type usageSegmentResolution struct {
	Phase        string
	AfterGroup   string
	Assessment   usageSegmentAssessment
	Reduction    usageSegmentReduction
	Window       usageSegmentWindow
	Lower, Upper string
}

func (r *Repository) resolveUsageSegmentPageTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, choice usageSegmentChoice, limit int) error {
	state, err := r.loadUsageSegmentResolutionTx(ctx, tx, work)
	if err != nil {
		return err
	}
	limit = max(1, limit-choice.Probes-1)
	if state.Phase == "assessment" {
		return r.assessUsageSegmentPageTx(ctx, tx, work, state, limit)
	}
	if state.Reduction.Group == "" {
		ready, err := r.beginUsageSegmentGroupTx(ctx, tx, work, &state, choice)
		if err != nil || !ready {
			return err
		}
	}
	return r.reduceUsageSegmentGroupTx(ctx, tx, work, state, limit)
}

func (r *Repository) loadUsageSegmentResolutionTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork) (usageSegmentResolution, error) {
	state := usageSegmentResolution{Phase: "groups"}
	var payload string
	err := tx.GetContext(ctx, &payload, r.db.Rebind(`SELECT payload FROM usage_projection_resolutions WHERE scope_id=? AND generation=? AND work_key=?`), work.Scope, work.Generation, work.Work)
	if errors.Is(err, sql.ErrNoRows) {
		return state, nil
	}
	if err == nil {
		err = decodeUsageProjection(payload, &state)
	}
	return state, err
}

func (r *Repository) beginUsageSegmentGroupTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, state *usageSegmentResolution, choice usageSegmentChoice) (bool, error) {
	refs, err := r.usageSegmentReferencesTx(ctx, tx, work.Scope, work.Work, state.AfterGroup, "", "", false, 1)
	if err != nil {
		return false, err
	}
	if len(refs) == 0 {
		return false, r.finishUsageSegmentResolutionTx(ctx, tx, work)
	}
	row, err := r.loadIndexedObservationTx(ctx, tx, refs[0].Table, refs[0].ID)
	if err != nil {
		return false, err
	}
	if !state.Assessment.Choice.Ready || !state.Window.reusable(row) {
		state.Window = usageSegmentRowWindow(row)
		state.Lower, state.Upper, err = r.usageSegmentWindowBoundsTx(ctx, tx, work.Scope, work.Work, state.Window)
		if err != nil {
			return false, err
		}
		state.Assessment = usageSegmentAssessment{Choice: choice}
		state.Phase = "assessment"
		return false, r.saveUsageSegmentResolutionTx(ctx, tx, work, *state)
	}
	state.Reduction, err = r.usageSegmentGroupStateTx(ctx, tx, state.Assessment.Choice, row)
	return true, err
}

func (r *Repository) reduceUsageSegmentGroupTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, state usageSegmentResolution, limit int) error {
	group := &state.Reduction
	refs, err := r.usageSegmentReferencesTx(ctx, tx, work.Scope, work.Work, group.Group, group.AfterTable, group.AfterID, true, limit)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		row, err := r.loadIndexedObservationTx(ctx, tx, ref.Table, ref.ID)
		if err != nil {
			return err
		}
		group.add(row)
		group.AfterTable, group.AfterID = ref.Table, ref.ID
	}
	if len(refs) == limit {
		return r.saveUsageSegmentResolutionTx(ctx, tx, work, state)
	}
	if fact, keep := group.fact(); keep {
		if err := r.stageUsageProjectionFactTx(ctx, tx, work, fact); err != nil {
			return err
		}
	}
	state.AfterGroup, state.Reduction = group.Group, usageSegmentReduction{}
	cursor := base64.RawURLEncoding.EncodeToString([]byte(work.Work + "\x00" + state.AfterGroup))
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_work SET cursor=? WHERE scope_id=? AND generation=? AND work_key=?`), cursor, work.Scope, work.Generation, work.Work); err != nil {
		return err
	}
	return r.saveUsageSegmentResolutionTx(ctx, tx, work, state)
}

func (r *Repository) assessUsageSegmentPageTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, state usageSegmentResolution, limit int) error {
	rows, err := r.usageSegmentScanRangeTx(ctx, tx, work.Scope, work.Work, state.Assessment.AfterKey, state.Lower, state.Upper, limit)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if state.Window.overlaps(row) {
			state.Assessment.add(row)
		}
		state.Assessment.AfterKey = row.ScanKey
	}
	if len(rows) < limit {
		state.Assessment.Choice = state.Assessment.finish()
		choice := state.Assessment.Choice
		if choice.TokenQuality.Coarse && (!choice.CostQuality.Present || choice.TokenQuality.Coverage == choice.CostQuality.Coverage) {
			state.Window = choice.TokenQuality.Window
		}
		state.Phase = "groups"
	}
	return r.saveUsageSegmentResolutionTx(ctx, tx, work, state)
}

func (r *Repository) finishUsageSegmentResolutionTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork) error {
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_projection_resolutions WHERE scope_id=? AND generation=? AND work_key=?`), work.Scope, work.Generation, work.Work); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_work SET state='complete' WHERE scope_id=? AND generation=? AND work_key=?`), work.Scope, work.Generation, work.Work); err != nil {
		return err
	}
	return r.publishUsageProjectionGroupTx(ctx, tx, work)
}

func (r *Repository) saveUsageSegmentResolutionTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, state usageSegmentResolution) error {
	payload, err := encodeUsageProjection(state)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_resolutions(scope_id,generation,work_key,payload) VALUES (?,?,?,?)
		ON CONFLICT(scope_id,generation,work_key) DO UPDATE SET payload=excluded.payload`), work.Scope, work.Generation, work.Work, payload)
	return err
}
