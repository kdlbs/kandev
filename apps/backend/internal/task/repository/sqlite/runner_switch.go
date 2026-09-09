package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// SwitchTaskRunner implements repository.TaskRepository.SwitchTaskRunner.
// See the interface doc comment for the contract; this method is the single
// transaction system-design's "Control flow" steps 5-7 describe.
func (r *Repository) SwitchTaskRunner(ctx context.Context, req models.RunnerSwitchRequest) (*models.RunnerSwitchResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", repoerrors.ErrRunnerEvaluationUnavailable, err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := kandevdb.LockTaskRowInTx(ctx, tx, r.db.DriverName(), req.TaskID); err != nil {
		if errors.Is(err, kandevdb.ErrTaskRowNotFound) {
			return nil, fmt.Errorf("%w: %s", ErrTaskNotFound, req.TaskID)
		}
		return nil, fmt.Errorf("%w: %v", repoerrors.ErrRunnerEvaluationUnavailable, err)
	}

	task, err := r.runnerSwitchReadTaskTx(ctx, tx, req.TaskID)
	if err != nil {
		return nil, err
	}

	verdict, repoSnapshot, err := r.runnerSwitchEvaluate(ctx, req, task)
	if err != nil {
		return nil, err
	}
	if !verdict.Editable {
		return nil, &repoerrors.ErrRunnerMutabilityConflict{Reason: verdict.Reason}
	}

	switch {
	case req.CompatibilityResolutionFailed:
		// The gate applies to this target, but a lookup pre-transaction
		// resolution needed (a repository read, or the clone-URL candidate
		// lookup) errored or timed out. Reported only here, after the
		// mutability gate above has already passed, so a task that is also
		// ineligible for a stable reason reports that reason instead of this
		// retriable one.
		return nil, fmt.Errorf("%w: compatibility resolution failed", repoerrors.ErrRunnerEvaluationUnavailable)
	case req.CompatibilityChecked:
		if err := runnerSwitchConfirmCompatibility(req, repoSnapshot); err != nil {
			return nil, err
		}
	case req.CompatibilityApplicable:
		// The gate applies to this target but pre-transaction resolution
		// skipped it because the repository shape didn't allow evaluation
		// then. Reaching this point already means the mutability gate found
		// exactly one repository now (its own RepositoryCount==0/>1 checks
		// come first and would have rejected otherwise), so the shape did
		// change since resolution and that stale skip cannot be trusted.
		return nil, fmt.Errorf("%w: repository shape changed since compatibility resolution", repoerrors.ErrRunnerEvaluationUnavailable)
	}

	result, err := r.runnerSwitchApply(ctx, tx, task, req.ExecutorProfileID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("%w: %v", repoerrors.ErrRunnerEvaluationUnavailable, err)
	}
	return result, nil
}

func (r *Repository) runnerSwitchReadTaskTx(ctx context.Context, tx *sqlx.Tx, taskID string) (*models.Task, error) {
	row := tx.QueryRowContext(ctx, r.db.Rebind(
		`SELECT `+taskSelectColumns("t")+` FROM tasks t WHERE t.id = ?`), taskID)
	task, err := r.scanSingleTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
		}
		return nil, fmt.Errorf("%w: %v", repoerrors.ErrRunnerEvaluationUnavailable, err)
	}
	return task, nil
}

// runnerRepositoryLinkSnapshot is the single attached repository's identity
// and last-updated column, read inside the switch transaction. Empty when
// the task does not have exactly one repository at that moment.
type runnerRepositoryLinkSnapshot struct {
	count        int
	repositoryID string
	updatedAt    time.Time
}

// runnerSwitchEvaluate gathers every mutability signal inside the open
// transaction (after the row lock, so every read is as current as the write
// it guards) and evaluates the mutability gate.
func (r *Repository) runnerSwitchEvaluate(
	ctx context.Context, req models.RunnerSwitchRequest, task *models.Task,
) (models.RunnerMutabilityVerdict, runnerRepositoryLinkSnapshot, error) {
	unavailable := func(err error) (models.RunnerMutabilityVerdict, runnerRepositoryLinkSnapshot, error) {
		return models.RunnerMutabilityVerdict{}, runnerRepositoryLinkSnapshot{},
			fmt.Errorf("%w: %v", repoerrors.ErrRunnerEvaluationUnavailable, err)
	}

	repoSnapshot, err := r.runnerRepositoryLinkSnapshotTx(ctx, task.ID)
	if err != nil {
		return unavailable(err)
	}
	hasSession, err := r.runnerHasSession(ctx, task.ID)
	if err != nil {
		return unavailable(err)
	}
	hasEnvironment, err := r.runnerHasEnvironment(ctx, task.ID)
	if err != nil {
		return unavailable(err)
	}
	hasExecutorRunning, err := r.runnerHasExecutorRunning(ctx, task.ID)
	if err != nil {
		return unavailable(err)
	}
	hasWorkspaceFolder, err := r.runnerHasWorkspaceFolder(ctx, task.ID)
	if err != nil {
		return unavailable(err)
	}
	hasGroupMembership, err := runnerHasGroupMembership(ctx, req, task.ID)
	if err != nil {
		return unavailable(err)
	}

	signals := models.RunnerSignalsFromTask(task)
	signals.RepositoryCount = repoSnapshot.count
	signals.HasSession = hasSession
	signals.HasEnvironment = hasEnvironment
	signals.HasExecutorRunning = hasExecutorRunning
	signals.HasWorkspaceFolder = hasWorkspaceFolder
	signals.HasActiveGroupMembership = hasGroupMembership

	return models.EvaluateRunnerMutability(signals), repoSnapshot, nil
}

