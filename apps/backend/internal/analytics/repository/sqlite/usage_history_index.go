package sqlite

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

// BackfillUsageSelection resumes from durable unindexed source rows. Accepted
// writers index their own rows, so no mutable offset or full-history scan is needed.
func (r *Repository) BackfillUsageSelection(ctx context.Context, limit int) (bool, error) {
	if limit < 1 || limit > 200 {
		limit = 200
	}
	return r.repairUsageWorkBatch(ctx, limit)
}

// ResolveUsagePartitionsTx expands old coverage-only dirty keys and current
// dimension keys into bounded work groups for the canonical selector.
func (r *Repository) ResolveUsagePartitionsTx(ctx context.Context, tx *sqlx.Tx, partition models.UsagePartition, cursor string, limit int) ([]models.UsagePartition, string, error) {
	if limit < 1 || limit > 200 {
		limit = 200
	}
	work := partition.WorkKey
	if work == "" {
		prefix := strings.SplitN(partition.PartitionID, ":", 2)[0]
		if _, err := uuid.Parse(prefix); err == nil {
			work = prefix
		}
	}
	if work != "" {
		partition.WorkKey = work
		return []models.UsagePartition{partition}, "", nil
	}
	coverage := partition.PartitionID
	if coverage == usageLifetime {
		coverage = ""
	}
	var works []string
	query := `SELECT selection_partition FROM session_usage_measurements WHERE scope_id = ? AND subject_id = ? AND coverage_key = ? AND selection_partition > ?
		UNION SELECT selection_partition FROM session_usage_buckets WHERE scope_id = ? AND subject_id = ? AND coverage_key = ? AND selection_partition > ? ORDER BY selection_partition LIMIT ?`
	err := tx.SelectContext(ctx, &works, r.db.Rebind(query), partition.ScopeID, partition.SubjectID, coverage, cursor, partition.ScopeID, partition.SubjectID, coverage, cursor, limit+1)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(works) > limit {
		works = works[:limit]
		next = works[len(works)-1]
	}
	out := make([]models.UsagePartition, 0, len(works))
	for _, key := range works {
		out = append(out, models.UsagePartition{ScopeID: partition.ScopeID, SubjectID: partition.SubjectID, WorkKey: key, PartitionID: key})
	}
	return out, next, nil
}

// UsageSourceUpperBoundsTx captures source keys under the same retained-scope
// fence as admission. The projection owner pairs these bounds with its replay
// sequence to include later inserts and corrections during a rebuild.
func (r *Repository) UsageSourceUpperBoundsTx(ctx context.Context, tx *sqlx.Tx, scope string) (map[string]string, error) {
	var workspace string
	if err := tx.GetContext(ctx, &workspace, r.db.Rebind(`SELECT workspace_id FROM usage_scopes WHERE scope_id = ?`), scope); err != nil {
		return nil, ErrUsageNotVisible
	}
	current, err := r.authorizedUsageScopeTx(ctx, tx, workspace)
	if err != nil || current != scope {
		return nil, ErrUsageNotVisible
	}
	return r.usageSourceBoundsTx(ctx, tx, scope)
}

func (r *Repository) usageSourceBoundsTx(ctx context.Context, tx *sqlx.Tx, scope string) (map[string]string, error) {
	bounds := make(map[string]string, 2)
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		var upper string
		if err := tx.GetContext(ctx, &upper, r.db.Rebind(`SELECT COALESCE(MAX(id), '') FROM `+table+` WHERE scope_id = ?`), scope); err != nil {
			return nil, err
		}
		bounds[table] = upper
	}
	return bounds, nil
}

func (r *Repository) indexContractMetadataTx(ctx context.Context, tx *sqlx.Tx, row indexedObservation) error {
	var input pluginsdk.UsageObservationV1
	if err := json.Unmarshal([]byte(row.Metadata), &input); err != nil {
		return err
	}
	var target retainedTarget
	if err := tx.GetContext(ctx, &target, r.db.Rebind(`SELECT * FROM usage_collection_bindings WHERE binding_id = ?`), input.SubjectBinding); err != nil {
		return err
	}
	work, status, eventID, err := r.resolveContractWorkIdentityTx(ctx, tx, target, row.Measurement.Source, input.WorkIdentity)
	if err != nil {
		return err
	}
	coverage := contractCoverageKey(row.Measurement)
	if eventID != "" {
		coverage = "event:" + eventID + ":" + coverage
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE `+row.Table+` SET selection_partition = ?, selection_coverage = ?, work_status = ?, work_event_id = ?, selection_version = (SELECT work_version FROM usage_scopes WHERE scope_id = ?) WHERE id = ?`), work, coverage, status, eventID, row.Measurement.ScopeID, row.Measurement.ID); err != nil {
		return err
	}
	row.WorkKey, row.CoverageKey, row.WorkStatus, row.EventID = work, coverage, status, eventID
	return r.persistUsageSegmentTx(ctx, tx, &row, &input)
}
