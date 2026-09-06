package canvas

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

// Repository stores the canvas-owned metadata that is not part of a plugin
// instance. It intentionally uses a separate table name so databases that
// still contain the superseded declarative canvases table are not read as if
// they used the current plugin-backed lifecycle schema.
type Repository struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// SchemaSQL is exported for schema replay and backup tests. The statements
// use the common SQLite/PostgreSQL subset and are safe to run repeatedly.
const SchemaSQL = `
CREATE TABLE IF NOT EXISTS canvas_lifecycle_metadata (
  id TEXT PRIMARY KEY,
  plugin_instance_id TEXT NOT NULL UNIQUE,
  workspace_id TEXT NOT NULL,
  task_id TEXT NOT NULL DEFAULT '',
  origin_task_id TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  created_by_session_id TEXT NOT NULL DEFAULT '',
  promoted_by_user_id TEXT NOT NULL DEFAULT '',
  promoted_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_canvas_lifecycle_workspace
  ON canvas_lifecycle_metadata(workspace_id, updated_at, id);
CREATE INDEX IF NOT EXISTS idx_canvas_lifecycle_task
  ON canvas_lifecycle_metadata(task_id, updated_at, id);
CREATE TABLE IF NOT EXISTS canvas_lifecycle_admission (
  id INTEGER PRIMARY KEY,
  version INTEGER NOT NULL
);
INSERT INTO canvas_lifecycle_admission (id, version)
VALUES (1, 1) ON CONFLICT (id) DO NOTHING;
`

// NewRepository constructs the canvas metadata repository on an existing
// application database pool. Plugin instance schema initialization remains
// owned by internal/plugins/instances.
func NewRepository(pool *db.Pool) (*Repository, error) {
	if pool == nil || pool.Writer() == nil || pool.Reader() == nil {
		return nil, errors.New("canvas: database pool is required")
	}
	return NewRepositoryWithDB(pool.Writer(), pool.Reader())
}

// NewRepositoryWithDB is useful for tests and for callers that already own
// separate read and write sqlx handles.
func NewRepositoryWithDB(writer, reader *sqlx.DB) (*Repository, error) {
	if writer == nil || reader == nil {
		return nil, errors.New("canvas: database connections are required")
	}
	repo := &Repository{db: writer, ro: reader}
	if err := repo.initSchema(); err != nil {
		return nil, fmt.Errorf("canvas schema: %w", err)
	}
	return repo, nil
}

// NewStore is a naming-compatible convenience for callers that use Store for
// database-backed domain repositories.
func NewStore(pool *db.Pool) (*Repository, error) {
	return NewRepository(pool)
}

