package gitlab

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// --- MR Watch CRUD ---

// CreateMRWatch inserts a new MR watch row.
func (s *Store) CreateMRWatch(ctx context.Context, w *MRWatch) error {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	w.UpdatedAt = now
	_, err := s.db.NamedExecContext(ctx, `
		INSERT INTO gitlab_mr_watches (
			id, session_id, task_id, repository_id, project_path, mr_iid, branch,
			last_checked_at, last_note_at, last_pipeline_state, last_approval_state,
			created_at, updated_at
		) VALUES (
			:id, :session_id, :task_id, :repository_id, :project_path, :mr_iid, :branch,
			:last_checked_at, :last_note_at, :last_pipeline_state, :last_approval_state,
			:created_at, :updated_at
		)`, w)
	return err
}

const mrWatchSelectCols = `id, session_id, task_id, repository_id, project_path,
	mr_iid, branch, last_checked_at, last_note_at, last_pipeline_state,
	last_approval_state, created_at, updated_at`

// GetMRWatchBySession returns the MR watch for a session (legacy single-repo).
func (s *Store) GetMRWatchBySession(ctx context.Context, sessionID string) (*MRWatch, error) {
	return s.GetMRWatchBySessionAndRepo(ctx, sessionID, "")
}

// GetMRWatchBySessionAndRepo returns the MR watch keyed by (session, repository).
func (s *Store) GetMRWatchBySessionAndRepo(ctx context.Context, sessionID, repositoryID string) (*MRWatch, error) {
	var w MRWatch
	err := s.ro.GetContext(ctx, &w,
		`SELECT `+mrWatchSelectCols+` FROM gitlab_mr_watches
		 WHERE session_id = ? AND repository_id = ? LIMIT 1`, sessionID, repositoryID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}

// ListMRWatchesBySession returns every MR watch for a session.
func (s *Store) ListMRWatchesBySession(ctx context.Context, sessionID string) ([]*MRWatch, error) {
	var ws []MRWatch
	if err := s.ro.SelectContext(ctx, &ws,
		`SELECT `+mrWatchSelectCols+` FROM gitlab_mr_watches
		 WHERE session_id = ? ORDER BY created_at ASC`, sessionID); err != nil {
		return nil, err
	}
	out := make([]*MRWatch, 0, len(ws))
	for i := range ws {
		out = append(out, &ws[i])
	}
	return out, nil
}

// ListMRWatchesBySessionForWorkspace scopes a session lookup through the
// owning task. Legacy rows whose task no longer exists are intentionally not
// exposed by workspace-facing routes.
func (s *Store) ListMRWatchesBySessionForWorkspace(ctx context.Context, workspaceID, sessionID string) ([]*MRWatch, error) {
	return s.listMRWatchesForWorkspace(ctx, workspaceID, "w.session_id = ?", sessionID)
}

// ListMRWatchesByTask returns every MR watch for a task.
func (s *Store) ListMRWatchesByTask(ctx context.Context, taskID string) ([]*MRWatch, error) {
	var ws []MRWatch
	if err := s.ro.SelectContext(ctx, &ws,
		`SELECT `+mrWatchSelectCols+` FROM gitlab_mr_watches
		 WHERE task_id = ? ORDER BY created_at ASC`, taskID); err != nil {
		return nil, err
	}
	out := make([]*MRWatch, 0, len(ws))
	for i := range ws {
		out = append(out, &ws[i])
	}
	return out, nil
}

func (s *Store) ListMRWatchesByTaskForWorkspace(ctx context.Context, workspaceID, taskID string) ([]*MRWatch, error) {
	return s.listMRWatchesForWorkspace(ctx, workspaceID, "w.task_id = ?", taskID)
}

// ListActiveMRWatches returns every persisted MR watch (used by the poller).
func (s *Store) ListActiveMRWatches(ctx context.Context) ([]*MRWatch, error) {
	var ws []MRWatch
	if err := s.ro.SelectContext(ctx, &ws,
		`SELECT `+mrWatchSelectCols+` FROM gitlab_mr_watches ORDER BY created_at ASC`); err != nil {
		return nil, err
	}
	out := make([]*MRWatch, 0, len(ws))
	for i := range ws {
		out = append(out, &ws[i])
	}
	return out, nil
}

func (s *Store) ListActiveMRWatchesForWorkspace(ctx context.Context, workspaceID string) ([]*MRWatch, error) {
	return s.listMRWatchesForWorkspace(ctx, workspaceID, "1 = 1")
}

func (s *Store) listMRWatchesForWorkspace(ctx context.Context, workspaceID, predicate string, args ...interface{}) ([]*MRWatch, error) {
	queryArgs := append([]interface{}{workspaceID}, args...)
	var rows []MRWatch
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT w.id, w.session_id, w.task_id, w.repository_id, w.project_path,
			w.mr_iid, w.branch, w.last_checked_at, w.last_note_at,
			w.last_pipeline_state, w.last_approval_state, w.created_at, w.updated_at
		 FROM gitlab_mr_watches w
		 JOIN tasks t ON t.id = w.task_id
		 WHERE t.workspace_id = ? AND `+predicate+` ORDER BY w.created_at ASC`, queryArgs...); err != nil {
		return nil, err
	}
	out := make([]*MRWatch, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

// UpdateMRWatchTimestamps records the last poll cycle's observation.
func (s *Store) UpdateMRWatchTimestamps(ctx context.Context, id string, checkedAt time.Time, noteAt *time.Time, pipelineState, approvalState string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE gitlab_mr_watches SET
			last_checked_at = ?, last_note_at = ?,
			last_pipeline_state = ?, last_approval_state = ?, updated_at = ?
		WHERE id = ?`, checkedAt, noteAt, pipelineState, approvalState, time.Now().UTC(), id)
	return err
}

