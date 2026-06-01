package sentry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Store persists the install-wide Sentry configuration. The secret token is
// delegated to the shared encrypted secret store and not stored here.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// NewStore creates a new Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("sentry schema init: %w", err)
	}
	return s, nil
}

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS sentry_configs (
		id TEXT PRIMARY KEY CHECK(id = 'singleton'),
		auth_method TEXT NOT NULL,
		default_org_slug TEXT NOT NULL DEFAULT '',
		default_project_slug TEXT NOT NULL DEFAULT '',
		last_checked_at DATETIME,
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS sentry_issue_watches (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL,
		filter_json TEXT NOT NULL DEFAULT '{}',
		agent_profile_id TEXT NOT NULL DEFAULT '',
		executor_profile_id TEXT NOT NULL DEFAULT '',
		prompt TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT 1,
		poll_interval_seconds INTEGER NOT NULL DEFAULT 300,
		max_inflight_tasks INTEGER DEFAULT 5,
		last_polled_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_sentry_issue_watches_workspace
		ON sentry_issue_watches(workspace_id);

	CREATE TABLE IF NOT EXISTS sentry_issue_watch_tasks (
		id TEXT PRIMARY KEY,
		issue_watch_id TEXT NOT NULL,
		issue_short_id TEXT NOT NULL,
		issue_url TEXT NOT NULL,
		task_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		UNIQUE(issue_watch_id, issue_short_id),
		FOREIGN KEY(issue_watch_id) REFERENCES sentry_issue_watches(id) ON DELETE CASCADE
	);
`

// singletonID is the synthetic primary key of the (only) row in sentry_configs.
const singletonID = "singleton"

func (s *Store) initSchema() error {
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	return s.addMaxInflightTasksColumn()
}

// addMaxInflightTasksColumn brings older databases up to the current schema by
// adding the max_inflight_tasks column to sentry_issue_watches when missing.
// Existing rows backfill to the default (5). A fresh install hits the
// column-already-present branch since createTablesSQL declares the column.
func (s *Store) addMaxInflightTasksColumn() error {
	cols, err := s.tableColumns("sentry_issue_watches")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["max_inflight_tasks"]; ok {
		return nil
	}
	if _, err := s.db.Exec(`ALTER TABLE sentry_issue_watches ADD COLUMN max_inflight_tasks INTEGER DEFAULT 5`); err != nil {
		return fmt.Errorf("add max_inflight_tasks column: %w", err)
	}
	return nil
}

// tableColumns returns the set of column names for a table via PRAGMA
// table_info, used by the lightweight ADD COLUMN migrations above.
func (s *Store) tableColumns(table string) (map[string]struct{}, error) {
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	cols := make(map[string]struct{})
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[name] = struct{}{}
	}
	return cols, rows.Err()
}

const selectConfigColumns = `auth_method, default_org_slug, default_project_slug,
		last_checked_at, last_ok, last_error, created_at, updated_at`

// GetConfig returns the singleton Sentry config, or nil when no row exists.
func (s *Store) GetConfig(ctx context.Context) (*SentryConfig, error) {
	var cfg SentryConfig
	err := s.ro.GetContext(ctx, &cfg,
		`SELECT `+selectConfigColumns+` FROM sentry_configs WHERE id = ?`, singletonID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpsertConfig inserts or updates the singleton config row. The last_* health
// columns are owned by the poller and written via UpdateAuthHealth.
func (s *Store) UpsertConfig(ctx context.Context, cfg *SentryConfig) error {
	now := time.Now().UTC()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sentry_configs (id, auth_method, default_org_slug, default_project_slug, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			auth_method = excluded.auth_method,
			default_org_slug = excluded.default_org_slug,
			default_project_slug = excluded.default_project_slug,
			updated_at = excluded.updated_at`,
		singletonID, cfg.AuthMethod, cfg.DefaultOrgSlug, cfg.DefaultProjectSlug,
		cfg.CreatedAt, cfg.UpdatedAt)
	return err
}

// DeleteConfig removes the singleton config row.
func (s *Store) DeleteConfig(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sentry_configs WHERE id = ?`, singletonID)
	return err
}

// HasConfig reports whether the singleton row exists. Used by the auth-health
// poller to decide whether to probe at all.
func (s *Store) HasConfig(ctx context.Context) (bool, error) {
	var present int
	err := s.ro.GetContext(ctx, &present,
		`SELECT COUNT(*) FROM sentry_configs WHERE id = ?`, singletonID)
	if err != nil {
		return false, err
	}
	return present > 0, nil
}

// UpdateAuthHealth records the result of a credential probe.
func (s *Store) UpdateAuthHealth(ctx context.Context, ok bool, errMsg string, checkedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sentry_configs
		SET last_checked_at = ?, last_ok = ?, last_error = ?
		WHERE id = ?`,
		checkedAt, ok, errMsg, singletonID)
	return err
}
