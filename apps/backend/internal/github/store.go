package github

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
const createTablesSQL = `
	CREATE TABLE IF NOT EXISTS github_pr_watches (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL UNIQUE,
		task_id TEXT NOT NULL,
		owner TEXT NOT NULL,
		repo TEXT NOT NULL,
		pr_number INTEGER NOT NULL,
		branch TEXT NOT NULL,
		last_checked_at DATETIME,
		last_comment_at DATETIME,
		last_check_status TEXT DEFAULT '',
		last_review_state TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS github_task_prs (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
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
		UNIQUE(task_id, pr_number)
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
`

func (s *Store) initSchema() error {
	if _, err := s.db.Exec(createTablesSQL); err != nil {
		return err
	}
	// Idempotent migrations for existing databases.
	_, _ = s.db.Exec(`ALTER TABLE github_pr_watches ADD COLUMN last_review_state TEXT DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE github_task_prs ADD COLUMN mergeable_state TEXT NOT NULL DEFAULT ''`)
	return nil
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
		INSERT INTO github_pr_watches (id, session_id, task_id, owner, repo, pr_number, branch, last_check_status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.SessionID, w.TaskID, w.Owner, w.Repo, w.PRNumber, w.Branch, w.LastCheckStatus, w.CreatedAt, w.UpdatedAt)
	return err
}

// GetPRWatchBySession returns the PR watch for a session.
func (s *Store) GetPRWatchBySession(ctx context.Context, sessionID string) (*PRWatch, error) {
	var w PRWatch
	err := s.ro.GetContext(ctx, &w, `SELECT * FROM github_pr_watches WHERE session_id = ?`, sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &w, err
}

// GetPRWatchByTask returns the PR watch for a task (first match).
func (s *Store) GetPRWatchByTask(ctx context.Context, taskID string) (*PRWatch, error) {
	var w PRWatch
	err := s.ro.GetContext(ctx, &w, `SELECT * FROM github_pr_watches WHERE task_id = ? ORDER BY updated_at DESC LIMIT 1`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &w, err
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

// CreateTaskPR associates a PR with a task.
func (s *Store) CreateTaskPR(ctx context.Context, tp *TaskPR) error {
	if tp.ID == "" {
		tp.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	tp.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_task_prs (id, task_id, owner, repo, pr_number, pr_url, pr_title, head_branch, base_branch, author_login,
			state, review_state, checks_state, mergeable_state, review_count, pending_review_count, comment_count, additions, deletions,
			created_at, merged_at, closed_at, last_synced_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tp.ID, tp.TaskID, tp.Owner, tp.Repo, tp.PRNumber, tp.PRURL, tp.PRTitle, tp.HeadBranch, tp.BaseBranch, tp.AuthorLogin,
		tp.State, tp.ReviewState, tp.ChecksState, tp.MergeableState, tp.ReviewCount, tp.PendingReviewCount, tp.CommentCount, tp.Additions, tp.Deletions,
		tp.CreatedAt, tp.MergedAt, tp.ClosedAt, tp.LastSyncedAt, tp.UpdatedAt)
	return err
}

// GetTaskPR returns the PR association for a task.
func (s *Store) GetTaskPR(ctx context.Context, taskID string) (*TaskPR, error) {
	var tp TaskPR
	err := s.ro.GetContext(ctx, &tp, `SELECT * FROM github_task_prs WHERE task_id = ? LIMIT 1`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &tp, err
}

// ListTaskPRsByTaskIDs returns PR associations for multiple tasks.
func (s *Store) ListTaskPRsByTaskIDs(ctx context.Context, taskIDs []string) (map[string]*TaskPR, error) {
	if len(taskIDs) == 0 {
		return make(map[string]*TaskPR), nil
	}
	query, args, err := sqlx.In(`SELECT * FROM github_task_prs WHERE task_id IN (?)`, taskIDs)
	if err != nil {
		return nil, err
	}
	query = s.ro.Rebind(query)
	var prs []TaskPR
	if err := s.ro.SelectContext(ctx, &prs, query, args...); err != nil {
		return nil, err
	}
	result := make(map[string]*TaskPR, len(prs))
	for i := range prs {
		result[prs[i].TaskID] = &prs[i]
	}
	return result, nil
}

// ListTaskPRsByWorkspaceID returns all PR associations for tasks in a workspace.
func (s *Store) ListTaskPRsByWorkspaceID(ctx context.Context, workspaceID string) (map[string]*TaskPR, error) {
	var prs []TaskPR
	if err := s.ro.SelectContext(ctx, &prs,
		`SELECT gtp.* FROM github_task_prs gtp
		 INNER JOIN tasks t ON gtp.task_id = t.id
		 WHERE t.workspace_id = ?`, workspaceID); err != nil {
		return nil, err
	}
	result := make(map[string]*TaskPR, len(prs))
	for i := range prs {
		result[prs[i].TaskID] = &prs[i]
	}
	return result, nil
}

// ReplaceTaskPR atomically replaces the task→PR association for a task: any
// existing rows for task_id are deleted and the new row is inserted inside a
// single transaction, matching the effective 1:1 task→PR mapping used by
// reads. Use this when a task's active PR changes (e.g. the first PR was
// closed and a follow-up was opened).
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

	if _, err := tx.ExecContext(ctx, `DELETE FROM github_task_prs WHERE task_id = ?`, tp.TaskID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO github_task_prs (id, task_id, owner, repo, pr_number, pr_url, pr_title, head_branch, base_branch, author_login,
			state, review_state, checks_state, mergeable_state, review_count, pending_review_count, comment_count, additions, deletions,
			created_at, merged_at, closed_at, last_synced_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tp.ID, tp.TaskID, tp.Owner, tp.Repo, tp.PRNumber, tp.PRURL, tp.PRTitle, tp.HeadBranch, tp.BaseBranch, tp.AuthorLogin,
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
