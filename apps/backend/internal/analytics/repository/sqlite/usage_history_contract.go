package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
)

type observationItem struct {
	Source        string `db:"source"`
	ItemID        string `db:"item_id"`
	ScopeID       string `db:"scope_id"`
	SubjectID     string `db:"subject_id"`
	BindingID     string `db:"binding_id"`
	SourceTable   string `db:"source_table"`
	ObservationID string `db:"observation_id"`
	Revision      int64  `db:"revision"`
	PayloadDigest string `db:"payload_digest"`
	Retired       int    `db:"retired"`
	SetID         string `db:"set_id"`
	SetRevision   int64  `db:"set_revision"`
}

// ReportUsageObservations owns the commit for ordinary Host calls. Projection
// integration can use MutateUsageObservationsTx inside its ordered scope Tx.
func (r *Repository) ReportUsageObservations(ctx context.Context, collector string, batch pluginsdk.UsageObservationBatchV1) (pluginsdk.UsageObservationBatchResultV1, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return pluginsdk.UsageObservationBatchResultV1{}, err
	}
	defer func() { _ = tx.Rollback() }()
	result, _, err := r.MutateUsageObservationsTx(ctx, tx, collector, batch)
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

// MutateUsageObservationsTx returns replay input and old/new invalidations in
// the same transaction as the accepted source mutation. It allocates no
// projection sequence and publishes no generation.
func (r *Repository) MutateUsageObservationsTx(ctx context.Context, tx *sqlx.Tx, collector string, batch pluginsdk.UsageObservationBatchV1) (pluginsdk.UsageObservationBatchResultV1, []models.UsageMutation, error) {
	result := pluginsdk.UsageObservationBatchResultV1{Status: usageWriteAccepted, SupportedSchemaVersion: pluginsdk.UsageObservationSchemaVersionV2}
	if tx == nil {
		return result, nil, errors.New("usage mutation requires a transaction")
	}
	source, err := collectorSource(collector)
	if err != nil {
		return result, nil, err
	}
	result.CollectorNamespace = source
	if unsupportedUsageSegmentVersion(batch) {
		result.Status = "unsupported_version"
		return result, nil, nil
	}
	if len(batch.Observations)+len(batch.Tombstones) > maxSessionUsageBatch {
		return result, nil, fmt.Errorf("%w: maximum 200 mutations", ErrInvalidSessionUsage)
	}
	if err := r.lockUsageBatchScopesTx(ctx, tx, source, batch); err != nil {
		return result, nil, err
	}
	if batch.CompleteSetRevision != nil {
		status, err := r.admitCompleteSetTx(ctx, tx, source, batch)
		if err != nil {
			return result, nil, err
		}
		if status != usageWriteAccepted {
			result.Status = usageCoveragePartial
			result.Results = []pluginsdk.UsageObservationWriteResultV1{{Operation: "complete_set", ItemID: batch.CompleteSetRevision.SetID, Status: status}}
			return result, nil, nil
		}
	}
	if err := validateUsageMutationIDs(batch); err != nil {
		return result, nil, err
	}
	result, changes, err := r.applyUsageBatchTx(ctx, tx, source, batch, result)
	if err == nil {
		err = r.groupUsageProjectionMutationsTx(ctx, tx, changes)
	}
	return result, changes, err
}

func validateUsageMutationIDs(batch pluginsdk.UsageObservationBatchV1) error {
	seen := make(map[string]bool)
	ids := make([]string, 0, len(batch.Observations)+len(batch.Tombstones))
	for _, item := range batch.Observations {
		ids = append(ids, item.ItemID)
	}
	for _, item := range batch.Tombstones {
		ids = append(ids, item.ItemID)
	}
	for _, id := range ids {
		if seen[id] {
			return fmt.Errorf("%w: duplicate mutation id", ErrInvalidSessionUsage)
		}
		seen[id] = true
	}
	return nil
}

func appendUsageReceipt(result *pluginsdk.UsageObservationBatchResultV1, receipt pluginsdk.UsageObservationWriteResultV1) {
	result.Results = append(result.Results, receipt)
	if receipt.Status != usageWriteAccepted && receipt.Status != usageWriteUnchanged {
		result.Status = usageCoveragePartial
	}
}

