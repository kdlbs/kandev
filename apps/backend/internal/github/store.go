package github

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// Store provides SQLite persistence for GitHub integration data.
type Store struct {
	db *sqlx.DB // writer
	ro *sqlx.DB // reader
}

// NewStore creates a new GitHub store and initializes the schema.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("github schema init: %w", err)
	}
	return s, nil
}

// createTablesSQL holds the DDL for all GitHub integration tables.
//
// Multi-repo: both `github_pr_watches` and `github_task_prs` carry a
// `repository_id` that names the per-task repository (when the task spans
// multiple repos). The uniqueness constraints include `repository_id` so
// each repo can have its own watch/PR for the same session/task. Existing
// installs migrated from the single-repo schema get the column dropped to
// `”` (empty) and the constraints rebuilt by `migratePRTablesForMultiRepo`.
const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS github_pr_watches (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		pr_number INTEGER NOT NULL,
		branch TEXT NOT NULL,
		last_checked_at DATETIME,
		last_comment_at DATETIME,
		last_check_status TEXT DEFAULT '',
		last_review_state TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(session_id, repository_id)
	);

	CREATE TABLE IF NOT EXISTS github_task_prs (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		pr_number INTEGER NOT NULL,
		pr_url TEXT NOT NULL,
		pr_title TEXT NOT NULL,
		head_branch TEXT NOT NULL,
		base_branch TEXT NOT NULL,
		author_login TEXT NOT NULL,
		state TEXT NOT NULL DEFAULT 'open',
		review_state TEXT NOT NULL DEFAULT '',
		checks_state TEXT NOT NULL DEFAULT '',
		mergeable_state TEXT NOT NULL DEFAULT '',
		review_count INTEGER DEFAULT 0,
		pending_review_count INTEGER DEFAULT 0,
		comment_count INTEGER DEFAULT 0,
		additions INTEGER DEFAULT 0,
		deletions INTEGER DEFAULT 0,
		created_at DATETIME NOT NULL,
		merged_at DATETIME,
		closed_at DATETIME,
		last_synced_at DATETIME,
		updated_at DATETIME NOT NULL,
		UNIQUE(task_id, repository_id, pr_number)
	);

	CREATE TABLE IF NOT EXISTS github_review_watches (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL,
		repos TEXT NOT NULL DEFAULT '[]',
		agent_profile_id TEXT NOT NULL,
		executor_profile_id TEXT NOT NULL,
		prompt TEXT DEFAULT '',
		review_scope TEXT NOT NULL DEFAULT 'user_and_teams',
		custom_query TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN DEFAULT 1,
		poll_interval_seconds INTEGER DEFAULT 300,
		last_polled_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS github_review_pr_tasks (
		id TEXT PRIMARY KEY,
		review_watch_id TEXT NOT NULL,
		repo_owner TEXT NOT NULL DEFAULT '',
		repo_name TEXT NOT NULL DEFAULT '',
		pr_number INTEGER NOT NULL,
		pr_url TEXT NOT NULL,
		task_id TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		UNIQUE(review_watch_id, repo_owner, repo_name, pr_number)
	);

	CREATE TABLE IF NOT EXISTS github_issue_watches (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL,
		repos TEXT NOT NULL DEFAULT '[]',
		agent_profile_id TEXT NOT NULL,
		executor_profile_id TEXT NOT NULL,
		prompt TEXT DEFAULT '',
		labels TEXT NOT NULL DEFAULT '[]',
		custom_query TEXT NOT NULL DEFAULT '',
		enabled BOOLEAN DEFAULT 1,
		poll_interval_seconds INTEGER DEFAULT 300,
		last_polled_at DATETIME,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS github_issue_watch_tasks (
		id TEXT PRIMARY KEY,
		issue_watch_id TEXT NOT NULL,
		repo_owner TEXT NOT NULL DEFAULT '',
		repo_name TEXT NOT NULL DEFAULT '',
		issue_number INTEGER NOT NULL,
		issue_url TEXT NOT NULL,
		task_id TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		UNIQUE(issue_watch_id, repo_owner, repo_name, issue_number)
	);

	CREATE TABLE IF NOT EXISTS github_action_presets (
		workspace_id TEXT PRIMARY KEY,
		pr_presets TEXT NOT NULL DEFAULT '[]',
		issue_presets TEXT NOT NULL DEFAULT '[]',
		updated_at DATETIME NOT NULL
	);
`

func (s *Store) initSchema() error {
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	// Idempotent migrations for existing databases.
	_, _ = s.db.Exec(`ALTER TABLE github_pr_watches ADD COLUMN last_review_state TEXT DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE github_task_prs ADD COLUMN mergeable_state TEXT NOT NULL DEFAULT ''`)
	// Phase 4 (multi-repo): per-repo PR association on github_task_prs.
	_, _ = s.db.Exec(`ALTER TABLE github_task_prs ADD COLUMN repository_id TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE github_pr_watches ADD COLUMN repository_id TEXT NOT NULL DEFAULT ''`)
	if err := s.migratePRTablesForMultiRepo(); err != nil {
		return fmt.Errorf("migrate PR tables for multi-repo: %w", err)
	}
	if err := s.backfillTaskPRsRepositoryID(); err != nil {
		return fmt.Errorf("backfill github_task_prs.repository_id: %w", err)
	}
	if err := s.backfillPRWatchesRepositoryID(); err != nil {
		return fmt.Errorf("backfill github_pr_watches.repository_id: %w", err)
	}
	return nil
}

// backfillTaskPRsRepositoryID heals github_task_prs rows that pre-date the
// per-repo schema (i.e. have repository_id = ”). Two passes:
//
//  1. Dedup: for each (task_id, owner, repo, pr_number) tuple, if a legacy
//     row (repository_id = ”) AND a newer per-repo row coexist, drop the
//     legacy one. This happens when an old single-repo install upgraded and
//     a subsequent sync inserted a new row under the resolved repository_id
//     instead of updating the legacy row, leaving two entries for one PR
//     (and so two badges on the kanban card).
//
//  2. Backfill: for any remaining empty-repo rows, look up the task's
//     primary repository (from `task_repositories` ordered by position) and
//     stamp its id. Skipped silently when the task has zero per-repo rows
//     (e.g. quick-chat tasks) — the row keeps its empty repository_id.
//
// Idempotent: re-running on a healed db is a no-op.
func (s *Store) backfillTaskPRsRepositoryID() error {
	if _, err := s.db.Exec(`
		DELETE FROM github_task_prs
		WHERE repository_id = ''
		  AND EXISTS (
		    SELECT 1 FROM github_task_prs other
		    WHERE other.task_id = github_task_prs.task_id
		      AND other.owner   = github_task_prs.owner
		      AND other.repo    = github_task_prs.repo
		      AND other.pr_number = github_task_prs.pr_number
		      AND other.repository_id != ''
		  )
	`); err != nil {
		return fmt.Errorf("dedup legacy task PR rows: %w", err)
	}
	// `task_repositories` lives in the task package's schema, not ours.
	// Skip the backfill when it doesn't exist (e.g. github-store unit tests
	// that init only this package's schema). The dedup pass above is the
	// load-bearing fix for the "two PRs on a single-repo task" symptom; the
	// backfill is a courtesy that converts orphan legacy rows in real
	// deployments.
	if !s.tableExists("task_repositories") {
		return nil
	}
	_, err := s.db.Exec(`
		UPDATE github_task_prs
		SET repository_id = (
		  SELECT tr.repository_id
		  FROM task_repositories tr
		  WHERE tr.task_id = github_task_prs.task_id
		  ORDER BY tr.position
		  LIMIT 1
		)
		WHERE repository_id = ''
		  AND EXISTS (
		    SELECT 1 FROM task_repositories tr
		    WHERE tr.task_id = github_task_prs.task_id
		  )
	`)
	if err != nil {
		return fmt.Errorf("backfill task PR repository_id: %w", err)
	}
	return nil
}

// backfillPRWatchesRepositoryID heals github_pr_watches rows that pre-date
// the per-repo schema (repository_id = ”). Same two-pass shape as
// backfillTaskPRsRepositoryID — without this the orchestrator's reconciler
// (which keys its existence-check by (session_id, repository_id)) would see
// the legacy `(sess, ”)` row as foreign and insert a SECOND watch row
// under the resolved repository_id. Two watches → two AssociatePRWithTask
// calls when the user opens a PR → two github_task_prs rows for the same
// PR, which is the "PR appears twice on a single-repo task" symptom we hit
// after the multi-repo rollout.
//
//  1. Dedup: drop legacy `”` rows whose session already has a non-empty
//     row — the reconciler-inserted row supersedes the legacy one.
//
//  2. Backfill: stamp the remaining `”` rows with the task's primary
//     repository_id from `task_repositories`. Skipped silently when the
//     table is absent (unit tests that init only this package's schema).
//
// Idempotent: re-running on a healed db is a no-op.
func (s *Store) backfillPRWatchesRepositoryID() error {
	if _, err := s.db.Exec(`
		DELETE FROM github_pr_watches
		WHERE repository_id = ''
		  AND EXISTS (
		    SELECT 1 FROM github_pr_watches other
		    WHERE other.session_id = github_pr_watches.session_id
		      AND other.repository_id != ''
		  )
	`); err != nil {
		return fmt.Errorf("dedup legacy PR watch rows: %w", err)
	}
	if !s.tableExists("task_repositories") {
		return nil
	}
	_, err := s.db.Exec(`
		UPDATE github_pr_watches
		SET repository_id = (
		  SELECT tr.repository_id
		  FROM task_repositories tr
		  WHERE tr.task_id = github_pr_watches.task_id
		  ORDER BY tr.position
		  LIMIT 1
		)
		WHERE repository_id = ''
		  AND EXISTS (
		    SELECT 1 FROM task_repositories tr
		    WHERE tr.task_id = github_pr_watches.task_id
		  )
	`)
	if err != nil {
		return fmt.Errorf("backfill PR watch repository_id: %w", err)
	}
	return nil
}

// tableExists returns true when the named table is present in sqlite_master.
// Used by the multi-repo backfill to skip cross-package healing in unit
// tests that don't bring up the task schema.
func (s *Store) tableExists(name string) bool {
	var n int
	err := s.db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n)
	return err == nil
}

// migratePRTablesForMultiRepo rebuilds `github_pr_watches` and
// `github_task_prs` to drop the legacy single-repo unique constraints
// (`UNIQUE(session_id)` and `UNIQUE(task_id, pr_number)`) and replace them
// with the multi-repo variants. SQLite can't ALTER TABLE DROP CONSTRAINT, so
// each table is rebuilt via the recommended copy-and-rename pattern. The
// migration is idempotent: it inspects `sqlite_master.sql` for the legacy
// constraint string and only runs the rebuild when found.
func (s *Store) migratePRTablesForMultiRepo() error {
	if err := s.rebuildIfHasLegacyConstraint(
		"github_pr_watches",
		"session_id TEXT NOT NULL UNIQUE",
		`CREATE TABLE github_pr_watches_new (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			task_id TEXT NOT NULL,
			repository_id TEXT NOT NULL DEFAULT '',
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			pr_number INTEGER NOT NULL,
			branch TEXT NOT NULL,
			last_checked_at DATETIME,
			last_comment_at DATETIME,
			last_check_status TEXT DEFAULT '',
			last_review_state TEXT DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE(session_id, repository_id)
		)`,
		`INSERT INTO github_pr_watches_new (
			id, session_id, task_id, repository_id, owner, repo, pr_number, branch,
			last_checked_at, last_comment_at, last_check_status, last_review_state,
			created_at, updated_at
		) SELECT
			id, session_id, task_id, COALESCE(repository_id, ''), owner, repo, pr_number, branch,
			last_checked_at, last_comment_at, last_check_status, last_review_state,
			created_at, updated_at
		FROM github_pr_watches`,
	); err != nil {
		return err
	}
	return s.rebuildIfHasLegacyConstraint(
		"github_task_prs",
		"UNIQUE(task_id, pr_number)",
		`CREATE TABLE github_task_prs_new (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			repository_id TEXT NOT NULL DEFAULT '',
			owner TEXT NOT NULL,
			repo TEXT NOT NULL,
			pr_number INTEGER NOT NULL,
			pr_url TEXT NOT NULL,
			pr_title TEXT NOT NULL,
			head_branch TEXT NOT NULL,
			base_branch TEXT NOT NULL,
			author_login TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'open',
			review_state TEXT NOT NULL DEFAULT '',
			checks_state TEXT NOT NULL DEFAULT '',
			mergeable_state TEXT NOT NULL DEFAULT '',
			review_count INTEGER DEFAULT 0,
			pending_review_count INTEGER DEFAULT 0,
			comment_count INTEGER DEFAULT 0,
			additions INTEGER DEFAULT 0,
			deletions INTEGER DEFAULT 0,
			created_at DATETIME NOT NULL,
			merged_at DATETIME,
			closed_at DATETIME,
			last_synced_at DATETIME,
			updated_at DATETIME NOT NULL,
			UNIQUE(task_id, repository_id, pr_number)
		)`,
		`INSERT INTO github_task_prs_new (
			id, task_id, repository_id, owner, repo, pr_number, pr_url, pr_title,
			head_branch, base_branch, author_login, state, review_state, checks_state,
			mergeable_state, review_count, pending_review_count, comment_count,
			additions, deletions, created_at, merged_at, closed_at, last_synced_at, updated_at
		) SELECT
			id, task_id, COALESCE(repository_id, ''), owner, repo, pr_number, pr_url, pr_title,
			head_branch, base_branch, author_login, state, review_state, checks_state,
			mergeable_state, review_count, pending_review_count, comment_count,
			additions, deletions, created_at, merged_at, closed_at, last_synced_at, updated_at
		FROM github_task_prs`,
	)
}

// rebuildIfHasLegacyConstraint checks the table's stored CREATE statement in
// `sqlite_master` for the literal `legacyConstraint` substring; if present,
// runs the table rebuild (create new, copy data, drop old, rename) inside a
// transaction. No-op when the legacy substring is absent — fresh installs
// already use the new schema and previously-migrated databases skip too.
func (s *Store) rebuildIfHasLegacyConstraint(table, legacyConstraint, createNew, copyData string) error {
	var existingSQL string
	row := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, table)
	if err := row.Scan(&existingSQL); err != nil {
		// Table missing entirely shouldn't happen after createTablesSQL ran;
		// treat as no-op to keep the migration robust under unexpected drift.
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if !strings.Contains(existingSQL, legacyConstraint) {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, stmt := range []string{
		createNew,
		copyData,
		"DROP TABLE " + table,
		"ALTER TABLE " + table + "_new RENAME TO " + table,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("rebuild %s: %w", table, err)
		}
	}
	return tx.Commit()
}

// --- PR Watch operations ---

// CreatePRWatch creates a new PR watch.
func (s *Store) CreatePRWatch(ctx context.Context, w *PRWatch) error {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	w.CreatedAt = now
	w.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_pr_watches (id, session_id, task_id, repository_id, owner, repo, pr_number, branch, last_check_status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.SessionID, w.TaskID, w.RepositoryID, w.Owner, w.Repo, w.PRNumber, w.Branch, w.LastCheckStatus, w.CreatedAt, w.UpdatedAt)
	return err
}

// GetPRWatchBySession returns the first PR watch for a session. For
// multi-repo sessions the result is non-deterministic across repos — use
// GetPRWatchBySessionAndRepo or ListPRWatchesBySession instead.
func (s *Store) GetPRWatchBySession(ctx context.Context, sessionID string) (*PRWatch, error) {
	var w PRWatch
	err := s.ro.GetContext(ctx, &w,
		`SELECT * FROM github_pr_watches WHERE session_id = ? LIMIT 1`, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &w, err
}

// GetPRWatchBySessionAndRepo returns the PR watch for a (session, repository)
// pair, or nil. Used by per-repo branch-switch / commit handlers so each
// repo's watch is reset independently.
func (s *Store) GetPRWatchBySessionAndRepo(ctx context.Context, sessionID, repositoryID string) (*PRWatch, error) {
	var w PRWatch
	err := s.ro.GetContext(ctx, &w,
		`SELECT * FROM github_pr_watches WHERE session_id = ? AND repository_id = ? LIMIT 1`,
		sessionID, repositoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &w, err
}

// ListPRWatchesBySession returns every PR watch for a session (one per repo
// in multi-repo workspaces). Empty slice when no watches exist.
func (s *Store) ListPRWatchesBySession(ctx context.Context, sessionID string) ([]*PRWatch, error) {
	var watches []*PRWatch
	err := s.ro.SelectContext(ctx, &watches,
		`SELECT * FROM github_pr_watches WHERE session_id = ? ORDER BY created_at ASC`, sessionID)
	return watches, err
}

// GetPRWatchByTask returns the first PR watch for a task. For multi-repo
// tasks the result is non-deterministic across repos — use
// ListPRWatchesByTask when every repo's watch is needed.
func (s *Store) GetPRWatchByTask(ctx context.Context, taskID string) (*PRWatch, error) {
	var w PRWatch
	err := s.ro.GetContext(ctx, &w, `SELECT * FROM github_pr_watches WHERE task_id = ? ORDER BY updated_at DESC LIMIT 1`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &w, err
}

// ListPRWatchesByTask returns every PR watch for a task (one per repo in
// multi-repo workspaces).
func (s *Store) ListPRWatchesByTask(ctx context.Context, taskID string) ([]*PRWatch, error) {
	var watches []*PRWatch
	err := s.ro.SelectContext(ctx, &watches,
		`SELECT * FROM github_pr_watches WHERE task_id = ? ORDER BY created_at ASC`, taskID)
	return watches, err
}

// ListActivePRWatches returns all active PR watches whose task is not archived.
// Watches for archived tasks (and orphaned watches whose task row was hard-deleted)
// are excluded so the poller stops making GitHub API calls for them. An INNER JOIN
// on `tasks` is used so orphans are dropped automatically.
func (s *Store) ListActivePRWatches(ctx context.Context) ([]*PRWatch, error) {
	var watches []*PRWatch
	err := s.ro.SelectContext(ctx, &watches, `
		SELECT w.* FROM github_pr_watches w
		INNER JOIN tasks t ON t.id = w.task_id
		WHERE t.archived_at IS NULL
		ORDER BY w.created_at`)
	return watches, err
}

// UpdatePRWatchTimestamps updates the last checked timestamps and status fields.
func (s *Store) UpdatePRWatchTimestamps(ctx context.Context, id string, checkedAt time.Time, commentAt *time.Time, checkStatus, reviewState string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE github_pr_watches SET last_checked_at = ?, last_comment_at = ?, last_check_status = ?, last_review_state = ?, updated_at = ?
		WHERE id = ?`,
		checkedAt, commentAt, checkStatus, reviewState, time.Now().UTC(), id)
	return err
}