func (r *Repository) initSchema() error {
	for _, statement := range strings.Split(SchemaSQL, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := r.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

// Create persists metadata after the plugin instance admission has succeeded.
func (r *Repository) Create(ctx context.Context, metadata CanvasMetadata) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.CreateTx(ctx, tx, metadata); err != nil {
		return err
	}
	return tx.Commit()
}

// CreateTx persists metadata in an existing lifecycle transaction. Canvas
// creation uses this with the plugin-instance insert so a crash cannot leave
// quota-consuming instance authority without its one-to-one canvas row.
func (r *Repository) CreateTx(ctx context.Context, tx *sqlx.Tx, metadata CanvasMetadata) error {
	if err := validateMetadata(metadata); err != nil {
		return err
	}
	createdAt := metadata.CreatedAt.UTC()
	if metadata.CreatedAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	updatedAt := metadata.UpdatedAt.UTC()
	if metadata.UpdatedAt.IsZero() {
		updatedAt = createdAt
	}
	// The singleton update serializes admission across service instances and
	// processes on PostgreSQL, and obtains SQLite's write lock before counts.
	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE canvas_lifecycle_admission SET version = version WHERE id = 1`)); err != nil {
		return err
	}
	if err := checkAdmission(ctx, tx, metadata); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`
INSERT INTO canvas_lifecycle_metadata
  (id, plugin_instance_id, workspace_id, task_id, origin_task_id, title,
   created_by_session_id, promoted_by_user_id, promoted_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		metadata.ID,
		metadata.PluginInstanceID,
		metadata.WorkspaceID,
		metadata.TaskID,
		metadata.OriginTaskID,
		metadata.Title,
		metadata.CreatedBySessionID,
		metadata.PromotedByUserID,
		formatOptionalTime(metadata.PromotedAt),
		createdAt.Format(time.RFC3339Nano),
		updatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}
	return nil
}

func checkAdmission(ctx context.Context, tx *sqlx.Tx, metadata CanvasMetadata) error {
	var count int
	if metadata.TaskID != "" {
		if err := tx.GetContext(ctx, &count, tx.Rebind(
			`SELECT COUNT(*) FROM canvas_lifecycle_metadata WHERE task_id = ?`), metadata.TaskID); err != nil {
			return err
		}
		if count >= MaxTaskCanvases {
			return ErrTaskCanvasLimit
		}
	}
	if err := tx.GetContext(ctx, &count, tx.Rebind(
		`SELECT COUNT(*) FROM canvas_lifecycle_metadata WHERE workspace_id = ?`), metadata.WorkspaceID); err != nil {
		return err
	}
	if count >= MaxWorkspaceCanvases {
		return ErrWorkspaceCanvasLimit
	}
	return nil
}

// Get returns one canvas-owned metadata record.
func (r *Repository) Get(ctx context.Context, id string) (CanvasMetadata, error) {
	if strings.TrimSpace(id) == "" {
		return CanvasMetadata{}, ErrCanvasNotFound
	}
	var row metadataRow
	err := r.ro.GetContext(ctx, &row, r.ro.Rebind(`
SELECT id, plugin_instance_id, workspace_id, task_id, origin_task_id, title,
       created_by_session_id, promoted_by_user_id, promoted_at, created_at, updated_at
FROM canvas_lifecycle_metadata WHERE id = ?`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return CanvasMetadata{}, ErrCanvasNotFound
	}
	if err != nil {
		return CanvasMetadata{}, err
	}
	return row.metadata()
}

// ListByWorkspace returns metadata in stable creation order. Lifecycle status
// filtering is performed by Service because status belongs to the plugin
// instance store.
func (r *Repository) ListByWorkspace(ctx context.Context, workspaceID string) ([]CanvasMetadata, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrInvalidCanvas
	}
	return r.list(ctx, `
SELECT id, plugin_instance_id, workspace_id, task_id, origin_task_id, title,
       created_by_session_id, promoted_by_user_id, promoted_at, created_at, updated_at
FROM canvas_lifecycle_metadata WHERE workspace_id = ? ORDER BY created_at, id`, workspaceID)
}

// ListByTask returns unpromoted task metadata. Promotion clears task_id, so a
// promoted canvas is naturally preserved by task cleanup.
func (r *Repository) ListByTask(ctx context.Context, taskID string) ([]CanvasMetadata, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, ErrInvalidCanvas
	}
	return r.list(ctx, `
SELECT id, plugin_instance_id, workspace_id, task_id, origin_task_id, title,
       created_by_session_id, promoted_by_user_id, promoted_at, created_at, updated_at
FROM canvas_lifecycle_metadata WHERE task_id = ? ORDER BY created_at, id`, taskID)
}

// ListAll returns every canvas metadata row for startup lifecycle
// reconciliation. Status and release authority remain owned by the instance
// store.
func (r *Repository) ListAll(ctx context.Context) ([]CanvasMetadata, error) {
	return r.list(ctx, `
SELECT id, plugin_instance_id, workspace_id, task_id, origin_task_id, title,
       created_by_session_id, promoted_by_user_id, promoted_at, created_at, updated_at
FROM canvas_lifecycle_metadata ORDER BY created_at, id`)
}

// ClearOriginTask removes provenance that points at a task after that task
// has been deleted. Promoted canvases have no current task_id and remain in
// the workspace.
func (r *Repository) ClearOriginTask(ctx context.Context, taskID string) error {
	if strings.TrimSpace(taskID) == "" {
		return ErrInvalidCanvas
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`UPDATE canvas_lifecycle_metadata SET origin_task_id = '', updated_at = ? WHERE origin_task_id = ? AND task_id = ''`),
		time.Now().UTC().Format(time.RFC3339Nano), taskID)
	return err
}

