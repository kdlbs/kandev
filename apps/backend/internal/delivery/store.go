package delivery

import (
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
// migration runner swallows errors. CASCADE matches task_id's own
// documented clause and the spec's persistence guarantee that deleting a
// task deletes its ledger rows.
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
		migrate: db.NewMigrateLogger(writer, log),
	}
	if err := r.initSchema(); err != nil {
		return nil, fmt.Errorf("delivery ledger schema init: %w", err)
	}
	return r, nil
}

func (r *Repository) initSchema() error {
	r.migrate.Apply("task_delivery_ledger.table", createTableSQL)
	r.migrate.Apply("task_delivery_ledger.idx_workspace_evaluated", createIndexSQL)
	r.activate()
	return nil
}

// activate writes activationKey only after a positive probe confirms
// task_delivery_ledger exists. See spec "Activation points": the
// migration runner swallows failures at WARN, so this probe is the only
// thing standing between a failed migration and a consumer wrongly
// believing the mechanism is live. WriteMetaKeyIfAbsent makes the write
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
