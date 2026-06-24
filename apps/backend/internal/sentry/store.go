package sentry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Store persists Sentry instance configurations. Each row is one named Sentry
// instance (a SaaS org or a self-hosted host). The secret token for an instance
// is delegated to the shared encrypted secret store (keyed by secretKeyFor) and
// is not stored here.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB

	// migratedSingletonID records the generated id assigned to the legacy
	// install-wide ('singleton') config row when it was promoted into the
	// multi-instance schema. Provider reads it to migrate the stored secret to
	// the per-instance key, and addWatchInstanceColumn reads it to backfill
	// existing watches. Empty when no migration ran (fresh install or already
	// migrated).
	migratedSingletonID string
}

// NewStore creates a new Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("sentry schema init: %w", err)
	}
	return s, nil
}

// MigratedSingletonID returns the id assigned to the legacy singleton config
// row during the singleton→multi-instance migration, or "" when no migration
// ran. Provider uses this to migrate the stored secret to the per-instance key.
func (s *Store) MigratedSingletonID() string {
	return s.migratedSingletonID
}

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS sentry_configs (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT '',
		auth_method TEXT NOT NULL,
		url TEXT NOT NULL DEFAULT 'https://sentry.io',
		last_checked_at DATETIME,
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS sentry_issue_watches (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		sentry_instance_id TEXT NOT NULL DEFAULT '',
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
		last_error TEXT NOT NULL DEFAULT '',
		last_error_at DATETIME,
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

// legacySingletonID is the synthetic primary key the integration used while it
// was an install-wide singleton. Detected and promoted to a generated id by
// migrateConfigsToInstances.
const legacySingletonID = "singleton"

// initSchema creates the integration tables when absent and applies the
// migrations that bring older databases to the current multi-instance schema.
// Order matters: the url column must exist before the table rebuild reads it,
// and the rebuild must record migratedSingletonID before the watch backfill.
func (s *Store) initSchema() error {
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	if err := s.addConfigURLColumn(); err != nil {
		return err
	}
	if err := s.migrateConfigsToInstances(); err != nil {
		return err
	}
	if err := s.addMaxInflightTasksColumn(); err != nil {
		return err
	}
	if err := s.addIssueWatchLastErrorColumns(); err != nil {
		return err
	}
	return s.addWatchInstanceColumn()
}

// addConfigURLColumn brings older databases up to the current schema by adding
// the url column to sentry_configs when missing. Existing rows — all SaaS
// installs, since this predates self-hosted support — backfill to the
// sentry.io default (mirrors DefaultSentryURL). Runs before the multi-instance
// rebuild so the rebuild's INSERT ... SELECT can read a url for every row.
func (s *Store) addConfigURLColumn() error {
	cols, err := s.tableColumns("sentry_configs")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["url"]; ok {
		return nil
	}
	if _, err := s.db.Exec(`ALTER TABLE sentry_configs ADD COLUMN url TEXT NOT NULL DEFAULT 'https://sentry.io'`); err != nil {
		return fmt.Errorf("add url column: %w", err)
	}
	return nil
}

// migrateConfigsToInstances rebuilds the legacy single-tenant sentry_configs
// table (PRIMARY KEY CHECK(id='singleton'), no name column) into the
// multi-instance shape. SQLite cannot DROP the CHECK via ALTER, so the table is
// rebuilt: create the new shape, copy the singleton row under a generated id
// (named after its instance URL host), drop the old table, and rename. No
// foreign key references sentry_configs, so the drop is safe. Idempotent: a
// table that already has a name column is left untouched.
func (s *Store) migrateConfigsToInstances() error {
	cols, err := s.tableColumns("sentry_configs")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["name"]; ok {
		return nil
	}

	var existing struct {
		AuthMethod    string     `db:"auth_method"`
		URL           string     `db:"url"`
		LastCheckedAt *time.Time `db:"last_checked_at"`
		LastOk        bool       `db:"last_ok"`
		LastError     string     `db:"last_error"`
		CreatedAt     time.Time  `db:"created_at"`
		UpdatedAt     time.Time  `db:"updated_at"`
	}
	hasRow := true
	err = s.ro.Get(&existing,
		`SELECT auth_method, url, last_checked_at, last_ok,
			COALESCE(last_error, '') AS last_error, created_at, updated_at
		 FROM sentry_configs WHERE id = ?`, legacySingletonID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		hasRow = false
	case err != nil:
		return fmt.Errorf("read legacy singleton config: %w", err)
	}

	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`
		CREATE TABLE sentry_configs_new (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL DEFAULT '',
			auth_method TEXT NOT NULL,
			url TEXT NOT NULL DEFAULT 'https://sentry.io',
			last_checked_at DATETIME,
			last_ok INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`); err != nil {
		return fmt.Errorf("create rebuilt configs table: %w", err)
	}
	newID := ""
	if hasRow {
		newID = uuid.New().String()
		if _, err := tx.Exec(`
			INSERT INTO sentry_configs_new
				(id, name, auth_method, url, last_checked_at, last_ok, last_error, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			newID, instanceNameFromURL(existing.URL), existing.AuthMethod, existing.URL,
			existing.LastCheckedAt, existing.LastOk, existing.LastError,
			existing.CreatedAt, existing.UpdatedAt); err != nil {
			return fmt.Errorf("copy legacy config row: %w", err)
		}
	}
	if _, err := tx.Exec(`DROP TABLE sentry_configs`); err != nil {
		return fmt.Errorf("drop legacy configs table: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE sentry_configs_new RENAME TO sentry_configs`); err != nil {
		return fmt.Errorf("rename rebuilt configs table: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.migratedSingletonID = newID
	return nil
}

// instanceNameFromURL derives a human-readable default instance name from an
// instance base URL: the host when parseable (e.g. "sentry.io"), the raw value
// as a fallback, or "Sentry" when blank.
func instanceNameFromURL(raw string) string {
	if u, err := url.Parse(strings.TrimSpace(raw)); err == nil && u.Host != "" {
		return u.Host
	}
	if strings.TrimSpace(raw) != "" {
		return strings.TrimSpace(raw)
	}
	return "Sentry"
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

// addIssueWatchLastErrorColumns brings older databases up to the current
// schema by appending last_error / last_error_at to sentry_issue_watches when
// missing. Fresh installs hit the column-already-present branch since
// createTablesSQL declares both columns. Idempotent — column lookup before
// each ALTER avoids the "duplicate column name" error.
func (s *Store) addIssueWatchLastErrorColumns() error {
	cols, err := s.tableColumns("sentry_issue_watches")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["last_error"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE sentry_issue_watches ADD COLUMN last_error TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add last_error column: %w", err)
		}
	}
	if _, ok := cols["last_error_at"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE sentry_issue_watches ADD COLUMN last_error_at DATETIME`); err != nil {
			return fmt.Errorf("add last_error_at column: %w", err)
		}
	}
	return nil
}

// addWatchInstanceColumn adds the sentry_instance_id column to
// sentry_issue_watches when missing, backfills existing watches to the migrated
// singleton instance id (so pre-multi-instance watches keep firing against the
// promoted config), and ensures the lookup index exists. Idempotent.
func (s *Store) addWatchInstanceColumn() error {
	cols, err := s.tableColumns("sentry_issue_watches")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, ok := cols["sentry_instance_id"]; !ok {
		if _, err := s.db.Exec(`ALTER TABLE sentry_issue_watches ADD COLUMN sentry_instance_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add sentry_instance_id column: %w", err)
		}
	}
	if s.migratedSingletonID != "" {
		if _, err := s.db.Exec(
			`UPDATE sentry_issue_watches SET sentry_instance_id = ? WHERE sentry_instance_id = ''`,
			s.migratedSingletonID); err != nil {
			return fmt.Errorf("backfill sentry_instance_id: %w", err)
		}
	}
	if _, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_sentry_issue_watches_instance ON sentry_issue_watches(sentry_instance_id)`); err != nil {
		return fmt.Errorf("create instance index: %w", err)
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

const selectConfigColumns = `id, name, auth_method, url,
		last_checked_at, last_ok, last_error, created_at, updated_at`

// ListConfigs returns every configured Sentry instance, oldest first.
func (s *Store) ListConfigs(ctx context.Context) ([]*SentryConfig, error) {
	var rows []*SentryConfig
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT `+selectConfigColumns+` FROM sentry_configs ORDER BY created_at, id`); err != nil {
		return nil, err
	}
	return rows, nil
}

// GetConfig returns one instance config by id, or nil when no row matches.
func (s *Store) GetConfig(ctx context.Context, id string) (*SentryConfig, error) {
	var cfg SentryConfig
	err := s.ro.GetContext(ctx, &cfg,
		`SELECT `+selectConfigColumns+` FROM sentry_configs WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// CreateConfig inserts a new instance config row. ID and timestamps are
// assigned here when unset so callers can pass a partially-populated struct.
func (s *Store) CreateConfig(ctx context.Context, cfg *SentryConfig) error {
	if cfg.ID == "" {
		cfg.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sentry_configs (id, name, auth_method, url, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		cfg.ID, cfg.Name, cfg.AuthMethod, cfg.URL, cfg.CreatedAt, cfg.UpdatedAt)
	return err
}

// UpdateConfig overwrites the mutable fields of an existing instance config.
// The last_* health columns are owned by the poller (UpdateAuthHealth) and are
// left untouched here.
func (s *Store) UpdateConfig(ctx context.Context, cfg *SentryConfig) error {
	cfg.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE sentry_configs SET name = ?, auth_method = ?, url = ?, updated_at = ?
		WHERE id = ?`,
		cfg.Name, cfg.AuthMethod, cfg.URL, cfg.UpdatedAt, cfg.ID)
	return err
}

// DeleteConfig removes one instance config row by id.
func (s *Store) DeleteConfig(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sentry_configs WHERE id = ?`, id)
	return err
}

// HasAnyConfig reports whether at least one instance is configured. Used by the
// auth-health poller to decide whether to probe at all.
func (s *Store) HasAnyConfig(ctx context.Context) (bool, error) {
	var present int
	if err := s.ro.GetContext(ctx, &present, `SELECT COUNT(*) FROM sentry_configs`); err != nil {
		return false, err
	}
	return present > 0, nil
}

// CountWatchesForInstance returns how many issue watches reference the instance.
// The instance-delete flow uses this to block deletion of an in-use instance.
func (s *Store) CountWatchesForInstance(ctx context.Context, instanceID string) (int, error) {
	var n int
	if err := s.ro.GetContext(ctx, &n,
		`SELECT COUNT(*) FROM sentry_issue_watches WHERE sentry_instance_id = ?`, instanceID); err != nil {
		return 0, err
	}
	return n, nil
}

// UpdateAuthHealth records the result of a credential probe for one instance.
func (s *Store) UpdateAuthHealth(ctx context.Context, id string, ok bool, errMsg string, checkedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sentry_configs
		SET last_checked_at = ?, last_ok = ?, last_error = ?
		WHERE id = ?`,
		checkedAt, ok, errMsg, id)
	return err
}
