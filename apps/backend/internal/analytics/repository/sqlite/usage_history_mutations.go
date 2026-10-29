package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

type indexedObservation struct {
	Measurement models.SessionUsageMeasurement
	Table       string
	ItemID      string
	EventID     string
	WorkKey     string
	WorkStatus  string
	CoverageKey string
	Metadata    string
	Retired     int
	Bundle      string
	Segment     string
	GroupKey    string
	Layout      int
	TokenBundle string
	CostBundle  string
	ScanKey     string
}

func contractCoverageKey(m models.SessionUsageMeasurement) string {
	if m.CoverageStart == nil {
		return usageLifetime
	}
	kind := "interval:"
	if canonicalCoverageKind(m) == usageCoveragePoint {
		return "point:" + m.CoverageStart.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	return kind + m.CoverageStart.UTC().Format("2006-01-02T15:04:05.999999999Z07:00") + ":" + m.CoverageEnd.UTC().Format("2006-01-02T15:04:05.999999999Z07:00") + ":" + m.SourceTimezone + ":" + m.SourceDate
}

func (v indexedObservation) semantics() string {
	var metadata pluginsdk.UsageObservationV1
	if v.Metadata != "" && json.Unmarshal([]byte(v.Metadata), &metadata) == nil {
		encoded, _ := json.Marshal(metadata.TokenSemantics)
		return string(encoded)
	}
	if v.Measurement.TokenSemanticsVersion == 0 {
		return "null"
	}
	encoded, _ := json.Marshal([]any{v.Measurement.TokenSemanticsVersion, v.Measurement.CacheReadInsideInput, v.Measurement.ReasoningInsideOutput})
	return string(encoded)
}

func (v indexedObservation) partition() models.UsagePartition {
	dimensions := strings.Join([]string{v.Measurement.Model, v.Measurement.Provider, v.semantics(), v.CoverageKey}, "\x00")
	dimensionID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(dimensions)).String()
	return models.UsagePartition{ScopeID: v.Measurement.ScopeID, SubjectID: v.Measurement.SubjectID, WorkKey: v.WorkKey, PartitionID: v.WorkKey + ":" + dimensionID}
}

func (r *Repository) loadIndexedObservationTx(ctx context.Context, tx *sqlx.Tx, table, id string) (indexedObservation, error) {
	if table != usageMeasurementsTable && table != usageBucketsTable {
		return indexedObservation{}, errors.New("invalid retained observation representation")
	}
	row, err := scanUsageRow(tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT `+usageSelectColumns+` FROM `+table+` WHERE id = ?`), id))
	if err != nil {
		return indexedObservation{}, err
	}
	v := indexedObservation{Measurement: row.stored().measurement(), Table: table}
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT item_id, selection_partition, selection_coverage, work_status, observation_metadata, retired, work_event_id,
		selection_bundle,selection_segment,selection_group,selection_layout,selection_token_bundle,selection_cost_bundle,selection_scan_key FROM `+table+` WHERE id = ?`), id).
		Scan(&v.ItemID, &v.WorkKey, &v.CoverageKey, &v.WorkStatus, &v.Metadata, &v.Retired, &v.EventID,
			&v.Bundle, &v.Segment, &v.GroupKey, &v.Layout, &v.TokenBundle, &v.CostBundle, &v.ScanKey)
	return v, err
}

func (v indexedObservation) observation() *pluginsdk.UsageObservationV1 {
	if v.Metadata == "" {
		return nil
	}
	var observation pluginsdk.UsageObservationV1
	if json.Unmarshal([]byte(v.Metadata), &observation) != nil {
		return nil
	}
	m := v.Measurement
	if observation.Tokens != nil {
		observation.Tokens = &pluginsdk.UsageTokenAmountsV1{InputTokens: m.InputTokens, OutputTokens: m.OutputTokens, CacheReadTokens: m.CacheReadTokens, CacheWriteTokens: m.CacheWriteTokens, ReasoningTokens: m.ReasoningTokens, TotalTokens: m.TotalTokens}
	}
	if observation.Cost != nil {
		observation.Cost.Amount = m.CostSubcents
	}
	return &observation
}