// UpdateMRWatchMRIID stamps the watch with the MR iid once detected.
func (s *Store) UpdateMRWatchMRIID(ctx context.Context, id string, iid int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE gitlab_mr_watches SET mr_iid = ?, updated_at = ? WHERE id = ?`,
		iid, time.Now().UTC(), id)
	return err
}

// DeleteMRWatch removes a single MR watch by id.
func (s *Store) DeleteMRWatch(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM gitlab_mr_watches WHERE id = ?`, id)
	return err
}

func (s *Store) DeleteMRWatchForWorkspace(ctx context.Context, workspaceID, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM gitlab_mr_watches
		WHERE id = ? AND EXISTS (
			SELECT 1 FROM tasks t WHERE t.id = gitlab_mr_watches.task_id AND t.workspace_id = ?
		)`, id, workspaceID)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

// DeleteMRWatchesByTaskID removes all MR watches associated with a task.
func (s *Store) DeleteMRWatchesByTaskID(ctx context.Context, taskID string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM gitlab_mr_watches WHERE task_id = ?`, taskID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// --- Review Watch CRUD ---

const reviewWatchSelectCols = `id, workspace_id, workflow_id, workflow_step_id,
	projects, agent_profile_id, executor_profile_id, prompt, repository_id, base_branch, review_scope,
	custom_query, enabled, poll_interval_seconds, cleanup_policy,
	max_inflight_tasks, generation, deleting, last_error, last_error_at, last_polled_at, created_at, updated_at`

// CreateReviewWatch inserts a review watch and serializes Projects to JSON.
func (s *Store) CreateReviewWatch(ctx context.Context, rw *ReviewWatch) error {
	if rw.ID == "" {
		rw.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if rw.CreatedAt.IsZero() {
		rw.CreatedAt = now
	}
	rw.UpdatedAt = now
	projectsJSON, err := encodeProjects(rw.Projects)
	if err != nil {
		return err
	}
	rw.ProjectsJSON = projectsJSON
	if rw.PollIntervalSeconds <= 0 {
		rw.PollIntervalSeconds = 300
	}
	if rw.ReviewScope == "" {
		rw.ReviewScope = ReviewScopeUserAndTeams
	}
	if rw.Generation <= 0 {
		rw.Generation = 1
	}
	rw.CleanupPolicy = NormalizeCleanupPolicy(rw.CleanupPolicy)
	_, err = s.db.NamedExecContext(ctx, `
		INSERT INTO gitlab_review_watches (
			id, workspace_id, workflow_id, workflow_step_id, projects,
			agent_profile_id, executor_profile_id, prompt, repository_id, base_branch, review_scope, custom_query,
			enabled, poll_interval_seconds, cleanup_policy, max_inflight_tasks, generation, deleting, last_error, last_error_at, last_polled_at,
			created_at, updated_at
		) VALUES (
			:id, :workspace_id, :workflow_id, :workflow_step_id, :projects,
			:agent_profile_id, :executor_profile_id, :prompt, :repository_id, :base_branch, :review_scope, :custom_query,
			:enabled, :poll_interval_seconds, :cleanup_policy, :max_inflight_tasks, :generation, :deleting, :last_error, :last_error_at, :last_polled_at,
			:created_at, :updated_at
		)`, rw)
	return err
}

// GetReviewWatch returns the review watch row by id.
func (s *Store) GetReviewWatch(ctx context.Context, id string) (*ReviewWatch, error) {
	return s.getReviewWatch(ctx, id, false)
}

func (s *Store) GetReviewWatchIncludingDeleting(ctx context.Context, id string) (*ReviewWatch, error) {
	return s.getReviewWatch(ctx, id, true)
}

func (s *Store) getReviewWatch(ctx context.Context, id string, includeDeleting bool) (*ReviewWatch, error) {
	var rw ReviewWatch
	deletingClause := " AND deleting = 0"
	if includeDeleting {
		deletingClause = ""
	}
	err := s.ro.GetContext(ctx, &rw,
		`SELECT `+reviewWatchSelectCols+` FROM gitlab_review_watches WHERE id = ?`+deletingClause, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := decodeProjectsInto(&rw); err != nil {
		return nil, err
	}
	return &rw, nil
}

// ListReviewWatches lists review watches in a workspace.
func (s *Store) ListReviewWatches(ctx context.Context, workspaceID string) ([]*ReviewWatch, error) {
	return s.listReviewWatches(ctx,
		`SELECT `+reviewWatchSelectCols+` FROM gitlab_review_watches
		 WHERE workspace_id = ? AND deleting = 0 ORDER BY created_at ASC`, workspaceID)
}

// ListAllReviewWatches lists every review watch (used by the poller).
func (s *Store) ListAllReviewWatches(ctx context.Context) ([]*ReviewWatch, error) {
	return s.listReviewWatches(ctx,
		`SELECT `+reviewWatchSelectCols+` FROM gitlab_review_watches WHERE deleting = 0 ORDER BY created_at ASC`)
}

// ListEnabledReviewWatches lists every enabled review watch.
func (s *Store) ListEnabledReviewWatches(ctx context.Context) ([]*ReviewWatch, error) {
	return s.listReviewWatches(ctx,
		`SELECT `+reviewWatchSelectCols+` FROM gitlab_review_watches
		 WHERE enabled = 1 AND deleting = 0 ORDER BY created_at ASC`)
}

func (s *Store) ListEnabledReviewWatchesForWorkspace(ctx context.Context, workspaceID string) ([]*ReviewWatch, error) {
	return s.listReviewWatches(ctx,
		`SELECT `+reviewWatchSelectCols+` FROM gitlab_review_watches
		 WHERE enabled = 1 AND deleting = 0 AND workspace_id = ? ORDER BY created_at ASC`, workspaceID)
}

func (s *Store) listReviewWatches(ctx context.Context, query string, args ...interface{}) ([]*ReviewWatch, error) {
	var rows []ReviewWatch
	if err := s.ro.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make([]*ReviewWatch, 0, len(rows))
	for i := range rows {
		if err := decodeProjectsInto(&rows[i]); err != nil {
			return nil, err
		}
		out = append(out, &rows[i])
	}
	return out, nil
}

// UpdateReviewWatch persists changes to a review watch (full replacement).
func (s *Store) UpdateReviewWatch(ctx context.Context, rw *ReviewWatch) error {
	projectsJSON, err := encodeProjects(rw.Projects)
	if err != nil {
		return err
	}
	rw.ProjectsJSON = projectsJSON
	rw.UpdatedAt = time.Now().UTC()
	rw.CleanupPolicy = NormalizeCleanupPolicy(rw.CleanupPolicy)
	res, err := s.db.NamedExecContext(ctx, `
		UPDATE gitlab_review_watches SET
			workflow_id = :workflow_id, workflow_step_id = :workflow_step_id,
			projects = :projects, agent_profile_id = :agent_profile_id,
			executor_profile_id = :executor_profile_id, prompt = :prompt,
			repository_id = :repository_id, base_branch = :base_branch,
			review_scope = :review_scope, custom_query = :custom_query,
			enabled = :enabled, poll_interval_seconds = :poll_interval_seconds,
			cleanup_policy = :cleanup_policy, max_inflight_tasks = :max_inflight_tasks,
			last_error = :last_error, last_error_at = :last_error_at, last_polled_at = :last_polled_at,
			updated_at = :updated_at
		WHERE id = :id`, rw)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("review watch %s not found", rw.ID)
	}
	return nil
}

// RecordReviewWatchPoll stamps last_polled_at without touching other fields.
func (s *Store) RecordReviewWatchPoll(ctx context.Context, id string, polledAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE gitlab_review_watches SET last_polled_at = ?, updated_at = ? WHERE id = ?`,
		polledAt, time.Now().UTC(), id)
	return err
}

func (s *Store) DisableReviewWatchWithError(ctx context.Context, id, cause string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE gitlab_review_watches
		SET enabled = 0, last_error = ?, last_error_at = ?, updated_at = ? WHERE id = ?`,
		cause, now, now, id)
	return err
}

// DeleteReviewWatch removes a review watch by id.
func (s *Store) DeleteReviewWatch(ctx context.Context, id string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM gitlab_review_mr_tasks WHERE review_watch_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gitlab_review_watches WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Review MR Task dedup ---

const reviewMRTaskSelectCols = `id, review_watch_id, project_path, mr_iid,
	mr_url, task_id, generation, created_at`

// HasReviewMRTask reports whether the given (watch, project, iid) is already
// dedup-claimed.
func (s *Store) HasReviewMRTask(ctx context.Context, reviewWatchID, projectPath string, iid int) (bool, error) {
	var count int
	err := s.ro.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM gitlab_review_mr_tasks
		 WHERE review_watch_id = ? AND project_path = ? AND mr_iid = ?`,
		reviewWatchID, projectPath, iid)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ReserveReviewMRTask inserts a placeholder row (task_id="") for an MR; returns
// true if reservation succeeded (this caller wins the race), false if another
// caller already reserved.
func (s *Store) ReserveReviewMRTask(ctx context.Context, reviewWatchID, projectPath string, iid int, mrURL string) (bool, error) {
	generation, err := s.reviewWatchGeneration(ctx, reviewWatchID)
	if err != nil {
		return false, err
	}
	return s.reserveReviewMRTask(ctx, reviewWatchID, generation, projectPath, iid, mrURL, false)
}

func (s *Store) ReserveReviewMRTaskForGeneration(ctx context.Context, reviewWatchID string, generation int64, projectPath string, iid int, mrURL string) (bool, error) {
	return s.reserveReviewMRTask(ctx, reviewWatchID, generation, projectPath, iid, mrURL, true)
}

func (s *Store) reserveReviewMRTask(ctx context.Context, reviewWatchID string, generation int64, projectPath string, iid int, mrURL string, requireActive bool) (bool, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	query := `INSERT INTO gitlab_review_mr_tasks
		(id, review_watch_id, project_path, mr_iid, mr_url, task_id, generation, created_at)
		VALUES (?, ?, ?, ?, ?, '', ?, ?)
		ON CONFLICT(review_watch_id, project_path, mr_iid) DO NOTHING`
	args := []interface{}{id, reviewWatchID, projectPath, iid, mrURL, generation, now}
	if requireActive {
		query = `
		INSERT INTO gitlab_review_mr_tasks (id, review_watch_id, project_path, mr_iid, mr_url, task_id, generation, created_at)
		SELECT ?, ?, ?, ?, ?, '', ?, ?
		WHERE EXISTS (SELECT 1 FROM gitlab_review_watches
			WHERE id = ? AND generation = ? AND enabled = 1 AND deleting = 0)
		ON CONFLICT(review_watch_id, project_path, mr_iid) DO NOTHING`
		args = append(args, reviewWatchID, generation)
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// AssignReviewMRTaskID swaps the reserved placeholder task_id for the real id.
func (s *Store) AssignReviewMRTaskID(ctx context.Context, reviewWatchID, projectPath string, iid int, taskID string) error {
	generation, err := s.reviewWatchGeneration(ctx, reviewWatchID)
	if err != nil {
		return err
	}
	return s.assignReviewMRTaskID(ctx, reviewWatchID, generation, projectPath, iid, taskID, false)
}

func (s *Store) AssignReviewMRTaskIDForGeneration(ctx context.Context, reviewWatchID string, generation int64, projectPath string, iid int, taskID string) error {
	return s.assignReviewMRTaskID(ctx, reviewWatchID, generation, projectPath, iid, taskID, true)
}

func (s *Store) assignReviewMRTaskID(ctx context.Context, reviewWatchID string, generation int64, projectPath string, iid int, taskID string, requireActive bool) error {
	activeClause := ""
	if requireActive {
		activeClause = ` AND EXISTS (SELECT 1 FROM gitlab_review_watches
			WHERE id = ? AND generation = ? AND enabled = 1 AND deleting = 0)`
	}
	args := []interface{}{taskID, reviewWatchID, projectPath, iid, generation}
	if requireActive {
		args = append(args, reviewWatchID, generation)
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE gitlab_review_mr_tasks SET task_id = ?
		WHERE review_watch_id = ? AND project_path = ? AND mr_iid = ? AND generation = ?`+activeClause,
		args...)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		if requireActive {
			return ErrWatchOwnershipLost
		}
		return fmt.Errorf("review MR task not found: watch=%s project=%s iid=%d", reviewWatchID, projectPath, iid)
	}
	return nil
}

