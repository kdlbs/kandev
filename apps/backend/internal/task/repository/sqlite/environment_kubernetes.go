package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const kubernetesEnvironmentSchemaDDL = `CREATE TABLE IF NOT EXISTS task_environment_kubernetes (
 environment_id TEXT PRIMARY KEY,
 task_id TEXT NOT NULL UNIQUE,
 ownership_generation BIGINT NOT NULL,
 revision BIGINT NOT NULL DEFAULT 1,
 operation_id TEXT NOT NULL DEFAULT '',
 metadata TEXT NOT NULL DEFAULT '{}',
 control_secret_id TEXT NOT NULL DEFAULT '',
 bootstrap_secret_id TEXT NOT NULL DEFAULT ''
)`

// ClaimKubernetesEnvironment serializes physical resource operations while leaving
// the database transaction closed during remote Kubernetes API calls.
func (r *Repository) ClaimKubernetesEnvironment(ctx context.Context, environmentID, taskID string, generation int64, operationID string) (*models.KubernetesEnvironment, error) {
	if environmentID == "" || taskID == "" || generation < 1 || operationID == "" {
		return nil, models.ErrKubernetesEnvironmentConflict
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.validateKubernetesEnvironmentOwner(ctx, tx, environmentID, taskID, generation); err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO task_environment_kubernetes (environment_id, task_id, ownership_generation)
 VALUES (?, ?, ?) ON CONFLICT DO NOTHING`), environmentID, taskID, generation)
	if err != nil {
		return nil, err
	}
	record, err := scanKubernetesEnvironment(tx.QueryRowContext(ctx, r.db.Rebind(kubernetesEnvironmentSelect+` WHERE environment_id = ?`), environmentID))
	if err != nil {
		return nil, err
	}
	if record.TaskID != taskID || record.OwnershipGeneration != generation || (record.OperationID != "" && record.OperationID != operationID) {
		return nil, models.ErrKubernetesEnvironmentConflict
	}
	if record.OperationID == "" {
		result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_environment_kubernetes SET operation_id = ?, revision = revision + 1 WHERE environment_id = ? AND revision = ? AND operation_id = ''`), operationID, environmentID, record.Revision)
		if err := requireKubernetesEnvironmentWrite(result, err); err != nil {
			return nil, err
		}
		record.OperationID = operationID
		record.Revision++
	}
	return record, tx.Commit()
}

const kubernetesEnvironmentSelect = `SELECT environment_id, task_id, ownership_generation, revision, operation_id, metadata, control_secret_id, bootstrap_secret_id FROM task_environment_kubernetes`

type kubernetesInventoryScanner interface{ Scan(...interface{}) error }

func scanKubernetesEnvironment(row kubernetesInventoryScanner) (*models.KubernetesEnvironment, error) {
	record := &models.KubernetesEnvironment{}
	var raw string
	err := row.Scan(&record.EnvironmentID, &record.TaskID, &record.OwnershipGeneration, &record.Revision, &record.OperationID, &raw, &record.ControlSecretID, &record.BootstrapSecretID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.ErrKubernetesEnvironmentNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), &record.Metadata); err != nil {
		return nil, fmt.Errorf("decode Kubernetes environment inventory: %w", err)
	}
	return record, nil
}

func (r *Repository) GetKubernetesEnvironment(ctx context.Context, environmentID string) (*models.KubernetesEnvironment, error) {
	return scanKubernetesEnvironment(r.db.QueryRowContext(ctx, r.db.Rebind(kubernetesEnvironmentSelect+` WHERE environment_id = ?`), environmentID))
}

// SaveKubernetesEnvironment checkpoints only the current operation and revision.
// Releasing its operation allows the next attachment, recovery, or cleanup.
func (r *Repository) SaveKubernetesEnvironment(ctx context.Context, record *models.KubernetesEnvironment, release bool) error {
	if record == nil || record.OperationID == "" {
		return models.ErrKubernetesEnvironmentConflict
	}
	raw, err := json.Marshal(record.Metadata)
	if err != nil {
		return err
	}
	operation := record.OperationID
	if release {
		operation = ""
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE task_environment_kubernetes SET metadata = ?, control_secret_id = ?, bootstrap_secret_id = ?, operation_id = ?, revision = revision + 1
 WHERE environment_id = ? AND task_id = ? AND ownership_generation = ? AND revision = ? AND operation_id = ? AND (`+kubernetesEnvironmentWritable+`)`), string(raw), record.ControlSecretID, record.BootstrapSecretID, operation, record.EnvironmentID, record.TaskID, record.OwnershipGeneration, record.Revision, record.OperationID)
	if err := requireKubernetesEnvironmentWrite(result, err); err != nil {
		return err
	}
	record.Revision++
	record.OperationID = operation
	return nil
}

func requireKubernetesEnvironmentWrite(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return models.ErrKubernetesEnvironmentConflict
	}
	return nil
}

// DeleteKubernetesEnvironment releases inventory after exact remote cleanup.
func (r *Repository) DeleteKubernetesEnvironment(ctx context.Context, record *models.KubernetesEnvironment) error {
	if record == nil || record.OperationID == "" {
		return models.ErrKubernetesEnvironmentConflict
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_environment_kubernetes
 WHERE environment_id = ? AND task_id = ? AND ownership_generation = ? AND revision = ? AND operation_id = ?
 AND (`+kubernetesEnvironmentWritable+`)`),
		record.EnvironmentID, record.TaskID, record.OwnershipGeneration, record.Revision, record.OperationID)
	return requireKubernetesEnvironmentWrite(result, err)
}