func (r *Repository) observationReceiptTx(ctx context.Context, tx *sqlx.Tx, item observationItem, operation, status string) (pluginsdk.UsageObservationWriteResultV1, []models.UsageMutation, error) {
	result := pluginsdk.UsageObservationWriteResultV1{Operation: operation, ItemID: item.ItemID, Status: status, CurrentRevision: item.Revision, CurrentDigest: item.PayloadDigest}
	if item.SourceTable == "" {
		return result, nil, nil
	}
	current, err := r.loadIndexedObservationTx(ctx, tx, item.SourceTable, item.ObservationID)
	if err != nil {
		return result, nil, err
	}
	result.CurrentObservation = current.observation()
	result.ActorAttributionStatus, result.WorkIdentityStatus = current.Measurement.ActorStatus, current.WorkStatus
	result.AcceptedAt = current.Measurement.CollectedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	return result, nil, nil
}

func (r *Repository) recordUsageMutationTx(ctx context.Context, tx *sqlx.Tx, operation string, before, after *indexedObservation) (models.UsageMutation, error) {
	change := models.UsageMutation{Operation: operation}
	for _, pair := range []struct {
		row *indexedObservation
		old bool
	}{{before, true}, {after, false}} {
		if pair.row == nil {
			continue
		}
		row := pair.row
		partition := row.partition()
		value := row.Measurement
		payload, _ := json.Marshal(row.observation())
		change.Source, change.ItemID = value.Source, row.ItemID
		if pair.old {
			change.Before, change.OldPartition, change.BeforePayload = &value, &partition, payload
		} else {
			change.After, change.NewPartition, change.AfterPayload = &value, &partition, payload
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_dirty_partitions (scope_id, subject_id, partition_key, updated_at) VALUES (?, ?, ?, ?)
			ON CONFLICT(scope_id, subject_id, partition_key) DO UPDATE SET updated_at = excluded.updated_at`), partition.ScopeID, partition.SubjectID, partition.PartitionID, value.CollectedAt); err != nil {
			return change, err
		}
	}
	if err := r.appendUsageProjectionMutationTx(ctx, tx, &change, before, after); err != nil {
		return change, err
	}
	return change, nil
}

func (r *Repository) indexLegacyUsageTx(ctx context.Context, tx *sqlx.Tx, table string, m models.SessionUsageMeasurement) error {
	if m.ScopeID == "" || m.SubjectID == "" {
		return nil
	}
	// Reindexing preserves the source context frozen with this observation.
	// Later live-session metadata cannot establish an earlier event's thread.
	transcript := m.TranscriptID
	work := transcriptUsageWork(m.ScopeID, m.SubjectID, transcript)
	status := usageIdentityProven
	if m.Source != usageSourceNative && m.TranscriptID == "" {
		status = usageIdentityUnproven
	}
	if m.Source != usageSourceNative && m.TranscriptID != "" {
		var trusted int
		if err := tx.GetContext(ctx, &trusted, r.db.Rebind(`SELECT COUNT(*) FROM usage_collection_bindings WHERE scope_id=? AND subject_id = ? AND external_session_id = ? AND source_scope_status='eligible'`), m.ScopeID, m.SubjectID, m.TranscriptID); err != nil {
			return err
		}
		if trusted == 0 {
			status = usageIdentityUnproven
			work = uuid.NewSHA1(uuid.NameSpaceURL, []byte("usage-legacy-unproven:"+m.SubjectID+":"+m.Source+":"+m.TranscriptID)).String()
		}
	}
	itemID, eventID := m.ID, ""
	coverage := contractCoverageKey(m)
	if m.Source == usageSourceNative {
		itemID, eventID = m.SourceRecordID, m.SourceRecordID
		coverage = "event:" + eventID + ":" + coverage
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE `+table+` SET item_id = ?, selection_partition = ?, selection_coverage = ?, work_status = ?, work_event_id = ?, selection_version = (SELECT work_version FROM usage_scopes WHERE scope_id = ?) WHERE id = ? AND observation_metadata = ''`), itemID, work, coverage, status, eventID, m.ScopeID, m.ID)
	if err != nil {
		return err
	}
	row, err := r.loadIndexedObservationTx(ctx, tx, table, m.ID)
	if err != nil {
		return err
	}
	return r.persistUsageSegmentTx(ctx, tx, &row, nil)
}

