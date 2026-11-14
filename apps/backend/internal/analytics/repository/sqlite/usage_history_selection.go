package sqlite

import (
	"context"
	"encoding/base64"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
)

type selectionReference struct {
	ID       string `db:"id"`
	Table    string `db:"source_table"`
	Coverage string `db:"selection_coverage"`
}

// SelectUsagePartitionTx resolves at most 200 source candidates in one work
// group. Its cursor advances only over complete equivalence groups. A larger
// indivisible conflict returns explicit preparation state.
func (r *Repository) SelectUsagePartitionTx(ctx context.Context, tx *sqlx.Tx, query models.UsageSelectionQuery) (models.UsageSelectionPage, error) {
	page := models.UsageSelectionPage{}
	if tx == nil || query.Partition.ScopeID == "" {
		return page, errors.New("selector requires a transaction and retained scope")
	}
	scope, work, preparing, err := r.prepareUsageSelectionTx(ctx, tx, query.Partition)
	if err != nil {
		return page, err
	}
	return r.selectUsageWorkTx(ctx, tx, query, scope, work, preparing)
}

func (r *Repository) selectUsageWorkTx(ctx context.Context, tx *sqlx.Tx, query models.UsageSelectionQuery, scope, work string, preparing bool) (models.UsageSelectionPage, error) {
	page := models.UsageSelectionPage{}
	if work == "" {
		page.Preparing = preparing
		return page, nil
	}
	choice, err := r.usageSegmentChoiceTx(ctx, tx, scope, work)
	if err != nil {
		return page, err
	}
	if choice.Enabled {
		return r.selectUsageSegmentedWorkTx(ctx, tx, query, scope, work, choice)
	}
	cursor, err := decodeUsageSelectionCursor(query.Cursor, work)
	if err != nil {
		return page, err
	}
	refs, err := r.selectionReferencesTx(ctx, tx, scope, work, cursor, nil)
	if err != nil {
		return page, err
	}
	page.VisitedRows = len(refs)
	refs, page.HasMore, page.Preparing = boundUsageSelectionReferences(refs)
	if page.Preparing {
		return page, nil
	}
	limit := query.Limit
	if limit < 1 || limit > 200 {
		limit = 200
	}
	selected, last, err := r.selectUsageReferenceGroupsTx(ctx, tx, refs, limit, cursor)
	if err != nil {
		return page, err
	}
	page.Facts, page.Preparing = selected.Facts, selected.Preparing
	page.HasMore = page.HasMore || selected.HasMore
	cursor = last
	if page.HasMore && cursor != "" {
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(work + "\x00" + cursor))
	}
	return page, nil
}

func (r *Repository) selectionReferencesTx(ctx context.Context, tx *sqlx.Tx, scope, work, after string, overlap *indexedObservation) ([]selectionReference, error) {
	var selects []string
	var args []any
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		if overlap == nil {
			query := `SELECT id, '` + table + `' AS source_table, selection_coverage FROM ` + table + ` WHERE scope_id = ? AND selection_partition = ? AND retired = 0`
			query += ` AND selection_coverage > ?`
			selects = append(selects, query)
			args = append(args, scope, work, after)
			continue
		}
		pointPredicate, pointArgs, coarsePredicate, coarseArgs := usageSelectionReferencePredicates(*overlap)
		for _, branch := range []struct {
			predicate string
			args      []any
			index     string
		}{{pointPredicate, pointArgs, table + "_interval"}, {coarsePredicate, coarseArgs, table + "_coarse_interval_end"}} {
			index := ""
			if r.db.DriverName() == usageDriverSQLite {
				index = ` INDEXED BY idx_` + branch.index
			}
			query := `SELECT id, '` + table + `' AS source_table, selection_coverage FROM ` + table + index + ` WHERE scope_id = ? AND selection_partition = ? AND retired = 0` + branch.predicate + ` AND selection_coverage <> ?`
			if overlap.EventID != "" {
				query += ` AND (work_event_id = '' OR work_event_id = ?)`
			}
			selects = append(selects, query)
			args = append(args, scope, work)
			args = append(args, branch.args...)
			args = append(args, overlap.CoverageKey)
			if overlap.EventID != "" {
				args = append(args, overlap.EventID)
			}
		}
	}
	query := strings.Join(selects, ` UNION ALL `)
	if overlap == nil {
		query += ` ORDER BY selection_coverage, id`
	} else {
		// The overlap callers treat any 201 references as preparing before loading
		// them. When at most 200 match, every reference is present and sorting in
		// memory preserves deterministic processing without making SQL scan history.
		query = `SELECT id, source_table, selection_coverage FROM (` + query + `) overlap_references`
	}
	query += ` LIMIT 201`
	var refs []selectionReference
	err := tx.SelectContext(ctx, &refs, r.db.Rebind(query), args...)
	if overlap != nil {
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].Coverage != refs[j].Coverage {
				return refs[i].Coverage < refs[j].Coverage
			}
			if refs[i].ID != refs[j].ID {
				return refs[i].ID < refs[j].ID
			}
			return refs[i].Table < refs[j].Table
		})
	}
	return refs, err
}

