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

type taskIssueMetadataRow struct {
	TaskID    string `db:"task_id"`
	TaskTitle string `db:"task_title"`
	Metadata  string `db:"metadata"`
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
		UNIQUE(session_id, repository_id, branch)
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
		required_reviews INTEGER,
		comment_count INTEGER DEFAULT 0,
		unresolved_review_threads INTEGER DEFAULT 0,
		checks_total INTEGER DEFAULT 0,
		checks_passing INTEGER DEFAULT 0,
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
		cleanup_policy TEXT NOT NULL DEFAULT 'auto',
		last_polled_at DATETIME,
		last_error TEXT NOT NULL DEFAULT '',
		last_error_at DATETIME,
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
		cleanup_policy TEXT NOT NULL DEFAULT 'auto',
		last_polled_at DATETIME,
		last_error TEXT NOT NULL DEFAULT '',
		last_error_at DATETIME,
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

	CREATE TABLE IF NOT EXISTS github_workspace_settings (
		workspace_id TEXT PRIMARY KEY,
		repo_scope_mode TEXT NOT NULL DEFAULT 'all',
		repo_scope_orgs TEXT NOT NULL DEFAULT '[]',
		repo_scope_repos TEXT NOT NULL DEFAULT '[]',
		saved_presets TEXT NOT NULL DEFAULT '[]',
		default_query_presets TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS github_task_ci_options (
		task_id TEXT PRIMARY KEY,
		auto_fix_enabled BOOLEAN NOT NULL DEFAULT 0,
		auto_merge_enabled BOOLEAN NOT NULL DEFAULT 0,
		auto_fix_prompt_override TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS github_task_ci_pr_state (
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		pr_number INTEGER NOT NULL,
		last_fix_signature TEXT NOT NULL DEFAULT '',
		last_fix_checkpoint_json TEXT NOT NULL DEFAULT '',
		last_fix_enqueued_at DATETIME,
		last_fix_session_id TEXT,
		auto_fix_round_count INTEGER NOT NULL DEFAULT 0,
		auto_fix_exhausted_at DATETIME,
		last_merge_signature TEXT NOT NULL DEFAULT '',
		last_merge_attempt_at DATETIME,
		last_error TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		PRIMARY KEY (task_id, repository_id, pr_number)
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
	// CI popover: aggregate counts + branch protection's required_approving_review_count
	// + unresolved review-threads, surfaced in the PR top-bar hover popover so the
	// frontend can render the counts row without a second round-trip.
	_, _ = s.db.Exec(`ALTER TABLE github_task_prs ADD COLUMN required_reviews INTEGER`)
	_, _ = s.db.Exec(`ALTER TABLE github_task_prs ADD COLUMN unresolved_review_threads INTEGER DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE github_task_prs ADD COLUMN checks_total INTEGER DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE github_task_prs ADD COLUMN checks_passing INTEGER DEFAULT 0`)
	// Per-watch cleanup policy for review/issue watches: controls whether the
	// poller deletes auto-created tasks when the underlying PR/issue reaches
	// a terminal state. Values: 'auto' (default — preserve only when user
	// engaged), 'always' (delete on terminal state), 'never' (manual only).
	_, _ = s.db.Exec(`ALTER TABLE github_review_watches ADD COLUMN cleanup_policy TEXT NOT NULL DEFAULT 'auto'`)
	_, _ = s.db.Exec(`ALTER TABLE github_issue_watches ADD COLUMN cleanup_policy TEXT NOT NULL DEFAULT 'auto'`)
	// Watcher self-heal columns: when the dispatch pipeline detects an
	// orphaned watcher (e.g. its agent profile has been soft-deleted), it
	// disables the row and stamps a human-readable cause + timestamp here
	// for the settings page to surface. Unlike the cleanup_policy column
	// above, the readers (IssueWatch.LastError / LastErrorAt) scan these
	// columns unconditionally — a driver-level ALTER failure here would
	// turn into a confusing scan panic on the next poll instead of a
	// clear boot error. Use the same fail-loud column-precheck idiom the
	// sibling jira/linear stores already use.
	if err := s.addWatchSelfHealColumns(); err != nil {
		return err
	}
	if err := s.addTaskCIRoundColumns(); err != nil {
		return err
	}
	if err := s.migratePRTablesForMultiRepo(); err != nil {
		return fmt.Errorf("migrate PR tables for multi-repo: %w", err)
	}
	if err := s.backfillTaskPRsRepositoryID(); err != nil {
		return fmt.Errorf("backfill github_task_prs.repository_id: %w", err)
	}
	if err := s.backfillPRWatchesRepositoryID(); err != nil {
		return fmt.Errorf("backfill github_pr_watches.repository_id: %w", err)
	}
	// pr_number is the 3rd column of UNIQUE(task_id, repository_id, pr_number),
	// so SQLite can't use that index for the PR-number task search. Add a
	// dedicated leading-key index so lookups by PR number stay index-backed.
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_github_task_prs_pr_number ON github_task_prs (pr_number)`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_github_task_ci_pr_state_task ON github_task_ci_pr_state (task_id)`)
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

// addWatchSelfHealColumns adds last_error / last_error_at to the issue and
// review watch tables using a column-precheck (mirroring the jira and linear
// stores). Unlike the cleanup_policy ALTER above, the readers
// (IssueWatch.LastError / LastErrorAt) scan these columns unconditionally,
// so a driver-level failure must bubble up at boot rather than turn into
// a scan panic on the next poll.
func (s *Store) addWatchSelfHealColumns() error {
	for _, table := range []string{"github_review_watches", "github_issue_watches"} {
		cols, err := s.tableColumns(table)
		if err != nil {
			return fmt.Errorf("read %s columns: %w", table, err)
		}
		if _, ok := cols["last_error"]; !ok {
			if _, err := s.db.Exec("ALTER TABLE " + table + " ADD COLUMN last_error TEXT NOT NULL DEFAULT ''"); err != nil {
				return fmt.Errorf("add %s.last_error: %w", table, err)
			}
		}
		if _, ok := cols["last_error_at"]; !ok {
			if _, err := s.db.Exec("ALTER TABLE " + table + " ADD COLUMN last_error_at DATETIME"); err != nil {
				return fmt.Errorf("add %s.last_error_at: %w", table, err)
			}
		}
	}
	return nil
}

func (s *Store) addTaskCIRoundColumns() error {
	cols, err := s.tableColumns("github_task_ci_pr_state")
	if err != nil {
		return fmt.Errorf("read github_task_ci_pr_state columns: %w", err)
	}
	if _, ok := cols["auto_fix_round_count"]; !ok {
		if _, err := s.db.Exec("ALTER TABLE github_task_ci_pr_state ADD COLUMN auto_fix_round_count INTEGER NOT NULL DEFAULT 0"); err != nil {
			return fmt.Errorf("add github_task_ci_pr_state.auto_fix_round_count: %w", err)
		}
	}
	if _, ok := cols["auto_fix_exhausted_at"]; !ok {
		if _, err := s.db.Exec("ALTER TABLE github_task_ci_pr_state ADD COLUMN auto_fix_exhausted_at DATETIME"); err != nil {
			return fmt.Errorf("add github_task_ci_pr_state.auto_fix_exhausted_at: %w", err)
		}
	}
	return nil
}

// tableColumns returns the set of column names declared on `table`. Cheap
// SQLite PRAGMA lookup; used by addWatchSelfHealColumns to skip ALTERs on a
// fresh install whose createTablesSQL already includes the columns. Mirrors
// the helper in jira/store.go.
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
// with the multi-repo / multi-branch variants. SQLite can't ALTER TABLE
// DROP CONSTRAINT, so each table is rebuilt via the recommended
// copy-and-rename pattern. The migration is idempotent: it inspects
// `sqlite_master.sql` for the legacy constraint string and only runs the
// rebuild when found. The watch rebuild fires twice — once for the original
// single-repo shape, once for the interim multi-repo shape — so DBs caught
// in either state upgrade cleanly to the multi-branch shape.
func (s *Store) migratePRTablesForMultiRepo() error {
	for _, trigger := range []string{
		"session_id TEXT NOT NULL UNIQUE",
		"UNIQUE(session_id, repository_id)\n",
	} {
		if err := s.rebuildIfHasLegacyConstraint(
			"github_pr_watches",
			trigger,
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
				UNIQUE(session_id, repository_id, branch)
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
			required_reviews INTEGER,
			comment_count INTEGER DEFAULT 0,
			unresolved_review_threads INTEGER DEFAULT 0,
			checks_total INTEGER DEFAULT 0,
			checks_passing INTEGER DEFAULT 0,
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
//
// Multi-branch caveat: a task can hold multiple watches for the same
// (session, repository) on different branches. This lookup returns the
// most-recently-updated row — callers that need branch-specific lookup
// must use GetPRWatchBySessionRepoAndBranch.
func (s *Store) GetPRWatchBySessionAndRepo(ctx context.Context, sessionID, repositoryID string) (*PRWatch, error) {
	var w PRWatch
	err := s.ro.GetContext(ctx, &w,
		`SELECT * FROM github_pr_watches WHERE session_id = ? AND repository_id = ?
		 ORDER BY updated_at DESC LIMIT 1`,
		sessionID, repositoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &w, err
}

// GetPRWatchBySessionRepoAndBranch returns the PR watch for the precise
// (session, repository, branch) triple. Required for multi-branch tasks
// where each branch needs its own watch — querying by (session, repo)
// alone would collapse the secondary branch's push detection onto the
// primary's watch and the secondary PR would never land in github_task_prs.
func (s *Store) GetPRWatchBySessionRepoAndBranch(ctx context.Context, sessionID, repositoryID, branch string) (*PRWatch, error) {
	var w PRWatch
	err := s.ro.GetContext(ctx, &w,
		`SELECT * FROM github_pr_watches
		 WHERE session_id = ? AND repository_id = ? AND branch = ? LIMIT 1`,
		sessionID, repositoryID, branch)
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
//
// Collision semantics: a sibling watch may already own the destination
// (session_id, repository_id, branch) triple — e.g. multi-branch task where
// the agent's live branch collapsed onto a peer watch's branch. In that
// case the raw UPDATE would trip the UNIQUE constraint. We instead drop the
// source row (which is still searching, pr_number=0, so it owns no PR
// state) and let the sibling continue to track the branch.
func (s *Store) UpdatePRWatchBranchIfSearching(ctx context.Context, id, branch string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var sessionID, repositoryID string
	var prNumber int
	err = tx.QueryRowContext(ctx,
		`SELECT session_id, repository_id, pr_number FROM github_pr_watches WHERE id = ?`, id).
		Scan(&sessionID, &repositoryID, &prNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	if prNumber != 0 {
		return tx.Commit()
	}

	var probe int // existence probe only; value unused
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM github_pr_watches
		 WHERE session_id = ? AND repository_id = ? AND branch = ? AND id <> ?`,
		sessionID, repositoryID, branch, id).Scan(&probe)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		return dropSourceAndCommit(ctx, tx, id)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE github_pr_watches SET branch = ?, updated_at = ? WHERE id = ? AND pr_number = 0`,
		branch, time.Now().UTC(), id); err != nil {
		// Defensive belt-and-suspenders: the SQLite writer pool is
		// SetMaxOpenConns(1), so an in-process CreatePRWatch cannot
		// commit a sibling row between our probe and this UPDATE. But
		// an external writer (separate process touching the same file,
		// future pool reshuffle) could; if the UPDATE still trips
		// UNIQUE, treat it identically to the probe-found path.
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return dropSourceAndCommit(ctx, tx, id)
		}
		return err
	}
	return tx.Commit()
}

// dropSourceAndCommit removes a still-searching source watch (pr_number=0)
// whose destination branch is already owned by a sibling row, then commits.
func dropSourceAndCommit(ctx context.Context, tx *sql.Tx, id string) error {
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM github_pr_watches WHERE id = ? AND pr_number = 0`, id); err != nil {
		return err
	}
	return tx.Commit()
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
			state, review_state, checks_state, mergeable_state, review_count, pending_review_count, required_reviews, comment_count,
			unresolved_review_threads, checks_total, checks_passing, additions, deletions,
			created_at, merged_at, closed_at, last_synced_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tp.ID, tp.TaskID, tp.RepositoryID, tp.Owner, tp.Repo, tp.PRNumber, tp.PRURL, tp.PRTitle, tp.HeadBranch, tp.BaseBranch, tp.AuthorLogin,
		tp.State, tp.ReviewState, tp.ChecksState, tp.MergeableState, tp.ReviewCount, tp.PendingReviewCount, tp.RequiredReviews, tp.CommentCount,
		tp.UnresolvedReviewThreads, tp.ChecksTotal, tp.ChecksPassing, tp.Additions, tp.Deletions,
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
//
// Multi-branch caveat: a task can hold N rows per (task, repo) — one per
// PR number. This lookup returns the most-recently-updated row so callers
// that need a deterministic single value still get one. Callers that need
// the row for a specific PR number must use GetTaskPRByRepoAndNumber.
func (s *Store) GetTaskPRByRepository(ctx context.Context, taskID, repositoryID string) (*TaskPR, error) {
	var tp TaskPR
	err := s.ro.GetContext(ctx, &tp,
		`SELECT * FROM github_task_prs WHERE task_id = ? AND repository_id = ?
		 ORDER BY updated_at DESC LIMIT 1`,
		taskID, repositoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &tp, err
}

// GetTaskPRByRepoAndNumber returns the exact PR row matching the
// (task, repository, pr_number) triple. Required for multi-branch tasks
// where AssociatePRWithTask's "already-current" short-circuit must check
// the same PR number, not a sibling PR that happens to be the first
// row returned by the legacy by-repo query.
func (s *Store) GetTaskPRByRepoAndNumber(ctx context.Context, taskID, repositoryID string, prNumber int) (*TaskPR, error) {
	var tp TaskPR
	err := s.ro.GetContext(ctx, &tp,
		`SELECT * FROM github_task_prs
		 WHERE task_id = ? AND repository_id = ? AND pr_number = ? LIMIT 1`,
		taskID, repositoryID, prNumber)
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

// ListTaskIssueMetadataByWorkspaceID projects task metadata for issue-link normalization.
func (s *Store) ListTaskIssueMetadataByWorkspaceID(ctx context.Context, workspaceID string) ([]taskIssueMetadataRow, error) {
	var rows []taskIssueMetadataRow
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(
		`SELECT id AS task_id, title AS task_title, COALESCE(metadata, '{}') AS metadata
		 FROM tasks
		 WHERE workspace_id = ?
		 ORDER BY created_at ASC, id ASC`,
	), workspaceID)
	return rows, err
}

// ListTaskIDsByPRNumber returns the IDs of tasks in a workspace that have a PR
// association with the given PR number. Workspace-scoped via the JOIN on tasks
// so a PR number shared across workspaces never leaks results. A task with
// multiple PR rows for the same number (multi-repo) is returned once.
func (s *Store) ListTaskIDsByPRNumber(ctx context.Context, workspaceID string, prNumber int) ([]string, error) {
	var ids []string
	if err := s.ro.SelectContext(ctx, &ids,
		`SELECT DISTINCT gtp.task_id FROM github_task_prs gtp
		 INNER JOIN tasks t ON gtp.task_id = t.id
		 WHERE t.workspace_id = ? AND gtp.pr_number = ?`, workspaceID, prNumber); err != nil {
		return nil, err
	}
	return ids, nil
}

func groupTaskPRsByTask(prs []TaskPR) map[string][]*TaskPR {
	result := make(map[string][]*TaskPR)
	for i := range prs {
		taskID := prs[i].TaskID
		result[taskID] = append(result[taskID], &prs[i])
	}
	return result
}

// ReplaceTaskPR atomically associates a PR with a task, replacing only the
// row that matches the exact (task_id, repository_id, pr_number) triple.
// Multi-branch tasks may hold multiple PR rows per (task, repo) — one per
// branch — so the delete MUST NOT wipe sibling PR rows. Single-repo
// callers (RepositoryID == "") only delete legacy untagged rows for the
// same PR number.
//
// The DELETE+INSERT pair inside one transaction is the upsert form; an
// ON CONFLICT would also work but the per-row delete pattern matches the
// existing migration layout (rebuilds are easier to reason about) and
// avoids leaking SQLite-specific syntax into the service layer.
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

	if tp.RepositoryID != "" {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM github_task_prs
			 WHERE task_id = ? AND repository_id = ? AND pr_number = ?`,
			tp.TaskID, tp.RepositoryID, tp.PRNumber); err != nil {
			return err
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM github_task_prs
			 WHERE task_id = ? AND repository_id = '' AND pr_number = ?`,
			tp.TaskID, tp.PRNumber); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO github_task_prs (id, task_id, repository_id, owner, repo, pr_number, pr_url, pr_title, head_branch, base_branch, author_login,
			state, review_state, checks_state, mergeable_state, review_count, pending_review_count, required_reviews, comment_count,
			unresolved_review_threads, checks_total, checks_passing, additions, deletions,
			created_at, merged_at, closed_at, last_synced_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tp.ID, tp.TaskID, tp.RepositoryID, tp.Owner, tp.Repo, tp.PRNumber, tp.PRURL, tp.PRTitle, tp.HeadBranch, tp.BaseBranch, tp.AuthorLogin,
		tp.State, tp.ReviewState, tp.ChecksState, tp.MergeableState, tp.ReviewCount, tp.PendingReviewCount, tp.RequiredReviews, tp.CommentCount,
		tp.UnresolvedReviewThreads, tp.ChecksTotal, tp.ChecksPassing, tp.Additions, tp.Deletions,
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
			review_count = ?, pending_review_count = ?, required_reviews = ?, comment_count = ?,
			unresolved_review_threads = ?, checks_total = ?, checks_passing = ?,
			additions = ?, deletions = ?, pr_title = ?, base_branch = ?,
			merged_at = ?, closed_at = ?, last_synced_at = ?, updated_at = ?
		WHERE id = ?`,
		tp.State, tp.ReviewState, tp.ChecksState, tp.MergeableState,
		tp.ReviewCount, tp.PendingReviewCount, tp.RequiredReviews, tp.CommentCount,
		tp.UnresolvedReviewThreads, tp.ChecksTotal, tp.ChecksPassing,
		tp.Additions, tp.Deletions, tp.PRTitle, tp.BaseBranch,
		tp.MergedAt, tp.ClosedAt, tp.LastSyncedAt, tp.UpdatedAt, tp.ID)
	return err
}

// --- Task CI automation operations ---

// GetTaskCIOptions returns persisted task CI automation options, or disabled defaults.
func (s *Store) GetTaskCIOptions(ctx context.Context, taskID string) (*TaskCIOptions, error) {
	var opts TaskCIOptions
	err := s.ro.GetContext(ctx, &opts, `SELECT * FROM github_task_ci_options WHERE task_id = ?`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		now := time.Now().UTC()
		return &TaskCIOptions{TaskID: taskID, CreatedAt: now, UpdatedAt: now}, nil
	}
	return &opts, err
}

// UpdateTaskCIOptions applies a partial update to task CI automation options.
func (s *Store) UpdateTaskCIOptions(ctx context.Context, taskID string, patch TaskCIOptionsPatch) (*TaskCIOptions, error) {
	writeCtx := context.WithoutCancel(ctx)
	now := time.Now().UTC()
	tx, err := s.db.BeginTxx(writeCtx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(writeCtx, `
		INSERT INTO github_task_ci_options (
			task_id, auto_fix_enabled, auto_merge_enabled, auto_fix_prompt_override, created_at, updated_at
		) VALUES (?, 0, 0, NULL, ?, ?)
		ON CONFLICT(task_id) DO NOTHING`,
		taskID, now, now); err != nil {
		return nil, err
	}
	var previous TaskCIOptions
	if err := tx.GetContext(writeCtx, &previous, `SELECT * FROM github_task_ci_options WHERE task_id = ?`, taskID); err != nil {
		return nil, err
	}
	autoFixSet, autoFixValue := boolPatchValue(patch.AutoFixEnabled)
	autoMergeSet, autoMergeValue := boolPatchValue(patch.AutoMergeEnabled)
	promptSet := patch.AutoFixPromptOverride != nil
	var promptValue *string
	if promptSet {
		trimmed := strings.TrimSpace(*patch.AutoFixPromptOverride)
		if trimmed != "" {
			promptValue = &trimmed
		}
	}
	if _, err := tx.ExecContext(writeCtx, `
		UPDATE github_task_ci_options SET
			auto_fix_enabled = CASE WHEN ? THEN ? ELSE auto_fix_enabled END,
			auto_merge_enabled = CASE WHEN ? THEN ? ELSE auto_merge_enabled END,
			auto_fix_prompt_override = CASE WHEN ? THEN ? ELSE auto_fix_prompt_override END,
			updated_at = ?
		WHERE task_id = ?`,
		autoFixSet, autoFixValue, autoMergeSet, autoMergeValue, promptSet, promptValue, now, taskID); err != nil {
		return nil, err
	}
	if autoFixSet && autoFixValue && !previous.AutoFixEnabled {
		if _, err := tx.ExecContext(writeCtx, `
			UPDATE github_task_ci_pr_state
			SET auto_fix_round_count = 0,
			    last_fix_signature = '',
			    last_fix_checkpoint_json = '',
			    last_fix_enqueued_at = NULL,
			    last_fix_session_id = NULL,
			    last_error = CASE WHEN auto_fix_exhausted_at IS NOT NULL THEN NULL ELSE last_error END,
			    auto_fix_exhausted_at = NULL,
			    updated_at = ?
			WHERE task_id = ?`, now, taskID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetTaskCIOptions(writeCtx, taskID)
}

// ListTaskCIPRStates returns CI automation state rows for a task.
func (s *Store) ListTaskCIPRStates(ctx context.Context, taskID string) ([]*TaskCIPRAutomationState, error) {
	var rows []TaskCIPRAutomationState
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT * FROM github_task_ci_pr_state WHERE task_id = ? ORDER BY repository_id ASC, pr_number ASC`,
		taskID); err != nil {
		return nil, err
	}
	out := make([]*TaskCIPRAutomationState, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

// GetTaskCIPRState returns one task/PR automation state row, or nil.
func (s *Store) GetTaskCIPRState(ctx context.Context, taskID, repositoryID string, prNumber int) (*TaskCIPRAutomationState, error) {
	var state TaskCIPRAutomationState
	err := s.ro.GetContext(ctx, &state,
		`SELECT * FROM github_task_ci_pr_state
		 WHERE task_id = ? AND repository_id = ? AND pr_number = ?`,
		taskID, repositoryID, prNumber)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &state, err
}

// RecordTaskCIFixAttempt records the feedback checkpoint that produced an auto-fix prompt.
func (s *Store) RecordTaskCIFixAttempt(ctx context.Context, attempt TaskCIFixAttempt) error {
	ctx = context.WithoutCancel(ctx)
	when := attempt.EnqueuedAt
	if when.IsZero() {
		when = time.Now().UTC()
	}
	now := time.Now().UTC()
	roundCount := 0
	if attempt.IncrementRound {
		roundCount = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_task_ci_pr_state (
			task_id, repository_id, pr_number, last_fix_signature, last_fix_checkpoint_json,
			last_fix_enqueued_at, last_fix_session_id, auto_fix_round_count, auto_fix_exhausted_at,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
		ON CONFLICT(task_id, repository_id, pr_number) DO UPDATE SET
			last_fix_signature = excluded.last_fix_signature,
			last_fix_checkpoint_json = excluded.last_fix_checkpoint_json,
			last_fix_enqueued_at = excluded.last_fix_enqueued_at,
			last_fix_session_id = excluded.last_fix_session_id,
			auto_fix_round_count = github_task_ci_pr_state.auto_fix_round_count + excluded.auto_fix_round_count,
			last_error = NULL,
			updated_at = excluded.updated_at`,
		attempt.TaskID, attempt.RepositoryID, attempt.PRNumber, attempt.Signature,
		attempt.CheckpointJSON, when, nullableString(attempt.SessionID), roundCount, now, now)
	return err
}

// RefreshTaskCIFixCheckpoint updates the current feedback checkpoint without recording a new prompt dispatch.
func (s *Store) RefreshTaskCIFixCheckpoint(ctx context.Context, taskID, repositoryID string, prNumber int, signature, checkpointJSON string) error {
	ctx = context.WithoutCancel(ctx)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_task_ci_pr_state (
			task_id, repository_id, pr_number, last_fix_signature, last_fix_checkpoint_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, pr_number) DO UPDATE SET
			last_fix_signature = excluded.last_fix_signature,
			last_fix_checkpoint_json = excluded.last_fix_checkpoint_json,
			last_fix_enqueued_at = NULL,
			last_fix_session_id = NULL,
			last_error = NULL,
			updated_at = excluded.updated_at`,
		taskID, repositoryID, prNumber, signature, checkpointJSON, now, now)
	return err
}

// RecordTaskCIMergeAttempt records an auto-merge attempt signature.
func (s *Store) RecordTaskCIMergeAttempt(ctx context.Context, attempt TaskCIMergeAttempt) error {
	ctx = context.WithoutCancel(ctx)
	when := attempt.AttemptedAt
	if when.IsZero() {
		when = time.Now().UTC()
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_task_ci_pr_state (
			task_id, repository_id, pr_number, last_merge_signature, last_merge_attempt_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, pr_number) DO UPDATE SET
			last_merge_signature = excluded.last_merge_signature,
			last_merge_attempt_at = excluded.last_merge_attempt_at,
			last_error = NULL,
			updated_at = excluded.updated_at`,
		attempt.TaskID, attempt.RepositoryID, attempt.PRNumber, attempt.Signature, when, now, now)
	return err
}

// RecordTaskCIError stores the latest user-visible CI automation error for a task PR.
func (s *Store) RecordTaskCIError(ctx context.Context, taskID, repositoryID string, prNumber int, message string) error {
	ctx = context.WithoutCancel(ctx)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_task_ci_pr_state (
			task_id, repository_id, pr_number, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, pr_number) DO UPDATE SET
			last_error = excluded.last_error,
			updated_at = excluded.updated_at`,
		taskID, repositoryID, prNumber, strings.TrimSpace(message), now, now)
	return err
}

// MarkTaskCIAutoFixExhausted records that auto-fix reached its per-PR round cap.
func (s *Store) MarkTaskCIAutoFixExhausted(ctx context.Context, taskID, repositoryID string, prNumber int, message string) error {
	ctx = context.WithoutCancel(ctx)
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_task_ci_pr_state (
			task_id, repository_id, pr_number, auto_fix_exhausted_at, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, pr_number) DO UPDATE SET
			auto_fix_exhausted_at = excluded.auto_fix_exhausted_at,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at`,
		taskID, repositoryID, prNumber, now, strings.TrimSpace(message), now, now)
	return err
}

// ClearTaskCIError clears the latest CI automation error for a task PR.
func (s *Store) ClearTaskCIError(ctx context.Context, taskID, repositoryID string, prNumber int) error {
	ctx = context.WithoutCancel(ctx)
	_, err := s.db.ExecContext(ctx, `
		UPDATE github_task_ci_pr_state SET last_error = NULL, updated_at = ?
		WHERE task_id = ? AND repository_id = ? AND pr_number = ?`,
		time.Now().UTC(), taskID, repositoryID, prNumber)
	return err
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func boolPatchValue(value *bool) (bool, bool) {
	if value == nil {
		return false, false
	}
	return true, *value
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
	rw.CleanupPolicy = NormalizeCleanupPolicy(rw.CleanupPolicy)
	reposJSON, err := json.Marshal(rw.Repos)
	if err != nil {
		return fmt.Errorf("marshal repos: %w", err)
	}
	rw.ReposJSON = string(reposJSON)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO github_review_watches (id, workspace_id, workflow_id, workflow_step_id, repos,
			agent_profile_id, executor_profile_id, prompt, review_scope, custom_query,
			enabled, poll_interval_seconds, cleanup_policy, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rw.ID, rw.WorkspaceID, rw.WorkflowID, rw.WorkflowStepID, rw.ReposJSON,
		rw.AgentProfileID, rw.ExecutorProfileID, rw.Prompt, rw.ReviewScope, rw.CustomQuery,
		rw.Enabled, rw.PollIntervalSeconds, rw.CleanupPolicy, rw.CreatedAt, rw.UpdatedAt)
	return err
}

// hydrateReviewWatchRepos unmarshals the ReposJSON field into the Repos slice
// and normalizes the cleanup policy so legacy rows (or zero values) surface
// as the documented default.
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
	rw.CleanupPolicy = NormalizeCleanupPolicy(rw.CleanupPolicy)
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

// ListAllReviewWatches returns every review watch across all workspaces. Used
// by the install-wide settings UI when no workspace filter is supplied.
func (s *Store) ListAllReviewWatches(ctx context.Context) ([]*ReviewWatch, error) {
	var watches []*ReviewWatch
	err := s.ro.SelectContext(ctx, &watches,
		`SELECT * FROM github_review_watches ORDER BY workspace_id, created_at`)
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
	rw.CleanupPolicy = NormalizeCleanupPolicy(rw.CleanupPolicy)
	reposJSON, err := json.Marshal(rw.Repos)
	if err != nil {
		return fmt.Errorf("marshal repos: %w", err)
	}
	rw.ReposJSON = string(reposJSON)
	_, err = s.db.ExecContext(ctx, `
		UPDATE github_review_watches SET workflow_id = ?, workflow_step_id = ?, repos = ?,
			agent_profile_id = ?, executor_profile_id = ?,
			prompt = ?, review_scope = ?, custom_query = ?,
			enabled = ?, poll_interval_seconds = ?, cleanup_policy = ?, last_polled_at = ?, updated_at = ?
		WHERE id = ?`,
		rw.WorkflowID, rw.WorkflowStepID, rw.ReposJSON,
		rw.AgentProfileID, rw.ExecutorProfileID,
		rw.Prompt, rw.ReviewScope, rw.CustomQuery,
		rw.Enabled, rw.PollIntervalSeconds, rw.CleanupPolicy, rw.LastPolledAt, rw.UpdatedAt, rw.ID)
	return err
}

// DeleteReviewWatch deletes a review watch and all its associated dedup rows
// in one transaction. Dedup rows have no foreign key (SQLite never enforced
// one for this table), so the explicit cascade is required — otherwise the
// rows survive after the watch is gone, become invisible to the per-watch
// poller, and the tasks they reference leak forever.
func (s *Store) DeleteReviewWatch(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM github_review_pr_tasks WHERE review_watch_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM github_review_watches WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// DisableReviewWatchWithError is the self-heal write: it disables the watch
// and stamps a human-readable cause + timestamp so the settings UI can show
// a "disabled because ..." banner. Called by the orchestrator when the
// watcher's bound agent profile is detected as soft-deleted.
func (s *Store) DisableReviewWatchWithError(ctx context.Context, id, cause string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE github_review_watches
		   SET enabled = 0, last_error = ?, last_error_at = ?, updated_at = ?
		 WHERE id = ?`,
		cause, now, now, id)
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

// ListAllReviewPRTasks lists every dedup record across all watches. Used by
// the global cleanup sweep so orphaned rows (whose watch was deleted or
// disabled) still get evaluated for terminal-state cleanup.
func (s *Store) ListAllReviewPRTasks(ctx context.Context) ([]*ReviewPRTask, error) {
	var tasks []*ReviewPRTask
	err := s.ro.SelectContext(ctx, &tasks,
		`SELECT id, review_watch_id, repo_owner, repo_name, pr_number, pr_url, task_id, created_at
		 FROM github_review_pr_tasks`)
	return tasks, err
}

// DeleteReviewPRTask deletes a dedup record by ID.
func (s *Store) DeleteReviewPRTask(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM github_review_pr_tasks WHERE id = ?`, id)
	return err
}

// ListReviewPRTaskIDsByWatch returns every task_id recorded against a
// review watch, including empty-string reservations. Used by the watch
// reset flow to enumerate the tasks to cascade-delete.
func (s *Store) ListReviewPRTaskIDsByWatch(ctx context.Context, watchID string) ([]string, error) {
	var ids []string
	err := s.ro.SelectContext(ctx, &ids,
		`SELECT task_id FROM github_review_pr_tasks WHERE review_watch_id = ?`, watchID)
	return ids, err
}

// ResetReviewWatchState wipes a review watch's dedup rows and nulls its
// last_polled_at in a single transaction. Used by the reset flow after
// the cascade-delete loop so the next poll re-imports every currently
// matching PR as if the watch were freshly created.
func (s *Store) ResetReviewWatchState(ctx context.Context, watchID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM github_review_pr_tasks WHERE review_watch_id = ?`, watchID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE github_review_watches SET last_polled_at = NULL, updated_at = ? WHERE id = ?`,
		time.Now().UTC(), watchID); err != nil {
		return err
	}
	return tx.Commit()
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

// hydrateIssueWatch unmarshals JSON fields into their Go slices and
// normalizes the cleanup policy so legacy rows (or zero values) surface as
// the documented default.
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
	iw.CleanupPolicy = NormalizeCleanupPolicy(iw.CleanupPolicy)
}

// CreateIssueWatch creates a new issue watch configuration.
func (s *Store) CreateIssueWatch(ctx context.Context, iw *IssueWatch) error {
	if iw.ID == "" {
		iw.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	iw.CreatedAt = now
	iw.UpdatedAt = now
	iw.CleanupPolicy = NormalizeCleanupPolicy(iw.CleanupPolicy)
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
			enabled, poll_interval_seconds, cleanup_policy, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		iw.ID, iw.WorkspaceID, iw.WorkflowID, iw.WorkflowStepID, iw.ReposJSON,
		iw.AgentProfileID, iw.ExecutorProfileID, iw.Prompt, iw.LabelsJSON, iw.CustomQuery,
		iw.Enabled, iw.PollIntervalSeconds, iw.CleanupPolicy, iw.CreatedAt, iw.UpdatedAt)
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

// ListAllIssueWatches returns every issue watch across all workspaces.
func (s *Store) ListAllIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	var watches []*IssueWatch
	err := s.ro.SelectContext(ctx, &watches,
		`SELECT * FROM github_issue_watches ORDER BY workspace_id, created_at`)
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
	iw.CleanupPolicy = NormalizeCleanupPolicy(iw.CleanupPolicy)
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
			enabled = ?, poll_interval_seconds = ?, cleanup_policy = ?, last_polled_at = ?, updated_at = ?
		WHERE id = ?`,
		iw.WorkflowID, iw.WorkflowStepID, iw.ReposJSON,
		iw.AgentProfileID, iw.ExecutorProfileID,
		iw.Prompt, iw.LabelsJSON, iw.CustomQuery,
		iw.Enabled, iw.PollIntervalSeconds, iw.CleanupPolicy, iw.LastPolledAt, iw.UpdatedAt, iw.ID)
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

// DisableIssueWatchWithError is the self-heal write: disables the watch and
// stamps a human-readable cause + timestamp. Symmetric with
// DisableReviewWatchWithError; called by the orchestrator when the
// watcher's bound agent profile is detected as soft-deleted.
func (s *Store) DisableIssueWatchWithError(ctx context.Context, id, cause string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE github_issue_watches
		   SET enabled = 0, last_error = ?, last_error_at = ?, updated_at = ?
		 WHERE id = ?`,
		cause, now, now, id)
	return err
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

// ListAllIssueWatchTasks lists every dedup record across all watches. Used by
// the global cleanup sweep so orphaned rows still get evaluated.
func (s *Store) ListAllIssueWatchTasks(ctx context.Context) ([]*IssueWatchTask, error) {
	var tasks []*IssueWatchTask
	err := s.ro.SelectContext(ctx, &tasks,
		`SELECT id, issue_watch_id, repo_owner, repo_name, issue_number, issue_url, task_id, created_at
		 FROM github_issue_watch_tasks`)
	return tasks, err
}

// DeleteIssueWatchTask deletes a dedup record by ID.
func (s *Store) DeleteIssueWatchTask(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM github_issue_watch_tasks WHERE id = ?`, id)
	return err
}

// ListIssueWatchTaskIDsByWatch returns every task_id recorded against an
// issue watch, including empty-string reservations. Used by the watch
// reset flow to enumerate the tasks to cascade-delete.
func (s *Store) ListIssueWatchTaskIDsByWatch(ctx context.Context, watchID string) ([]string, error) {
	var ids []string
	err := s.ro.SelectContext(ctx, &ids,
		`SELECT task_id FROM github_issue_watch_tasks WHERE issue_watch_id = ?`, watchID)
	return ids, err
}

// ResetIssueWatchState wipes an issue watch's dedup rows and nulls its
// last_polled_at in a single transaction. Used by the reset flow after
// the cascade-delete loop so the next poll re-imports every currently
// matching issue as if the watch were freshly created.
func (s *Store) ResetIssueWatchState(ctx context.Context, watchID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM github_issue_watch_tasks WHERE issue_watch_id = ?`, watchID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE github_issue_watches SET last_polled_at = NULL, updated_at = ? WHERE id = ?`,
		time.Now().UTC(), watchID); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Workspace settings operations ---

func defaultWorkspaceSettings(workspaceID string) *WorkspaceSettings {
	now := time.Now().UTC()
	return &WorkspaceSettings{
		WorkspaceID:         workspaceID,
		RepoScopeMode:       RepoScopeModeAll,
		RepoScopeOrgs:       []string{},
		RepoScopeRepos:      []RepoFilter{},
		SavedPresets:        json.RawMessage("[]"),
		DefaultQueryPresets: nil,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}

const maxWorkspaceSettingsJSONBytes = 64 * 1024

func normalizeRepoScopeMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	switch mode {
	case RepoScopeModeOrgs, RepoScopeModeRepos:
		return mode
	default:
		return RepoScopeModeAll
	}
}

func normalizeWorkspaceSettings(settings *WorkspaceSettings) *WorkspaceSettings {
	if settings == nil {
		return nil
	}
	out := *settings
	out.WorkspaceID = strings.TrimSpace(out.WorkspaceID)
	out.RepoScopeMode = normalizeRepoScopeMode(out.RepoScopeMode)
	if out.RepoScopeMode != RepoScopeModeOrgs {
		out.RepoScopeOrgs = nil
	}
	if out.RepoScopeMode != RepoScopeModeRepos {
		out.RepoScopeRepos = nil
	}
	orgs := make([]string, 0, len(out.RepoScopeOrgs))
	seenOrgs := make(map[string]struct{}, len(out.RepoScopeOrgs))
	for _, org := range out.RepoScopeOrgs {
		org = strings.TrimSpace(org)
		if org == "" {
			continue
		}
		key := strings.ToLower(org)
		if _, ok := seenOrgs[key]; ok {
			continue
		}
		seenOrgs[key] = struct{}{}
		orgs = append(orgs, org)
	}
	out.RepoScopeOrgs = orgs
	repos := make([]RepoFilter, 0, len(out.RepoScopeRepos))
	seenRepos := make(map[string]struct{}, len(out.RepoScopeRepos))
	for _, repo := range out.RepoScopeRepos {
		owner := strings.TrimSpace(repo.Owner)
		name := strings.TrimSpace(repo.Name)
		if owner == "" || name == "" {
			continue
		}
		key := strings.ToLower(owner + "/" + name)
		if _, ok := seenRepos[key]; ok {
			continue
		}
		seenRepos[key] = struct{}{}
		repos = append(repos, RepoFilter{Owner: owner, Name: name})
	}
	out.RepoScopeRepos = repos
	if len(out.SavedPresets) == 0 {
		out.SavedPresets = json.RawMessage("[]")
	} else {
		out.SavedPresets = cloneRawMessage(out.SavedPresets)
	}
	out.DefaultQueryPresets = cloneRawMessage(out.DefaultQueryPresets)
	return &out
}

// GetWorkspaceSettings returns per-workspace GitHub settings. Missing rows
// resolve to the backwards-compatible All repos defaults.
func (s *Store) GetWorkspaceSettings(ctx context.Context, workspaceID string) (*WorkspaceSettings, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	var row struct {
		WorkspaceID         string         `db:"workspace_id"`
		RepoScopeMode       string         `db:"repo_scope_mode"`
		RepoScopeOrgsJSON   string         `db:"repo_scope_orgs"`
		RepoScopeReposJSON  string         `db:"repo_scope_repos"`
		SavedPresets        string         `db:"saved_presets"`
		DefaultQueryPresets sql.NullString `db:"default_query_presets"`
		CreatedAt           time.Time      `db:"created_at"`
		UpdatedAt           time.Time      `db:"updated_at"`
	}
	err := s.ro.GetContext(ctx, &row, `
		SELECT workspace_id, repo_scope_mode, repo_scope_orgs, repo_scope_repos,
		       saved_presets, default_query_presets, created_at, updated_at
		FROM github_workspace_settings
		WHERE workspace_id = ?`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultWorkspaceSettings(workspaceID), nil
	}
	if err != nil {
		return nil, err
	}
	settings := &WorkspaceSettings{
		WorkspaceID:        row.WorkspaceID,
		RepoScopeMode:      row.RepoScopeMode,
		RepoScopeOrgsJSON:  row.RepoScopeOrgsJSON,
		RepoScopeReposJSON: row.RepoScopeReposJSON,
		SavedPresets:       json.RawMessage(row.SavedPresets),
		CreatedAt:          row.CreatedAt,
		UpdatedAt:          row.UpdatedAt,
	}
	if row.DefaultQueryPresets.Valid {
		settings.DefaultQueryPresets = json.RawMessage(row.DefaultQueryPresets.String)
	}
	if err := json.Unmarshal([]byte(row.RepoScopeOrgsJSON), &settings.RepoScopeOrgs); err != nil {
		return nil, fmt.Errorf("unmarshal repo scope orgs: %w", err)
	}
	if err := json.Unmarshal([]byte(row.RepoScopeReposJSON), &settings.RepoScopeRepos); err != nil {
		return nil, fmt.Errorf("unmarshal repo scope repos: %w", err)
	}
	return normalizeWorkspaceSettings(settings), nil
}

// UpsertWorkspaceSettings stores per-workspace GitHub settings.
func (s *Store) UpsertWorkspaceSettings(ctx context.Context, settings *WorkspaceSettings) error {
	settings = normalizeWorkspaceSettings(settings)
	if settings == nil || settings.WorkspaceID == "" {
		return fmt.Errorf("workspace_id is required")
	}
	orgsJSON, err := json.Marshal(settings.RepoScopeOrgs)
	if err != nil {
		return fmt.Errorf("marshal repo scope orgs: %w", err)
	}
	reposJSON, err := json.Marshal(settings.RepoScopeRepos)
	if err != nil {
		return fmt.Errorf("marshal repo scope repos: %w", err)
	}
	now := time.Now().UTC()
	defaults := sql.NullString{}
	if len(settings.DefaultQueryPresets) > 0 {
		defaults.Valid = true
		defaults.String = string(settings.DefaultQueryPresets)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO github_workspace_settings (
			workspace_id, repo_scope_mode, repo_scope_orgs, repo_scope_repos,
			saved_presets, default_query_presets, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			repo_scope_mode = excluded.repo_scope_mode,
			repo_scope_orgs = excluded.repo_scope_orgs,
			repo_scope_repos = excluded.repo_scope_repos,
			saved_presets = excluded.saved_presets,
			default_query_presets = excluded.default_query_presets,
			updated_at = excluded.updated_at`,
		settings.WorkspaceID, settings.RepoScopeMode, string(orgsJSON), string(reposJSON),
		string(settings.SavedPresets), defaults, now, now)
	return err
}

// PatchWorkspaceSettings applies only the fields present in the request. This
// avoids lost updates when independent preference migrations run concurrently.
func (s *Store) PatchWorkspaceSettings(ctx context.Context, req *UpdateWorkspaceSettingsRequest) (*WorkspaceSettings, error) {
	if req == nil || strings.TrimSpace(req.WorkspaceID) == "" {
		return nil, fmt.Errorf("%w: workspace_id is required", ErrWorkspaceSettingsValidation)
	}
	workspaceID := strings.TrimSpace(req.WorkspaceID)
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO github_workspace_settings (
			workspace_id, repo_scope_mode, repo_scope_orgs, repo_scope_repos,
			saved_presets, default_query_presets, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		workspaceID, RepoScopeModeAll, "[]", "[]", "[]", nil, now, now); err != nil {
		return nil, err
	}

	patch := workspaceSettingsPatch{
		sets: make([]string, 0, 8),
		args: make([]any, 0, 10),
	}
	if err := appendWorkspaceScopePatch(&patch, req); err != nil {
		return nil, err
	}
	if err := appendWorkspacePresetPatch(&patch, req); err != nil {
		return nil, err
	}
	if len(patch.sets) > 0 {
		patch.add("updated_at = ?", now)
		patch.args = append(patch.args, workspaceID)
		query := "UPDATE github_workspace_settings SET " + strings.Join(patch.sets, ", ") + " WHERE workspace_id = ?"
		if _, err := s.db.ExecContext(ctx, query, patch.args...); err != nil {
			return nil, err
		}
	}
	return s.GetWorkspaceSettings(ctx, workspaceID)
}

type workspaceSettingsPatch struct {
	sets []string
	args []any
}

func (p *workspaceSettingsPatch) add(set string, arg any) {
	p.sets = append(p.sets, set)
	p.args = append(p.args, arg)
}

func appendWorkspaceScopePatch(patch *workspaceSettingsPatch, req *UpdateWorkspaceSettingsRequest) error {
	if err := validateWorkspaceScopePatch(req); err != nil {
		return err
	}
	if req.RepoScopeMode != nil {
		if err := appendWorkspaceScopeModePatch(patch, *req.RepoScopeMode); err != nil {
			return err
		}
	}
	if req.RepoScopeOrgs != nil {
		raw, err := marshalNormalizedRepoScopeOrgs(*req.RepoScopeOrgs)
		if err != nil {
			return err
		}
		patch.add("repo_scope_orgs = ?", string(raw))
	}
	if req.RepoScopeRepos != nil {
		raw, err := marshalNormalizedRepoScopeRepos(*req.RepoScopeRepos)
		if err != nil {
			return err
		}
		patch.add("repo_scope_repos = ?", string(raw))
	}
	return nil
}

func validateWorkspaceScopePatch(req *UpdateWorkspaceSettingsRequest) error {
	if req.RepoScopeMode == nil {
		return nil
	}
	mode := normalizeRepoScopeMode(*req.RepoScopeMode)
	hasOrgs := req.RepoScopeOrgs != nil && len(*req.RepoScopeOrgs) > 0
	hasRepos := req.RepoScopeRepos != nil && len(*req.RepoScopeRepos) > 0
	if mode == RepoScopeModeAll && (hasOrgs || hasRepos) {
		return fmt.Errorf("%w: repo_scope_mode all cannot be patched with repo scope filters", ErrWorkspaceSettingsValidation)
	}
	if mode == RepoScopeModeOrgs && hasRepos {
		return fmt.Errorf("%w: repo_scope_mode orgs cannot be patched with repo_scope_repos", ErrWorkspaceSettingsValidation)
	}
	if mode == RepoScopeModeRepos && hasOrgs {
		return fmt.Errorf("%w: repo_scope_mode repos cannot be patched with repo_scope_orgs", ErrWorkspaceSettingsValidation)
	}
	return nil
}

func appendWorkspaceScopeModePatch(patch *workspaceSettingsPatch, rawMode string) error {
	if !isValidRepoScopeMode(rawMode) {
		return fmt.Errorf("%w: invalid repo_scope_mode %q", ErrWorkspaceSettingsValidation, rawMode)
	}
	mode := normalizeRepoScopeMode(rawMode)
	patch.add("repo_scope_mode = ?", mode)
	switch mode {
	case RepoScopeModeAll:
		patch.add("repo_scope_orgs = ?", "[]")
		patch.add("repo_scope_repos = ?", "[]")
	case RepoScopeModeOrgs:
		patch.add("repo_scope_repos = ?", "[]")
	case RepoScopeModeRepos:
		patch.add("repo_scope_orgs = ?", "[]")
	}
	return nil
}

func appendWorkspacePresetPatch(patch *workspaceSettingsPatch, req *UpdateWorkspaceSettingsRequest) error {
	if req.SavedPresetsSet {
		raw := json.RawMessage("[]")
		if req.SavedPresets != nil {
			raw = cloneRawMessage(*req.SavedPresets)
		}
		if err := validateWorkspaceSettingsJSON("saved_presets", raw, '['); err != nil {
			return err
		}
		patch.add("saved_presets = ?", string(raw))
	}
	if !req.DefaultQueriesSet {
		return nil
	}
	if req.DefaultQueryPresets == nil {
		patch.add("default_query_presets = ?", sql.NullString{})
		return nil
	}
	raw := cloneRawMessage(*req.DefaultQueryPresets)
	if err := validateWorkspaceSettingsJSON("default_query_presets", raw, '{'); err != nil {
		return err
	}
	patch.add("default_query_presets = ?", sql.NullString{String: string(raw), Valid: true})
	return nil
}

func marshalNormalizedRepoScopeOrgs(orgs []string) ([]byte, error) {
	settings := normalizeWorkspaceSettings(&WorkspaceSettings{
		RepoScopeMode: RepoScopeModeOrgs,
		RepoScopeOrgs: orgs,
	})
	raw, err := json.Marshal(settings.RepoScopeOrgs)
	if err != nil {
		return nil, fmt.Errorf("marshal repo scope orgs: %w", err)
	}
	return raw, nil
}

func marshalNormalizedRepoScopeRepos(repos []RepoFilter) ([]byte, error) {
	settings := normalizeWorkspaceSettings(&WorkspaceSettings{
		RepoScopeMode:  RepoScopeModeRepos,
		RepoScopeRepos: repos,
	})
	raw, err := json.Marshal(settings.RepoScopeRepos)
	if err != nil {
		return nil, fmt.Errorf("marshal repo scope repos: %w", err)
	}
	return raw, nil
}

func validateWorkspaceSettingsJSON(field string, raw json.RawMessage, wantFirst byte) error {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || len(raw) > maxWorkspaceSettingsJSONBytes || !json.Valid(raw) {
		return fmt.Errorf("%w: invalid %s", ErrWorkspaceSettingsValidation, field)
	}
	if raw[0] != wantFirst {
		return fmt.Errorf("%w: invalid %s", ErrWorkspaceSettingsValidation, field)
	}
	return nil
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