// DeletePRWatch deletes a PR watch by ID.
func (s *Store) DeletePRWatch(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM github_pr_watches WHERE id = ?`, id)
	return err
}

// DeletePRWatchesByTaskID deletes all PR watches for a task. Returns the number
// of rows removed so callers can log meaningful diagnostics.
func (s *Store) DeletePRWatchesByTaskID(ctx context.Context, taskID string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM github_pr_watches WHERE task_id = ?`, taskID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}

// UpdatePRWatchPRNumber updates a PR watch's PR number after discovery.
func (s *Store) UpdatePRWatchPRNumber(ctx context.Context, id string, prNumber int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE github_pr_watches SET pr_number = ?, updated_at = ? WHERE id = ?`,
		prNumber, time.Now().UTC(), id)
	return err
}

// ResetPRWatch atomically resets a watch to the searching state: updates the
// tracked branch and clears pr_number in a single statement. Used when the
// session's active branch changes (rename, checkout) so the poller re-searches
// for a PR on the new branch without leaving an inconsistent intermediate
// state.
func (s *Store) ResetPRWatch(ctx context.Context, id, branch string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE github_pr_watches SET branch = ?, pr_number = 0, updated_at = ? WHERE id = ?`,
		branch, time.Now().UTC(), id)
	return err
}

