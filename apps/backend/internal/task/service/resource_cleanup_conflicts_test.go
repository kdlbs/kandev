package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

type recordingCleanupRepository struct {
	taskrepo.TaskResourceCleanupRepository
	updated   bool
	state     models.TaskResourceCleanupState
	lastError string
	next      *time.Time
	calls     int
}

func (r *recordingCleanupRepository) CompleteClaimedTaskResourceCleanupJob(
	_ context.Context, _ string, _ int, state models.TaskResourceCleanupState, lastError string, nextAttemptAt *time.Time,
) (bool, error) {
	r.calls++
	r.state = state
	r.lastError = lastError
	r.next = nextAttemptAt
	return r.updated, nil
}

func newCleanupConflictTestService(t *testing.T) (*Service, *recordingCleanupRepository, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	repository := &recordingCleanupRepository{updated: true}
	return &Service{resourceCleanups: repository, logger: log}, repository, logs
}

func cleanupConflictFixture(worktreeID, reason, comparison, action string) error {
	return fmt.Errorf("cleanup worktree %s: %w", worktreeID, &worktree.CleanupInspectionError{
		Stage:            worktree.CleanupInspectionStageRegistration,
		Reason:           reason,
		WorktreeID:       worktreeID,
		Comparison:       comparison,
		Action:           action,
		RecordedEvidence: true,
		ObservedEvidence: true,
		Err:              errors.New("raw git output must not be attributed as an identity field"),
	})
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.4
// @covers AC-TASKS-RUNTIME-CLEANUP-001.32
func TestCleanupConflictsExposeEveryAffectedResource(t *testing.T) {
	service, repository, logs := newCleanupConflictTestService(t)
	cleanupErr := errors.Join(
		cleanupConflictFixture("worktree-1", worktree.CleanupInspectionReasonCompetingRegistration,
			worktree.CleanupComparisonRecordedRegistration, worktree.CleanupInspectionActionInspectRegistration),
		cleanupConflictFixture("worktree-2", worktree.CleanupInspectionReasonCommitMismatch,
			worktree.CleanupComparisonRecordedCommit, worktree.CleanupInspectionActionCompareCommit),
		errors.New("transient environment failure"),
	)
	job := &models.TaskResourceCleanupJob{
		ID: "job-1", TaskID: "task-1", Trigger: models.TaskResourceCleanupTriggerCascadeArchive, Attempts: 4,
	}

	require.Error(t, service.retryTaskResourceCleanupJob(context.Background(), job, cleanupErr))

	require.Equal(t, models.TaskResourceCleanupStateRetryWait, repository.state)
	require.NotNil(t, repository.next)
	require.Contains(t, repository.lastError, "worktree-1")
	require.Contains(t, repository.lastError, worktree.CleanupComparisonRecordedRegistration)
	require.Contains(t, repository.lastError, "worktree-2")
	require.Contains(t, repository.lastError, worktree.CleanupComparisonRecordedCommit)

	entries := logs.FilterMessage("task resource cleanup requires reconciliation").All()
	require.Len(t, entries, 2)
	byWorktree := map[string]map[string]interface{}{}
	for _, entry := range entries {
		fields := entry.ContextMap()
		require.Equal(t, cleanupConditionReconciliationRequired, fields["cleanup_condition"])
		require.Equal(t, "task-1", fields["task_id"])
		require.Equal(t, "job-1", fields["job_id"])
		require.Equal(t, int64(4), fields["attempt"])
		require.Equal(t, string(models.TaskResourceCleanupStateRetryWait), fields["state"])
		require.Equal(t, cleanupRetryPolicyUnlimited, fields["retry_policy"])
		require.Nil(t, fields["max_attempts"])
		require.Equal(t, true, fields["attempt_limit_unlimited"])
		require.Equal(t, int64(12*time.Hour/time.Second), fields["retry_delay_cap_seconds"])
		require.NotNil(t, fields["next_attempt_at"])
		byWorktree[fmt.Sprint(fields["worktree_id"])] = fields
	}
	require.Equal(t, worktree.CleanupComparisonRecordedRegistration, byWorktree["worktree-1"]["comparison"])
	require.Equal(t, worktree.CleanupInspectionActionInspectRegistration, byWorktree["worktree-1"]["action"])
	require.Equal(t, worktree.CleanupComparisonRecordedCommit, byWorktree["worktree-2"]["comparison"])
	require.Equal(t, worktree.CleanupInspectionActionCompareCommit, byWorktree["worktree-2"]["action"])
	for _, entry := range entries {
		require.NotContains(t, fmt.Sprint(entry.ContextMap()), "transient environment failure")
	}
}

// @covers AC-TASKS-RUNTIME-CLEANUP-001.9
// @covers AC-TASKS-RUNTIME-CLEANUP-001.32
// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.5
func TestCleanupConflictRetainsCascadeRetryAcrossRestart(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	svc.StopTaskResourceCleanupWorker()
	ctx := context.Background()
	workspace, err := repo.GetWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	created, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1", WorkflowID: workspace.OfficeWorkflowID, Title: "Cleanup conflict restart",
	})
	require.NoError(t, err)
	snapshot := `{"worktrees":[{"id":"worktree-1"}],"archive_source_manifest":[{"worktree_id":"worktree-1"}]}`
	job := &models.TaskResourceCleanupJob{
		ID: "job-conflict-restart", OperationID: "cascade_archive:job-conflict-restart",
		TaskID: created.Task.ID, Trigger: models.TaskResourceCleanupTriggerCascadeArchive,
		State: models.TaskResourceCleanupStateRunning, Attempts: 13, ResourceSnapshot: snapshot,
	}
	require.NoError(t, repo.CreateTaskResourceCleanupJob(ctx, job))
	loaded, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	cleanupErr := cleanupConflictFixture("worktree-1", worktree.CleanupInspectionReasonCommitMismatch,
		worktree.CleanupComparisonRecordedCommit, worktree.CleanupInspectionActionCompareCommit)

	started := time.Now().UTC()
	require.Error(t, svc.retryTaskResourceCleanupJob(ctx, loaded, cleanupErr))
	persisted, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskResourceCleanupStateRetryWait, persisted.State)
	require.Equal(t, 13, persisted.Attempts)
	require.Equal(t, snapshot, persisted.ResourceSnapshot)
	require.NotNil(t, persisted.NextAttemptAt)
	if delay := persisted.NextAttemptAt.Sub(started); delay < 11*time.Hour+59*time.Minute || delay > 12*time.Hour+time.Minute {
		t.Fatalf("retry delay after 13 attempts = %v, want capped 12h", delay)
	}
	require.Contains(t, persisted.LastError, worktree.CleanupComparisonRecordedCommit)
	require.Contains(t, persisted.LastError, worktree.CleanupInspectionActionCompareCommit)

	// Reopen the service over the same SQLite database, as a backend restart
	// does, and resume the next claimed attempt from persisted evidence.
	restarted := newServiceOverRepository(t, repo)
	_, err = repo.DB().ExecContext(ctx, `UPDATE task_resource_cleanup_jobs SET state='running', attempts=14 WHERE id=?`, job.ID)
	require.NoError(t, err)
	resumed, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, 14, resumed.Attempts)
	require.Error(t, restarted.retryTaskResourceCleanupJob(ctx, resumed, cleanupErr))
	afterRestart, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskResourceCleanupStateRetryWait, afterRestart.State)
	require.Equal(t, snapshot, afterRestart.ResourceSnapshot)
	require.Contains(t, afterRestart.LastError, worktree.CleanupComparisonRecordedCommit)
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.4
func TestCleanupConflictDiagnosticsAreBounded(t *testing.T) {
	service, _, logs := newCleanupConflictTestService(t)
	var errs []error
	for index := range cleanupConflictMaxDiagnostics + 8 {
		errs = append(errs, cleanupConflictFixture(
			fmt.Sprintf("worktree-%02d", index),
			worktree.CleanupInspectionReasonCompetingRegistration,
			worktree.CleanupComparisonRecordedRegistration,
			worktree.CleanupInspectionActionInspectRegistration,
		))
	}
	job := &models.TaskResourceCleanupJob{
		ID: "job-overflow", TaskID: "task-overflow", Trigger: models.TaskResourceCleanupTriggerCascadeArchive, Attempts: 2,
	}
	require.Error(t, service.retryTaskResourceCleanupJob(context.Background(), job, errors.Join(errs...)))

	require.Len(t, logs.FilterMessage("task resource cleanup requires reconciliation").All(), cleanupConflictMaxDiagnostics)
	truncated := logs.FilterMessage("task resource cleanup conflict diagnostics truncated").All()
	require.Len(t, truncated, 1)
	require.Equal(t, true, truncated[0].ContextMap()["diagnostics_truncated"])
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.4
func TestCleanupStaleAttemptDoesNotPublishReconciliation(t *testing.T) {
	service, repository, logs := newCleanupConflictTestService(t)
	repository.updated = false
	job := &models.TaskResourceCleanupJob{
		ID: "job-stale", TaskID: "task-stale", Trigger: models.TaskResourceCleanupTriggerCascadeArchive, Attempts: 5,
	}
	cleanupErr := cleanupConflictFixture("worktree-1", worktree.CleanupInspectionReasonCompetingRegistration,
		worktree.CleanupComparisonRecordedRegistration, worktree.CleanupInspectionActionInspectRegistration)

	require.Error(t, service.retryTaskResourceCleanupJob(context.Background(), job, cleanupErr))
	require.Empty(t, logs.FilterMessage("task resource cleanup requires reconciliation").All())
	require.Equal(t, 1, repository.calls)
}

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.4
func TestCleanupConflictDiagnosticsIgnoreNonIdentityFailures(t *testing.T) {
	service, repository, logs := newCleanupConflictTestService(t)
	job := &models.TaskResourceCleanupJob{
		ID: "job-transient", TaskID: "task-transient", Trigger: models.TaskResourceCleanupTriggerArchive, Attempts: 1,
	}
	cleanupErr := errors.Join(
		&worktree.CleanupInspectionError{
			Stage: worktree.CleanupInspectionStageBranch, Reason: worktree.CleanupInspectionReasonDeadlineExceeded,
			WorktreeID: "worktree-1", Err: context.DeadlineExceeded,
		},
		&worktree.CleanupInspectionError{
			Stage: worktree.CleanupInspectionStageRegistration, Reason: worktree.CleanupInspectionReasonUnregisteredPath,
			WorktreeID: "worktree-2",
		},
	)
	require.Error(t, service.retryTaskResourceCleanupJob(context.Background(), job, cleanupErr))
	require.Empty(t, logs.FilterMessage("task resource cleanup requires reconciliation").All())
	require.Equal(t, models.TaskResourceCleanupStateRetryWait, repository.state)
}
