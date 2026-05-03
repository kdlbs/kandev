package jira

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Store persists the Jira configuration. Secret values are delegated to the
// shared encrypted secret store and not stored here. The configuration is a
// singleton — there is one Jira account per install, not one per workspace.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB

	// migratedFromWorkspace records the workspace_id of the row that was
	// promoted into the singleton during initSchema. Provider reads this to
	// migrate the per-workspace secret to the new global key. Empty when no
	// migration ran (fresh install or already migrated).
	migratedFromWorkspace string
}

// NewStore creates a new Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("jira schema init: %w", err)
	}
	return s, nil
}

// MigratedFromWorkspace returns the workspace_id of the row promoted to the
// singleton during the per-workspace → singleton schema migration, or "" when
// no migration ran. Provider uses this to copy the secret over.
func (s *Store) MigratedFromWorkspace() string {
	return s.migratedFromWorkspace
}

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS jira_configs (
		id TEXT PRIMARY KEY CHECK(id = 'singleton'),
		site_url TEXT NOT NULL,
		email TEXT NOT NULL DEFAULT '',
		auth_method TEXT NOT NULL,
		default_project_key TEXT NOT NULL DEFAULT '',
		last_checked_at DATETIME,
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS jira_issue_watches (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL,
		jql TEXT NOT NULL,
		agent_profile_id TEXT NOT NULL DEFAULT '',
		executor_profile_id TEXT NOT NULL DEFAULT '',
		prompt TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN NOT NULL DEFAULT 1,
		poll_interval_seconds INTEGER NOT NULL DEFAULT 300,
		last_polled_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_jira_issue_watches_workspace
		ON jira_issue_watches(workspace_id);

	CREATE TABLE IF NOT EXISTS jira_issue_watch_tasks (
		id TEXT PRIMARY KEY,
		issue_watch_id TEXT NOT NULL,
		issue_key TEXT NOT NULL,
		issue_url TEXT NOT NULL,
		task_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		UNIQUE(issue_watch_id, issue_key),
		FOREIGN KEY(issue_watch_id) REFERENCES jira_issue_watches(id) ON DELETE CASCADE
	);
`

// singletonID is the synthetic primary key of the (only) row in jira_configs.
// The CHECK constraint on the column makes inserting any other id an error,
// so the table can never accidentally grow back to per-workspace rows.
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
// jira_configs was keyed by workspace_id) and rewrites it into the singleton
// shape. Picks the most-recently-updated row as the surviving config and
// records the source workspace_id so the provider can migrate the secret.
//
// Idempotent: a fresh install has no legacy table and falls through to the
// CREATE TABLE IF NOT EXISTS in createTablesSQL.
func (s *Store) migrateLegacyPerWorkspaceTable() error {
	cols, err := s.tableColumns("jira_configs")
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

	// `last_checked_at`, `last_ok`, and `last_error` were added to the legacy
	// schema in a later release via ALTER TABLE. A deployment that upgrades
	// from the original schema would have a `workspace_id` column but not
	// these — selecting them unconditionally would crash startup. Build the
	// SELECT against only columns present in this database.
	healthCols := healthColumnsPresent(cols)
	selectCols := "workspace_id, site_url, email, auth_method, default_project_key"
	if healthCols {
		selectCols += ", last_checked_at, last_ok, last_error"
	} else {
		selectCols += ", NULL AS last_checked_at, 0 AS last_ok, '' AS last_error"
	}
	selectCols += ", created_at, updated_at"
	var sourceWorkspace, siteURL, email, authMethod, defaultProjectKey, lastError sql.NullString
	var lastCheckedAt sql.NullTime
	var lastOk sql.NullInt64
	var createdAt, updatedAt sql.NullTime
	row := tx.QueryRow(`SELECT ` + selectCols + ` FROM jira_configs ORDER BY updated_at DESC LIMIT 1`)
	switch err := row.Scan(&sourceWorkspace, &siteURL, &email, &authMethod, &defaultProjectKey,
		&lastCheckedAt, &lastOk, &lastError, &createdAt, &updatedAt); {
	case errors.Is(err, sql.ErrNoRows):
		// Empty legacy table — drop and let createTablesSQL build the new shape.
		if _, err := tx.Exec(`DROP TABLE jira_configs`); err != nil {
			return err
		}
		return tx.Commit()
	case err != nil:
		return err
	}
	if _, err := tx.Exec(`DROP TABLE jira_configs`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		CREATE TABLE jira_configs (
			id TEXT PRIMARY KEY CHECK(id = 'singleton'),
			site_url TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			auth_method TEXT NOT NULL,
			default_project_key TEXT NOT NULL DEFAULT '',
			last_checked_at DATETIME,
			last_ok INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO jira_configs (id, site_url, email, auth_method, default_project_key,
			last_checked_at, last_ok, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		singletonID, siteURL.String, email.String, authMethod.String, defaultProjectKey.String,
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

// healthColumnsPresent reports whether the legacy jira_configs table has the
// auth-health columns that were added in a later release. When all three are
// missing we fall back to NULL/zero defaults rather than crashing on the
// SELECT.
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

const selectConfigColumns = `site_url, email, auth_method, default_project_key,
		last_checked_at, last_ok, last_error, created_at, updated_at`

// GetConfig returns the singleton Jira config, or nil when no row exists.
func (s *Store) GetConfig(ctx context.Context) (*JiraConfig, error) {
	var cfg JiraConfig
	err := s.ro.GetContext(ctx, &cfg,
		`SELECT `+selectConfigColumns+` FROM jira_configs WHERE id = ?`, singletonID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpsertConfig inserts or updates the singleton config row. It never touches
// the secret store — callers must persist the token separately. The last_*
// health columns are deliberately not touched here; the poller owns those and
// writes them via UpdateAuthHealth.
func (s *Store) UpsertConfig(ctx context.Context, cfg *JiraConfig) error {
	now := time.Now().UTC()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO jira_configs (id, site_url, email, auth_method, default_project_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			site_url = excluded.site_url,
			email = excluded.email,
			auth_method = excluded.auth_method,
			default_project_key = excluded.default_project_key,
			updated_at = excluded.updated_at`,
		singletonID, cfg.SiteURL, cfg.Email, cfg.AuthMethod, cfg.DefaultProjectKey, cfg.CreatedAt, cfg.UpdatedAt)
	return err
}

