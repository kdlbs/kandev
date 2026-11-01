package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

// Only the current winners and cursors are retained. Candidate rows are read
// through the source indexes in bounded pages, rather than copied into a shadow
// source store or loaded as an indivisible equivalence group.
type usageProjectionResolution struct {
	Coverage          string
	Phase             string
	AfterTable        string
	AfterID           string
	Tokens            *indexedObservation
	Cost              *indexedObservation
	Dimensions        string
	DimensionConflict bool
	Currency          string
	CurrencyConflict  bool
	Keep              bool
	Ambiguous         bool
	Representative    *indexedObservation
	Native            *usageProjectionNativeCoverage
	PendingOther      *indexedObservation
	LegacySource      string
	LegacyConflict    bool
}

const (
	projectionResolutionDone    = "done"
	projectionResolutionOverlap = "overlap"
	projectionResolutionNative  = "native_coverage"
)

func (r *Repository) resolveUsageProjectionPageTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, limit int) error {
	choice, err := r.usageSegmentChoiceTx(ctx, tx, work.Scope, work.Work)
	if err != nil {
		return err
	}
	if choice.Enabled {
		return r.resolveUsageSegmentPageTx(ctx, tx, work, choice, limit)
	}
	state, err := r.loadUsageProjectionResolutionTx(ctx, tx, work)
	if err != nil {
		return err
	}
	refs, err := r.usageProjectionResolutionRefsTx(ctx, tx, work, state, limit)
	if err != nil {
		return err
	}
	interrupted := false
	for _, ref := range refs {
		row, err := r.loadIndexedObservationTx(ctx, tx, ref.Table, ref.ID)
		if err != nil {
			return err
		}
		if state.Phase == projectionResolutionNative {
			accumulateUsageNativeCoverage(&state.Native.Total, &state.Native.Found, row, state.Native.Coarse)
			state.Native.AfterID, state.Native.AfterTable = ref.ID, ref.Table
			continue
		}
		state.AfterID, state.AfterTable = ref.ID, ref.Table
		if state.Phase != projectionResolutionOverlap && !equivalentUsageCandidate(*state.Representative, row) {
			continue
		}
		if advanceUsageProjectionResolution(&state, row) {
			interrupted = true
			break
		}
	}
	if !interrupted && len(refs) < limit {
		finishUsageProjectionResolutionPass(&state)
	}
	if state.Phase == projectionResolutionDone {
		return r.finishUsageProjectionResolutionTx(ctx, tx, work, state)
	}
	return r.saveUsageProjectionResolutionTx(ctx, tx, work, state)
}

func (r *Repository) loadUsageProjectionResolutionTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork) (usageProjectionResolution, error) {
	state := usageProjectionResolution{Phase: projectionMetricTokens, Keep: true}
	var payload string
	err := tx.GetContext(ctx, &payload, r.db.Rebind(`SELECT payload FROM usage_projection_resolutions WHERE scope_id = ? AND generation = ? AND work_key = ?`), work.Scope, work.Generation, work.Work)
	if err == nil {
		state = usageProjectionResolution{}
		err = decodeUsageProjection(payload, &state)
		if err == nil {
			err = r.ensureUsageResolutionRepresentativeTx(ctx, tx, work, &state)
		}
		return state, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return state, err
	}
	after, err := decodeUsageSelectionCursor(work.Cursor, work.Work)
	if err != nil {
		return state, err
	}
	state.Coverage, _, err = r.nextUsageResolutionCoverageTx(ctx, tx, work, after)
	if err == nil {
		err = r.ensureUsageResolutionRepresentativeTx(ctx, tx, work, &state)
	}
	return state, err
}

func (r *Repository) usageProjectionResolutionRefsTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, state usageProjectionResolution, limit int) ([]selectionReference, error) {
	var selects []string
	var args []any
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		query := `SELECT id, '` + table + `' AS source_table, selection_coverage FROM ` + table + ` WHERE scope_id = ? AND selection_partition = ? AND retired = 0`
		args = append(args, work.Scope, work.Work)
		predicate, bounds := usageResolutionCandidatePredicate(state)
		query += predicate
		args = append(args, bounds...)
		selects = append(selects, query)
	}
	query := `SELECT id, source_table, selection_coverage FROM (` + strings.Join(selects, ` UNION ALL `) + `) candidates
		WHERE source_table > ? OR (source_table = ? AND id > ?) ORDER BY source_table, id LIMIT ?`
	afterTable, afterID := state.AfterTable, state.AfterID
	if state.Phase == projectionResolutionNative {
		afterTable, afterID = state.Native.AfterTable, state.Native.AfterID
	}
	args = append(args, afterTable, afterTable, afterID, limit)
	var refs []selectionReference
	err := tx.SelectContext(ctx, &refs, r.db.Rebind(query), args...)
	return refs, err
}