// ReleaseReviewMRTask removes the (failed) reservation so future polls can
// retry on the same MR.
func (s *Store) ReleaseReviewMRTask(ctx context.Context, reviewWatchID, projectPath string, iid int) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM gitlab_review_mr_tasks
		 WHERE review_watch_id = ? AND project_path = ? AND mr_iid = ?`,
		reviewWatchID, projectPath, iid)
	return err
}

func (s *Store) ReleaseReviewMRTaskForGeneration(ctx context.Context, reviewWatchID string, generation int64, projectPath string, iid int) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM gitlab_review_mr_tasks
		 WHERE review_watch_id = ? AND project_path = ? AND mr_iid = ? AND generation = ?`,
		reviewWatchID, projectPath, iid, generation)
	return err
}

func (s *Store) reviewWatchGeneration(ctx context.Context, watchID string) (int64, error) {
	var generation int64
	if err := s.ro.GetContext(ctx, &generation,
		`SELECT generation FROM gitlab_review_watches WHERE id = ?`, watchID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 1, nil
		}
		return 0, err
	}
	return generation, nil
}

// ListReviewMRTasksByWatch returns dedup rows for a given watch.
func (s *Store) ListReviewMRTasksByWatch(ctx context.Context, watchID string) ([]*ReviewMRTask, error) {
	var rows []ReviewMRTask
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT `+reviewMRTaskSelectCols+` FROM gitlab_review_mr_tasks
		 WHERE review_watch_id = ? ORDER BY created_at ASC`, watchID); err != nil {
		return nil, err
	}
	out := make([]*ReviewMRTask, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

// ListAllReviewMRTasks returns every dedup row (used by cleanup sweepers).
func (s *Store) ListAllReviewMRTasks(ctx context.Context) ([]*ReviewMRTask, error) {
	var rows []ReviewMRTask
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT `+reviewMRTaskSelectCols+` FROM gitlab_review_mr_tasks ORDER BY created_at ASC`); err != nil {
		return nil, err
	}
	out := make([]*ReviewMRTask, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

func (s *Store) ListReviewMRTasksForWorkspace(ctx context.Context, workspaceID string) ([]*ReviewMRTask, error) {
	var rows []ReviewMRTask
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT t.id, t.review_watch_id, t.project_path, t.mr_iid,
			t.mr_url, t.task_id, t.generation, t.created_at
		 FROM gitlab_review_mr_tasks t
		 JOIN gitlab_review_watches w ON w.id = t.review_watch_id
		 WHERE w.workspace_id = ? ORDER BY t.created_at ASC`, workspaceID); err != nil {
		return nil, err
	}
	out := make([]*ReviewMRTask, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

// DeleteReviewMRTask removes a dedup row by id.
func (s *Store) DeleteReviewMRTask(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM gitlab_review_mr_tasks WHERE id = ?`, id)
	return err
}