func (r *Repository) indexRetainedSourceRowTx(ctx context.Context, tx *sqlx.Tx, table, id string) error {
	row, err := r.loadIndexedObservationTx(ctx, tx, table, id)
	if err != nil {
		return err
	}
	if row.WorkKey != "" {
		return nil
	}
	if err := r.indexLegacyUsageTx(ctx, tx, table, row.Measurement); err != nil {
		return err
	}
	indexed, err := r.loadIndexedObservationTx(ctx, tx, table, id)
	if err != nil {
		return err
	}
	_, err = r.recordUsageMutationTx(ctx, tx, "index", nil, &indexed)
	return err
}

func (r *Repository) findRetirableItemTx(ctx context.Context, tx *sqlx.Tx, source string, target retainedTarget, id string) (observationItem, error) {
	item, err := r.findObservationItemTx(ctx, tx, source, id)
	if !errors.Is(err, sql.ErrNoRows) {
		return item, err
	}
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		var found string
		err := tx.GetContext(ctx, &found, r.db.Rebind(`SELECT id FROM `+table+` WHERE id = ? AND source = ? AND scope_id = ? AND subject_id = ? AND retired = 0`), id, source, target.ScopeID, target.SubjectID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return item, err
		}
		row, err := r.loadIndexedObservationTx(ctx, tx, table, found)
		if err != nil {
			return item, err
		}
		_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_observation_items (source, item_id, scope_id, subject_id, binding_id, source_table, observation_id, revision, payload_digest)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(source, item_id) DO NOTHING`), source, id, target.ScopeID, target.SubjectID, target.BindingID, table, found, row.Measurement.Revision, row.Measurement.PayloadDigest)
		if err != nil {
			return item, err
		}
		return r.findObservationItemTx(ctx, tx, source, id)
	}
	return item, sql.ErrNoRows
}

func (r *Repository) tombstoneObservationTx(ctx context.Context, tx *sqlx.Tx, source string, input pluginsdk.UsageObservationTombstoneV1) (pluginsdk.UsageObservationWriteResultV1, *models.UsageMutation, error) {
	result := pluginsdk.UsageObservationWriteResultV1{Operation: "tombstone", ItemID: input.ItemID, Status: usageWriteRejected}
	digest, err := input.ComputePayloadDigestV1()
	if err != nil || digest != input.PayloadDigest || input.Revision < 1 {
		result.Error = "invalid tombstone digest or revision"
		return result, nil, nil
	}
	target, err := r.resolveCollectionBindingTx(ctx, tx, source, input.SubjectBinding)
	if err != nil {
		result.Error = ErrUsageNotVisible.Error()
		return result, nil, nil
	}
	item, err := r.findObservationItemTx(ctx, tx, source, input.ItemID)
	if errors.Is(err, sql.ErrNoRows) {
		result.Error = "source item is unavailable"
		return result, nil, nil
	}
	if err != nil {
		return result, nil, err
	}
	if item.SubjectID != target.SubjectID || item.BindingID != target.BindingID {
		result.Error = ErrUsageNotVisible.Error()
		return result, nil, nil
	}
	if err := r.admitRetainedCorrectionTx(ctx, tx, target); err != nil {
		return result, nil, err
	}
	if input.Revision <= item.Revision {
		status := contractRevisionStatus(input.Revision, input.PayloadDigest, item)
		if status == usageWriteUnchanged && item.Retired == 0 {
			status = usageWriteConflict
		}
		receipt, _, err := r.observationReceiptTx(ctx, tx, item, "tombstone", status)
		return receipt, nil, err
	}
	change, err := r.retireObservationItemTx(ctx, tx, item, input.Revision, input.PayloadDigest)
	if err != nil {
		return result, nil, err
	}
	item.Revision, item.PayloadDigest = input.Revision, input.PayloadDigest
	receipt, _, err := r.observationReceiptTx(ctx, tx, item, "tombstone", usageWriteAccepted)
	return receipt, &change, err
}

func (r *Repository) retireObservationItemTx(ctx context.Context, tx *sqlx.Tx, item observationItem, revision int64, digest string) (models.UsageMutation, error) {
	old, err := r.loadIndexedObservationTx(ctx, tx, item.SourceTable, item.ObservationID)
	if err != nil {
		return models.UsageMutation{}, err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE `+item.SourceTable+` SET retired = 1 WHERE id = ?`), item.ObservationID); err != nil {
		return models.UsageMutation{}, err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_observation_items SET retired = 1, revision = ?, payload_digest = ? WHERE source = ? AND item_id = ?`), revision, digest, item.Source, item.ItemID)
	if err != nil {
		return models.UsageMutation{}, err
	}
	return r.recordUsageMutationTx(ctx, tx, "tombstone", &old, nil)
}

func (r *Repository) admitCompleteSetTx(ctx context.Context, tx *sqlx.Tx, source string, batch pluginsdk.UsageObservationBatchV1) (string, error) {
	set := batch.CompleteSetRevision
	target, err := r.resolveCollectionBindingTx(ctx, tx, source, set.SubjectBinding)
	if err != nil {
		return usageWriteRejected, nil
	}
	if !validCompleteSetBatch(batch) {
		return usageWriteRejected, nil
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_observation_sets (source, binding_id, scope_id, set_id, revision, payload_digest) VALUES (?, ?, ?, ?, 0, '') ON CONFLICT(source, binding_id, set_id) DO NOTHING`), source, set.SubjectBinding, target.ScopeID, set.SetID)
	if err != nil {
		return "", err
	}
	query := `SELECT revision, payload_digest FROM usage_observation_sets WHERE source = ? AND binding_id = ? AND set_id = ?`
	if r.db.DriverName() == usageDriverPostgres {
		query += usageForUpdate
	}
	var revision int64
	var current string
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(query), source, set.SubjectBinding, set.SetID).Scan(&revision, &current); err != nil {
		return "", err
	}
	if set.Revision < revision {
		return "stale", nil
	}
	if set.Revision == revision && current != set.PayloadDigest {
		return usageWriteConflict, nil
	}
	return usageWriteAccepted, nil
}