// UpdatePRWatchBranchIfSearching atomically updates branch only when pr_number = 0,
// preventing races with concurrent PR association.
func (s *Store) UpdatePRWatchBranchIfSearching(ctx context.Context, id, branch string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE github_pr_watches SET branch = ?, updated_at = ? WHERE id = ? AND pr_number = 0`,
		branch, time.Now().UTC(), id)
	return err
}

// --- TaskPR operations ---

// CreateTaskPR associates a PR with a task. RepositoryID may be empty for
// single-repo tasks; multi-repo task launches set it so each repo's PR is
// distinguishable.
func (s *Store) CreateTaskPR(ctx context.Context, tp *TaskPR) error {
	if tp.ID == "" {
		tp.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	tp.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_task_prs (id, task_id, repository_id, owner, repo, pr_number, pr_url, pr_title, head_branch, base_branch, author_login,
			state, review_state, checks_state, mergeable_state, review_count, pending_review_count, comment_count, additions, deletions,
			created_at, merged_at, closed_at, last_synced_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tp.ID, tp.TaskID, tp.RepositoryID, tp.Owner, tp.Repo, tp.PRNumber, tp.PRURL, tp.PRTitle, tp.HeadBranch, tp.BaseBranch, tp.AuthorLogin,
		tp.State, tp.ReviewState, tp.ChecksState, tp.MergeableState, tp.ReviewCount, tp.PendingReviewCount, tp.CommentCount, tp.Additions, tp.Deletions,
		tp.CreatedAt, tp.MergedAt, tp.ClosedAt, tp.LastSyncedAt, tp.UpdatedAt)
	return err
}

// GetTaskPR returns the first PR association for a task. For multi-repo tasks
// the result is non-deterministic across repos — use ListTaskPRsByTask instead.
func (s *Store) GetTaskPR(ctx context.Context, taskID string) (*TaskPR, error) {
	var tp TaskPR
	err := s.ro.GetContext(ctx, &tp, `SELECT * FROM github_task_prs WHERE task_id = ? LIMIT 1`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &tp, err
}

// GetTaskPRByRepository returns the PR association for a (task, repository)
// pair, or nil if none. Use this for multi-repo tasks.
func (s *Store) GetTaskPRByRepository(ctx context.Context, taskID, repositoryID string) (*TaskPR, error) {
	var tp TaskPR
	err := s.ro.GetContext(ctx, &tp,
		`SELECT * FROM github_task_prs WHERE task_id = ? AND repository_id = ? LIMIT 1`,
		taskID, repositoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &tp, err
}

// ListTaskPRsByTask returns every PR association for a task (one per repo
// when multi-repo). Empty slice when no PRs exist.
func (s *Store) ListTaskPRsByTask(ctx context.Context, taskID string) ([]*TaskPR, error) {
	var prs []TaskPR
	if err := s.ro.SelectContext(ctx, &prs,
		`SELECT * FROM github_task_prs WHERE task_id = ? ORDER BY created_at ASC`, taskID); err != nil {
		return nil, err
	}
	out := make([]*TaskPR, 0, len(prs))
	for i := range prs {
		out = append(out, &prs[i])
	}
	return out, nil
}

// ListTaskPRsByTaskIDs returns PR associations for multiple tasks. Each task
// may have multiple PRs (one per repository for multi-repo tasks); rows are
// returned grouped by task_id, ordered by created_at ascending within a group.
func (s *Store) ListTaskPRsByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]*TaskPR, error) {
	if len(taskIDs) == 0 {
		return make(map[string][]*TaskPR), nil
	}
	query, args, err := sqlx.In(
		`SELECT * FROM github_task_prs WHERE task_id IN (?) ORDER BY created_at ASC`,
		taskIDs,
	)
	if err != nil {
		return nil, err
	}
	query = s.ro.Rebind(query)
	var prs []TaskPR
	if err := s.ro.SelectContext(ctx, &prs, query, args...); err != nil {
		return nil, err
	}
	return groupTaskPRsByTask(prs), nil
}

// ListTaskPRsByWorkspaceID returns all PR associations for tasks in a workspace.
// Each task may have multiple PRs (one per repository for multi-repo tasks);
// rows are returned grouped by task_id, ordered by created_at ascending.
func (s *Store) ListTaskPRsByWorkspaceID(ctx context.Context, workspaceID string) (map[string][]*TaskPR, error) {
	var prs []TaskPR
	if err := s.ro.SelectContext(ctx, &prs,
		`SELECT gtp.* FROM github_task_prs gtp
		 INNER JOIN tasks t ON gtp.task_id = t.id
		 WHERE t.workspace_id = ?
		 ORDER BY gtp.created_at ASC`, workspaceID); err != nil {
		return nil, err
	}
	return groupTaskPRsByTask(prs), nil
}

func groupTaskPRsByTask(prs []TaskPR) map[string][]*TaskPR {
	result := make(map[string][]*TaskPR)
	for i := range prs {
		taskID := prs[i].TaskID
		result[taskID] = append(result[taskID], &prs[i])
	}
	return result
}

// ReplaceTaskPR atomically replaces the task→PR association for a task: any
// existing rows are deleted and the new row is inserted inside a single
// transaction. The delete is scoped to (task_id, repository_id) so multi-repo
// tasks only replace the row for the affected repo; for single-repo tasks
// (RepositoryID == "") the legacy semantics are preserved (delete all).
func (s *Store) ReplaceTaskPR(ctx context.Context, tp *TaskPR) error {
	if tp.ID == "" {
		tp.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	tp.UpdatedAt = now

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Multi-repo: only delete the row for the same (task, repo) pair, leaving
	// other repos' rows alone. Single-repo callers (RepositoryID == "")
	// only delete legacy untagged rows so a multi-repo task's per-repo rows
	// don't get wiped if a single-repo callsite slips in. The poller and
	// service.AssociatePR* paths now always pass RepositoryID explicitly;
	// any future caller that forgets does the safer thing (preserve tagged
	// rows) instead of the old "nuke everything" behavior.
	if tp.RepositoryID != "" {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM github_task_prs WHERE task_id = ? AND repository_id = ?`,
			tp.TaskID, tp.RepositoryID); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM github_task_prs WHERE task_id = ? AND repository_id = ''`, tp.TaskID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO github_task_prs (id, task_id, repository_id, owner, repo, pr_number, pr_url, pr_title, head_branch, base_branch, author_login,
			state, review_state, checks_state, mergeable_state, review_count, pending_review_count, comment_count, additions, deletions,
			created_at, merged_at, closed_at, last_synced_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tp.ID, tp.TaskID, tp.RepositoryID, tp.Owner, tp.Repo, tp.PRNumber, tp.PRURL, tp.PRTitle, tp.HeadBranch, tp.BaseBranch, tp.AuthorLogin,
		tp.State, tp.ReviewState, tp.ChecksState, tp.MergeableState, tp.ReviewCount, tp.PendingReviewCount, tp.CommentCount, tp.Additions, tp.Deletions,
		tp.CreatedAt, tp.MergedAt, tp.ClosedAt, tp.LastSyncedAt, tp.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateTaskPR updates a task-PR association.
func (s *Store) UpdateTaskPR(ctx context.Context, tp *TaskPR) error {
	tp.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE github_task_prs SET state = ?, review_state = ?, checks_state = ?, mergeable_state = ?,
			review_count = ?, pending_review_count = ?, comment_count = ?,
			additions = ?, deletions = ?, pr_title = ?,
			merged_at = ?, closed_at = ?, last_synced_at = ?, updated_at = ?
		WHERE id = ?`,
		tp.State, tp.ReviewState, tp.ChecksState, tp.MergeableState,
		tp.ReviewCount, tp.PendingReviewCount, tp.CommentCount,
		tp.Additions, tp.Deletions, tp.PRTitle,
		tp.MergedAt, tp.ClosedAt, tp.LastSyncedAt, tp.UpdatedAt, tp.ID)
	return err
}

