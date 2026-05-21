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
		unresolved_threads INTEGER DEFAULT 0,
		discussion_count INTEGER DEFAULT 0,
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
`

func (s *Store) createTables() error {
	_, err := s.db.Exec(createTablesSQL)
	return err
}

// UpsertTaskMR creates or updates a task↔MR association keyed by
// (task_id, repository_id, project_path, mr_iid). Generates an ID on first
// insert; otherwise replaces the row's mutable fields and bumps updated_at.
func (s *Store) UpsertTaskMR(ctx context.Context, tm *TaskMR) error {
	now := time.Now().UTC()
	tm.UpdatedAt = now
	existing, err := s.findTaskMR(ctx, tm.TaskID, tm.RepositoryID, tm.ProjectPath, tm.MRIID)
	if err != nil {
		return err
	}
	if existing != nil {
		tm.ID = existing.ID
		tm.CreatedAt = existing.CreatedAt
		return s.updateTaskMR(ctx, tm)
	}
	if tm.ID == "" {
		tm.ID = uuid.New().String()
	}
	if tm.CreatedAt.IsZero() {
		tm.CreatedAt = now
	}
	_, err = s.db.NamedExecContext(ctx, `
		INSERT INTO gitlab_task_mrs (
			id, task_id, repository_id, host, project_path, mr_iid, mr_url, mr_title,
			head_branch, base_branch, author_username, state, approval_state, pipeline_state,
			merge_status, draft, approval_count, required_approvals, unresolved_threads,
			discussion_count, pipeline_jobs_total, pipeline_jobs_pass,
			created_at, merged_at, closed_at, last_synced_at, updated_at
		) VALUES (
			:id, :task_id, :repository_id, :host, :project_path, :mr_iid, :mr_url, :mr_title,
			:head_branch, :base_branch, :author_username, :state, :approval_state, :pipeline_state,
			:merge_status, :draft, :approval_count, :required_approvals, :unresolved_threads,
			:discussion_count, :pipeline_jobs_total, :pipeline_jobs_pass,
			:created_at, :merged_at, :closed_at, :last_synced_at, :updated_at
		)`, tm)
	return err
}

func (s *Store) updateTaskMR(ctx context.Context, tm *TaskMR) error {
	_, err := s.db.NamedExecContext(ctx, `
		UPDATE gitlab_task_mrs SET
			host = :host, mr_url = :mr_url, mr_title = :mr_title,
			head_branch = :head_branch, base_branch = :base_branch,
			author_username = :author_username, state = :state,
			approval_state = :approval_state, pipeline_state = :pipeline_state,
			merge_status = :merge_status, draft = :draft,
			approval_count = :approval_count, required_approvals = :required_approvals,
			unresolved_threads = :unresolved_threads, discussion_count = :discussion_count,
			pipeline_jobs_total = :pipeline_jobs_total, pipeline_jobs_pass = :pipeline_jobs_pass,
			merged_at = :merged_at, closed_at = :closed_at,
			last_synced_at = :last_synced_at, updated_at = :updated_at
		WHERE id = :id`, tm)
	return err
}

func (s *Store) findTaskMR(ctx context.Context, taskID, repositoryID, projectPath string, iid int) (*TaskMR, error) {
	var tm TaskMR
	err := s.ro.GetContext(ctx, &tm,
		`SELECT * FROM gitlab_task_mrs
		 WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?
		 LIMIT 1`, taskID, repositoryID, projectPath, iid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &tm, nil
}

// ListTaskMRsByTask returns every MR association for a task, oldest first.
func (s *Store) ListTaskMRsByTask(ctx context.Context, taskID string) ([]*TaskMR, error) {
	var mrs []TaskMR
	if err := s.ro.SelectContext(ctx, &mrs,
		`SELECT * FROM gitlab_task_mrs WHERE task_id = ? ORDER BY created_at ASC`, taskID); err != nil {
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
		`SELECT gtm.* FROM gitlab_task_mrs gtm
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