// --- Issue Watch CRUD ---

const issueWatchSelectCols = `id, workspace_id, workflow_id, workflow_step_id,
	projects, agent_profile_id, executor_profile_id, prompt, repository_id, base_branch, labels, custom_query,
	enabled, poll_interval_seconds, cleanup_policy, max_inflight_tasks, last_error, last_error_at,
	generation, deleting, last_polled_at, created_at, updated_at`

// CreateIssueWatch inserts an issue watch and serializes Projects + Labels.
func (s *Store) CreateIssueWatch(ctx context.Context, iw *IssueWatch) error {
	if iw.ID == "" {
		iw.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	if iw.CreatedAt.IsZero() {
		iw.CreatedAt = now
	}
	iw.UpdatedAt = now
	projectsJSON, err := encodeProjects(iw.Projects)
	if err != nil {
		return err
	}
	iw.ProjectsJSON = projectsJSON
	labelsJSON, err := encodeStringList(iw.Labels)
	if err != nil {
		return err
	}
	iw.LabelsJSON = labelsJSON
	if iw.PollIntervalSeconds <= 0 {
		iw.PollIntervalSeconds = 300
	}
	if iw.Generation <= 0 {
		iw.Generation = 1
	}
	iw.CleanupPolicy = NormalizeCleanupPolicy(iw.CleanupPolicy)
	_, err = s.db.NamedExecContext(ctx, `
		INSERT INTO gitlab_issue_watches (
			id, workspace_id, workflow_id, workflow_step_id, projects,
			agent_profile_id, executor_profile_id, prompt, repository_id, base_branch, labels, custom_query,
			enabled, poll_interval_seconds, cleanup_policy, max_inflight_tasks, generation, deleting, last_error, last_error_at, last_polled_at,
			created_at, updated_at
		) VALUES (
			:id, :workspace_id, :workflow_id, :workflow_step_id, :projects,
			:agent_profile_id, :executor_profile_id, :prompt, :repository_id, :base_branch, :labels, :custom_query,
			:enabled, :poll_interval_seconds, :cleanup_policy, :max_inflight_tasks, :generation, :deleting, :last_error, :last_error_at, :last_polled_at,
			:created_at, :updated_at
		)`, iw)
	return err
}

// GetIssueWatch returns an issue watch by id.
func (s *Store) GetIssueWatch(ctx context.Context, id string) (*IssueWatch, error) {
	return s.getIssueWatch(ctx, id, false)
}

func (s *Store) GetIssueWatchIncludingDeleting(ctx context.Context, id string) (*IssueWatch, error) {
	return s.getIssueWatch(ctx, id, true)
}

func (s *Store) getIssueWatch(ctx context.Context, id string, includeDeleting bool) (*IssueWatch, error) {
	var iw IssueWatch
	deletingClause := " AND deleting = 0"
	if includeDeleting {
		deletingClause = ""
	}
	err := s.ro.GetContext(ctx, &iw,
		`SELECT `+issueWatchSelectCols+` FROM gitlab_issue_watches WHERE id = ?`+deletingClause, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := decodeIssueWatch(&iw); err != nil {
		return nil, err
	}
	return &iw, nil
}

// ListIssueWatches lists issue watches in a workspace.
func (s *Store) ListIssueWatches(ctx context.Context, workspaceID string) ([]*IssueWatch, error) {
	return s.listIssueWatches(ctx,
		`SELECT `+issueWatchSelectCols+` FROM gitlab_issue_watches
		 WHERE workspace_id = ? AND deleting = 0 ORDER BY created_at ASC`, workspaceID)
}

// ListAllIssueWatches lists every issue watch.
func (s *Store) ListAllIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	return s.listIssueWatches(ctx,
		`SELECT `+issueWatchSelectCols+` FROM gitlab_issue_watches WHERE deleting = 0 ORDER BY created_at ASC`)
}

// ListEnabledIssueWatches lists every enabled issue watch.
func (s *Store) ListEnabledIssueWatches(ctx context.Context) ([]*IssueWatch, error) {
	return s.listIssueWatches(ctx,
		`SELECT `+issueWatchSelectCols+` FROM gitlab_issue_watches
		 WHERE enabled = 1 AND deleting = 0 ORDER BY created_at ASC`)
}

func (s *Store) ListEnabledIssueWatchesForWorkspace(ctx context.Context, workspaceID string) ([]*IssueWatch, error) {
	return s.listIssueWatches(ctx,
		`SELECT `+issueWatchSelectCols+` FROM gitlab_issue_watches
		 WHERE enabled = 1 AND deleting = 0 AND workspace_id = ? ORDER BY created_at ASC`, workspaceID)
}

func (s *Store) listIssueWatches(ctx context.Context, query string, args ...interface{}) ([]*IssueWatch, error) {
	var rows []IssueWatch
	if err := s.ro.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	out := make([]*IssueWatch, 0, len(rows))
	for i := range rows {
		if err := decodeIssueWatch(&rows[i]); err != nil {
			return nil, err
		}
		out = append(out, &rows[i])
	}
	return out, nil
}

// UpdateIssueWatch persists changes to an issue watch.
func (s *Store) UpdateIssueWatch(ctx context.Context, iw *IssueWatch) error {
	projectsJSON, err := encodeProjects(iw.Projects)
	if err != nil {
		return err
	}
	iw.ProjectsJSON = projectsJSON
	labelsJSON, err := encodeStringList(iw.Labels)
	if err != nil {
		return err
	}
	iw.LabelsJSON = labelsJSON
	iw.UpdatedAt = time.Now().UTC()
	iw.CleanupPolicy = NormalizeCleanupPolicy(iw.CleanupPolicy)
	res, err := s.db.NamedExecContext(ctx, `
		UPDATE gitlab_issue_watches SET
			workflow_id = :workflow_id, workflow_step_id = :workflow_step_id,
			projects = :projects, agent_profile_id = :agent_profile_id,
			executor_profile_id = :executor_profile_id, prompt = :prompt,
			repository_id = :repository_id, base_branch = :base_branch,
			labels = :labels, custom_query = :custom_query,
			enabled = :enabled, poll_interval_seconds = :poll_interval_seconds,
			cleanup_policy = :cleanup_policy, max_inflight_tasks = :max_inflight_tasks,
			last_error = :last_error, last_error_at = :last_error_at, last_polled_at = :last_polled_at,
			updated_at = :updated_at
		WHERE id = :id`, iw)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("issue watch %s not found", iw.ID)
	}
	return nil
}