func runnerHasGroupMembership(ctx context.Context, req models.RunnerSwitchRequest, taskID string) (bool, error) {
	if req.GroupMembershipChecker == nil {
		return false, nil
	}
	return req.GroupMembershipChecker(ctx, taskID)
}

// runnerSwitchConfirmCompatibility enforces that a compatibility verdict
// resolved before the transaction is only valid for the repository it was
// resolved against. A mismatch discards the verdict and rejects as the
// retriable evaluation_unavailable rather than applying a verdict computed
// against a repository the task no longer has.
func runnerSwitchConfirmCompatibility(req models.RunnerSwitchRequest, snapshot runnerRepositoryLinkSnapshot) error {
	if snapshot.count != 1 ||
		snapshot.repositoryID != req.ResolvedRepositoryID ||
		!snapshot.updatedAt.Equal(req.ResolvedRepositoryUpdatedAt) {
		return fmt.Errorf("%w: repository link changed since compatibility resolution", repoerrors.ErrRunnerEvaluationUnavailable)
	}
	if !req.CompatibilityCloneURLFound {
		return repoerrors.ErrRunnerCompatibilityConflict
	}
	return nil
}

// runnerSwitchApply is stage 7 of the control flow: compare the requested
// profile against the stored one and either report a no-op success or write
// the sole metadata change.
func (r *Repository) runnerSwitchApply(
	ctx context.Context, tx *sqlx.Tx, task *models.Task, executorProfileID string,
) (*models.RunnerSwitchResult, error) {
	stored, _ := task.Metadata[models.MetaKeyExecutorProfileID].(string)
	if stored == executorProfileID {
		return &models.RunnerSwitchResult{Task: task, Changed: false}, nil
	}
	now := time.Now().UTC()
	if err := r.setTaskMetadataKeyWithExecutor(ctx, tx, task.ID, models.MetaKeyExecutorProfileID, executorProfileID, now); err != nil {
		return nil, fmt.Errorf("%w: %v", repoerrors.ErrRunnerEvaluationUnavailable, err)
	}
	if task.Metadata == nil {
		task.Metadata = map[string]interface{}{}
	}
	task.Metadata[models.MetaKeyExecutorProfileID] = executorProfileID
	task.UpdatedAt = now
	return &models.RunnerSwitchResult{Task: task, Changed: true}, nil
}

// setTaskMetadataKeyWithExecutor is SetTaskMetadataKey's tx-capable sibling,
// following the same shape as removeTaskMetadataKeyWithExecutor: every
// existing single-key metadata writer executes on the shared handle and
// none accepts a transaction, but the switch's write must happen inside the
// serialized transaction or it would land even when the transaction rolls
// back, defeating the guarantee that a rejected switch persists nothing.
// updatedAt is supplied by the caller, rather than sampled here, so the
// value written to the row and the value the caller carries forward (into
// the returned task and the published event) are the same instant rather
// than two independent clock reads either side of the write.
func (r *Repository) setTaskMetadataKeyWithExecutor(
	ctx context.Context, exec taskSessionExecutor, taskID, key string, value interface{}, updatedAt time.Time,
) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var query string
	if dialect.IsPostgres(r.db.DriverName()) {
		query = `UPDATE tasks SET metadata = jsonb_set(CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}'::jsonb ELSE metadata::jsonb END, ARRAY[?]::text[], ?::jsonb, true)::text, updated_at = ? WHERE id = ?`
	} else {
		query = `UPDATE tasks SET metadata = json_set(CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}' ELSE metadata END, ?, json(?)), updated_at = ? WHERE id = ?`
	}
	path := key
	if !dialect.IsPostgres(r.db.DriverName()) {
		path = jsonPath(key)
	}
	_, err = exec.ExecContext(ctx, r.db.Rebind(query), path, string(payload), updatedAt, taskID)
	return err
}

func (r *Repository) runnerRepositoryLinkSnapshotTx(ctx context.Context, taskID string) (runnerRepositoryLinkSnapshot, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(
		`SELECT repository_id, updated_at FROM task_repositories WHERE task_id = ?`), taskID)
	if err != nil {
		return runnerRepositoryLinkSnapshot{}, err
	}
	defer func() { _ = rows.Close() }()

	var snapshot runnerRepositoryLinkSnapshot
	for rows.Next() {
		var repositoryID string
		var updatedAt time.Time
		if err := rows.Scan(&repositoryID, &updatedAt); err != nil {
			return runnerRepositoryLinkSnapshot{}, err
		}
		snapshot.count++
		snapshot.repositoryID = repositoryID
		snapshot.updatedAt = updatedAt
	}
	return snapshot, rows.Err()
}

func (r *Repository) runnerHasSession(ctx context.Context, taskID string) (bool, error) {
	var exists bool
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT EXISTS (SELECT 1 FROM task_sessions WHERE task_id = ?)`), taskID).Scan(&exists)
	return exists, err
}

func (r *Repository) runnerHasEnvironment(ctx context.Context, taskID string) (bool, error) {
	var exists bool
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT EXISTS (SELECT 1 FROM task_environments WHERE task_id = ?)`), taskID).Scan(&exists)
	return exists, err
}

func (r *Repository) runnerHasExecutorRunning(ctx context.Context, taskID string) (bool, error) {
	var exists bool
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT EXISTS (SELECT 1 FROM executors_running WHERE task_id = ?)`), taskID).Scan(&exists)
	return exists, err
}

func (r *Repository) runnerHasWorkspaceFolder(ctx context.Context, taskID string) (bool, error) {
	var exists bool
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT EXISTS (SELECT 1 FROM task_workspace_folders WHERE task_id = ?)`), taskID).Scan(&exists)
	return exists, err
}