// --- Review Watch operations ---

// CreateReviewWatch creates a new review watch configuration.
func (s *Store) CreateReviewWatch(ctx context.Context, rw *ReviewWatch) error {
	if rw.ID == "" {
		rw.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	rw.CreatedAt = now
	rw.UpdatedAt = now
	reposJSON, err := json.Marshal(rw.Repos)
	if err != nil {
		return fmt.Errorf("marshal repos: %w", err)
	}
	rw.ReposJSON = string(reposJSON)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO github_review_watches (id, workspace_id, workflow_id, workflow_step_id, repos,
			agent_profile_id, executor_profile_id, prompt, review_scope, custom_query,
			enabled, poll_interval_seconds, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rw.ID, rw.WorkspaceID, rw.WorkflowID, rw.WorkflowStepID, rw.ReposJSON,
		rw.AgentProfileID, rw.ExecutorProfileID, rw.Prompt, rw.ReviewScope, rw.CustomQuery,
		rw.Enabled, rw.PollIntervalSeconds, rw.CreatedAt, rw.UpdatedAt)
	return err
}

// hydrateReviewWatchRepos unmarshals the ReposJSON field into the Repos slice.
func hydrateReviewWatchRepos(rw *ReviewWatch) {
	if rw.ReposJSON != "" {
		if err := json.Unmarshal([]byte(rw.ReposJSON), &rw.Repos); err != nil {
			// Log but don't fail — the watch can still function with no repo filter.
			fmt.Fprintf(os.Stderr, "WARN: failed to unmarshal repos JSON for review watch %s: %v\n", rw.ID, err)
		}
	}
	if rw.Repos == nil {
		rw.Repos = []RepoFilter{}
	}
}

// GetReviewWatch returns a review watch by ID.
func (s *Store) GetReviewWatch(ctx context.Context, id string) (*ReviewWatch, error) {
	var rw ReviewWatch
	err := s.ro.GetContext(ctx, &rw, `SELECT * FROM github_review_watches WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	hydrateReviewWatchRepos(&rw)
	return &rw, nil
}

// ListReviewWatches returns all review watches for a workspace.
func (s *Store) ListReviewWatches(ctx context.Context, workspaceID string) ([]*ReviewWatch, error) {
	var watches []*ReviewWatch
	err := s.ro.SelectContext(ctx, &watches,
		`SELECT * FROM github_review_watches WHERE workspace_id = ? ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, w := range watches {
		hydrateReviewWatchRepos(w)
	}
	return watches, nil
}

// ListEnabledReviewWatches returns all enabled review watches.
func (s *Store) ListEnabledReviewWatches(ctx context.Context) ([]*ReviewWatch, error) {
	var watches []*ReviewWatch
	err := s.ro.SelectContext(ctx, &watches,
		`SELECT * FROM github_review_watches WHERE enabled = 1 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	for _, w := range watches {
		hydrateReviewWatchRepos(w)
	}
	return watches, nil
}

// UpdateReviewWatch updates a review watch.
func (s *Store) UpdateReviewWatch(ctx context.Context, rw *ReviewWatch) error {
	rw.UpdatedAt = time.Now().UTC()
	reposJSON, err := json.Marshal(rw.Repos)
	if err != nil {
		return fmt.Errorf("marshal repos: %w", err)
	}
	rw.ReposJSON = string(reposJSON)
	_, err = s.db.ExecContext(ctx, `
		UPDATE github_review_watches SET workflow_id = ?, workflow_step_id = ?, repos = ?,
			agent_profile_id = ?, executor_profile_id = ?,
			prompt = ?, review_scope = ?, custom_query = ?,
			enabled = ?, poll_interval_seconds = ?, last_polled_at = ?, updated_at = ?
		WHERE id = ?`,
		rw.WorkflowID, rw.WorkflowStepID, rw.ReposJSON,
		rw.AgentProfileID, rw.ExecutorProfileID,
		rw.Prompt, rw.ReviewScope, rw.CustomQuery,
		rw.Enabled, rw.PollIntervalSeconds, rw.LastPolledAt, rw.UpdatedAt, rw.ID)
	return err
}

// DeleteReviewWatch deletes a review watch.
func (s *Store) DeleteReviewWatch(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM github_review_watches WHERE id = ?`, id)
	return err
}

// --- Review PR Task deduplication ---

// CreateReviewPRTask records that a task was created for a review PR.
func (s *Store) CreateReviewPRTask(ctx context.Context, rpt *ReviewPRTask) error {
	if rpt.ID == "" {
		rpt.ID = uuid.New().String()
	}
	rpt.CreatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_review_pr_tasks (id, review_watch_id, repo_owner, repo_name, pr_number, pr_url, task_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rpt.ID, rpt.ReviewWatchID, rpt.RepoOwner, rpt.RepoName, rpt.PRNumber, rpt.PRURL, rpt.TaskID, rpt.CreatedAt)
	return err
}

// HasReviewPRTask checks if a task was already created for a PR in a review watch.
func (s *Store) HasReviewPRTask(ctx context.Context, reviewWatchID, repoOwner, repoName string, prNumber int) (bool, error) {
	var count int
	err := s.ro.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM github_review_pr_tasks WHERE review_watch_id = ? AND repo_owner = ? AND repo_name = ? AND pr_number = ?`,
		reviewWatchID, repoOwner, repoName, prNumber)
	return count > 0, err
}

// ReserveReviewPRTask atomically claims a slot for a (watch, repo, PR) tuple
// using INSERT OR IGNORE against the UNIQUE constraint. Returns true if this
// caller won the race and should proceed to create the task, false if another
// caller already holds the slot. The caller is expected to call
// AssignReviewPRTaskID once the task is created, or ReleaseReviewPRTask if
// task creation fails.
func (s *Store) ReserveReviewPRTask(ctx context.Context, reviewWatchID, repoOwner, repoName string, prNumber int, prURL string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO github_review_pr_tasks (id, review_watch_id, repo_owner, repo_name, pr_number, pr_url, task_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), reviewWatchID, repoOwner, repoName, prNumber, prURL, "", time.Now().UTC())
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

// AssignReviewPRTaskID sets the task_id on a reserved dedup row. Called after
// the task has been created so cleanup logic can locate and delete it later.
// Returns an error if no row was updated, which surfaces the narrow race where
// the reservation was removed (e.g. by a concurrent cleanup sweep) between
// Reserve and Assign — otherwise the task would leak with no dedup record.
func (s *Store) AssignReviewPRTaskID(ctx context.Context, reviewWatchID, repoOwner, repoName string, prNumber int, taskID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE github_review_pr_tasks SET task_id = ?
		WHERE review_watch_id = ? AND repo_owner = ? AND repo_name = ? AND pr_number = ?`,
		taskID, reviewWatchID, repoOwner, repoName, prNumber)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("assign task ID: reservation row not found for watch=%s pr=%d", reviewWatchID, prNumber)
	}
	return nil
}

