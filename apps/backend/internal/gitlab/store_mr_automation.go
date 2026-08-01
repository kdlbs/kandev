package gitlab

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const createMRAutomationTablesSQL = `
	CREATE TABLE IF NOT EXISTS gitlab_task_mr_options (
		task_id TEXT PRIMARY KEY,
		prompt_on_review_requested BOOLEAN NOT NULL DEFAULT 0,
		prompt_on_merged BOOLEAN NOT NULL DEFAULT 0,
		prompt_on_closed BOOLEAN NOT NULL DEFAULT 0,
		review_reviewer_username TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS gitlab_task_mr_state (
		task_id TEXT NOT NULL,
		repository_id TEXT NOT NULL DEFAULT '',
		project_path TEXT NOT NULL,
		mr_iid INTEGER NOT NULL,
		review_request_initialized BOOLEAN NOT NULL DEFAULT 0,
		last_review_requested BOOLEAN NOT NULL DEFAULT 0,
		last_observed_state TEXT NOT NULL DEFAULT '',
		last_lifecycle_event TEXT NOT NULL DEFAULT '',
		last_lifecycle_prompt_at DATETIME,
		last_lifecycle_session_id TEXT,
		last_error TEXT,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		PRIMARY KEY (task_id, repository_id, project_path, mr_iid)
	);
`

// createMRAutomationTables is called from Store.createTables. Both tables are
// new (no pre-existing rows anywhere), so per apps/backend/AGENTS.md's
// migration rule they are defined directly in CREATE TABLE IF NOT EXISTS
// rather than through an ALTER TABLE migration step.
func (s *Store) createMRAutomationTables() error {
	_, err := s.db.Exec(createMRAutomationTablesSQL)
	return err
}