// Delete removes only the canvas-owned metadata. Service.Remove deletes the
// plugin instance first so its artifact cleanup inventory is durable before
// this authority record disappears.
func (r *Repository) Delete(ctx context.Context, id string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.DeleteTx(ctx, tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteTx removes metadata in an existing lifecycle transaction.
func (r *Repository) DeleteTx(ctx context.Context, tx *sqlx.Tx, id string) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(
		`DELETE FROM canvas_lifecycle_metadata WHERE id = ?`), id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrCanvasNotFound
	}
	return nil
}

// Promote records the human-owned provenance and clears the current task
// relationship. The instance scope/grants are changed by the plugin
// instance store immediately before this metadata transaction.
func (r *Repository) Promote(ctx context.Context, id, userID string, promotedAt time.Time) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.PromoteTx(ctx, tx, id, userID, promotedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// PromoteTx records workspace provenance in an existing lifecycle
// transaction. The canvas service uses this with the instance scope and grant
// mutation so promotion commits both authorities together.
func (r *Repository) PromoteTx(ctx context.Context, tx *sqlx.Tx, id, userID string, promotedAt time.Time) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(userID) == "" {
		return ErrInvalidCanvas
	}
	if promotedAt.IsZero() {
		promotedAt = time.Now().UTC()
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE canvas_lifecycle_metadata SET task_id = '', promoted_by_user_id = ?, promoted_at = ?, updated_at = ? WHERE id = ? AND task_id <> ''`,
	), userID, promotedAt.UTC().Format(time.RFC3339Nano), promotedAt.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrInvalidCanvas
	}
	return nil
}

func (r *Repository) list(ctx context.Context, query string, args ...any) ([]CanvasMetadata, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	metadata := make([]CanvasMetadata, 0)
	for rows.Next() {
		var row metadataRow
		if err := rows.StructScan(&row); err != nil {
			return nil, err
		}
		item, err := row.metadata()
		if err != nil {
			return nil, err
		}
		metadata = append(metadata, item)
	}
	return metadata, rows.Err()
}

func validateMetadata(metadata CanvasMetadata) error {
	if strings.TrimSpace(metadata.ID) == "" || strings.TrimSpace(metadata.PluginInstanceID) == "" || strings.TrimSpace(metadata.WorkspaceID) == "" || strings.TrimSpace(metadata.Title) == "" {
		return ErrInvalidCanvas
	}
	return nil
}

type metadataRow struct {
	ID                 string `db:"id"`
	PluginInstanceID   string `db:"plugin_instance_id"`
	WorkspaceID        string `db:"workspace_id"`
	TaskID             string `db:"task_id"`
	OriginTaskID       string `db:"origin_task_id"`
	Title              string `db:"title"`
	CreatedBySessionID string `db:"created_by_session_id"`
	PromotedByUserID   string `db:"promoted_by_user_id"`
	PromotedAt         string `db:"promoted_at"`
	CreatedAt          string `db:"created_at"`
	UpdatedAt          string `db:"updated_at"`
}

func (r metadataRow) metadata() (CanvasMetadata, error) {
	createdAt, err := time.Parse(time.RFC3339Nano, r.CreatedAt)
	if err != nil {
		return CanvasMetadata{}, fmt.Errorf("canvas: parse created_at: %w", err)
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, r.UpdatedAt)
	if err != nil {
		return CanvasMetadata{}, fmt.Errorf("canvas: parse updated_at: %w", err)
	}
	promotedAt, err := parseOptionalTime(r.PromotedAt)
	if err != nil {
		return CanvasMetadata{}, fmt.Errorf("canvas: parse promoted_at: %w", err)
	}
	return CanvasMetadata{
		ID:                 r.ID,
		PluginInstanceID:   r.PluginInstanceID,
		WorkspaceID:        r.WorkspaceID,
		TaskID:             r.TaskID,
		OriginTaskID:       r.OriginTaskID,
		Title:              r.Title,
		CreatedBySessionID: r.CreatedBySessionID,
		PromotedByUserID:   r.PromotedByUserID,
		PromotedAt:         promotedAt,
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
	}, nil
}

func formatOptionalTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseOptionalTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}