// RecordIssueWatchPoll stamps last_polled_at on the watch.
func (s *Store) RecordIssueWatchPoll(ctx context.Context, id string, polledAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE gitlab_issue_watches SET last_polled_at = ?, updated_at = ? WHERE id = ?`,
		polledAt, time.Now().UTC(), id)
	return err
}

func (s *Store) DisableIssueWatchWithError(ctx context.Context, id, cause string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE gitlab_issue_watches
		SET enabled = 0, last_error = ?, last_error_at = ?, updated_at = ? WHERE id = ?`,
		cause, now, now, id)
	return err
}

// DeleteIssueWatch removes an issue watch.
func (s *Store) DeleteIssueWatch(ctx context.Context, id string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM gitlab_issue_watch_tasks WHERE issue_watch_id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gitlab_issue_watches WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Issue Watch Task dedup ---

const issueWatchTaskSelectCols = `id, issue_watch_id, project_path, issue_iid,
	issue_url, task_id, generation, created_at`

// HasIssueWatchTask reports whether the (watch, project, iid) is dedup-claimed.
func (s *Store) HasIssueWatchTask(ctx context.Context, issueWatchID, projectPath string, iid int) (bool, error) {
	var count int
	err := s.ro.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM gitlab_issue_watch_tasks
		 WHERE issue_watch_id = ? AND project_path = ? AND issue_iid = ?`,
		issueWatchID, projectPath, iid)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// ReserveIssueWatchTask inserts a placeholder row claiming dedup.
func (s *Store) ReserveIssueWatchTask(ctx context.Context, issueWatchID, projectPath string, iid int, issueURL string) (bool, error) {
	generation, err := s.issueWatchGeneration(ctx, issueWatchID)
	if err != nil {
		return false, err
	}
	return s.reserveIssueWatchTask(ctx, issueWatchID, generation, projectPath, iid, issueURL, false)
}

func (s *Store) ReserveIssueWatchTaskForGeneration(ctx context.Context, issueWatchID string, generation int64, projectPath string, iid int, issueURL string) (bool, error) {
	return s.reserveIssueWatchTask(ctx, issueWatchID, generation, projectPath, iid, issueURL, true)
}

func (s *Store) reserveIssueWatchTask(ctx context.Context, issueWatchID string, generation int64, projectPath string, iid int, issueURL string, requireActive bool) (bool, error) {
	id := uuid.New().String()
	now := time.Now().UTC()
	query := `INSERT INTO gitlab_issue_watch_tasks
		(id, issue_watch_id, project_path, issue_iid, issue_url, task_id, generation, created_at)
		VALUES (?, ?, ?, ?, ?, '', ?, ?)
		ON CONFLICT(issue_watch_id, project_path, issue_iid) DO NOTHING`
	args := []interface{}{id, issueWatchID, projectPath, iid, issueURL, generation, now}
	if requireActive {
		query = `
		INSERT INTO gitlab_issue_watch_tasks (id, issue_watch_id, project_path, issue_iid, issue_url, task_id, generation, created_at)
		SELECT ?, ?, ?, ?, ?, '', ?, ?
		WHERE EXISTS (SELECT 1 FROM gitlab_issue_watches
			WHERE id = ? AND generation = ? AND enabled = 1 AND deleting = 0)
		ON CONFLICT(issue_watch_id, project_path, issue_iid) DO NOTHING`
		args = append(args, issueWatchID, generation)
	}
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

// AssignIssueWatchTaskID swaps the placeholder for the real task_id.
func (s *Store) AssignIssueWatchTaskID(ctx context.Context, issueWatchID, projectPath string, iid int, taskID string) error {
	generation, err := s.issueWatchGeneration(ctx, issueWatchID)
	if err != nil {
		return err
	}
	return s.assignIssueWatchTaskID(ctx, issueWatchID, generation, projectPath, iid, taskID, false)
}

func (s *Store) AssignIssueWatchTaskIDForGeneration(ctx context.Context, issueWatchID string, generation int64, projectPath string, iid int, taskID string) error {
	return s.assignIssueWatchTaskID(ctx, issueWatchID, generation, projectPath, iid, taskID, true)
}

func (s *Store) assignIssueWatchTaskID(ctx context.Context, issueWatchID string, generation int64, projectPath string, iid int, taskID string, requireActive bool) error {
	activeClause := ""
	if requireActive {
		activeClause = ` AND EXISTS (SELECT 1 FROM gitlab_issue_watches
			WHERE id = ? AND generation = ? AND enabled = 1 AND deleting = 0)`
	}
	args := []interface{}{taskID, issueWatchID, projectPath, iid, generation}
	if requireActive {
		args = append(args, issueWatchID, generation)
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE gitlab_issue_watch_tasks SET task_id = ?
		WHERE issue_watch_id = ? AND project_path = ? AND issue_iid = ? AND generation = ?`+activeClause,
		args...)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		if requireActive {
			return ErrWatchOwnershipLost
		}
		return fmt.Errorf("issue watch task not found: watch=%s project=%s iid=%d", issueWatchID, projectPath, iid)
	}
	return nil
}

