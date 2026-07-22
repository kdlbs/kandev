package gitlab

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Store backs the task↔MR association used by the topbar / review surface.
// Parallel to internal/github.Store's TaskPR plumbing. Kept intentionally
// minimal in v1 — watchers and review queues live behind their own tables
// when phase 3 lands.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// NewStore initialises the gitlab Store and creates its tables. db is the
// writer pool; ro is the read-only pool (same SQLite file, separate handle
// to avoid serialising reads behind writes). Both must be non-nil.
func NewStore(db, ro *sqlx.DB) (*Store, error) {
	if db == nil || ro == nil {
		return nil, errors.New("gitlab store: db and ro must be non-nil")
	}
	s := &Store{db: db, ro: ro}
	if err := s.createTables(); err != nil {
		return nil, fmt.Errorf("create gitlab tables: %w", err)
	}
	return s, nil
}

const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS gitlab_configs (
		workspace_id TEXT PRIMARY KEY,
		host TEXT NOT NULL,
		auth_method TEXT NOT NULL,
		username TEXT NOT NULL DEFAULT '',
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		last_checked_at DATETIME,
		revision INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
		-- The workspaces table is created before integration stores in production.
		-- Isolated package tests may omit it, which SQLite permits at DDL time.
		, FOREIGN KEY(workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS gitlab_task_mrs (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		host TEXT NOT NULL DEFAULT '',
		project_path TEXT NOT NULL,
		mr_iid INTEGER NOT NULL,
		mr_url TEXT NOT NULL,
		mr_title TEXT NOT NULL,
		head_branch TEXT NOT NULL,
		base_branch TEXT NOT NULL,
		author_username TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT 'open',
		approval_state TEXT NOT NULL DEFAULT '',
		pipeline_state TEXT NOT NULL DEFAULT '',
		merge_status TEXT NOT NULL DEFAULT '',
		draft INTEGER NOT NULL DEFAULT 0,
		approval_count INTEGER DEFAULT 0,
		required_approvals INTEGER DEFAULT 0,
		pipeline_jobs_total INTEGER DEFAULT 0,
		pipeline_jobs_pass INTEGER DEFAULT 0,
		created_at DATETIME NOT NULL,
		merged_at DATETIME,
		closed_at DATETIME,
		last_synced_at DATETIME,
		updated_at DATETIME NOT NULL,
		UNIQUE(task_id, repository_id, project_path, mr_iid)
	);
	CREATE INDEX IF NOT EXISTS idx_gitlab_task_mrs_task_id ON gitlab_task_mrs(task_id);

	CREATE TABLE IF NOT EXISTS gitlab_mr_watches (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		project_path TEXT NOT NULL,
		mr_iid INTEGER NOT NULL,
		branch TEXT NOT NULL,
		last_checked_at DATETIME,
		last_note_at DATETIME,
		last_pipeline_state TEXT DEFAULT '',
		last_approval_state TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(session_id, repository_id)
	);
	CREATE INDEX IF NOT EXISTS idx_gitlab_mr_watches_task_id ON gitlab_mr_watches(task_id);

	CREATE TABLE IF NOT EXISTS gitlab_review_watches (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL,
		projects TEXT NOT NULL DEFAULT '[]',
		agent_profile_id TEXT NOT NULL,
		executor_profile_id TEXT NOT NULL,
		prompt TEXT DEFAULT '',
		repository_id TEXT NOT NULL DEFAULT '',
		base_branch TEXT NOT NULL DEFAULT '',
		review_scope TEXT NOT NULL DEFAULT 'user_and_teams',
		custom_query TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN DEFAULT 1,
		poll_interval_seconds INTEGER DEFAULT 300,
		cleanup_policy TEXT NOT NULL DEFAULT 'auto',
		max_inflight_tasks INTEGER,
		generation INTEGER NOT NULL DEFAULT 1,
		deleting BOOLEAN NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		last_error_at DATETIME,
		last_polled_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_gitlab_review_watches_workspace_id ON gitlab_review_watches(workspace_id);

	CREATE TABLE IF NOT EXISTS gitlab_review_mr_tasks (
		id TEXT PRIMARY KEY,
		review_watch_id TEXT NOT NULL,
		project_path TEXT NOT NULL DEFAULT '',
		mr_iid INTEGER NOT NULL,
		mr_url TEXT NOT NULL,
		task_id TEXT NOT NULL,
		generation INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL,
		UNIQUE(review_watch_id, project_path, mr_iid)
	);

	CREATE TABLE IF NOT EXISTS gitlab_issue_watches (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL,
		projects TEXT NOT NULL DEFAULT '[]',
		agent_profile_id TEXT NOT NULL,
		executor_profile_id TEXT NOT NULL,
		prompt TEXT DEFAULT '',
		repository_id TEXT NOT NULL DEFAULT '',
		base_branch TEXT NOT NULL DEFAULT '',
		labels TEXT NOT NULL DEFAULT '[]',
		custom_query TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN DEFAULT 1,
		poll_interval_seconds INTEGER DEFAULT 300,
		cleanup_policy TEXT NOT NULL DEFAULT 'auto',
		max_inflight_tasks INTEGER,
		generation INTEGER NOT NULL DEFAULT 1,
		deleting BOOLEAN NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		last_error_at DATETIME,
		last_polled_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_gitlab_issue_watches_workspace_id ON gitlab_issue_watches(workspace_id);

	CREATE TABLE IF NOT EXISTS gitlab_issue_watch_tasks (
		id TEXT PRIMARY KEY,
		issue_watch_id TEXT NOT NULL,
		project_path TEXT NOT NULL DEFAULT '',
		issue_iid INTEGER NOT NULL,
		issue_url TEXT NOT NULL,
		task_id TEXT NOT NULL,
		generation INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL,
		UNIQUE(issue_watch_id, project_path, issue_iid)
	);

	CREATE TABLE IF NOT EXISTS gitlab_action_presets (
		workspace_id TEXT PRIMARY KEY,
		mr_presets TEXT NOT NULL DEFAULT '[]',
		issue_presets TEXT NOT NULL DEFAULT '[]',
		updated_at DATETIME NOT NULL
	);
`

func (s *Store) createTables() error {
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	if err := s.migrateConfigRevision(); err != nil {
		return err
	}
	return s.migrateWatchColumns()
}

func (s *Store) migrateConfigRevision() error {
	existing, err := s.tableColumns("gitlab_configs")
	if err != nil {
		return err
	}
	if _, ok := existing["revision"]; ok {
		return nil
	}
	if _, err := s.db.Exec(`ALTER TABLE gitlab_configs ADD COLUMN revision INTEGER NOT NULL DEFAULT 1`); err != nil {
		return fmt.Errorf("migrate gitlab_configs.revision: %w", err)
	}
	return nil
}

func (s *Store) migrateWatchColumns() error {
	columns := []struct {
		name string
		ddl  string
	}{
		{"repository_id", "TEXT NOT NULL DEFAULT ''"},
		{"base_branch", "TEXT NOT NULL DEFAULT ''"},
		{"max_inflight_tasks", "INTEGER"},
		{"last_error", "TEXT NOT NULL DEFAULT ''"},
		{"last_error_at", "DATETIME"},
		{"generation", "INTEGER NOT NULL DEFAULT 1"},
		{"deleting", "BOOLEAN NOT NULL DEFAULT 0"},
	}
	for _, table := range []string{"gitlab_review_watches", "gitlab_issue_watches"} {
		existing, err := s.tableColumns(table)
		if err != nil {
			return err
		}
		for _, column := range columns {
			if _, ok := existing[column.name]; ok {
				continue
			}
			if _, err := s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column.name, column.ddl)); err != nil {
				return fmt.Errorf("migrate %s.%s: %w", table, column.name, err)
			}
		}
	}
	for _, table := range []string{"gitlab_review_mr_tasks", "gitlab_issue_watch_tasks"} {
		existing, err := s.tableColumns(table)
		if err != nil {
			return err
		}
		if _, ok := existing["generation"]; !ok {
			if _, err := s.db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN generation INTEGER NOT NULL DEFAULT 1", table)); err != nil {
				return fmt.Errorf("migrate %s.generation: %w", table, err)
			}
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
	columns := make(map[string]struct{})
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = struct{}{}
	}
	return columns, rows.Err()
}

// taskMRSelectCols is the projection used by every SELECT so dev DBs that
// still have dropped columns (older revisions defined `unresolved_threads`
// and `discussion_count` here) don't break sqlx struct binding.
const taskMRSelectCols = `id, task_id, repository_id, host, project_path, mr_iid,
	mr_url, mr_title, head_branch, base_branch, author_username, state,
	approval_state, pipeline_state, merge_status, draft, approval_count,
	required_approvals, pipeline_jobs_total, pipeline_jobs_pass,
	created_at, merged_at, closed_at, last_synced_at, updated_at`

// taskMRSelectColsQualified is the same projection prefixed with `gtm.`,
// for the workspace JOIN where `tasks` shares column names (id, created_at,
// updated_at).
const taskMRSelectColsQualified = `gtm.id, gtm.task_id, gtm.repository_id,
	gtm.host, gtm.project_path, gtm.mr_iid, gtm.mr_url, gtm.mr_title,
	gtm.head_branch, gtm.base_branch, gtm.author_username, gtm.state,
	gtm.approval_state, gtm.pipeline_state, gtm.merge_status, gtm.draft,
	gtm.approval_count, gtm.required_approvals, gtm.pipeline_jobs_total,
	gtm.pipeline_jobs_pass, gtm.created_at, gtm.merged_at, gtm.closed_at,
	gtm.last_synced_at, gtm.updated_at`

// UpsertTaskMR creates or updates a task↔MR association keyed by
// (task_id, repository_id, project_path, mr_iid). Generates an ID on first
// insert; otherwise replaces the row's mutable fields and bumps updated_at.
func (s *Store) UpsertTaskMR(ctx context.Context, tm *TaskMR) error {
	now := time.Now().UTC()
	tm.UpdatedAt = now
	if tm.ID == "" {
		tm.ID = uuid.New().String()
	}
	if tm.CreatedAt.IsZero() {
		tm.CreatedAt = now
	}
	rows, err := s.db.NamedQueryContext(ctx, `
		INSERT INTO gitlab_task_mrs (
			id, task_id, repository_id, host, project_path, mr_iid, mr_url, mr_title,
			head_branch, base_branch, author_username, state, approval_state, pipeline_state,
			merge_status, draft, approval_count, required_approvals,
			pipeline_jobs_total, pipeline_jobs_pass,
			created_at, merged_at, closed_at, last_synced_at, updated_at
		) VALUES (
			:id, :task_id, :repository_id, :host, :project_path, :mr_iid, :mr_url, :mr_title,
			:head_branch, :base_branch, :author_username, :state, :approval_state, :pipeline_state,
			:merge_status, :draft, :approval_count, :required_approvals,
			:pipeline_jobs_total, :pipeline_jobs_pass,
			:created_at, :merged_at, :closed_at, :last_synced_at, :updated_at
		)
		ON CONFLICT(task_id, repository_id, project_path, mr_iid) DO UPDATE SET
			host = excluded.host, mr_url = excluded.mr_url, mr_title = excluded.mr_title,
			head_branch = excluded.head_branch, base_branch = excluded.base_branch,
			author_username = excluded.author_username, state = excluded.state,
			approval_state = excluded.approval_state, pipeline_state = excluded.pipeline_state,
			merge_status = excluded.merge_status, draft = excluded.draft,
			approval_count = excluded.approval_count,
			required_approvals = excluded.required_approvals,
			pipeline_jobs_total = excluded.pipeline_jobs_total,
			pipeline_jobs_pass = excluded.pipeline_jobs_pass,
			merged_at = excluded.merged_at, closed_at = excluded.closed_at,
			last_synced_at = excluded.last_synced_at, updated_at = excluded.updated_at
		RETURNING id, created_at, updated_at`, tm)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return errors.New("upsert task MR returned no row")
	}
	return rows.Scan(&tm.ID, &tm.CreatedAt, &tm.UpdatedAt)
}

// ListTaskMRsByTask returns every MR association for a task, oldest first.
func (s *Store) ListTaskMRsByTask(ctx context.Context, taskID string) ([]*TaskMR, error) {
	var mrs []TaskMR
	if err := s.ro.SelectContext(ctx, &mrs,
		`SELECT `+taskMRSelectCols+` FROM gitlab_task_mrs
		 WHERE task_id = ? ORDER BY created_at ASC`, taskID); err != nil {
		return nil, err
	}
	out := make([]*TaskMR, 0, len(mrs))
	for i := range mrs {
		out = append(out, &mrs[i])
	}
	return out, nil
}

// ListTaskMRsByWorkspaceID returns every MR association for tasks inside the
// given workspace, grouped by task_id. Empty map when the workspace has no
// GitLab MRs.
func (s *Store) ListTaskMRsByWorkspaceID(ctx context.Context, workspaceID string) (map[string][]*TaskMR, error) {
	var mrs []TaskMR
	if err := s.ro.SelectContext(ctx, &mrs,
		`SELECT `+taskMRSelectColsQualified+` FROM gitlab_task_mrs gtm
		 INNER JOIN tasks t ON gtm.task_id = t.id
		 WHERE t.workspace_id = ?
		 ORDER BY gtm.created_at ASC`, workspaceID); err != nil {
		return nil, err
	}
	out := make(map[string][]*TaskMR)
	for i := range mrs {
		out[mrs[i].TaskID] = append(out[mrs[i].TaskID], &mrs[i])
	}
	return out, nil
}

// DeleteTaskMR removes a single task↔MR row.
func (s *Store) DeleteTaskMR(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM gitlab_task_mrs WHERE id = ?`, id)
	return err
}