// ReleaseReviewPRTask removes a reservation for a (watch, repo, PR) tuple.
// Used when task creation fails so a later poll can retry instead of the PR
// being permanently blocked by an orphan reservation.
func (s *Store) ReleaseReviewPRTask(ctx context.Context, reviewWatchID, repoOwner, repoName string, prNumber int) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM github_review_pr_tasks
		WHERE review_watch_id = ? AND repo_owner = ? AND repo_name = ? AND pr_number = ?`,
		reviewWatchID, repoOwner, repoName, prNumber)
	return err
}

// ListReviewPRTasksByWatch lists all dedup records for a given review watch.
func (s *Store) ListReviewPRTasksByWatch(ctx context.Context, watchID string) ([]*ReviewPRTask, error) {
	var tasks []*ReviewPRTask
	err := s.ro.SelectContext(ctx, &tasks,
		`SELECT id, review_watch_id, repo_owner, repo_name, pr_number, pr_url, task_id, created_at
		 FROM github_review_pr_tasks WHERE review_watch_id = ?`, watchID)
	return tasks, err
}

// DeleteReviewPRTask deletes a dedup record by ID.
func (s *Store) DeleteReviewPRTask(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM github_review_pr_tasks WHERE id = ?`, id)
	return err
}

// --- Stats queries ---