// ReleaseIssueWatchTask removes a failed reservation.
func (s *Store) ReleaseIssueWatchTask(ctx context.Context, issueWatchID, projectPath string, iid int) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM gitlab_issue_watch_tasks
		 WHERE issue_watch_id = ? AND project_path = ? AND issue_iid = ?`,
		issueWatchID, projectPath, iid)
	return err
}

func (s *Store) ReleaseIssueWatchTaskForGeneration(ctx context.Context, issueWatchID string, generation int64, projectPath string, iid int) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM gitlab_issue_watch_tasks
		 WHERE issue_watch_id = ? AND project_path = ? AND issue_iid = ? AND generation = ?`,
		issueWatchID, projectPath, iid, generation)
	return err
}

func (s *Store) issueWatchGeneration(ctx context.Context, watchID string) (int64, error) {
	var generation int64
	if err := s.ro.GetContext(ctx, &generation,
		`SELECT generation FROM gitlab_issue_watches WHERE id = ?`, watchID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 1, nil
		}
		return 0, err
	}
	return generation, nil
}

// ListIssueWatchTasksByWatch lists dedup rows for an issue watch.
func (s *Store) ListIssueWatchTasksByWatch(ctx context.Context, watchID string) ([]*IssueWatchTask, error) {
	var rows []IssueWatchTask
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT `+issueWatchTaskSelectCols+` FROM gitlab_issue_watch_tasks
		 WHERE issue_watch_id = ? ORDER BY created_at ASC`, watchID); err != nil {
		return nil, err
	}
	out := make([]*IssueWatchTask, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

// ListAllIssueWatchTasks returns every dedup row.
func (s *Store) ListAllIssueWatchTasks(ctx context.Context) ([]*IssueWatchTask, error) {
	var rows []IssueWatchTask
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT `+issueWatchTaskSelectCols+` FROM gitlab_issue_watch_tasks ORDER BY created_at ASC`); err != nil {
		return nil, err
	}
	out := make([]*IssueWatchTask, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

func (s *Store) ListIssueWatchTasksForWorkspace(ctx context.Context, workspaceID string) ([]*IssueWatchTask, error) {
	var rows []IssueWatchTask
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT t.id, t.issue_watch_id, t.project_path, t.issue_iid,
			t.issue_url, t.task_id, t.generation, t.created_at
		 FROM gitlab_issue_watch_tasks t
		 JOIN gitlab_issue_watches w ON w.id = t.issue_watch_id
		 WHERE w.workspace_id = ? ORDER BY t.created_at ASC`, workspaceID); err != nil {
		return nil, err
	}
	out := make([]*IssueWatchTask, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

// DeleteIssueWatchTask removes a dedup row by id.
func (s *Store) DeleteIssueWatchTask(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM gitlab_issue_watch_tasks WHERE id = ?`, id)
	return err
}