func (r *Repository) finishCompleteSetTx(ctx context.Context, tx *sqlx.Tx, source string, batch pluginsdk.UsageObservationBatchV1) ([]models.UsageMutation, error) {
	set := batch.CompleteSetRevision
	var old []observationItem
	query := `SELECT * FROM usage_observation_items WHERE source = ? AND binding_id = ? AND set_id = ? AND retired = 0`
	args := []any{source, set.SubjectBinding, set.SetID}
	if len(batch.Observations) > 0 {
		query += ` AND item_id NOT IN (` + placeholders(len(batch.Observations)) + `)`
		for _, item := range batch.Observations {
			args = append(args, item.ItemID)
		}
	}
	query += ` ORDER BY item_id LIMIT 201`
	if err := tx.SelectContext(ctx, &old, r.db.Rebind(query), args...); err != nil {
		return nil, err
	}
	if len(old) > 200 {
		return nil, errors.New("complete-set retirement exceeds the bounded mutation limit")
	}
	keep := map[string]bool{}
	for _, item := range batch.Observations {
		keep[item.ItemID] = true
	}
	var changes []models.UsageMutation
	for _, item := range old {
		if !keep[item.ItemID] {
			change, err := r.retireObservationItemTx(ctx, tx, item, item.Revision, item.PayloadDigest)
			if err != nil {
				return nil, err
			}
			changes = append(changes, change)
		}
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_observation_sets SET revision = ?, payload_digest = ? WHERE source = ? AND binding_id = ? AND set_id = ?`), set.Revision, set.PayloadDigest, source, set.SubjectBinding, set.SetID)
	return changes, err
}

func validCompleteSetBatch(batch pluginsdk.UsageObservationBatchV1) bool {
	set := batch.CompleteSetRevision
	ids := make([]string, 0, len(batch.Observations))
	for _, item := range batch.Observations {
		if item.SubjectBinding != set.SubjectBinding || stringValue(item.SourceSetID) != set.SetID || item.SourceSetRevision == nil || *item.SourceSetRevision != set.Revision {
			return false
		}
		if _, err := validateContractObservation(item); err != nil {
			return false
		}
		ids = append(ids, item.ItemID)
	}
	digest, err := set.ComputePayloadDigestV1(ids)
	if err != nil || digest != set.PayloadDigest || set.Revision < 1 {
		return false
	}
	return true
}
