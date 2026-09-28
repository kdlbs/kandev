package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	postgresBackgroundWorkSchema = `CREATE TABLE IF NOT EXISTS task_session_background_work (
		id TEXT NOT NULL,
		task_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		kind TEXT NOT NULL DEFAULT 'unknown',
		title TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT 'unknown',
		parent_work_id TEXT,
		origin_turn_id TEXT NOT NULL DEFAULT '',
		source_message_id TEXT NOT NULL DEFAULT '',
		source_call_id TEXT NOT NULL DEFAULT '',
		exit_code INTEGER,
		output TEXT NOT NULL DEFAULT '',
		output_truncated INTEGER NOT NULL DEFAULT 0,
		output_offset BIGINT NOT NULL DEFAULT 0,
		started_at TIMESTAMP,
		finished_at TIMESTAMP,
		revision BIGINT NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (session_id, id),
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_background_work_session ON task_session_background_work(session_id);
	CREATE INDEX IF NOT EXISTS idx_background_work_task ON task_session_background_work(task_id);

	CREATE TABLE IF NOT EXISTS task_session_background_runs (
		id TEXT NOT NULL,
		workload_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		provider_run_key TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT 'unknown',
		exit_code INTEGER,
		started_at TIMESTAMP,
		finished_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (session_id, id),
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_background_runs_workload ON task_session_background_runs(session_id, workload_id);
	CREATE INDEX IF NOT EXISTS idx_background_runs_session ON task_session_background_runs(session_id);

	CREATE TABLE IF NOT EXISTS task_session_background_action_receipts (
		id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workload_id TEXT NOT NULL,
		run_id TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL,
		operation_id TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		uncertain INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (session_id, id),
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_background_action_receipts_session_op ON task_session_background_action_receipts(session_id, operation_id);
	CREATE INDEX IF NOT EXISTS idx_background_action_receipts_session ON task_session_background_action_receipts(session_id);`

	sqliteBackgroundWorkSchema = `CREATE TABLE IF NOT EXISTS task_session_background_work (
		id TEXT NOT NULL,
		task_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		kind TEXT NOT NULL DEFAULT 'unknown',
		title TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT 'unknown',
		parent_work_id TEXT,
		origin_turn_id TEXT NOT NULL DEFAULT '',
		source_message_id TEXT NOT NULL DEFAULT '',
		source_call_id TEXT NOT NULL DEFAULT '',
		exit_code INTEGER,
		output TEXT NOT NULL DEFAULT '',
		output_truncated INTEGER NOT NULL DEFAULT 0,
		output_offset INTEGER NOT NULL DEFAULT 0,
		started_at TIMESTAMP,
		finished_at TIMESTAMP,
		revision INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (session_id, id),
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_background_work_session ON task_session_background_work(session_id);
	CREATE INDEX IF NOT EXISTS idx_background_work_task ON task_session_background_work(task_id);

	CREATE TABLE IF NOT EXISTS task_session_background_runs (
		id TEXT NOT NULL,
		workload_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		provider_run_key TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT 'unknown',
		exit_code INTEGER,
		started_at TIMESTAMP,
		finished_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (session_id, id),
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_background_runs_workload ON task_session_background_runs(session_id, workload_id);
	CREATE INDEX IF NOT EXISTS idx_background_runs_session ON task_session_background_runs(session_id);

	CREATE TABLE IF NOT EXISTS task_session_background_action_receipts (
		id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workload_id TEXT NOT NULL,
		run_id TEXT NOT NULL DEFAULT '',
		action TEXT NOT NULL,
		operation_id TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		uncertain INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (session_id, id),
		FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_background_action_receipts_session_op ON task_session_background_action_receipts(session_id, operation_id);
	CREATE INDEX IF NOT EXISTS idx_background_action_receipts_session ON task_session_background_action_receipts(session_id);`
)

func (r *Repository) initBackgroundWorkSchema() error {
	schema := sqliteBackgroundWorkSchema
	if dialect.IsPostgres(r.db.DriverName()) {
		schema = postgresBackgroundWorkSchema
	}
	_, err := r.db.Exec(schema)
	return err
}