// --- Action Presets ---

// GetActionPresets returns the workspace's stored presets, or empty struct
// (caller substitutes defaults) when none are stored.
func (s *Store) GetActionPresets(ctx context.Context, workspaceID string) (*ActionPresets, error) {
	var row struct {
		WorkspaceID  string    `db:"workspace_id"`
		MRPresets    string    `db:"mr_presets"`
		IssuePresets string    `db:"issue_presets"`
		UpdatedAt    time.Time `db:"updated_at"`
	}
	err := s.ro.GetContext(ctx, &row,
		`SELECT workspace_id, mr_presets, issue_presets, updated_at
		 FROM gitlab_action_presets WHERE workspace_id = ?`, workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return &ActionPresets{WorkspaceID: workspaceID}, nil
	}
	if err != nil {
		return nil, err
	}
	out := &ActionPresets{WorkspaceID: row.WorkspaceID}
	if err := json.Unmarshal([]byte(row.MRPresets), &out.MR); err != nil {
		return nil, fmt.Errorf("decode mr_presets: %w", err)
	}
	if err := json.Unmarshal([]byte(row.IssuePresets), &out.Issue); err != nil {
		return nil, fmt.Errorf("decode issue_presets: %w", err)
	}
	return out, nil
}

// UpsertActionPresets writes (or replaces) a workspace's preset row.
func (s *Store) UpsertActionPresets(ctx context.Context, presets *ActionPresets) error {
	mrJSON, err := json.Marshal(presets.MR)
	if err != nil {
		return fmt.Errorf("encode mr_presets: %w", err)
	}
	issueJSON, err := json.Marshal(presets.Issue)
	if err != nil {
		return fmt.Errorf("encode issue_presets: %w", err)
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO gitlab_action_presets (workspace_id, mr_presets, issue_presets, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			mr_presets = excluded.mr_presets,
			issue_presets = excluded.issue_presets,
			updated_at = excluded.updated_at`,
		presets.WorkspaceID, string(mrJSON), string(issueJSON), now)
	return err
}

// DeleteActionPresets resets a workspace to the default presets.
func (s *Store) DeleteActionPresets(ctx context.Context, workspaceID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM gitlab_action_presets WHERE workspace_id = ?`, workspaceID)
	return err
}

// --- JSON helpers ---

func encodeProjects(projects []ProjectFilter) (string, error) {
	if len(projects) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(projects)
	if err != nil {
		return "", fmt.Errorf("encode projects: %w", err)
	}
	return string(b), nil
}

func decodeProjectsInto(rw *ReviewWatch) error {
	if strings.TrimSpace(rw.ProjectsJSON) == "" {
		rw.Projects = nil
		return nil
	}
	if err := json.Unmarshal([]byte(rw.ProjectsJSON), &rw.Projects); err != nil {
		return fmt.Errorf("decode review watch projects: %w", err)
	}
	return nil
}

func decodeIssueWatch(iw *IssueWatch) error {
	if strings.TrimSpace(iw.ProjectsJSON) != "" {
		if err := json.Unmarshal([]byte(iw.ProjectsJSON), &iw.Projects); err != nil {
			return fmt.Errorf("decode issue watch projects: %w", err)
		}
	}
	if strings.TrimSpace(iw.LabelsJSON) != "" {
		if err := json.Unmarshal([]byte(iw.LabelsJSON), &iw.Labels); err != nil {
			return fmt.Errorf("decode issue watch labels: %w", err)
		}
	}
	return nil
}

func encodeStringList(items []string) (string, error) {
	if len(items) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "", fmt.Errorf("encode string list: %w", err)
	}
	return string(b), nil
}
