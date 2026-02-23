package github

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
`

func (s *Store) initSchema() error {
	_, err := s.db.Exec(createTablesSQL)
	return err
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

// ListActivePRWatches returns all active PR watches.
func (s *Store) ListActivePRWatches(ctx context.Context) ([]*PRWatch, error) {
	var watches []*PRWatch
	err := s.ro.SelectContext(ctx, &watches, `SELECT * FROM github_pr_watches ORDER BY created_at`)
	return watches, err
}

// UpdatePRWatchTimestamps updates the last checked timestamps.
func (s *Store) UpdatePRWatchTimestamps(ctx context.Context, id string, checkedAt time.Time, commentAt *time.Time, checkStatus string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE github_pr_watches SET last_checked_at = ?, last_comment_at = ?, last_check_status = ?, updated_at = ?
		WHERE id = ?`,
		checkedAt, commentAt, checkStatus, time.Now().UTC(), id)
	return err
}

// DeletePRWatch deletes a PR watch by ID.
func (s *Store) DeletePRWatch(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM github_pr_watches WHERE id = ?`, id)
	return err
}

// UpdatePRWatchPRNumber updates a PR watch's PR number after discovery.
func (s *Store) UpdatePRWatchPRNumber(ctx context.Context, id string, prNumber int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE github_pr_watches SET pr_number = ?, updated_at = ? WHERE id = ?`,
		prNumber, time.Now().UTC(), id)
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
			state, review_state, checks_state, review_count, pending_review_count, comment_count, additions, deletions,
			created_at, merged_at, closed_at, last_synced_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tp.ID, tp.TaskID, tp.Owner, tp.Repo, tp.PRNumber, tp.PRURL, tp.PRTitle, tp.HeadBranch, tp.BaseBranch, tp.AuthorLogin,
		tp.State, tp.ReviewState, tp.ChecksState, tp.ReviewCount, tp.PendingReviewCount, tp.CommentCount, tp.Additions, tp.Deletions,
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

// UpdateTaskPR updates a task-PR association.
func (s *Store) UpdateTaskPR(ctx context.Context, tp *TaskPR) error {
	tp.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		UPDATE github_task_prs SET state = ?, review_state = ?, checks_state = ?,
			review_count = ?, pending_review_count = ?, comment_count = ?,
			additions = ?, deletions = ?, pr_title = ?,
			merged_at = ?, closed_at = ?, last_synced_at = ?, updated_at = ?
		WHERE id = ?`,
		tp.State, tp.ReviewState, tp.ChecksState,
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
			fmt.Printf("WARN: failed to unmarshal repos JSON for review watch %s: %v\n", rw.ID, err)
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
