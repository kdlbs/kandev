package share

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
)

// Backend identifiers persisted in task_shares.backend.
const (
	BackendGitHubGist = "github_gist"
)

// ErrNotFound is returned when a share row cannot be located.
var ErrNotFound = errors.New("share not found")

// Share is the persisted representation of a published share link. The
// snapshot payload itself lives at ExternalURL; only metadata is stored
// locally.
type Share struct {
	ID                string
	TaskSessionID     string
	Backend           string
	ExternalID        string
	ExternalURL       string
	SnapshotSizeBytes int64
	CreatedAt         time.Time
	RevokedAt         *time.Time
	ViewCount         int
}

// IsRevoked returns true when the share has been revoked.
func (s *Share) IsRevoked() bool { return s.RevokedAt != nil }

// Repository persists Share rows in SQLite.
type Repository struct {
	writer *sqlx.DB
	reader *sqlx.DB
	log    *logger.Logger
}

// NewRepository creates a share repository and applies its migration. The
// caller owns the database connections; Close is a no-op.
func NewRepository(writer, reader *sqlx.DB, log *logger.Logger) (*Repository, error) {
	r := &Repository{writer: writer, reader: reader, log: log}
	if err := r.initSchema(); err != nil {
		return nil, fmt.Errorf("share repo: init schema: %w", err)
	}
	return r, nil
}

func (r *Repository) initSchema() error {
	mig := db.NewMigrateLogger(r.writer, r.log)
	mig.Apply("table.task_shares", `CREATE TABLE IF NOT EXISTS task_shares (
		id                  TEXT PRIMARY KEY,
		task_session_id     TEXT NOT NULL,
		backend             TEXT NOT NULL,
		external_id         TEXT NOT NULL,
		external_url        TEXT NOT NULL,
		snapshot_size_bytes INTEGER NOT NULL,
		created_at          DATETIME NOT NULL,
		revoked_at          DATETIME NULL,
		view_count          INTEGER NOT NULL DEFAULT 0
	)`)
	mig.Apply("index.task_shares_session",
		`CREATE INDEX IF NOT EXISTS idx_task_shares_session ON task_shares(task_session_id)`)
	return nil
}

// Create inserts a new share row. ID, Backend, ExternalID, ExternalURL, and
// TaskSessionID must be set by the caller. CreatedAt defaults to time.Now()
// (UTC) if zero.
func (r *Repository) Create(ctx context.Context, s *Share) error {
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now().UTC()
	}
	_, err := r.writer.ExecContext(ctx, `INSERT INTO task_shares
		(id, task_session_id, backend, external_id, external_url, snapshot_size_bytes, created_at, view_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.TaskSessionID, s.Backend, s.ExternalID, s.ExternalURL,
		s.SnapshotSizeBytes, s.CreatedAt, s.ViewCount,
	)
	if err != nil {
		return fmt.Errorf("insert task_share: %w", err)
	}
	return nil
}

// GetByID returns the share with the given ID, or ErrNotFound.
func (r *Repository) GetByID(ctx context.Context, id string) (*Share, error) {
	row := r.reader.QueryRowxContext(ctx, `SELECT
		id, task_session_id, backend, external_id, external_url,
		snapshot_size_bytes, created_at, revoked_at, view_count
		FROM task_shares WHERE id = ?`, id)
	return scanShareRow(row)
}

// ListByTaskSession returns every share row for a session ordered by creation
// time (newest first), including revoked rows.
func (r *Repository) ListByTaskSession(ctx context.Context, sessionID string) ([]*Share, error) {
	rows, err := r.reader.QueryxContext(ctx, `SELECT
		id, task_session_id, backend, external_id, external_url,
		snapshot_size_bytes, created_at, revoked_at, view_count
		FROM task_shares
		WHERE task_session_id = ?
		ORDER BY created_at DESC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list task_shares: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []*Share
	for rows.Next() {
		s, err := scanShareRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// MarkRevoked sets revoked_at on the share row. If the row is already revoked,
// the existing revoked_at is preserved.
func (r *Repository) MarkRevoked(ctx context.Context, id string, at time.Time) error {
	res, err := r.writer.ExecContext(ctx,
		`UPDATE task_shares SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		at.UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update task_share revoked_at: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		// Either missing or already revoked. Distinguish so the caller can
		// return a clean 404 vs treating the second call as a no-op.
		existing, getErr := r.GetByID(ctx, id)
		if errors.Is(getErr, ErrNotFound) {
			return ErrNotFound
		}
		if getErr != nil {
			return getErr
		}
		_ = existing // already revoked — caller treats this as success.
	}
	return nil
}

// scanShareRow is shared between QueryRowx and Queryx callers.
type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanShareRow(row rowScanner) (*Share, error) {
	var s Share
	var revokedAt sql.NullTime
	if err := row.Scan(
		&s.ID, &s.TaskSessionID, &s.Backend, &s.ExternalID, &s.ExternalURL,
		&s.SnapshotSizeBytes, &s.CreatedAt, &revokedAt, &s.ViewCount,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan task_share: %w", err)
	}
	if revokedAt.Valid {
		t := revokedAt.Time
		s.RevokedAt = &t
	}
	return &s, nil
}
