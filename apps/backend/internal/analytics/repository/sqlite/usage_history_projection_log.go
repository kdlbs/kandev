package sqlite

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"math"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/db/dialect"
)

type usageProjectionReplay struct {
	Version     int
	Mutation    models.UsageMutation
	BeforeTable string
	AfterTable  string
}

func encodeUsageProjection(value any) (string, error) {
	var buffer bytes.Buffer
	if err := gob.NewEncoder(&buffer).Encode(value); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buffer.Bytes()), nil
}

func decodeUsageProjection(value string, target any) error {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return err
	}
	return gob.NewDecoder(bytes.NewReader(data)).Decode(target)
}

// The row lock is held until the source transaction commits. A later sequence
// cannot be allocated while a lower sequence is still uncommitted.
func (r *Repository) lockUsageProjectionScopeTx(ctx context.Context, tx *sqlx.Tx, scope string) (int64, string, error) {
	var workspace, organization string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT workspace_id, organization_id FROM usage_scopes WHERE scope_id = ?`), scope).Scan(&workspace, &organization)
	if err != nil {
		return 0, "", ErrUsageNotVisible
	}
	if err := r.ensureUsageScopeWritableTx(ctx, tx, workspace, organization); err != nil {
		return 0, "", err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_scopes (scope_id, updated_at) VALUES (?, ?) ON CONFLICT(scope_id) DO NOTHING`), scope, time.Now().UTC())
	if err != nil {
		return 0, "", err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_scopes SET projection_registered = 1 WHERE scope_id = ? AND projection_registered = 0`), scope); err != nil {
		return 0, "", err
	}
	query := `SELECT commit_sequence, active_generation FROM usage_projection_scopes WHERE scope_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += usageForUpdate
	}
	var sequence int64
	var generation string
	err = tx.QueryRowxContext(ctx, r.db.Rebind(query), scope).Scan(&sequence, &generation)
	return sequence, generation, err
}

func (r *Repository) appendUsageProjectionMutationTx(ctx context.Context, tx *sqlx.Tx, change *models.UsageMutation, before, after *indexedObservation) error {
	scope := ""
	for _, partition := range []*models.UsagePartition{change.OldPartition, change.NewPartition} {
		if partition == nil {
			continue
		}
		if scope != "" && scope != partition.ScopeID {
			return errors.New("usage mutation cannot move between retained scopes")
		}
		scope = partition.ScopeID
	}
	if scope == "" {
		return errors.New("usage mutation has no retained scope")
	}
	sequence, _, err := r.lockUsageProjectionScopeTx(ctx, tx, scope)
	if err != nil {
		return err
	}
	if sequence == math.MaxInt64 {
		return errors.New("usage commit sequence exhausted")
	}
	sequence++
	change.CommitSequence = sequence
	replay := usageProjectionReplay{Version: 1, Mutation: *change}
	if before != nil {
		replay.BeforeTable = before.Table
	}
	if after != nil {
		replay.AfterTable = after.Table
	}
	payload, err := encodeUsageProjection(replay)
	if err != nil {
		return fmt.Errorf("encode usage replay: %w", err)
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_log (scope_id, commit_sequence, payload, publication_group, created_at) VALUES (?, ?, ?, ?, ?)`), scope, sequence, payload, uuid.NewString(), now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE usage_projection_scopes SET commit_sequence = ?, updated_at = ? WHERE scope_id = ?`), sequence, now, scope); err != nil {
		return err
	}
	if err := r.recordNativeUsageAccountingStateTx(ctx, tx, scope, sequence, before, after); err != nil {
		return err
	}
	return r.recordUsageProjectionVersionsTx(ctx, tx, sequence, *change, before, after)
}

func (r *Repository) recordUsageProjectionVersionsTx(ctx context.Context, tx *sqlx.Tx, sequence int64, change models.UsageMutation, before, after *indexedObservation) error {
	for _, partition := range []*models.UsagePartition{change.OldPartition, change.NewPartition} {
		if partition == nil {
			continue
		}
		_, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_partitions (scope_id, work_key, subject_id, commit_sequence) VALUES (?, ?, ?, ?)
			ON CONFLICT(scope_id, work_key) DO UPDATE SET commit_sequence = excluded.commit_sequence`), partition.ScopeID, partition.WorkKey, partition.SubjectID, sequence)
		if err != nil {
			return err
		}
	}
	for _, row := range []*indexedObservation{before, after} {
		if row == nil {
			continue
		}
		tombstone := 1
		if row == after {
			tombstone = 0
		}
		_, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO usage_projection_source_versions (scope_id, source_table, source_id, commit_sequence, tombstone) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(scope_id, source_table, source_id) DO UPDATE SET commit_sequence = excluded.commit_sequence, tombstone = excluded.tombstone`), row.Measurement.ScopeID, row.Table, row.Measurement.ID, sequence, tombstone)
		if err != nil {
			return err
		}
	}
	return nil
}