// DeleteConfig removes the singleton config row. Secrets must be cleared
// separately by the caller.
func (s *Store) DeleteConfig(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM jira_configs WHERE id = ?`, singletonID)
	return err
}

// HasConfig reports whether the singleton row exists. Used by the auth-health
// poller to decide whether to probe at all.
func (s *Store) HasConfig(ctx context.Context) (bool, error) {
	var present int
	err := s.ro.GetContext(ctx, &present,
		`SELECT COUNT(*) FROM jira_configs WHERE id = ?`, singletonID)
	if err != nil {
		return false, err
	}
	return present > 0, nil
}

// UpdateAuthHealth records the result of a credential probe on the singleton
// config row. errMsg is the empty string when ok is true. If the row no longer
// exists (e.g. the user removed the config concurrently with the poller), the
// update is a silent no-op rather than an error.
func (s *Store) UpdateAuthHealth(ctx context.Context, ok bool, errMsg string, checkedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE jira_configs
		SET last_checked_at = ?, last_ok = ?, last_error = ?
		WHERE id = ?`,
		checkedAt, ok, errMsg, singletonID)
	return err
}

// --- Issue watch operations ---

const issueWatchColumns = `id, workspace_id, workflow_id, workflow_step_id, jql,
	agent_profile_id, executor_profile_id, prompt, enabled,
	poll_interval_seconds, last_polled_at, created_at, updated_at`

