package delivery

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/persistence"
)

// Repository owns the task_delivery_ledger table: its schema, its
// migration, its activation instant, and the single upsert statement that
// is the only writer. See docs/specs/task-delivery-ledger/spec.md, "Data
// model".
type Repository struct {
	db      *sqlx.DB // writer
	ro      *sqlx.DB // reader
	log     *logger.Logger
	migrate *db.MigrateLogger
}

// LedgerRecord is the persisted evaluation state for one task/repository
// pair. It is intentionally read-only from the public API; Upsert remains the
// sole writer of the ledger row.
type LedgerRecord struct {
	TaskID          string
	RepositoryID    string
	WorkspaceID     string
	Outcome         *Outcome
	Basis           *Basis
	EvidenceRank    int
	LastEvaluatedAt time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Get reads one ledger row by its owning task and repository pair.
func (r *Repository) Get(ctx context.Context, taskID, repositoryID string) (LedgerRecord, error) {
	var row struct {
		TaskID          string         `db:"task_id"`
		RepositoryID    string         `db:"repository_id"`
		WorkspaceID     string         `db:"workspace_id"`
		Outcome         sql.NullString `db:"delivery_outcome"`
		Basis           sql.NullString `db:"delivery_basis"`
		EvidenceRank    int            `db:"evidence_rank"`
		LastEvaluatedAt time.Time      `db:"last_evaluated_at"`
		CreatedAt       time.Time      `db:"created_at"`
		UpdatedAt       time.Time      `db:"updated_at"`
	}
	err := r.ro.GetContext(ctx, &row, r.ro.Rebind(`
		SELECT task_id, repository_id, workspace_id, delivery_outcome,
			delivery_basis, evidence_rank, last_evaluated_at, created_at, updated_at
		FROM task_delivery_ledger WHERE task_id = ? AND repository_id = ?
	`), taskID, repositoryID)
	if err != nil {
		return LedgerRecord{}, err
	}
	record := LedgerRecord{
		TaskID: row.TaskID, RepositoryID: row.RepositoryID, WorkspaceID: row.WorkspaceID,
		EvidenceRank: row.EvidenceRank, LastEvaluatedAt: row.LastEvaluatedAt,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.Outcome.Valid {
		value := Outcome(row.Outcome.String)
		record.Outcome = &value
	}
	if row.Basis.Valid {
		value := Basis(row.Basis.String)
		record.Basis = &value
	}
	return record, nil
}

// Delete removes one ledger pair. Normal delivery sweeps only upsert rows;
// this method exists for explicit task cleanup and conformance lifecycle
// probes.
func (r *Repository) Delete(ctx context.Context, taskID, repositoryID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_delivery_ledger WHERE task_id = ? AND repository_id = ?
	`), taskID, repositoryID)
	return err
}

// activationKey is the kandev_meta key published once a boot has verified
// task_delivery_ledger is present and queryable. See spec "Activation
// points".
const activationKey = "telemetry.delivery_ledger.activated_at"

// createTableSQL declares the FK on repository_id ON DELETE CASCADE
// (Build decision R5-F2, the highest-risk of the spec's open gaps): the
// spec's repository_id row states no ON DELETE clause, but repositories
// itself cascades from workspaces (FOREIGN KEY (workspace_id) REFERENCES
// workspaces(id) ON DELETE CASCADE), and DELETE FROM workspaces is a live
// production path (internal/task/repository/sqlite/workspace.go). A bare
// FK (dialect default NO ACTION) would make workspace deletion start
// failing the moment any ledger row exists, invisibly, because the
// migration runner used to swallow errors. CASCADE matches task_id's own
// documented clause and the spec's persistence guarantee that deleting a
// task deletes its ledger rows. Required-store migration errors are now
// surfaced by initSchema.
//
// Both this table and its FK targets (tasks, repositories) must already
// exist at CREATE TABLE time on PostgreSQL (unlike SQLite, which allows
// forward-declared FK targets), so Provide must run after
// task/repository.Provide in the boot sequence.
const createTableSQL = `
CREATE TABLE IF NOT EXISTS task_delivery_ledger (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	repository_id TEXT NOT NULL,
	workspace_id TEXT NOT NULL,
	delivery_outcome TEXT,
	delivery_basis TEXT,
	delivery_ref TEXT,
	evidence_rank INTEGER NOT NULL DEFAULT 0,
	reached_default_at TIMESTAMP,
	reached_default_basis TEXT,
	reached_default_ref TEXT,
	observed_branch_commits INTEGER,
	first_classified_at TIMESTAMP,
	last_evaluated_at TIMESTAMP NOT NULL,
	evaluation_seq INTEGER NOT NULL,
	created_at TIMESTAMP NOT NULL,
	updated_at TIMESTAMP NOT NULL,
	UNIQUE(task_id, repository_id),
	FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
	FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
)`

const createIndexSQL = `
CREATE INDEX IF NOT EXISTS idx_task_delivery_ledger_workspace_evaluated
	ON task_delivery_ledger(workspace_id, last_evaluated_at)`

// NewWithDB creates the delivery ledger repository and initializes its
// schema. writer and reader must already have the tasks and repositories
// tables present (see createTableSQL).
func NewWithDB(writer, reader *sqlx.DB, log *logger.Logger) (*Repository, error) {
	r := &Repository{
		db:      writer,
		ro:      reader,
		log:     log,
		migrate: db.NewRequiredMigrateLogger(writer, log),
	}
	if err := r.initSchema(); err != nil {
		return nil, fmt.Errorf("delivery ledger schema init: %w", err)
	}
	return r, nil
}

func (r *Repository) initSchema() error {
	r.migrate.Apply("task_delivery_ledger.table", createTableSQL)
	r.migrate.Apply("task_delivery_ledger.idx_workspace_evaluated", createIndexSQL)
	if err := r.migrate.Err(); err != nil {
		return fmt.Errorf("required delivery migration: %w", err)
	}
	r.activate()
	return nil
}

// activate writes activationKey only after a positive probe confirms
// task_delivery_ledger exists. See spec "Activation points": the
// required migration runner fails startup for unexpected schema errors, so
// this probe runs only after the schema contract is complete. WriteMetaKeyIfAbsent makes the write
// replay-safe. Any failure here is logged and swallowed — activation is a
// published fact for consumers, never a boot-blocking requirement, and it
// does not gate the sweep.
func (r *Repository) activate() {
	if err := persistence.EnsureMetaTable(r.db); err != nil {
		r.logWarn("ensure kandev_meta table failed", err)
		return
	}
	exists, err := tableExists(r.db, "task_delivery_ledger")
	if err != nil {
		r.logWarn("task_delivery_ledger schema probe failed", err)
		return
	}
	if !exists {
		return
	}
	if _, err := persistence.WriteMetaKeyIfAbsent(
		r.db, activationKey, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		r.logWarn("write delivery ledger activation key failed", err)
	}
}

func (r *Repository) logWarn(msg string, err error) {
	if r.log != nil {
		r.log.Warn(msg, zap.Error(err))
	}
}

// tableExists reports whether table exists, on either dialect.
func tableExists(conn *sqlx.DB, table string) (bool, error) {
	if dialect.IsPostgres(conn.DriverName()) {
		var exists bool
		err := conn.QueryRow(conn.Rebind(`
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = current_schema() AND table_name = ?
			)
		`), table).Scan(&exists)
		return exists, err
	}
	var name string
	err := conn.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