// prStatsQuery builds parameterised SELECT queries against the github_task_prs table.
type prStatsQuery struct {
	from  string
	where string
	args  []interface{}
}

func newPRStatsQuery(req *PRStatsRequest) *prStatsQuery {
	q := &prStatsQuery{
		from:  "github_task_prs gtp",
		where: "1=1",
	}
	if req.WorkspaceID != "" {
		q.from += " INNER JOIN tasks t ON gtp.task_id = t.id"
		q.where += " AND t.workspace_id = ?"
		q.args = append(q.args, req.WorkspaceID)
	}
	if req.StartDate != nil {
		q.where += " AND gtp.created_at >= ?"
		q.args = append(q.args, req.StartDate)
	}
	if req.EndDate != nil {
		q.where += " AND gtp.created_at <= ?"
		q.args = append(q.args, req.EndDate)
	}
	return q
}

func (q *prStatsQuery) build(sel, extraWhere string) string {
	w := q.where
	if extraWhere != "" {
		w += " AND " + extraWhere
	}
	return fmt.Sprintf(`SELECT %s FROM %s WHERE %s`, sel, q.from, w)
}

// GetPRStats returns aggregated PR statistics.
func (s *Store) GetPRStats(ctx context.Context, req *PRStatsRequest) (*PRStats, error) {
	return s.runPRStatsQueries(ctx, newPRStatsQuery(req))
}