// CreateIssueWatch persists a new issue watch row. ID and timestamps are
// assigned here so callers can pass a partially-populated struct.
func (s *Store) CreateIssueWatch(ctx context.Context, w *IssueWatch) error {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	w.CreatedAt = now
	w.UpdatedAt = now
	if w.PollIntervalSeconds <= 0 {
		w.PollIntervalSeconds = DefaultIssueWatchPollInterval
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO jira_issue_watches (`+issueWatchColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.WorkspaceID, w.WorkflowID, w.WorkflowStepID, w.JQL,
		w.AgentProfileID, w.ExecutorProfileID, w.Prompt, w.Enabled,
		w.PollIntervalSeconds, w.LastPolledAt, w.CreatedAt, w.UpdatedAt)
	return err
}

// GetIssueWatch returns a single watch by ID, or nil when no row matches.
func (s *Store) GetIssueWatch(ctx context.Context, id string) (*IssueWatch, error) {
	var w IssueWatch
	err := s.ro.GetContext(ctx, &w,
		`SELECT `+issueWatchColumns+` FROM jira_issue_watches WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// ListIssueWatches returns all watches configured for a workspace, in
// insertion order. The UI uses this to render the watcher table.
func (s *Store) ListIssueWatches(ctx context.Context, workspaceID string) ([]*IssueWatch, error) {
	var watches []*IssueWatch
	err := s.ro.SelectContext(ctx, &watches,
		`SELECT `+issueWatchColumns+` FROM jira_issue_watches
		 WHERE workspace_id = ? ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	return watches, nil
}

// ListAllIssueWatches returns every watch across all workspaces, in insertion
// order. Used by the install-wide settings UI when no workspace filter is
// supplied — the table renders a Workspace column so the user can manage
// watches without first picking a workspace context.
func (s *Store) ListAllIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	var watches []*IssueWatch
	err := s.ro.SelectContext(ctx, &watches,
		`SELECT `+issueWatchColumns+` FROM jira_issue_watches ORDER BY workspace_id, created_at`)
	if err != nil {
		return nil, err
	}
	return watches, nil
}

// ListEnabledIssueWatches returns every enabled watch across all workspaces,
// used by the poller to decide what to query each tick.
func (s *Store) ListEnabledIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	var watches []*IssueWatch
	err := s.ro.SelectContext(ctx, &watches,
		`SELECT `+issueWatchColumns+` FROM jira_issue_watches
		 WHERE enabled = 1 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	return watches, nil
}

// UpdateIssueWatch overwrites the mutable fields of an existing watch row.
// updated_at is bumped automatically; last_polled_at is preserved unless the
// caller explicitly sets it.
func (s *Store) UpdateIssueWatch(ctx context.Context, w *IssueWatch) error {
	w.UpdatedAt = time.Now().UTC()
	if w.PollIntervalSeconds <= 0 {
		w.PollIntervalSeconds = DefaultIssueWatchPollInterval
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE jira_issue_watches SET workflow_id = ?, workflow_step_id = ?, jql = ?,
			agent_profile_id = ?, executor_profile_id = ?, prompt = ?,
			enabled = ?, poll_interval_seconds = ?, last_polled_at = ?, updated_at = ?
		WHERE id = ?`,
		w.WorkflowID, w.WorkflowStepID, w.JQL,
		w.AgentProfileID, w.ExecutorProfileID, w.Prompt,
		w.Enabled, w.PollIntervalSeconds, w.LastPolledAt, w.UpdatedAt, w.ID)
	return err
}

// UpdateIssueWatchLastPolled stamps the last-polled timestamp without touching
// the rest of the row. The poller calls this after every check so the UI can
// show "polled X seconds ago".
func (s *Store) UpdateIssueWatchLastPolled(ctx context.Context, id string, t time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE jira_issue_watches SET last_polled_at = ?, updated_at = ? WHERE id = ?`,
		t, time.Now().UTC(), id)
	return err
}

// DeleteIssueWatch removes a watch and (via FK ON DELETE CASCADE) its dedup
// rows in a single transaction. The explicit DELETE on the child table guards
// older databases where foreign_keys may not have been enabled at attach time.
func (s *Store) DeleteIssueWatch(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM jira_issue_watch_tasks WHERE issue_watch_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM jira_issue_watches WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ReserveIssueWatchTask atomically claims a slot for a (watch, ticket) pair via
// INSERT OR IGNORE. Returns true when this caller won the race and should
// proceed to create the task. False either means another handler already
// reserved the same ticket or the row already exists from a prior run.
func (s *Store) ReserveIssueWatchTask(ctx context.Context, watchID, issueKey, issueURL string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO jira_issue_watch_tasks (id, issue_watch_id, issue_key, issue_url, task_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), watchID, issueKey, issueURL, "", time.Now().UTC())
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

// AssignIssueWatchTaskID stamps the created task ID onto a previously-reserved
// dedup row. Returns an error if no reservation matches — callers should treat
// that as a programming bug since they only call this after a successful
// reservation.
func (s *Store) AssignIssueWatchTaskID(ctx context.Context, watchID, issueKey, taskID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE jira_issue_watch_tasks SET task_id = ?
		WHERE issue_watch_id = ? AND issue_key = ?`,
		taskID, watchID, issueKey)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("assign task ID: reservation row not found for watch=%s issue=%s", watchID, issueKey)
	}
	return nil
}

// ReleaseIssueWatchTask drops a reservation so the next poll can retry. Used
// when task creation fails after a successful reserve.
func (s *Store) ReleaseIssueWatchTask(ctx context.Context, watchID, issueKey string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM jira_issue_watch_tasks WHERE issue_watch_id = ? AND issue_key = ?`,
		watchID, issueKey)
	return err
}

// ListSeenIssueKeys returns the set of ticket keys already reserved against a
// watch. Returning a set in one query is cheaper than per-ticket existence
// checks — a single JQL search can return up to 50 tickets per tick, so the
// per-call savings scale with the workspace's watch count.
func (s *Store) ListSeenIssueKeys(ctx context.Context, watchID string) (map[string]struct{}, error) {
	var keys []string
	err := s.ro.SelectContext(ctx, &keys,
		`SELECT issue_key FROM jira_issue_watch_tasks WHERE issue_watch_id = ?`, watchID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		out[k] = struct{}{}
	}
	return out, nil
}