func usageSelectionReferencePredicates(row indexedObservation) (string, []any, string, []any) {
	_, bounds := usageSelectionOverlap(row)
	end, start := bounds[0], bounds[1]
	point := ` AND coverage_start >= ? AND coverage_start < ? AND (source = 'native' OR coverage_key LIKE 'point:%')`
	coarse := ` AND coverage_start < ? AND coverage_end > ? AND source <> 'native' AND coverage_key NOT LIKE 'point:%'`
	return point, []any{start, end}, coarse, []any{end, start}
}

func selectionCandidateBetter(a, b indexedObservation) bool {
	if (a.WorkStatus == usageIdentityProven) != (b.WorkStatus == usageIdentityProven) {
		return a.WorkStatus == usageIdentityProven
	}
	left, right := a.Measurement, b.Measurement
	if usageCoverageRank(left.Coverage) != usageCoverageRank(right.Coverage) {
		return usageCoverageRank(left.Coverage) > usageCoverageRank(right.Coverage)
	}
	if (left.Source == usageSourceNative) != (right.Source == usageSourceNative) {
		return left.Source == usageSourceNative
	}
	if left.Source != right.Source {
		return left.Source < right.Source
	}
	if tokenCompletenessRank(left) != tokenCompletenessRank(right) {
		return tokenCompletenessRank(left) > tokenCompletenessRank(right)
	}
	if left.Stale != right.Stale {
		return !left.Stale
	}
	if left.Revision != right.Revision {
		return left.Revision > right.Revision
	}
	if a.CoverageKey != b.CoverageKey {
		return a.CoverageKey < b.CoverageKey
	}
	return a.ItemID < b.ItemID
}

func selectionCostCandidateBetter(a, b indexedObservation) bool {
	left, right := a.Measurement, b.Measurement
	if usageCoverageRank(left.CostCoverage) != usageCoverageRank(right.CostCoverage) {
		return usageCoverageRank(left.CostCoverage) > usageCoverageRank(right.CostCoverage)
	}
	if (left.CostSubcents != nil) != (right.CostSubcents != nil) {
		return left.CostSubcents != nil
	}
	if usageCostBasisRank(left.CostBasis) != usageCostBasisRank(right.CostBasis) {
		return usageCostBasisRank(left.CostBasis) > usageCostBasisRank(right.CostBasis)
	}
	if left.Stale != right.Stale {
		return !left.Stale
	}
	if (left.Source == usageSourceNative) != (right.Source == usageSourceNative) {
		return left.Source == usageSourceNative
	}
	if left.Source != right.Source {
		return left.Source < right.Source
	}
	if left.Revision != right.Revision {
		return left.Revision > right.Revision
	}
	return a.ItemID < b.ItemID
}

func selectEquivalentToken(group []indexedObservation) indexedObservation {
	winner := group[0]
	for _, row := range group[1:] {
		if selectionCandidateBetter(row, winner) {
			winner = row
		}
	}
	return winner
}