func advanceUsageProjectionResolution(state *usageProjectionResolution, row indexedObservation) bool {
	switch state.Phase {
	case projectionMetricTokens:
		advanceUsageResolutionToken(state, row)
	case projectionMetricCost:
		if usageCostDimensionsMatch(row, *state.Tokens) && selectionCostCandidateBetter(row, *state.Cost) {
			copy := row
			state.Cost = &copy
		}
	case projectionResolutionOverlap:
		return advanceUsageResolutionOverlap(state, row)
	}
	return false
}

func finishUsageProjectionResolutionPass(state *usageProjectionResolution) {
	if state.Phase != projectionResolutionNative {
		state.AfterID, state.AfterTable = "", ""
	}
	switch state.Phase {
	case projectionMetricTokens:
		state.Phase, state.Cost = projectionMetricCost, state.Tokens
	case projectionMetricCost:
		state.Phase = projectionResolutionOverlap
		if state.Tokens.Measurement.CoverageStart == nil {
			state.Phase = projectionResolutionDone
		}
	case projectionResolutionNative:
		state.Native.Ready = true
		applyUsageNativeResolution(state, *state.PendingOther)
		state.PendingOther, state.Phase = nil, projectionResolutionOverlap
	case projectionResolutionOverlap:
		state.Phase = projectionResolutionDone
	}
}

func (r *Repository) finishUsageProjectionResolutionTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, state usageProjectionResolution) error {
	if state.Tokens == nil || state.Cost == nil {
		return errors.New("usage resolution lost its source winner")
	}
	if state.Keep && state.Tokens.CoverageKey == state.Coverage {
		fact := selectEquivalentObservations([]indexedObservation{*state.Tokens, *state.Cost})
		applyUsageProjectionResolutionQuality(&fact, state)
		if err := r.stageUsageProjectionFactTx(ctx, tx, work, fact); err != nil {
			return err
		}
	}
	cursor := base64.RawURLEncoding.EncodeToString([]byte(work.Work + "\x00" + state.Coverage))
	status := "selecting"
	carry, err := r.carryUsageNativeCoverageTx(ctx, tx, work, state)
	if err != nil {
		return err
	}
	if carry {
		status = "resolve"
	} else if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_projection_resolutions WHERE scope_id = ? AND generation = ? AND work_key = ?`), work.Scope, work.Generation, work.Work); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_work SET state = ?, cursor = ? WHERE scope_id = ? AND generation = ? AND work_key = ?`), status, cursor, work.Scope, work.Generation, work.Work)
	return err
}

func applyUsageProjectionResolutionQuality(fact *models.UsageCanonicalFact, state usageProjectionResolution) {
	if state.DimensionConflict || state.CurrencyConflict {
		fact.Partial, fact.Reason = true, "incomplete_or_conflicting_evidence"
		fact.Measurement.Coverage, fact.Measurement.AttributionStatus = usageCoveragePartial, models.UsageAttributionAmbiguous
	}
	if state.CurrencyConflict {
		fact.Measurement.CostCoverage = usageCoveragePartial
	}
	if state.LegacyConflict {
		fact.Partial, fact.Reason = true, "legacy_equivalence_unknown"
		fact.Measurement.Coverage = usageCoveragePartial
	}
	if state.Ambiguous {
		markUsageOverlapAmbiguous(fact)
	}
}

func advanceUsageResolutionToken(state *usageProjectionResolution, row indexedObservation) {
	if state.Tokens == nil || selectionCandidateBetter(row, *state.Tokens) {
		copy := row
		state.Tokens = &copy
	}
	dimensions := strings.Join([]string{row.Measurement.Model, row.Measurement.Provider, row.semantics()}, "\x00")
	if state.Dimensions == "" {
		state.Dimensions = dimensions
	}
	state.DimensionConflict = state.DimensionConflict || state.Dimensions != dimensions
	if row.Metadata == "" && row.Measurement.Source != usageSourceNative {
		if state.LegacySource == "" {
			state.LegacySource = row.Measurement.Source
		}
		state.LegacyConflict = state.LegacyConflict || state.LegacySource != row.Measurement.Source
	}
	if row.Measurement.CostSubcents != nil {
		if state.Currency == "" {
			state.Currency = row.Measurement.Currency
		}
		state.CurrencyConflict = state.CurrencyConflict || state.Currency != row.Measurement.Currency
	}

}

func (r *Repository) saveUsageProjectionResolutionTx(ctx context.Context, tx *sqlx.Tx, work usageProjectionWork, state usageProjectionResolution) error {
	payload, err := encodeUsageProjection(state)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_resolutions (scope_id, generation, work_key, payload) VALUES (?, ?, ?, ?)
        ON CONFLICT(scope_id, generation, work_key) DO UPDATE SET payload = excluded.payload`), work.Scope, work.Generation, work.Work, payload)
	return err
}