// GetTaskMRAutomationOptions returns a task's MR automation options, or an
// implicit all-false default when no row has been persisted yet (AC1).
func (s *Store) GetTaskMRAutomationOptions(ctx context.Context, taskID string) (*TaskMRAutomationOptions, error) {
	var row TaskMRAutomationOptions
	err := s.ro.GetContext(ctx, &row, `
		SELECT task_id, prompt_on_review_requested, prompt_on_merged, prompt_on_closed,
			review_reviewer_username, created_at, updated_at
		FROM gitlab_task_mr_options WHERE task_id = ?`, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return &TaskMRAutomationOptions{TaskID: taskID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// UpdateTaskMRAutomationOptions applies a partial update, upserting the
// options row. reviewerUsername, when non-nil, replaces the stored
// review_reviewer_username (nil leaves it untouched); the caller resolves
// this server-side (AC5) — it is never taken from the patch directly.
func (s *Store) UpdateTaskMRAutomationOptions(
	ctx context.Context, taskID string, patch TaskMRAutomationPatch, reviewerUsername *string,
) (*TaskMRAutomationOptions, error) {
	current, err := s.GetTaskMRAutomationOptions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if patch.PromptOnReviewRequested != nil {
		current.PromptOnReviewRequested = *patch.PromptOnReviewRequested
	}
	if patch.PromptOnMerged != nil {
		current.PromptOnMerged = *patch.PromptOnMerged
	}
	if patch.PromptOnClosed != nil {
		current.PromptOnClosed = *patch.PromptOnClosed
	}
	if reviewerUsername != nil {
		current.ReviewReviewerUsername = *reviewerUsername
	}
	now := time.Now().UTC()
	current.TaskID = taskID
	current.UpdatedAt = now
	if current.CreatedAt.IsZero() {
		current.CreatedAt = now
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO gitlab_task_mr_options (
			task_id, prompt_on_review_requested, prompt_on_merged, prompt_on_closed,
			review_reviewer_username, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			prompt_on_review_requested = excluded.prompt_on_review_requested,
			prompt_on_merged = excluded.prompt_on_merged,
			prompt_on_closed = excluded.prompt_on_closed,
			review_reviewer_username = excluded.review_reviewer_username,
			updated_at = excluded.updated_at`,
		current.TaskID, current.PromptOnReviewRequested, current.PromptOnMerged, current.PromptOnClosed,
		current.ReviewReviewerUsername, current.CreatedAt, current.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return current, nil
}

const mrLifecycleStateSelectCols = `task_id, repository_id, project_path, mr_iid,
	review_request_initialized, last_review_requested, last_observed_state,
	last_lifecycle_event, last_lifecycle_prompt_at, last_lifecycle_session_id,
	last_error, created_at, updated_at`

// GetTaskMRLifecycleState returns the checkpoint row for one linked MR, or
// nil when no evaluation has run yet (silent-baseline case, AC10).
func (s *Store) GetTaskMRLifecycleState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int) (*TaskMRLifecycleState, error) {
	var row TaskMRLifecycleState
	err := s.ro.GetContext(ctx, &row, `
		SELECT `+mrLifecycleStateSelectCols+` FROM gitlab_task_mr_state
		WHERE task_id = ? AND repository_id = ? AND project_path = ? AND mr_iid = ?`,
		taskID, repositoryID, projectPath, mrIID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ListTaskMRLifecycleStates returns every checkpoint row for a task.
func (s *Store) ListTaskMRLifecycleStates(ctx context.Context, taskID string) ([]*TaskMRLifecycleState, error) {
	var rows []TaskMRLifecycleState
	if err := s.ro.SelectContext(ctx, &rows,
		`SELECT `+mrLifecycleStateSelectCols+` FROM gitlab_task_mr_state
		 WHERE task_id = ? ORDER BY project_path ASC, mr_iid ASC`, taskID); err != nil {
		return nil, err
	}
	out := make([]*TaskMRLifecycleState, 0, len(rows))
	for i := range rows {
		out = append(out, &rows[i])
	}
	return out, nil
}

func (s *Store) upsertMRLifecycleState(ctx context.Context, row *TaskMRLifecycleState) error {
	now := time.Now().UTC()
	row.UpdatedAt = now
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gitlab_task_mr_state (
			task_id, repository_id, project_path, mr_iid,
			review_request_initialized, last_review_requested, last_observed_state,
			last_lifecycle_event, last_lifecycle_prompt_at, last_lifecycle_session_id,
			last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, repository_id, project_path, mr_iid) DO UPDATE SET
			review_request_initialized = excluded.review_request_initialized,
			last_review_requested = excluded.last_review_requested,
			last_observed_state = excluded.last_observed_state,
			last_lifecycle_event = excluded.last_lifecycle_event,
			last_lifecycle_prompt_at = excluded.last_lifecycle_prompt_at,
			last_lifecycle_session_id = excluded.last_lifecycle_session_id,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at`,
		row.TaskID, row.RepositoryID, row.ProjectPath, row.MRIID,
		row.ReviewRequestInitialized, row.LastReviewRequested, row.LastObservedState,
		row.LastLifecycleEvent, row.LastLifecyclePromptAt, row.LastLifecycleSessionID,
		row.LastError, row.CreatedAt, row.UpdatedAt)
	return err
}

func (s *Store) loadOrDefaultMRLifecycleState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int) (*TaskMRLifecycleState, error) {
	row, err := s.GetTaskMRLifecycleState(ctx, taskID, repositoryID, projectPath, mrIID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = &TaskMRLifecycleState{TaskID: taskID, RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: mrIID}
	}
	return row, nil
}

// SetTaskMRReviewRequestState stamps the review-request baseline/observation
// used to detect a false→true edge (AC10-AC12).
func (s *Store) SetTaskMRReviewRequestState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, requested bool) error {
	row, err := s.loadOrDefaultMRLifecycleState(ctx, taskID, repositoryID, projectPath, mrIID)
	if err != nil {
		return err
	}
	row.ReviewRequestInitialized = true
	row.LastReviewRequested = requested
	return s.upsertMRLifecycleState(ctx, row)
}

// SetTaskMRObservedState stamps the last-observed MR state, used to detect a
// terminal-state transition (AC13-AC15).
func (s *Store) SetTaskMRObservedState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, state string) error {
	row, err := s.loadOrDefaultMRLifecycleState(ctx, taskID, repositoryID, projectPath, mrIID)
	if err != nil {
		return err
	}
	row.LastObservedState = state
	return s.upsertMRLifecycleState(ctx, row)
}

// RecordTaskMRLifecyclePrompt persists an accepted lifecycle prompt
// checkpoint, clearing any prior error for this MR.
func (s *Store) RecordTaskMRLifecyclePrompt(ctx context.Context, prompt TaskMRLifecyclePrompt) error {
	row, err := s.loadOrDefaultMRLifecycleState(ctx, prompt.TaskID, prompt.RepositoryID, prompt.ProjectPath, prompt.MRIID)
	if err != nil {
		return err
	}
	if prompt.ObservedState != "" {
		row.LastObservedState = prompt.ObservedState
	}
	if prompt.Event == mrLifecycleEventReviewRequested {
		row.ReviewRequestInitialized = true
		row.LastReviewRequested = prompt.ReviewRequested
	}
	row.LastLifecycleEvent = prompt.Event
	promptedAt := prompt.PromptedAt
	row.LastLifecyclePromptAt = &promptedAt
	if prompt.SessionID != "" {
		sessionID := prompt.SessionID
		row.LastLifecycleSessionID = &sessionID
	}
	row.LastError = nil
	return s.upsertMRLifecycleState(ctx, row)
}

// RecordTaskMRAutomationError persists a lifecycle evaluation error against
// the per-MR checkpoint row without aborting the caller's poll loop (AC25).
func (s *Store) RecordTaskMRAutomationError(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, message string) error {
	row, err := s.loadOrDefaultMRLifecycleState(ctx, taskID, repositoryID, projectPath, mrIID)
	if err != nil {
		return err
	}
	row.LastError = &message
	return s.upsertMRLifecycleState(ctx, row)
}

// ClearTaskMRAutomationError clears a previously recorded lifecycle error.
func (s *Store) ClearTaskMRAutomationError(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int) error {
	row, err := s.GetTaskMRLifecycleState(ctx, taskID, repositoryID, projectPath, mrIID)
	if err != nil || row == nil || row.LastError == nil {
		return err
	}
	row.LastError = nil
	return s.upsertMRLifecycleState(ctx, row)
}

// RebindTaskMRReviewer replaces the stored reviewer username and, when it
// changed, resets every review-request baseline for the task so a stale
// baseline recorded against the old identity cannot suppress or misfire a
// prompt evaluated against the new one (risk 3 in the plan).
func (s *Store) RebindTaskMRReviewer(ctx context.Context, taskID, username string) (bool, error) {
	current, err := s.GetTaskMRAutomationOptions(ctx, taskID)
	if err != nil {
		return false, err
	}
	if current.ReviewReviewerUsername == username {
		return false, nil
	}
	if _, err := s.UpdateTaskMRAutomationOptions(ctx, taskID, TaskMRAutomationPatch{}, &username); err != nil {
		return false, err
	}
	if err := s.resetReviewBaselinesForTask(ctx, taskID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) resetReviewBaselinesForTask(ctx context.Context, taskID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE gitlab_task_mr_state
		SET review_request_initialized = 0, last_review_requested = 0, updated_at = ?
		WHERE task_id = ?`, time.Now().UTC(), taskID)
	return err
}

// ListLifecycleSubscribedTaskMRs returns every linked MR (gitlab_task_mrs
// row) whose task has at least one lifecycle switch enabled. Drives the
// poller's lifecycle sync pass (AC22).
func (s *Store) ListLifecycleSubscribedTaskMRs(ctx context.Context) ([]*TaskMR, error) {
	var mrs []TaskMR
	if err := s.ro.SelectContext(ctx, &mrs, `
		SELECT `+taskMRSelectColsQualified+` FROM gitlab_task_mrs gtm
		INNER JOIN gitlab_task_mr_options o ON o.task_id = gtm.task_id
		WHERE o.prompt_on_review_requested = 1 OR o.prompt_on_merged = 1 OR o.prompt_on_closed = 1
		ORDER BY gtm.created_at ASC`); err != nil {
		return nil, err
	}
	out := make([]*TaskMR, 0, len(mrs))
	for i := range mrs {
		out = append(out, &mrs[i])
	}
	return out, nil
}

// deleteMRAutomationForWorkspace removes MR automation rows scoped to a
// workspace's tasks, ahead of task deletion in the shared E2E reset (see
// apps/backend/AGENTS.md's E2E reset invariant).
func (s *Store) deleteMRAutomationForWorkspace(ctx context.Context, tx execContext, workspaceID string) error {
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM gitlab_task_mr_state WHERE task_id IN
			(SELECT id FROM tasks WHERE workspace_id = ?)`, workspaceID); err != nil {
		return fmt.Errorf("delete gitlab_task_mr_state: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM gitlab_task_mr_options WHERE task_id IN
			(SELECT id FROM tasks WHERE workspace_id = ?)`, workspaceID); err != nil {
		return fmt.Errorf("delete gitlab_task_mr_options: %w", err)
	}
	return nil
}

// execContext is satisfied by *sqlx.Tx (and *sqlx.DB), narrowed to the one
// method deleteMRAutomationForWorkspace needs so it can run inside the
// existing ResetWorkspaceE2E transaction.
type execContext interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}