func selectEquivalentObservations(group []indexedObservation) models.UsageCanonicalFact {
	tokens := selectEquivalentToken(group)
	partial := tokens.WorkStatus != usageIdentityProven || tokens.Measurement.Coverage != usageCoverageComplete
	cost, conflicting, currencyConflict := selectEquivalentCost(group, tokens)
	partial = partial || conflicting
	m := cloneUsageMeasurement(tokens.Measurement)
	m.CostSubcents, m.Currency, m.CostBasis, m.CostCoverage = cloneInt64(cost.Measurement.CostSubcents), cost.Measurement.Currency, cost.Measurement.CostBasis, cost.Measurement.CostCoverage
	m.Estimated = cost.Measurement.Estimated || cost.Measurement.CostBasis == models.UsageCostBasisEstimated
	m.Stale = m.Stale || cost.Measurement.Stale
	if cost.Measurement.CollectedAt.After(m.CollectedAt) {
		m.CollectedAt = cost.Measurement.CollectedAt
	}
	if currencyConflict {
		m.CostCoverage = usageCoveragePartial
	}
	if partial {
		m.Coverage = usageCoveragePartial
		m.AttributionStatus = "ambiguous"
	}
	if tokens.Measurement.Source != cost.Measurement.Source {
		m.Source = usageMixedValue
	}
	fact := models.UsageCanonicalFact{Key: uuid.NewSHA1(uuid.NameSpaceURL, []byte("usage-fact:"+m.ScopeID+":"+tokens.WorkKey+":"+tokens.CoverageKey)).String(), Partition: tokens.partition(), Measurement: m, TokenSource: metricSource(tokens), CoverageKind: canonicalCoverageKind(tokens.Measurement), TokenSemantics: tokens.semantics(), Partial: partial}
	// Readers compare source decompositions before matching member lineage.
	// Preserve both identities on the unsegmented and legacy selection path.
	fact.SegmentKey, fact.BundleKey = tokens.Segment, tokens.Bundle
	if cost.Measurement.CostSubcents != nil {
		fact.CostSource = metricSource(cost)
	}
	if partial {
		fact.Reason = "incomplete_or_conflicting_evidence"
	}
	applySharedUsageAttribution(&fact, group, tokens)
	fact.NativeAccountingVersion = normalizeNativeCanonicalAccounting(&fact, fact.NativeAccountingVersion)
	return fact
}

func metricSource(row indexedObservation) *models.UsageMetricSource {
	m := row.Measurement
	return &models.UsageMetricSource{Source: m.Source, ItemID: row.ItemID, ActorID: m.ActorID, ActorStatus: m.ActorStatus, ActorKind: m.ActorKind}
}

func (r *Repository) prepareUsageSelectionTx(ctx context.Context, tx *sqlx.Tx, partition models.UsagePartition) (string, string, bool, error) {
	var workspace string
	if err := tx.GetContext(ctx, &workspace, r.db.Rebind(`SELECT workspace_id FROM usage_scopes WHERE scope_id = ?`), partition.ScopeID); err != nil {
		return "", "", false, ErrUsageNotVisible
	}
	scope, err := r.authorizedUsageScopeTx(ctx, tx, workspace)
	if err != nil || scope != partition.ScopeID {
		return "", "", false, ErrUsageNotVisible
	}
	resolved, _, err := r.ResolveUsagePartitionsTx(ctx, tx, partition, "", 2)
	if err != nil {
		return "", "", false, err
	}
	if len(resolved) == 0 {
		return scope, "", false, nil
	}
	if len(resolved) > 1 {
		return scope, "", true, nil
	}
	return scope, resolved[0].WorkKey, false, nil

}