func (r *Repository) applyUsageBatchTx(ctx context.Context, tx *sqlx.Tx, source string, batch pluginsdk.UsageObservationBatchV1, result pluginsdk.UsageObservationBatchResultV1) (pluginsdk.UsageObservationBatchResultV1, []models.UsageMutation, error) {
	var mutations []models.UsageMutation
	for _, input := range batch.Observations {
		receipt, changes, err := r.upsertObservationTx(ctx, tx, source, input)
		if err != nil {
			return result, nil, err
		}
		appendUsageReceipt(&result, receipt)
		mutations = append(mutations, changes...)
	}
	for _, tombstone := range batch.Tombstones {
		receipt, change, err := r.tombstoneObservationTx(ctx, tx, source, tombstone)
		if err != nil {
			return result, nil, err
		}
		appendUsageReceipt(&result, receipt)
		if change != nil {
			mutations = append(mutations, *change)
		}
	}
	if batch.CompleteSetRevision != nil {
		if result.Status != usageWriteAccepted {
			return result, nil, fmt.Errorf("%w: complete-set revision requires every member to be accepted", ErrInvalidSessionUsage)
		}
		changes, err := r.finishCompleteSetTx(ctx, tx, source, batch)
		if err != nil {
			return result, nil, err
		}
		mutations = append(mutations, changes...)
	}
	return result, mutations, nil
}

func (r *Repository) lockUsageBatchScopesTx(ctx context.Context, tx *sqlx.Tx, source string, batch pluginsdk.UsageObservationBatchV1) error {
	bindings := map[string]bool{}
	for _, item := range batch.Observations {
		bindings[item.SubjectBinding] = true
	}
	for _, item := range batch.Tombstones {
		bindings[item.SubjectBinding] = true
	}
	if batch.CompleteSetRevision != nil {
		bindings[batch.CompleteSetRevision.SubjectBinding] = true
	}
	workspaces := map[string]bool{}
	for binding := range bindings {
		var workspace string
		err := tx.GetContext(ctx, &workspace, r.db.Rebind(`SELECT b.workspace_id FROM usage_collection_bindings b JOIN usage_collection_grants g ON g.binding_id = b.binding_id WHERE b.binding_id = ? AND g.source = ?`), binding, source)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		workspaces[workspace] = true
	}
	ids := make([]string, 0, len(workspaces))
	for id := range workspaces {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if _, err := r.authorizedUsageScopeTx(ctx, tx, id); err != nil {
			return ErrUsageNotVisible
		}
	}
	return nil
}