// RecoverInterruptedKubernetesOperations fences claims from a terminated backend.
// Call only at startup under exclusive runtime-state ownership, before launching
// workers. Inventory remains intact for exact-resource reconciliation or cleanup.
func (r *Repository) RecoverInterruptedKubernetesOperations(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE task_environment_kubernetes SET operation_id = '', revision = revision + 1 WHERE operation_id <> ''`)
	return err
}

func (r *Repository) ListKubernetesEnvironments(ctx context.Context) ([]*models.KubernetesEnvironment, error) {
	rows, err := r.db.QueryContext(ctx, kubernetesEnvironmentSelect+` ORDER BY environment_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	records := make([]*models.KubernetesEnvironment, 0)
	for rows.Next() {
		record, err := scanKubernetesEnvironment(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (r *Repository) validateKubernetesEnvironmentOwner(ctx context.Context, tx *sqlx.Tx, environmentID, taskID string, generation int64) error {
	if dialect.IsPostgres(r.db.DriverName()) {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "kubernetes-environment:"+taskID); err != nil {
			return err
		}
	}
	if err := r.lockTaskRowInTx(ctx, tx, taskID); err != nil {
		return err
	}
	var owner string
	var currentGeneration int64
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id, ownership_generation FROM task_environments WHERE id = ?`), environmentID).Scan(&owner, &currentGeneration); err != nil {
		return err
	}
	if owner != taskID || generation != currentGeneration {
		return models.ErrKubernetesEnvironmentConflict
	}
	return nil
}

// Physical inventory survives task/environment cascades until durable cleanup
// finishes. Only cleanup claims may operate after both owner rows are gone.
const kubernetesEnvironmentWritable = `EXISTS (
 SELECT 1 FROM task_environments e WHERE e.id = task_environment_kubernetes.environment_id
 AND e.task_id = task_environment_kubernetes.task_id
 AND e.ownership_generation = task_environment_kubernetes.ownership_generation
) OR (operation_id LIKE 'cleanup:%'
 AND NOT EXISTS (SELECT 1 FROM task_environments e WHERE e.id = task_environment_kubernetes.environment_id)
 AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.id = task_environment_kubernetes.task_id))`

func (r *Repository) ClaimKubernetesEnvironmentCleanup(ctx context.Context, environmentID, taskID string, generation int64, operationID string) (*models.KubernetesEnvironment, error) {
	if operationID == "" {
		return nil, models.ErrKubernetesEnvironmentConflict
	}
	operationID = "cleanup:" + operationID
	// An existing record is mandatory: deletion never creates resource authority.
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE task_environment_kubernetes
 SET operation_id = ?, revision = revision + 1
 WHERE environment_id = ? AND task_id = ? AND ownership_generation = ? AND operation_id = ''
 AND (EXISTS (SELECT 1 FROM task_environments e WHERE e.id = task_environment_kubernetes.environment_id
 AND e.task_id = task_environment_kubernetes.task_id AND e.ownership_generation = task_environment_kubernetes.ownership_generation)
 OR (NOT EXISTS (SELECT 1 FROM task_environments e WHERE e.id = task_environment_kubernetes.environment_id)
 AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.id = task_environment_kubernetes.task_id)))`), operationID, environmentID, taskID, generation)
	if err := requireKubernetesEnvironmentWrite(result, err); err != nil {
		return nil, err
	}
	return r.GetKubernetesEnvironment(ctx, environmentID)
}

func (r *Repository) ensureKubernetesInventoryAbsent(ctx context.Context, tx *sqlx.Tx, environmentID string) error {
	var count int
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(*) FROM task_environment_kubernetes WHERE environment_id = ?`), environmentID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return models.ErrKubernetesEnvironmentConflict
	}
	return nil
}