func decodeUsageSelectionCursor(value, work string) (string, error) {
	cursor := ""
	if value != "" {
		raw, err := base64.RawURLEncoding.DecodeString(value)
		if err != nil {
			return "", errors.New("invalid selection cursor")
		}
		parts := strings.SplitN(string(raw), "\x00", 2)
		if len(parts) != 2 || parts[0] != work {
			return "", errors.New("selection cursor belongs to another partition")
		}
		cursor = parts[1]
	}
	return cursor, nil
}

func (r *Repository) selectUsageReferenceGroupsTx(ctx context.Context, tx *sqlx.Tx, refs []selectionReference, limit int, cursor string) (models.UsageSelectionPage, string, error) {
	page := models.UsageSelectionPage{}
	for pos := 0; pos < len(refs); {
		end := pos + 1
		for end < len(refs) && refs[end].Coverage == refs[pos].Coverage {
			end++
		}
		if len(page.Facts) >= limit {
			page.HasMore = true
			break
		}
		fact, keep, preparing, err := r.selectUsageReferenceGroupTx(ctx, tx, refs[pos:end])
		if err != nil {
			return page, cursor, err
		}
		if preparing {
			page.Preparing, page.HasMore = true, true
			break
		}
		if keep {
			page.Facts = append(page.Facts, fact)
		}
		cursor = refs[pos].Coverage
		pos = end
	}
	return page, cursor, nil
}

func (r *Repository) selectUsageReferenceGroupTx(ctx context.Context, tx *sqlx.Tx, refs []selectionReference) (models.UsageCanonicalFact, bool, bool, error) {
	group := make([]indexedObservation, 0, len(refs))
	for _, ref := range refs {
		row, err := r.loadIndexedObservationTx(ctx, tx, ref.Table, ref.ID)
		if err != nil {
			return models.UsageCanonicalFact{}, false, false, err
		}
		group = append(group, row)
	}
	return r.selectUsageOverlapTx(ctx, tx, group)

}

func canonicalCoverageKind(m models.SessionUsageMeasurement) string {
	if m.CoverageStart == nil {
		return usageLifetime
	}
	if m.Source == usageSourceNative || strings.HasPrefix(m.CoverageKey, "point:") ||
		(m.CoverageEnd != nil && m.CoverageEnd.Sub(*m.CoverageStart) == time.Nanosecond) {
		return usageCoveragePoint
	}
	if m.SourceDate != "" {
		return "calendar"
	}
	return "interval"
}

func usageSelectionOverlap(row indexedObservation) (string, []any) {
	m := row.Measurement
	end := *m.CoverageEnd
	if canonicalCoverageKind(m) == usageCoveragePoint && !end.After(*m.CoverageStart) {
		end = m.CoverageStart.Add(time.Microsecond)
	}
	return ` AND coverage_start < ? AND (coverage_end > ? OR (coverage_start = ? AND (source = 'native' OR coverage_key LIKE 'point:%')))`,
		[]any{end, m.CoverageStart, m.CoverageStart}
}

func selectEquivalentCost(group []indexedObservation, tokens indexedObservation) (indexedObservation, bool, bool) {
	cost := tokens
	partial := false
	currencies := map[string]bool{}
	for _, row := range group {
		if row.Measurement.Model != tokens.Measurement.Model || row.Measurement.Provider != tokens.Measurement.Provider || row.semantics() != tokens.semantics() {
			partial = true
		}
		if row.Measurement.CostSubcents != nil {
			currencies[row.Measurement.Currency] = true
		}
		if usageCostDimensionsMatch(row, tokens) && selectionCostCandidateBetter(row, cost) {
			cost = row
		}
	}
	if len(currencies) > 1 {
		partial = true
	}
	return cost, partial, len(currencies) > 1
}

func boundUsageSelectionReferences(refs []selectionReference) ([]selectionReference, bool, bool) {
	if len(refs) <= maxSessionUsageBatch {
		return refs, false, false
	}
	boundary := refs[maxSessionUsageBatch].Coverage
	refs = refs[:maxSessionUsageBatch]
	for len(refs) > 0 && refs[len(refs)-1].Coverage == boundary {
		refs = refs[:len(refs)-1]
	}
	return refs, true, len(refs) == 0
}