func (r *Repository) reserveObservationItemTx(ctx context.Context, tx *sqlx.Tx, source string, target retainedTarget, id string) (observationItem, error) {
	rowID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("usage-observation-v1:"+source+":"+id)).String()
	_, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_observation_items (source, item_id, scope_id, subject_id, binding_id, observation_id) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(source, item_id) DO NOTHING`), source, id, target.ScopeID, target.SubjectID, target.BindingID, rowID)
	if err != nil {
		return observationItem{}, err
	}
	return r.findObservationItemTx(ctx, tx, source, id)
}

func (r *Repository) findObservationItemTx(ctx context.Context, tx *sqlx.Tx, source, id string) (observationItem, error) {
	query := `SELECT * FROM usage_observation_items WHERE source = ? AND item_id = ?`
	if r.db.DriverName() == usageDriverPostgres {
		query += usageForUpdate
	}
	var item observationItem
	err := tx.GetContext(ctx, &item, r.db.Rebind(query), source, id)
	return item, err
}

func (r *Repository) upsertObservationTx(ctx context.Context, tx *sqlx.Tx, source string, input pluginsdk.UsageObservationV1) (pluginsdk.UsageObservationWriteResultV1, []models.UsageMutation, error) {
	receipt := pluginsdk.UsageObservationWriteResultV1{Operation: "observation", ItemID: input.ItemID, Status: usageWriteRejected}
	measurement, err := validateContractObservation(input)
	if err != nil {
		receipt.Error = err.Error()
		if errors.Is(err, errUsageSemanticsUnsupported) {
			receipt.Status = "unsupported_version"
		}
		return receipt, nil, nil
	}
	target, rejected, err := r.prepareUsageObservationTargetTx(ctx, tx, source, input, measurement)
	if err != nil || rejected != "" {
		receipt.Error = rejected
		return receipt, nil, err
	}
	stored, err := r.reserveObservationItemTx(ctx, tx, source, target, input.ItemID)
	if err != nil {
		return receipt, nil, err
	}
	if stored.BindingID != target.BindingID || stored.ScopeID != target.ScopeID || stored.SubjectID != target.SubjectID {
		receipt.Error = ErrUsageNotVisible.Error()
		return receipt, nil, nil
	}
	if stored.Revision > 0 && (input.Revision <= stored.Revision || stored.Retired != 0) {
		status := contractRevisionStatus(input.Revision, input.PayloadDigest, stored)
		if stored.Retired != 0 {
			status = usageWriteConflict
		}
		return r.observationReceiptTx(ctx, tx, stored, "observation", status)
	}
	return r.acceptObservationTx(ctx, tx, target, stored, measurement, input, receipt)
}

func (r *Repository) acceptObservationTx(ctx context.Context, tx *sqlx.Tx, target retainedTarget, stored observationItem, measurement models.SessionUsageMeasurement, input pluginsdk.UsageObservationV1, receipt pluginsdk.UsageObservationWriteResultV1) (pluginsdk.UsageObservationWriteResultV1, []models.UsageMutation, error) {
	source := stored.Source
	previous, rejected, err := r.previousUsageObservationTx(ctx, tx, stored, input)
	if err != nil || rejected != "" {
		receipt.Error = rejected
		return receipt, nil, err
	}
	measurement.ID, measurement.Source = stored.ObservationID, source
	measurement.WorkspaceID, measurement.TaskID, measurement.SessionID = target.WorkspaceID, target.TaskID, target.SessionID
	measurement.ScopeID, measurement.SubjectID, measurement.UsageBindingID = target.ScopeID, target.SubjectID, target.BindingID
	measurement.ActorID, measurement.ActorStatus, measurement.ActorKind = target.ActorID, target.ActorStatus, target.ActorKind
	measurement.UsageIdentity = "v1:" + input.ItemID
	measurement.TranscriptID = target.ExternalSessionID
	measurement.CollectedAt = time.Now().UTC()
	work, status, eventID, err := r.resolveContractWorkIdentityTx(ctx, tx, target, source, input.WorkIdentity)
	if err != nil {
		return receipt, nil, err
	}
	if err := r.deriveExternalUsageAttributionTx(ctx, tx, target, eventID, &measurement); err != nil {
		return receipt, nil, err
	}
	if err := r.admitUsageSourceContextTx(ctx, tx, target, eventID); err != nil {
		receipt.Error = err.Error()
		return receipt, nil, nil
	}
	var changes []models.UsageMutation
	lineage, retired, err := r.retireUsageSupersededItemTx(ctx, tx, source, target, input)
	if errors.Is(err, errUsageSupersessionUnavailable) {
		receipt.Error = err.Error()
		return receipt, nil, nil
	}
	if err != nil {
		return receipt, nil, err
	}
	if retired != nil {
		changes = append(changes, *retired)
	}
	current, err := r.saveContractObservationTx(ctx, tx, stored, measurement, input, previous, work, status, eventID)
	if err != nil {
		return receipt, nil, err
	}
	if err := r.inheritUsageSegmentTx(ctx, tx, &current, lineage, input); err != nil {
		return receipt, nil, err
	}
	change, err := r.recordUsageMutationTx(ctx, tx, "observation", previous, &current)
	if err != nil {
		return receipt, nil, err
	}
	changes = append(changes, change)
	stored.Revision, stored.PayloadDigest, stored.SourceTable = input.Revision, input.PayloadDigest, current.Table
	value, _, err := r.observationReceiptTx(ctx, tx, stored, "observation", usageWriteAccepted)
	return value, changes, err
}

func (r *Repository) saveContractObservationTx(ctx context.Context, tx *sqlx.Tx, stored observationItem, measurement models.SessionUsageMeasurement, input pluginsdk.UsageObservationV1, previous *indexedObservation, work, status, eventID string) (indexedObservation, error) {
	if previous != nil {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM `+stored.SourceTable+` WHERE id = ?`), stored.ObservationID); err != nil {
			return indexedObservation{}, err
		}
	}
	inserted, err := r.insertUsageTx(ctx, tx, measurement)
	if err != nil {
		return indexedObservation{}, err
	}
	if inserted != 1 {
		return indexedObservation{}, errors.New("retained item identity conflicted with another observation")
	}
	table := usageTableFor(measurement)
	encoded, err := encodeObservationMetadata(input)
	if err != nil {
		return indexedObservation{}, err
	}
	coverage := contractCoverageKey(measurement)
	if eventID != "" {
		coverage = "event:" + eventID + ":" + coverage
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE `+table+` SET item_id = ?, selection_partition = ?, selection_coverage = ?, work_status = ?, observation_metadata = ?, retired = 0, work_event_id = ?, selection_version = (SELECT work_version FROM usage_scopes WHERE scope_id = ?) WHERE id = ?`), input.ItemID, work, coverage, status, string(encoded), eventID, measurement.ScopeID, measurement.ID)
	if err != nil {
		return indexedObservation{}, err
	}
	setID, setRevision := "", int64(0)
	if input.SourceSetID != nil {
		setID = *input.SourceSetID
	}
	if input.SourceSetRevision != nil {
		setRevision = *input.SourceSetRevision
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_observation_items SET source_table = ?, revision = ?, payload_digest = ?, retired = 0, set_id = ?, set_revision = ? WHERE source = ? AND item_id = ?`), table, input.Revision, input.PayloadDigest, setID, setRevision, measurement.Source, input.ItemID)
	if err != nil {
		return indexedObservation{}, err
	}
	if err := r.insertUsageLiveLinkTx(ctx, tx, table, measurement.ID, measurement.ScopeID, measurement.SubjectID, measurement.TaskID, measurement.SessionID); err != nil {
		return indexedObservation{}, err
	}
	current := indexedObservation{Measurement: measurement, ItemID: input.ItemID, WorkKey: work, WorkStatus: status, CoverageKey: coverage, Metadata: string(encoded), Table: table, EventID: eventID}
	if previous != nil {
		current.Segment, current.Layout, current.Bundle = previous.Segment, previous.Layout, previous.Bundle
	}
	if err := r.persistUsageSegmentTx(ctx, tx, &current, &input); err != nil {
		return indexedObservation{}, err
	}
	return current, nil
}

