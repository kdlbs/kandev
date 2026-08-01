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

// execContext is satisfied by *sqlx.Tx (and *sqlx.DB), narrowed to the one
// method deleteMRAutomationForWorkspace needs so it can run inside the
// existing ResetWorkspaceE2E transaction. Also used to parametrize the MR
// automation mutators below over either the writer pool or a transaction.
type execContext interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// queryExecer adds a single-row read to execContext. *sqlx.DB and *sqlx.Tx
// both satisfy it, letting the read half of a read-modify-write mutator run
// against either the ad-hoc reader pool or the same transaction as its write
// half (the latter is what makes the mutators below race-free).
type queryExecer interface {
	execContext
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
}

// getTaskMRAutomationOptions returns a task's MR automation options, or an
// implicit all-false default when no row has been persisted yet (AC1).
func getTaskMRAutomationOptions(ctx context.Context, q queryExecer, taskID string) (*TaskMRAutomationOptions, error) {
	var row TaskMRAutomationOptions
	err := q.GetContext(ctx, &row, `
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

// GetTaskMRAutomationOptions is the read-only entry point (AC1).
func (s *Store) GetTaskMRAutomationOptions(ctx context.Context, taskID string) (*TaskMRAutomationOptions, error) {
	return getTaskMRAutomationOptions(ctx, s.ro, taskID)
}

// applyTaskMRAutomationPatch computes the post-patch option row in memory.
// reviewerUsername, when non-nil, replaces the stored review_reviewer_username
// (nil leaves it untouched); the caller resolves this server-side (AC5) — it
// is never taken from the patch directly.
func applyTaskMRAutomationPatch(
	current *TaskMRAutomationOptions, taskID string, patch TaskMRAutomationPatch, reviewerUsername *string,
) *TaskMRAutomationOptions {
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
	return current
}

func writeTaskMRAutomationOptions(ctx context.Context, exec execContext, opts *TaskMRAutomationOptions) error {
	_, err := exec.ExecContext(ctx, `
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
		opts.TaskID, opts.PromptOnReviewRequested, opts.PromptOnMerged, opts.PromptOnClosed,
		opts.ReviewReviewerUsername, opts.CreatedAt, opts.UpdatedAt)
	return err
}