func (s *Store) runPRStatsQueries(ctx context.Context, q *prStatsQuery) (*PRStats, error) {
	stats := &PRStats{}

	if err := s.ro.GetContext(ctx, &stats.TotalPRsCreated, q.build("COUNT(*)", ""), q.args...); err != nil {
		return nil, err
	}
	if err := s.ro.GetContext(ctx, &stats.TotalComments,
		q.build("COALESCE(SUM(gtp.comment_count), 0)", ""), q.args...); err != nil {
		return nil, err
	}
	if err := s.fetchCIPassRate(ctx, q, stats); err != nil {
		return nil, err
	}
	if err := s.fetchApprovalRate(ctx, q, stats); err != nil {
		return nil, err
	}

	var avgMerge sql.NullFloat64
	avgQ := q.build("AVG((julianday(gtp.merged_at) - julianday(gtp.created_at)) * 24)", "gtp.merged_at IS NOT NULL")
	if err := s.ro.GetContext(ctx, &avgMerge, avgQ, q.args...); err != nil {
		return nil, err
	}
	if avgMerge.Valid {
		stats.AvgTimeToMergeHours = avgMerge.Float64
	}

	dailyQ := q.build("date(gtp.created_at) as date, COUNT(*) as count", "") +
		" GROUP BY date(gtp.created_at) ORDER BY date"
	if err := s.ro.SelectContext(ctx, &stats.PRsByDay, dailyQ, q.args...); err != nil {
		return nil, err
	}
	return stats, nil
}

func (s *Store) fetchCIPassRate(ctx context.Context, q *prStatsQuery, stats *PRStats) error {
	var totalWithChecks, passed int
	if err := s.ro.GetContext(ctx, &totalWithChecks,
		q.build("COUNT(*)", "gtp.checks_state != ''"), q.args...); err != nil {
		return err
	}
	if err := s.ro.GetContext(ctx, &passed,
		q.build("COUNT(*)", "gtp.checks_state = 'success'"), q.args...); err != nil {
		return err
	}
	if totalWithChecks > 0 {
		stats.CIPassRate = float64(passed) / float64(totalWithChecks)
	}
	return nil
}

func (s *Store) fetchApprovalRate(ctx context.Context, q *prStatsQuery, stats *PRStats) error {
	var totalReviewed, approved int
	if err := s.ro.GetContext(ctx, &totalReviewed,
		q.build("COUNT(*)", "gtp.review_state != ''"), q.args...); err != nil {
		return err
	}
	if err := s.ro.GetContext(ctx, &approved,
		q.build("COUNT(*)", "gtp.review_state = 'approved'"), q.args...); err != nil {
		return err
	}
	stats.TotalPRsReviewed = totalReviewed
	if totalReviewed > 0 {
		stats.ApprovalRate = float64(approved) / float64(totalReviewed)
	}
	return nil
}

// --- Issue Watch operations ---

// hydrateIssueWatch unmarshals JSON fields into their Go slices.
func hydrateIssueWatch(iw *IssueWatch) {
	if iw.ReposJSON != "" {
		if err := json.Unmarshal([]byte(iw.ReposJSON), &iw.Repos); err != nil {
			fmt.Fprintf(os.Stderr, "WARN: failed to unmarshal repos JSON for issue watch %s: %v\n", iw.ID, err)
		}
	}
	if iw.Repos == nil {
		iw.Repos = []RepoFilter{}
	}
	if iw.LabelsJSON != "" {
		if err := json.Unmarshal([]byte(iw.LabelsJSON), &iw.Labels); err != nil {
			fmt.Fprintf(os.Stderr, "WARN: failed to unmarshal labels JSON for issue watch %s: %v\n", iw.ID, err)
		}
	}
	if iw.Labels == nil {
		iw.Labels = []string{}
	}
}