func encodeObservationMetadata(input pluginsdk.UsageObservationV1) ([]byte, error) {
	metadata := input
	if metadata.Tokens != nil {
		metadata.Tokens = &pluginsdk.UsageTokenAmountsV1{}
	}
	if metadata.Cost != nil {
		cost := *metadata.Cost
		cost.Amount = nil
		metadata.Cost = &cost
	}
	return json.Marshal(metadata)
}

func usageTableFor(item models.SessionUsageMeasurement) string {
	if item.CoverageKey != "" {
		return usageBucketsTable
	}
	return usageMeasurementsTable
}

func contractRevisionStatus(revision int64, digest string, stored observationItem) string {
	if revision < stored.Revision {
		return "stale"
	}
	if revision == stored.Revision && digest == stored.PayloadDigest {
		return usageWriteUnchanged
	}
	return usageWriteConflict
}

func (r *Repository) admitRetainedCorrectionTx(ctx context.Context, tx *sqlx.Tx, target retainedTarget) error {
	var created time.Time
	err := tx.GetContext(ctx, &created, r.db.Rebind(`SELECT created_at FROM tasks WHERE id = ?`), target.TaskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if usageTaskIncarnationID(created) != target.TaskIncarnationID {
		return nil
	}
	if target.SessionID != "" {
		var incarnation string
		err := tx.GetContext(ctx, &incarnation, r.db.Rebind(`SELECT COALESCE(NULLIF(queue_incarnation_id, ''), id) FROM task_sessions WHERE id = ? AND task_id = ?`), target.SessionID, target.TaskID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if incarnation != target.SessionIncarnationID {
			return nil
		}
	}
	return r.AdmitUsageWriteTx(ctx, tx, target.TaskID, target.WorkspaceID, target.SessionID)
}

func contractWorkIdentity(target retainedTarget, source string, identity *pluginsdk.UsageWorkIdentityV1) (string, string) {
	if identity == nil || (identity.Scheme == "transcript_binding" && identity.ID != "" && (identity.ID == target.ExternalSessionID || identity.ID == target.BindingID)) {
		return transcriptUsageWork(target.ScopeID, target.SubjectID, target.ExternalSessionID), usageIdentityProven
	}
	encoded, _ := json.Marshal(identity)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("usage-unproven:"+target.SubjectID+":"+source+":"+string(encoded))).String(), usageIdentityUnproven
}