// UpdateTaskMRAutomationOptions applies a partial update atomically. The read
// of the pre-patch row, the option write, and (when prompt_on_review_requested
// changes value) the review-request baseline reset all run in one
// transaction: a stale baseline from before the switch flipped can otherwise
// suppress or misfire the next prompt (functional-correctness review
// finding), and a bare two-statement version could partially apply under a
// mid-write failure (data-integrity review finding).
func (s *Store) UpdateTaskMRAutomationOptions(
	ctx context.Context, taskID string, patch TaskMRAutomationPatch, reviewerUsername *string,
) (*TaskMRAutomationOptions, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	before, err := getTaskMRAutomationOptions(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}
	// Reset on either a boolean flip or a reviewer-identity change: a patch
	// that resends prompt_on_review_requested=true while it was already true
	// still re-resolves the authenticated username (resolveReviewerUsernameForPatch),
	// which can differ from the stored one after the workspace's connected
	// GitLab account changes. Without this second condition, a baseline
	// recorded against the old identity would survive and could suppress or
	// misfire the next prompt evaluated against the new one.
	reviewChanged := (patch.PromptOnReviewRequested != nil &&
		before.PromptOnReviewRequested != *patch.PromptOnReviewRequested) ||
		(reviewerUsername != nil && before.ReviewReviewerUsername != *reviewerUsername)

	updated := applyTaskMRAutomationPatch(before, taskID, patch, reviewerUsername)
	if err := writeTaskMRAutomationOptions(ctx, tx, updated); err != nil {
		return nil, err
	}
	if reviewChanged {
		if err := resetReviewBaselinesForTask(ctx, tx, taskID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return updated, nil
}

const mrLifecycleStateSelectCols = `task_id, repository_id, project_path, mr_iid,
	review_request_initialized, last_review_requested, last_observed_state,
	last_lifecycle_event, last_lifecycle_prompt_at, last_lifecycle_session_id,
	last_error, created_at, updated_at`

func getTaskMRLifecycleState(
	ctx context.Context, q queryExecer, taskID, repositoryID, projectPath string, mrIID int,
) (*TaskMRLifecycleState, error) {
	var row TaskMRLifecycleState
	err := q.GetContext(ctx, &row, `
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

// GetTaskMRLifecycleState returns the checkpoint row for one linked MR, or
// nil when no evaluation has run yet (silent-baseline case, AC10).
func (s *Store) GetTaskMRLifecycleState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int) (*TaskMRLifecycleState, error) {
	return getTaskMRLifecycleState(ctx, s.ro, taskID, repositoryID, projectPath, mrIID)
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

func upsertMRLifecycleState(ctx context.Context, exec execContext, row *TaskMRLifecycleState) error {
	now := time.Now().UTC()
	row.UpdatedAt = now
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	_, err := exec.ExecContext(ctx, `
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

func loadOrDefaultMRLifecycleState(
	ctx context.Context, q queryExecer, taskID, repositoryID, projectPath string, mrIID int,
) (*TaskMRLifecycleState, error) {
	row, err := getTaskMRLifecycleState(ctx, q, taskID, repositoryID, projectPath, mrIID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		row = &TaskMRLifecycleState{TaskID: taskID, RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: mrIID}
	}
	return row, nil
}

// mutateMRLifecycleState reads the checkpoint row and writes mutate's result
// back inside one transaction, so two independent mutators racing on the same
// (task_id, repository_id, project_path, mr_iid) row (for example the
// poller's RecordTaskMRAutomationError and an orchestrator evaluation's
// SetTaskMRReviewRequestState) cannot lose one side's field change to a
// stale full-row overwrite (data-integrity review finding).
func (s *Store) mutateMRLifecycleState(
	ctx context.Context, taskID, repositoryID, projectPath string, mrIID int,
	mutate func(*TaskMRLifecycleState),
) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	row, err := loadOrDefaultMRLifecycleState(ctx, tx, taskID, repositoryID, projectPath, mrIID)
	if err != nil {
		return err
	}
	mutate(row)
	if err := upsertMRLifecycleState(ctx, tx, row); err != nil {
		return err
	}
	return tx.Commit()
}

// SetTaskMRReviewRequestState stamps the review-request baseline/observation
// used to detect a false→true edge (AC10-AC12).
func (s *Store) SetTaskMRReviewRequestState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, requested bool) error {
	return s.mutateMRLifecycleState(ctx, taskID, repositoryID, projectPath, mrIID, func(row *TaskMRLifecycleState) {
		row.ReviewRequestInitialized = true
		row.LastReviewRequested = requested
	})
}

// SetTaskMRObservedState stamps the last-observed MR state, used to detect a
// terminal-state transition (AC13-AC15).
func (s *Store) SetTaskMRObservedState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, state string) error {
	return s.mutateMRLifecycleState(ctx, taskID, repositoryID, projectPath, mrIID, func(row *TaskMRLifecycleState) {
		row.LastObservedState = state
	})
}

// RecordTaskMRLifecyclePrompt persists an accepted lifecycle prompt
// checkpoint, clearing any prior error for this MR.
func (s *Store) RecordTaskMRLifecyclePrompt(ctx context.Context, prompt TaskMRLifecyclePrompt) error {
	return s.mutateMRLifecycleState(
		ctx, prompt.TaskID, prompt.RepositoryID, prompt.ProjectPath, prompt.MRIID,
		func(row *TaskMRLifecycleState) {
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
		})
}

// RecordTaskMRAutomationError persists a lifecycle evaluation error against
// the per-MR checkpoint row without aborting the caller's poll loop (AC25).
func (s *Store) RecordTaskMRAutomationError(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, message string) error {
	return s.mutateMRLifecycleState(ctx, taskID, repositoryID, projectPath, mrIID, func(row *TaskMRLifecycleState) {
		row.LastError = &message
	})
}

// ClearTaskMRAutomationError clears a previously recorded lifecycle error.
// The read-then-skip-if-already-nil check runs inside the same transaction as
// the write, for the same race reason as mutateMRLifecycleState.
func (s *Store) ClearTaskMRAutomationError(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	row, err := getTaskMRLifecycleState(ctx, tx, taskID, repositoryID, projectPath, mrIID)
	if err != nil || row == nil || row.LastError == nil {
		return err
	}
	row.LastError = nil
	if err := upsertMRLifecycleState(ctx, tx, row); err != nil {
		return err
	}
	return tx.Commit()
}

// RebindTaskMRReviewer replaces the stored reviewer username and, when it
// changed, atomically clears every review-request baseline for the task
// (risk 3 in the plan) — in the same transaction as the username write, so a
// failure between the two writes cannot leave the new identity paired with
// the old identity's baseline.
func (s *Store) RebindTaskMRReviewer(ctx context.Context, taskID, username string) (bool, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := getTaskMRAutomationOptions(ctx, tx, taskID)
	if err != nil {
		return false, err
	}
	if current.ReviewReviewerUsername == username {
		return false, nil
	}
	updated := applyTaskMRAutomationPatch(current, taskID, TaskMRAutomationPatch{}, &username)
	if err := writeTaskMRAutomationOptions(ctx, tx, updated); err != nil {
		return false, err
	}
	if err := resetReviewBaselinesForTask(ctx, tx, taskID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func resetReviewBaselinesForTask(ctx context.Context, exec execContext, taskID string) error {
	_, err := exec.ExecContext(ctx, `
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
