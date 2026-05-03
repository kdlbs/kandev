package linear

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Store persists the install-wide Linear configuration. The secret API key is
// delegated to the shared encrypted secret store and not stored here.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB

	// migratedFromWorkspace records the workspace_id of the row that was
	// promoted into the singleton during initSchema. Provider reads this to
	// migrate the per-workspace secret to the new global key. Empty when no
	// migration ran.
	migratedFromWorkspace string
}

// NewStore creates a new Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("linear schema init: %w", err)
	}
	return s, nil
}

// MigratedFromWorkspace returns the workspace_id of the row promoted to the
// singleton during the per-workspace → singleton schema migration, or "" when
// no migration ran.
func (s *Store) MigratedFromWorkspace() string {
	return s.migratedFromWorkspace
}

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS linear_configs (
		id TEXT PRIMARY KEY CHECK(id = 'singleton'),
		auth_method TEXT NOT NULL,
		default_team_key TEXT NOT NULL DEFAULT '',
		org_slug TEXT NOT NULL DEFAULT '',
		last_checked_at DATETIME,
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
`

// singletonID is the synthetic primary key of the (only) row in linear_configs.
const singletonID = "singleton"

func (s *Store) initSchema() error {
	if err := s.migrateLegacyPerWorkspaceTable(); err != nil {
		return err
	}
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	return nil
}

// migrateLegacyPerWorkspaceTable detects the pre-singleton schema (where
// linear_configs was keyed by workspace_id) and rewrites it into the singleton
// shape. Picks the most-recently-updated row and records the source
// workspace_id so the provider can migrate the secret.
func (s *Store) migrateLegacyPerWorkspaceTable() error {
	cols, err := s.tableColumns("linear_configs")
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return nil
	}
	if _, hasWorkspace := cols["workspace_id"]; !hasWorkspace {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// `org_slug`, `last_checked_at`, `last_ok`, and `last_error` were added to
	// the legacy schema in later releases. A deployment that upgrades from the
	// original schema would have a `workspace_id` column but not these —
	// selecting them unconditionally would crash startup. Build the SELECT
	// against only columns present in this database. Mirrors the Jira fix.
	healthCols := healthColumnsPresent(cols)
	_, hasOrgSlug := cols["org_slug"]
	selectCols := "workspace_id, auth_method, default_team_key"
	if hasOrgSlug {
		selectCols += ", org_slug"
	} else {
		selectCols += ", '' AS org_slug"
	}
	if healthCols {
		selectCols += ", last_checked_at, last_ok, last_error"
	} else {
		selectCols += ", NULL AS last_checked_at, 0 AS last_ok, '' AS last_error"
	}
	selectCols += ", created_at, updated_at"
	var sourceWorkspace, authMethod, defaultTeamKey, orgSlug, lastError sql.NullString
	var lastCheckedAt sql.NullTime
	var lastOk sql.NullInt64
	var createdAt, updatedAt sql.NullTime
	row := tx.QueryRow(`SELECT ` + selectCols + ` FROM linear_configs ORDER BY updated_at DESC LIMIT 1`)
	switch err := row.Scan(&sourceWorkspace, &authMethod, &defaultTeamKey, &orgSlug,
		&lastCheckedAt, &lastOk, &lastError, &createdAt, &updatedAt); {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.Exec(`DROP TABLE linear_configs`); err != nil {
			return err
		}
		return tx.Commit()
	case err != nil:
		return err
	}
	if _, err := tx.Exec(`DROP TABLE linear_configs`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		CREATE TABLE linear_configs (
			id TEXT PRIMARY KEY CHECK(id = 'singleton'),
			auth_method TEXT NOT NULL,
			default_team_key TEXT NOT NULL DEFAULT '',
			org_slug TEXT NOT NULL DEFAULT '',
			last_checked_at DATETIME,
			last_ok INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO linear_configs (id, auth_method, default_team_key, org_slug,
			last_checked_at, last_ok, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		singletonID, authMethod.String, defaultTeamKey.String, orgSlug.String,
		nullableTime(lastCheckedAt), lastOk.Int64, lastError.String,
		nullableTime(createdAt), nullableTime(updatedAt)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.migratedFromWorkspace = sourceWorkspace.String
	return nil
}

// healthColumnsPresent reports whether the legacy linear_configs table has the
// auth-health columns that were added in a later release. When all three are
// missing we fall back to NULL/zero defaults rather than crashing on the
// SELECT. Mirrors the helper of the same name in jira/store.go.
func healthColumnsPresent(cols map[string]struct{}) bool {
	for _, name := range []string{"last_checked_at", "last_ok", "last_error"} {
		if _, ok := cols[name]; !ok {
			return false
		}
	}
	return true
}

func nullableTime(t sql.NullTime) interface{} {
	if !t.Valid {
		return nil
	}
	return t.Time
}

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

const selectConfigColumns = `auth_method, default_team_key, org_slug,
		last_checked_at, last_ok, last_error, created_at, updated_at`

// GetConfig returns the singleton Linear config, or nil when no row exists.
func (s *Store) GetConfig(ctx context.Context) (*LinearConfig, error) {
	var cfg LinearConfig
	err := s.ro.GetContext(ctx, &cfg,
		`SELECT `+selectConfigColumns+` FROM linear_configs WHERE id = ?`, singletonID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpsertConfig inserts or updates the singleton config row. The last_* health
// columns and org_slug are deliberately not touched here; the poller owns
// those and writes them via UpdateAuthHealth.
func (s *Store) UpsertConfig(ctx context.Context, cfg *LinearConfig) error {
	now := time.Now().UTC()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO linear_configs (id, auth_method, default_team_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			auth_method = excluded.auth_method,
			default_team_key = excluded.default_team_key,
			updated_at = excluded.updated_at`,
		singletonID, cfg.AuthMethod, cfg.DefaultTeamKey, cfg.CreatedAt, cfg.UpdatedAt)
	return err
}

// DeleteConfig removes the singleton config row.
func (s *Store) DeleteConfig(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM linear_configs WHERE id = ?`, singletonID)
	return err
}

// HasConfig reports whether the singleton row exists. Used by the auth-health
// poller to decide whether to probe at all.
func (s *Store) HasConfig(ctx context.Context) (bool, error) {
	var present int
	err := s.ro.GetContext(ctx, &present,
		`SELECT COUNT(*) FROM linear_configs WHERE id = ?`, singletonID)
	if err != nil {
		return false, err
	}
	return present > 0, nil
}

// UpdateAuthHealth records the result of a credential probe. orgSlug is
// captured opportunistically from successful probes; pass "" to leave the
// existing slug unchanged.
func (s *Store) UpdateAuthHealth(ctx context.Context, ok bool, errMsg, orgSlug string, checkedAt time.Time) error {
	if orgSlug != "" {
		_, err := s.db.ExecContext(ctx, `
			UPDATE linear_configs
			SET last_checked_at = ?, last_ok = ?, last_error = ?, org_slug = ?
			WHERE id = ?`,
			checkedAt, ok, errMsg, orgSlug, singletonID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE linear_configs
		SET last_checked_at = ?, last_ok = ?, last_error = ?
		WHERE id = ?`,
		checkedAt, ok, errMsg, singletonID)
	return err
}