func (r *Repository) UpsertBackgroundWorkload(ctx context.Context, workload *models.BackgroundWorkload) error {
	if workload == nil {
		return errors.New("background workload is nil")
	}
	if workload.ID == "" {
		workload.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if workload.CreatedAt.IsZero() {
		workload.CreatedAt = now
	}
	workload.UpdatedAt = now

	var query string
	if dialect.IsPostgres(r.db.DriverName()) {
		query = `INSERT INTO task_session_background_work (
			id, task_id, session_id, kind, title, state, parent_work_id,
			origin_turn_id, source_message_id, source_call_id, exit_code,
			output, output_truncated, output_offset, started_at, finished_at,
			revision, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19
		) ON CONFLICT (session_id, id) DO UPDATE SET
			title = EXCLUDED.title,
			state = EXCLUDED.state,
			parent_work_id = EXCLUDED.parent_work_id,
			origin_turn_id = CASE WHEN task_session_background_work.origin_turn_id = '' THEN EXCLUDED.origin_turn_id ELSE task_session_background_work.origin_turn_id END,
			source_message_id = EXCLUDED.source_message_id,
			source_call_id = EXCLUDED.source_call_id,
			exit_code = EXCLUDED.exit_code,
			output = EXCLUDED.output,
			output_truncated = EXCLUDED.output_truncated,
			output_offset = EXCLUDED.output_offset,
			started_at = COALESCE(task_session_background_work.started_at, EXCLUDED.started_at),
			finished_at = EXCLUDED.finished_at,
			revision = EXCLUDED.revision,
			updated_at = EXCLUDED.updated_at
		WHERE EXCLUDED.revision >= task_session_background_work.revision`
	} else {
		query = `INSERT INTO task_session_background_work (
			id, task_id, session_id, kind, title, state, parent_work_id,
			origin_turn_id, source_message_id, source_call_id, exit_code,
			output, output_truncated, output_offset, started_at, finished_at,
			revision, created_at, updated_at
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		) ON CONFLICT (session_id, id) DO UPDATE SET
			title = excluded.title,
			state = excluded.state,
			parent_work_id = excluded.parent_work_id,
			origin_turn_id = CASE WHEN task_session_background_work.origin_turn_id = '' THEN excluded.origin_turn_id ELSE task_session_background_work.origin_turn_id END,
			source_message_id = excluded.source_message_id,
			source_call_id = excluded.source_call_id,
			exit_code = excluded.exit_code,
			output = excluded.output,
			output_truncated = excluded.output_truncated,
			output_offset = excluded.output_offset,
			started_at = COALESCE(task_session_background_work.started_at, excluded.started_at),
			finished_at = excluded.finished_at,
			revision = excluded.revision,
			updated_at = excluded.updated_at
		WHERE excluded.revision >= task_session_background_work.revision`
	}

	_, err := r.db.ExecContext(ctx, query,
		workload.ID, workload.TaskID, workload.SessionID, workload.Kind, workload.Title, workload.State,
		workload.ParentWorkID, workload.OriginTurnID, workload.SourceMessageID, workload.SourceCallID,
		workload.ExitCode, workload.Output, workload.OutputTruncated, workload.OutputOffset,
		workload.StartedAt, workload.FinishedAt, workload.Revision, workload.CreatedAt, workload.UpdatedAt,
	)
	return err
}

func (r *Repository) GetBackgroundWorkload(ctx context.Context, sessionID, id string) (*models.BackgroundWorkload, error) {
	var query string
	if dialect.IsPostgres(r.ro.DriverName()) {
		query = `SELECT id, task_id, session_id, kind, title, state, parent_work_id,
				origin_turn_id, source_message_id, source_call_id, exit_code,
				output, output_truncated, output_offset, started_at, finished_at,
				revision, created_at, updated_at
		   FROM task_session_background_work WHERE session_id = $1 AND id = $2`
	} else {
		query = `SELECT id, task_id, session_id, kind, title, state, parent_work_id,
				origin_turn_id, source_message_id, source_call_id, exit_code,
				output, output_truncated, output_offset, started_at, finished_at,
				revision, created_at, updated_at
		   FROM task_session_background_work WHERE session_id = ? AND id = ?`
	}
	var work models.BackgroundWorkload
	err := r.ro.GetContext(ctx, &work, query, sessionID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &work, nil
}

func (r *Repository) ListBackgroundWorkloadsBySession(ctx context.Context, sessionID string) ([]*models.BackgroundWorkload, error) {
	var query string
	if dialect.IsPostgres(r.ro.DriverName()) {
		query = `SELECT id, task_id, session_id, kind, title, state, parent_work_id,
				origin_turn_id, source_message_id, source_call_id, exit_code,
				output, output_truncated, output_offset, started_at, finished_at,
				revision, created_at, updated_at
		   FROM task_session_background_work WHERE session_id = $1 ORDER BY created_at ASC`
	} else {
		query = `SELECT id, task_id, session_id, kind, title, state, parent_work_id,
				origin_turn_id, source_message_id, source_call_id, exit_code,
				output, output_truncated, output_offset, started_at, finished_at,
				revision, created_at, updated_at
		   FROM task_session_background_work WHERE session_id = ? ORDER BY created_at ASC`
	}
	var workloads []*models.BackgroundWorkload
	err := r.ro.SelectContext(ctx, &workloads, query, sessionID)
	if err != nil {
		return nil, err
	}
	return workloads, nil
}

func (r *Repository) DeleteBackgroundWorkloadsBySession(ctx context.Context, sessionID string) error {
	var q1, q2, q3 string
	if dialect.IsPostgres(r.db.DriverName()) {
		q1 = `DELETE FROM task_session_background_action_receipts WHERE session_id = $1`
		q2 = `DELETE FROM task_session_background_runs WHERE session_id = $1`
		q3 = `DELETE FROM task_session_background_work WHERE session_id = $1`
	} else {
		q1 = `DELETE FROM task_session_background_action_receipts WHERE session_id = ?`
		q2 = `DELETE FROM task_session_background_runs WHERE session_id = ?`
		q3 = `DELETE FROM task_session_background_work WHERE session_id = ?`
	}
	_, _ = r.db.ExecContext(ctx, q1, sessionID)
	_, _ = r.db.ExecContext(ctx, q2, sessionID)
	_, err := r.db.ExecContext(ctx, q3, sessionID)
	return err
}

func (r *Repository) UpsertBackgroundRun(ctx context.Context, run *models.BackgroundRun) error {
	if run == nil {
		return errors.New("background run is nil")
	}
	if run.ID == "" {
		run.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if run.CreatedAt.IsZero() {
		run.CreatedAt = now
	}
	run.UpdatedAt = now

	var query string
	if dialect.IsPostgres(r.db.DriverName()) {
		query = `INSERT INTO task_session_background_runs (
			id, workload_id, session_id, provider_run_key, state, exit_code,
			started_at, finished_at, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		) ON CONFLICT (session_id, id) DO UPDATE SET
			state = EXCLUDED.state,
			exit_code = EXCLUDED.exit_code,
			started_at = COALESCE(task_session_background_runs.started_at, EXCLUDED.started_at),
			finished_at = EXCLUDED.finished_at,
			updated_at = EXCLUDED.updated_at`
	} else {
		query = `INSERT INTO task_session_background_runs (
			id, workload_id, session_id, provider_run_key, state, exit_code,
			started_at, finished_at, created_at, updated_at
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		) ON CONFLICT (session_id, id) DO UPDATE SET
			state = excluded.state,
			exit_code = excluded.exit_code,
			started_at = COALESCE(task_session_background_runs.started_at, excluded.started_at),
			finished_at = excluded.finished_at,
			updated_at = excluded.updated_at`
	}

	_, err := r.db.ExecContext(ctx, query,
		run.ID, run.WorkloadID, run.SessionID, run.ProviderRunKey, run.State, run.ExitCode,
		run.StartedAt, run.FinishedAt, run.CreatedAt, run.UpdatedAt,
	)
	return err
}

func (r *Repository) GetBackgroundRun(ctx context.Context, sessionID, id string) (*models.BackgroundRun, error) {
	var query string
	if dialect.IsPostgres(r.ro.DriverName()) {
		query = `SELECT id, workload_id, session_id, provider_run_key, state, exit_code,
				started_at, finished_at, created_at, updated_at
		   FROM task_session_background_runs WHERE session_id = $1 AND id = $2`
	} else {
		query = `SELECT id, workload_id, session_id, provider_run_key, state, exit_code,
				started_at, finished_at, created_at, updated_at
		   FROM task_session_background_runs WHERE session_id = ? AND id = ?`
	}
	var run models.BackgroundRun
	err := r.ro.GetContext(ctx, &run, query, sessionID, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *Repository) ListBackgroundRunsByWorkload(ctx context.Context, sessionID, workloadID string) ([]*models.BackgroundRun, error) {
	var query string
	if dialect.IsPostgres(r.ro.DriverName()) {
		query = `SELECT id, workload_id, session_id, provider_run_key, state, exit_code,
				started_at, finished_at, created_at, updated_at
		   FROM task_session_background_runs WHERE session_id = $1 AND workload_id = $2 ORDER BY created_at ASC`
	} else {
		query = `SELECT id, workload_id, session_id, provider_run_key, state, exit_code,
				started_at, finished_at, created_at, updated_at
		   FROM task_session_background_runs WHERE session_id = ? AND workload_id = ? ORDER BY created_at ASC`
	}
	var runs []*models.BackgroundRun
	err := r.ro.SelectContext(ctx, &runs, query, sessionID, workloadID)
	if err != nil {
		return nil, err
	}
	return runs, nil
}

func (r *Repository) executeActionReceiptWrite(ctx context.Context, receipt *models.BackgroundActionReceipt, upsert bool) error {
	if receipt == nil {
		return errors.New("background action receipt is nil")
	}
	if receipt.ID == "" {
		receipt.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if receipt.CreatedAt.IsZero() {
		receipt.CreatedAt = now
	}
	receipt.UpdatedAt = now

	var query string
	isPg := dialect.IsPostgres(r.db.DriverName())
	switch {
	case !upsert && isPg:
		query = `INSERT INTO task_session_background_action_receipts (
			id, session_id, workload_id, run_id, action, operation_id, status, error, uncertain, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`
	case !upsert:
		query = `INSERT INTO task_session_background_action_receipts (
			id, session_id, workload_id, run_id, action, operation_id, status, error, uncertain, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	case isPg:
		query = `INSERT INTO task_session_background_action_receipts (
			id, session_id, workload_id, run_id, action, operation_id, status, error, uncertain, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (session_id, operation_id) DO UPDATE SET
			status = EXCLUDED.status,
			error = EXCLUDED.error,
			uncertain = EXCLUDED.uncertain,
			updated_at = EXCLUDED.updated_at`
	default:
		query = `INSERT INTO task_session_background_action_receipts (
			id, session_id, workload_id, run_id, action, operation_id, status, error, uncertain, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (session_id, operation_id) DO UPDATE SET
			status = excluded.status,
			error = excluded.error,
			uncertain = excluded.uncertain,
			updated_at = excluded.updated_at`
	}

	_, err := r.db.ExecContext(ctx, query,
		receipt.ID, receipt.SessionID, receipt.WorkloadID, receipt.RunID, receipt.Action,
		receipt.OperationID, receipt.Status, receipt.Error, receipt.Uncertain,
		receipt.CreatedAt, receipt.UpdatedAt,
	)
	return err
}

func (r *Repository) ReserveBackgroundActionReceipt(ctx context.Context, receipt *models.BackgroundActionReceipt) error {
	return r.executeActionReceiptWrite(ctx, receipt, false)
}

func (r *Repository) RecordBackgroundActionReceipt(ctx context.Context, receipt *models.BackgroundActionReceipt) error {
	return r.executeActionReceiptWrite(ctx, receipt, true)
}

func (r *Repository) GetBackgroundActionReceipt(ctx context.Context, sessionID, operationID string) (*models.BackgroundActionReceipt, error) {
	var query string
	if dialect.IsPostgres(r.ro.DriverName()) {
		query = `SELECT id, session_id, workload_id, run_id, action, operation_id, status, error, uncertain, created_at, updated_at
		   FROM task_session_background_action_receipts WHERE session_id = $1 AND operation_id = $2`
	} else {
		query = `SELECT id, session_id, workload_id, run_id, action, operation_id, status, error, uncertain, created_at, updated_at
		   FROM task_session_background_action_receipts WHERE session_id = ? AND operation_id = ?`
	}
	var receipt models.BackgroundActionReceipt
	err := r.ro.GetContext(ctx, &receipt, query, sessionID, operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &receipt, nil
}
