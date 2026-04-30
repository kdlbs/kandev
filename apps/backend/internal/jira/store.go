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

// Store persists Jira workspace configurations. Secret values are delegated to
// the shared encrypted secret store and not stored here.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// NewStore creates a new Store and initializes the schema if needed.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("jira schema init: %w", err)
	}
	return s, nil
}

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS jira_configs (
		workspace_id TEXT PRIMARY KEY,
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

// addedColumns lists columns introduced after the initial schema. SQLite has no
// portable `ADD COLUMN IF NOT EXISTS`, so we ask sqlite_master for the existing
// CREATE TABLE statement and only ALTER when the column name is missing.
var addedColumns = []struct {
	name string
	sql  string
}{
	{"last_checked_at", "ALTER TABLE jira_configs ADD COLUMN last_checked_at DATETIME"},
	{"last_ok", "ALTER TABLE jira_configs ADD COLUMN last_ok INTEGER NOT NULL DEFAULT 0"},
	{"last_error", "ALTER TABLE jira_configs ADD COLUMN last_error TEXT NOT NULL DEFAULT ''"},
}

func (s *Store) initSchema() error {
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	return s.migrateAddedColumns()
}

// migrateAddedColumns applies ALTER TABLE statements for columns introduced
// after the initial schema. Existing databases created with the old CREATE
// TABLE need these columns backfilled; new databases already have them from
// createTablesSQL and the ALTERs are skipped.
func (s *Store) migrateAddedColumns() error {
	existing, err := s.tableColumns("jira_configs")
	if err != nil {
		return err
	}
	for _, col := range addedColumns {
		if _, ok := existing[col.name]; ok {
			continue
		}
		if _, err := s.db.Exec(col.sql); err != nil {
			return fmt.Errorf("add column %s: %w", col.name, err)
		}
	}
	return nil
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

const selectConfigColumns = `workspace_id, site_url, email, auth_method, default_project_key,
		last_checked_at, last_ok, last_error, created_at, updated_at`

// GetConfig returns the Jira config for a workspace, or nil when no row exists.
func (s *Store) GetConfig(ctx context.Context, workspaceID string) (*JiraConfig, error) {
	var cfg JiraConfig
	err := s.ro.GetContext(ctx, &cfg,
		`SELECT `+selectConfigColumns+` FROM jira_configs WHERE workspace_id = ?`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpsertConfig inserts or updates the config row for a workspace. It never
// touches the secret store — callers must persist the token separately. The
// last_* health columns are deliberately not touched here; the poller owns
// those and writes them via UpdateAuthHealth.
func (s *Store) UpsertConfig(ctx context.Context, cfg *JiraConfig) error {
	now := time.Now().UTC()
	if cfg.CreatedAt.IsZero() {
		cfg.CreatedAt = now
	}
	cfg.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO jira_configs (workspace_id, site_url, email, auth_method, default_project_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			site_url = excluded.site_url,
			email = excluded.email,
			auth_method = excluded.auth_method,
			default_project_key = excluded.default_project_key,
			updated_at = excluded.updated_at`,
		cfg.WorkspaceID, cfg.SiteURL, cfg.Email, cfg.AuthMethod, cfg.DefaultProjectKey, cfg.CreatedAt, cfg.UpdatedAt)
	return err
}

// DeleteConfig removes the Jira config row for a workspace. Secrets must be
// cleared separately by the caller.
func (s *Store) DeleteConfig(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM jira_configs WHERE workspace_id = ?`, workspaceID)
	return err
}

// ListConfiguredWorkspaces returns the IDs of all workspaces that have a Jira
// config row. Used by the auth-health poller to know which workspaces to probe.
func (s *Store) ListConfiguredWorkspaces(ctx context.Context) ([]string, error) {
	var ids []string
	err := s.ro.SelectContext(ctx, &ids,
		`SELECT workspace_id FROM jira_configs ORDER BY workspace_id`)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// UpdateAuthHealth records the result of a credential probe for a workspace.
// errMsg is the empty string when ok is true. If the workspace row no longer
// exists (e.g. the user removed the config concurrently with the poller), the
// update is a silent no-op rather than an error.
func (s *Store) UpdateAuthHealth(ctx context.Context, workspaceID string, ok bool, errMsg string, checkedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE jira_configs
		SET last_checked_at = ?, last_ok = ?, last_error = ?
		WHERE workspace_id = ?`,
		checkedAt, ok, errMsg, workspaceID)
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

// HasIssueWatchTask reports whether a reservation already exists for a ticket.
// Used by the poller to filter out previously-seen tickets before publishing
// duplicate events.
func (s *Store) HasIssueWatchTask(ctx context.Context, watchID, issueKey string) (bool, error) {
	var n int
	err := s.ro.GetContext(ctx, &n,
		`SELECT COUNT(*) FROM jira_issue_watch_tasks WHERE issue_watch_id = ? AND issue_key = ?`,
		watchID, issueKey)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ListSeenIssueKeys returns the set of ticket keys already reserved against a
// watch. Cheaper than calling HasIssueWatchTask once per ticket — a single
// JQL search can return up to 50 tickets per tick, so the per-call savings
// scale with the workspace's watch count.
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