// CreateIssueWatch creates a new issue watch configuration.
func (s *Store) CreateIssueWatch(ctx context.Context, iw *IssueWatch) error {
	if iw.ID == "" {
		iw.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	iw.CreatedAt = now
	iw.UpdatedAt = now
	reposJSON, err := json.Marshal(iw.Repos)
	if err != nil {
		return fmt.Errorf("marshal repos: %w", err)
	}
	iw.ReposJSON = string(reposJSON)
	labelsJSON, err := json.Marshal(iw.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	iw.LabelsJSON = string(labelsJSON)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO github_issue_watches (id, workspace_id, workflow_id, workflow_step_id, repos,
			agent_profile_id, executor_profile_id, prompt, labels, custom_query,
			enabled, poll_interval_seconds, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		iw.ID, iw.WorkspaceID, iw.WorkflowID, iw.WorkflowStepID, iw.ReposJSON,
		iw.AgentProfileID, iw.ExecutorProfileID, iw.Prompt, iw.LabelsJSON, iw.CustomQuery,
		iw.Enabled, iw.PollIntervalSeconds, iw.CreatedAt, iw.UpdatedAt)
	return err
}

// GetIssueWatch returns an issue watch by ID.
func (s *Store) GetIssueWatch(ctx context.Context, id string) (*IssueWatch, error) {
	var iw IssueWatch
	err := s.ro.GetContext(ctx, &iw, `SELECT * FROM github_issue_watches WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	hydrateIssueWatch(&iw)
	return &iw, nil
}

// ListIssueWatches returns all issue watches for a workspace.
func (s *Store) ListIssueWatches(ctx context.Context, workspaceID string) ([]*IssueWatch, error) {
	var watches []*IssueWatch
	err := s.ro.SelectContext(ctx, &watches,
		`SELECT * FROM github_issue_watches WHERE workspace_id = ? ORDER BY created_at`, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, w := range watches {
		hydrateIssueWatch(w)
	}
	return watches, nil
}

// ListEnabledIssueWatches returns all enabled issue watches.
func (s *Store) ListEnabledIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	var watches []*IssueWatch
	err := s.ro.SelectContext(ctx, &watches,
		`SELECT * FROM github_issue_watches WHERE enabled = 1 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	for _, w := range watches {
		hydrateIssueWatch(w)
	}
	return watches, nil
}

// UpdateIssueWatch updates an issue watch.
func (s *Store) UpdateIssueWatch(ctx context.Context, iw *IssueWatch) error {
	iw.UpdatedAt = time.Now().UTC()
	reposJSON, err := json.Marshal(iw.Repos)
	if err != nil {
		return fmt.Errorf("marshal repos: %w", err)
	}
	iw.ReposJSON = string(reposJSON)
	labelsJSON, err := json.Marshal(iw.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	iw.LabelsJSON = string(labelsJSON)
	_, err = s.db.ExecContext(ctx, `
		UPDATE github_issue_watches SET workflow_id = ?, workflow_step_id = ?, repos = ?,
			agent_profile_id = ?, executor_profile_id = ?,
			prompt = ?, labels = ?, custom_query = ?,
			enabled = ?, poll_interval_seconds = ?, last_polled_at = ?, updated_at = ?
		WHERE id = ?`,
		iw.WorkflowID, iw.WorkflowStepID, iw.ReposJSON,
		iw.AgentProfileID, iw.ExecutorProfileID,
		iw.Prompt, iw.LabelsJSON, iw.CustomQuery,
		iw.Enabled, iw.PollIntervalSeconds, iw.LastPolledAt, iw.UpdatedAt, iw.ID)
	return err
}

// DeleteIssueWatch deletes an issue watch and all its associated dedup task rows.
func (s *Store) DeleteIssueWatch(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM github_issue_watch_tasks WHERE issue_watch_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM github_issue_watches WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Issue Watch Task deduplication ---

// ReserveIssueWatchTask atomically claims a slot for a (watch, repo, issue) tuple.
// Returns true if this caller won the race and should proceed to create the task.
func (s *Store) ReserveIssueWatchTask(ctx context.Context, issueWatchID, repoOwner, repoName string, issueNumber int, issueURL string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO github_issue_watch_tasks (id, issue_watch_id, repo_owner, repo_name, issue_number, issue_url, task_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), issueWatchID, repoOwner, repoName, issueNumber, issueURL, "", time.Now().UTC())
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

// AssignIssueWatchTaskID sets the task_id on a reserved dedup row.
func (s *Store) AssignIssueWatchTaskID(ctx context.Context, issueWatchID, repoOwner, repoName string, issueNumber int, taskID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE github_issue_watch_tasks SET task_id = ?
		WHERE issue_watch_id = ? AND repo_owner = ? AND repo_name = ? AND issue_number = ?`,
		taskID, issueWatchID, repoOwner, repoName, issueNumber)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("assign task ID: reservation row not found for watch=%s issue=%d", issueWatchID, issueNumber)
	}
	return nil
}

// ReleaseIssueWatchTask removes a reservation for a (watch, repo, issue) tuple.
func (s *Store) ReleaseIssueWatchTask(ctx context.Context, issueWatchID, repoOwner, repoName string, issueNumber int) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM github_issue_watch_tasks
		WHERE issue_watch_id = ? AND repo_owner = ? AND repo_name = ? AND issue_number = ?`,
		issueWatchID, repoOwner, repoName, issueNumber)
	return err
}

// HasIssueWatchTask checks if a task was already created for an issue in an issue watch.
func (s *Store) HasIssueWatchTask(ctx context.Context, issueWatchID, repoOwner, repoName string, issueNumber int) (bool, error) {
	var count int
	err := s.ro.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM github_issue_watch_tasks WHERE issue_watch_id = ? AND repo_owner = ? AND repo_name = ? AND issue_number = ?`,
		issueWatchID, repoOwner, repoName, issueNumber)
	return count > 0, err
}

// ListIssueWatchTasksByWatch lists all dedup records for a given issue watch.
func (s *Store) ListIssueWatchTasksByWatch(ctx context.Context, watchID string) ([]*IssueWatchTask, error) {
	var tasks []*IssueWatchTask
	err := s.ro.SelectContext(ctx, &tasks,
		`SELECT id, issue_watch_id, repo_owner, repo_name, issue_number, issue_url, task_id, created_at
		 FROM github_issue_watch_tasks WHERE issue_watch_id = ?`, watchID)
	return tasks, err
}

// DeleteIssueWatchTask deletes a dedup record by ID.
func (s *Store) DeleteIssueWatchTask(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM github_issue_watch_tasks WHERE id = ?`, id)
	return err
}

// --- Action preset operations ---

// GetActionPresets returns stored PR/Issue presets for a workspace. Returns
// (nil, nil) when no row exists yet so the caller can apply defaults.
func (s *Store) GetActionPresets(ctx context.Context, workspaceID string) (*ActionPresets, error) {
	var row struct {
		PRJSON    string `db:"pr_presets"`
		IssueJSON string `db:"issue_presets"`
	}
	err := s.ro.GetContext(ctx, &row,
		`SELECT pr_presets, issue_presets FROM github_action_presets WHERE workspace_id = ?`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	presets := &ActionPresets{WorkspaceID: workspaceID}
	if err := json.Unmarshal([]byte(row.PRJSON), &presets.PR); err != nil {
		return nil, fmt.Errorf("unmarshal pr presets: %w", err)
	}
	if err := json.Unmarshal([]byte(row.IssueJSON), &presets.Issue); err != nil {
		return nil, fmt.Errorf("unmarshal issue presets: %w", err)
	}
	return presets, nil
}

// UpsertActionPresets stores PR/Issue presets for a workspace, replacing any
// existing row.
func (s *Store) UpsertActionPresets(ctx context.Context, presets *ActionPresets) error {
	prJSON, err := json.Marshal(presets.PR)
	if err != nil {
		return fmt.Errorf("marshal pr presets: %w", err)
	}
	issueJSON, err := json.Marshal(presets.Issue)
	if err != nil {
		return fmt.Errorf("marshal issue presets: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO github_action_presets (workspace_id, pr_presets, issue_presets, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			pr_presets = excluded.pr_presets,
			issue_presets = excluded.issue_presets,
			updated_at = excluded.updated_at`,
		presets.WorkspaceID, string(prJSON), string(issueJSON), time.Now().UTC())
	return err
}

// DeleteActionPresets removes the stored overrides for a workspace so defaults
// apply again.
func (s *Store) DeleteActionPresets(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM github_action_presets WHERE workspace_id = ?`, workspaceID)
	return err
}
