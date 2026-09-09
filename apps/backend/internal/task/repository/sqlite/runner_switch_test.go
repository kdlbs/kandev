package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// newRunnerSwitchTestRepo opens separate writer and reader connections
// against the same file, mirroring production's dual-pool setup
// (internal/db.Pool). SwitchTaskRunner reads other tables via r.ro while
// holding an open r.db transaction; a single shared *sqlx.DB for both (as
// some older test helpers use) self-deadlocks under the writer pool's
// MaxOpenConns(1), since the held transaction already owns the only
// connection the read would need.
func newRunnerSwitchTestRepo(t *testing.T) *Repository {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runner-switch.db")
	writerConn, err := dbutil.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open sqlite writer: %v", err)
	}
	writer := sqlx.NewDb(writerConn, "sqlite3")
	t.Cleanup(func() { _ = writer.Close() })

	readerConn, err := dbutil.OpenSQLiteReader(path)
	if err != nil {
		t.Fatalf("open sqlite reader: %v", err)
	}
	reader := sqlx.NewDb(readerConn, "sqlite3")
	t.Cleanup(func() { _ = reader.Close() })

	repo, err := NewWithDB(writer, reader, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo
}

func seedRunnerSwitchWorkspace(t *testing.T, repo *Repository, workspaceID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`),
		workspaceID, workspaceID, now, now); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
}

type seedRunnerSwitchTaskOpts struct {
	ParentID string
	Metadata string
	Archived bool
}

func seedRunnerSwitchTask(t *testing.T, repo *Repository, taskID, workspaceID string, opts seedRunnerSwitchTaskOpts) {
	t.Helper()
	now := time.Now().UTC()
	metadata := opts.Metadata
	if metadata == "" {
		metadata = "{}"
	}
	var archivedAt interface{}
	if opts.Archived {
		archivedAt = now
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, parent_id, title, metadata, archived_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		taskID, workspaceID, opts.ParentID, "runner switch task", metadata, archivedAt, now, now); err != nil {
		t.Fatalf("seed task: %v", err)
	}
}

func seedRunnerSwitchRepository(t *testing.T, repo *Repository, repositoryID, workspaceID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO repositories (id, workspace_id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`),
		repositoryID, workspaceID, repositoryID, now, now); err != nil {
		t.Fatalf("seed repository: %v", err)
	}
}

func seedRunnerSwitchTaskRepository(t *testing.T, repo *Repository, taskID, repositoryID string) *models.TaskRepository {
	t.Helper()
	taskRepo := &models.TaskRepository{
		ID:           "task-repo-" + taskID + "-" + repositoryID,
		TaskID:       taskID,
		RepositoryID: repositoryID,
		BaseBranch:   "main",
	}
	if err := repo.CreateTaskRepository(context.Background(), taskRepo); err != nil {
		t.Fatalf("seed task repository: %v", err)
	}
	stored, err := repo.GetTaskRepository(context.Background(), taskRepo.ID)
	if err != nil {
		t.Fatalf("reload task repository: %v", err)
	}
	return stored
}

// baseRunnerSwitchRequest builds an eligible-shaped request for taskID
// switching to targetProfileID, with the compatibility gate reporting a
// found clone URL against repoSnapshot — the shape most tests start from
// before overriding the field(s) under test.
func baseRunnerSwitchRequest(taskID, targetProfileID string, repoSnapshot *models.TaskRepository) models.RunnerSwitchRequest {
	req := models.RunnerSwitchRequest{
		TaskID:            taskID,
		ExecutorProfileID: targetProfileID,
	}
	if repoSnapshot != nil {
		req.CompatibilityChecked = true
		req.CompatibilityCloneURLFound = true
		req.ResolvedRepositoryID = repoSnapshot.RepositoryID
		req.ResolvedRepositoryUpdatedAt = repoSnapshot.UpdatedAt
	}
	return req
}

func TestSwitchTaskRunner_EligibleTopLevelSingleRepoWritesNewProfile(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{
		Metadata: `{"executor_profile_id":"profile-old"}`,
	})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	result, err := repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	if err != nil {
		t.Fatalf("SwitchTaskRunner error = %v, want nil", err)
	}
	if !result.Changed {
		t.Fatalf("result.Changed = false, want true")
	}
	if got := result.Task.Metadata[models.MetaKeyExecutorProfileID]; got != "profile-new" {
		t.Fatalf("result.Task metadata executor_profile_id = %v, want profile-new", got)
	}

	reloaded, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got := reloaded.Metadata[models.MetaKeyExecutorProfileID]; got != "profile-new" {
		t.Fatalf("persisted executor_profile_id = %v, want profile-new", got)
	}
}

// TestSwitchTaskRunner_ResultUpdatedAtMatchesPersistedValue pins
// AC-TASKS-RUNNER-SWITCH-002.19: the event and the response the caller
// receives must carry the task's updated-at value from the committing
// transaction, not an independently-sampled approximation of it.
func TestSwitchTaskRunner_ResultUpdatedAtMatchesPersistedValue(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{
		Metadata: `{"executor_profile_id":"profile-old"}`,
	})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	result, err := repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	if err != nil {
		t.Fatalf("SwitchTaskRunner error = %v, want nil", err)
	}

	reloaded, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if !result.Task.UpdatedAt.Equal(reloaded.UpdatedAt) {
		t.Fatalf("result.Task.UpdatedAt = %v, want the persisted value %v", result.Task.UpdatedAt, reloaded.UpdatedAt)
	}
}

// TestSwitchTaskRunner_UnrelatedMetadataSurvivesMerge pins AC-002.4: the
// switch writes executor_profile_id through a single-key JSON merge, so
// every other key already stored on the task's metadata column must come
// back unchanged, not be dropped by a whole-object overwrite.
func TestSwitchTaskRunner_UnrelatedMetadataSurvivesMerge(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{
		Metadata: `{"executor_profile_id":"profile-old","custom_key":"custom_value","nested":{"a":1}}`,
	})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	result, err := repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	if err != nil {
		t.Fatalf("SwitchTaskRunner error = %v, want nil", err)
	}
	if got := result.Task.Metadata["custom_key"]; got != "custom_value" {
		t.Fatalf("result.Task metadata custom_key = %v, want custom_value", got)
	}
	nested, ok := result.Task.Metadata["nested"].(map[string]interface{})
	if !ok || nested["a"] != float64(1) {
		t.Fatalf("result.Task metadata nested = %v, want map with a=1", result.Task.Metadata["nested"])
	}

	reloaded, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got := reloaded.Metadata["custom_key"]; got != "custom_value" {
		t.Fatalf("persisted custom_key = %v, want custom_value", got)
	}
	if got := reloaded.Metadata[models.MetaKeyExecutorProfileID]; got != "profile-new" {
		t.Fatalf("persisted executor_profile_id = %v, want profile-new", got)
	}
}

func TestSwitchTaskRunner_NoOpWhenAlreadyStoredProfile(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{
		Metadata: `{"executor_profile_id":"profile-same"}`,
	})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	before, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}

	result, err := repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-same", taskRepo))
	if err != nil {
		t.Fatalf("SwitchTaskRunner error = %v, want nil", err)
	}
	if result.Changed {
		t.Fatalf("result.Changed = true, want false for a no-op switch")
	}

	after, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("task updated_at changed on a no-op switch: before=%v after=%v", before.UpdatedAt, after.UpdatedAt)
	}
}

func TestSwitchTaskRunner_RejectsWhenArchived(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{Archived: true})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	_, err := repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	assertRunnerMutabilityConflict(t, err, models.RunnerReasonTaskArchived)
}

func TestSwitchTaskRunner_RejectsWhenNoRepository(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{})

	_, err := repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", nil))
	assertRunnerMutabilityConflict(t, err, models.RunnerReasonNoRepository)
}

func TestSwitchTaskRunner_RejectsWhenMultipleRepositories(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	seedRunnerSwitchRepository(t, repo, "repo-2", "ws-1")
	seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")
	seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-2")

	_, err := repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", nil))
	assertRunnerMutabilityConflict(t, err, models.RunnerReasonMultipleRepositories)
}

func TestSwitchTaskRunner_RejectsWhenSessionExists(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	_, err := repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	assertRunnerMutabilityConflict(t, err, models.RunnerReasonSessionExists)
}

func TestSwitchTaskRunner_RejectsWhenEnvironmentExists(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", ExecutorType: "worktree",
		WorkspacePath: "/tmp/task-1", Status: models.TaskEnvironmentStatusCreating,
	}); err != nil {
		t.Fatalf("seed environment: %v", err)
	}

	_, err := repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	assertRunnerMutabilityConflict(t, err, models.RunnerReasonEnvironmentExists)
}

func TestSwitchTaskRunner_RejectsWhenExecutorRunningExists(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		SessionID: "session-1", TaskID: "task-1", ExecutorID: "executor-1", Status: "starting",
	}); err != nil {
		t.Fatalf("seed executor running: %v", err)
	}

	_, err := repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	assertRunnerMutabilityConflict(t, err, models.RunnerReasonExecutorRunning)
}

func TestSwitchTaskRunner_RejectsWhenWorkspaceFolderAttached(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	if err := repo.CreateWorkspaceSourceBatch(ctx, &models.WorkspaceSourceBatch{
		TaskID: "task-1",
		Sources: []models.WorkspaceSource{
			{Folder: &models.TaskWorkspaceFolder{LocalPath: "/tmp/folder", DisplayName: "folder"}},
		},
	}); err != nil {
		t.Fatalf("seed workspace folder: %v", err)
	}

	_, err := repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	assertRunnerMutabilityConflict(t, err, models.RunnerReasonWorkspaceFolderAttached)
}

func TestSwitchTaskRunner_RejectsWhenWorkspacePathSet(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{
		Metadata: `{"workspace_path":"/tmp/materialized"}`,
	})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	_, err := repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	assertRunnerMutabilityConflict(t, err, models.RunnerReasonWorkspacePathSet)
}

func TestSwitchTaskRunner_RejectsWhenGroupMember(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	req := baseRunnerSwitchRequest("task-1", "profile-new", taskRepo)
	req.GroupMembershipChecker = func(ctx context.Context, taskID string) (bool, error) {
		return true, nil
	}

	_, err := repo.SwitchTaskRunner(ctx, req)
	assertRunnerMutabilityConflict(t, err, models.RunnerReasonWorkspaceGroupMember)
}

func TestSwitchTaskRunner_RejectsWhenSubtaskWithoutNewWorkspace(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "parent-1", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{ParentID: "parent-1"})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	_, err := repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	assertRunnerMutabilityConflict(t, err, models.RunnerReasonWorkspaceBindingNotIndependent)
}

func TestSwitchTaskRunner_AllowsSubtaskWithExplicitNewWorkspace(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "parent-1", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{
		ParentID: "parent-1",
		Metadata: `{"workspace":{"mode":"new_workspace"}}`,
	})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	result, err := repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	if err != nil {
		t.Fatalf("SwitchTaskRunner error = %v, want nil", err)
	}
	if !result.Changed {
		t.Fatalf("result.Changed = false, want true")
	}
}

func TestSwitchTaskRunner_NotFoundForMissingTask(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()

	_, err := repo.SwitchTaskRunner(ctx, models.RunnerSwitchRequest{
		TaskID: "does-not-exist", ExecutorProfileID: "profile-new",
	})
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("SwitchTaskRunner error = %v, want ErrTaskNotFound", err)
	}
}

func TestSwitchTaskRunner_CompatibilityConflictWhenCloneURLNotFound(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	req := baseRunnerSwitchRequest("task-1", "profile-new", taskRepo)
	req.CompatibilityCloneURLFound = false

	_, err := repo.SwitchTaskRunner(ctx, req)
	if !errors.Is(err, repoerrors.ErrRunnerCompatibilityConflict) {
		t.Fatalf("SwitchTaskRunner error = %v, want ErrRunnerCompatibilityConflict", err)
	}
}

func TestSwitchTaskRunner_EvaluationUnavailableWhenRepositoryLinkChangedSinceResolution(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	req := baseRunnerSwitchRequest("task-1", "profile-new", taskRepo)
	// Simulate the pre-transaction compatibility resolution having run
	// against a repository link snapshot that is no longer current.
	req.ResolvedRepositoryUpdatedAt = taskRepo.UpdatedAt.Add(-time.Hour)

	_, err := repo.SwitchTaskRunner(ctx, req)
	if !errors.Is(err, repoerrors.ErrRunnerEvaluationUnavailable) {
		t.Fatalf("SwitchTaskRunner error = %v, want ErrRunnerEvaluationUnavailable", err)
	}
}

// TestSwitchTaskRunner_EvaluationUnavailableWhenCompatibilityBecameApplicableSinceResolution
// covers the shape-changed-since-resolution race: the pre-transaction
// resolution skipped the compatibility check because the task did not have
// exactly one repository attachment at that moment, but the target executor
// does require a clone URL and the task now has exactly one repository
// attached (the only shape the mutability gate ever lets this codepath
// reach). The skip cannot be trusted retroactively, so the switch must
// reject as retriable rather than silently apply without ever validating
// compatibility against the now-attached repository.
func TestSwitchTaskRunner_EvaluationUnavailableWhenCompatibilityBecameApplicableSinceResolution(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	req := models.RunnerSwitchRequest{
		TaskID:                  "task-1",
		ExecutorProfileID:       "profile-new",
		CompatibilityApplicable: true,
		CompatibilityChecked:    false,
	}

	_, err := repo.SwitchTaskRunner(ctx, req)
	if !errors.Is(err, repoerrors.ErrRunnerEvaluationUnavailable) {
		t.Fatalf("SwitchTaskRunner error = %v, want ErrRunnerEvaluationUnavailable", err)
	}
}

// TestSwitchTaskRunner_SkipsCompatibilityRecheckWhenGateInapplicable is the
// control for the test above: when the target executor never requires a
// clone URL, CompatibilityApplicable is false and the switch must proceed
// normally even though the task has exactly one repository attached.
func TestSwitchTaskRunner_SkipsCompatibilityRecheckWhenGateInapplicable(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	req := models.RunnerSwitchRequest{
		TaskID:                  "task-1",
		ExecutorProfileID:       "profile-new",
		CompatibilityApplicable: false,
		CompatibilityChecked:    false,
	}

	result, err := repo.SwitchTaskRunner(ctx, req)
	if err != nil {
		t.Fatalf("SwitchTaskRunner = %v, want success", err)
	}
	if !result.Changed {
		t.Fatal("SwitchTaskRunner Changed = false, want true")
	}
}

// TestSwitchTaskRunner_EvaluationUnavailableWhenCompatibilityResolutionFailed
// covers a compatibility-gate resolution that errored or timed out
// pre-transaction (a git subprocess failure, for example) rather than being
// skipped for a shape reason: with the mutability gate otherwise passing,
// the switch must still reject as retriable.
func TestSwitchTaskRunner_EvaluationUnavailableWhenCompatibilityResolutionFailed(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	req := models.RunnerSwitchRequest{
		TaskID:                        "task-1",
		ExecutorProfileID:             "profile-new",
		CompatibilityApplicable:       true,
		CompatibilityResolutionFailed: true,
	}

	_, err := repo.SwitchTaskRunner(ctx, req)
	if !errors.Is(err, repoerrors.ErrRunnerEvaluationUnavailable) {
		t.Fatalf("SwitchTaskRunner error = %v, want ErrRunnerEvaluationUnavailable", err)
	}
}

// TestSwitchTaskRunner_MutabilityConflictOutranksCompatibilityResolutionFailure
// pins AC-TASKS-RUNNER-SWITCH-002.17's outcome precedence: when a task is
// both mutability-ineligible (session exists) and its compatibility-gate
// resolution failed, the stable mutability reason must be reported, not the
// retriable evaluation_unavailable one. Reversing this order would tell a
// caller with a genuinely stable conflict to just retry.
func TestSwitchTaskRunner_MutabilityConflictOutranksCompatibilityResolutionFailure(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCreated,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	req := models.RunnerSwitchRequest{
		TaskID:                        "task-1",
		ExecutorProfileID:             "profile-new",
		CompatibilityApplicable:       true,
		CompatibilityResolutionFailed: true,
	}

	_, err := repo.SwitchTaskRunner(ctx, req)
	assertRunnerMutabilityConflict(t, err, models.RunnerReasonSessionExists)
}

func assertRunnerMutabilityConflict(t *testing.T, err error, wantReason string) {
	t.Helper()
	var conflict *repoerrors.ErrRunnerMutabilityConflict
	if !errors.As(err, &conflict) {
		t.Fatalf("SwitchTaskRunner error = %v, want *ErrRunnerMutabilityConflict", err)
	}
	if conflict.Reason != wantReason {
		t.Fatalf("conflict reason = %q, want %q", conflict.Reason, wantReason)
	}
}

// TestSwitchTaskRunner_ConcurrentExecutorRunningWriteNeverBlendsWithSwitch is
// the AC-TASKS-RUNNER-SWITCH-002.3a/3b acceptance evidence for the
// UpsertExecutorRunning retrofit: a runner switch racing an executors_running
// write must land as one of exactly two outcomes — the switch commits first
// and the running row is then created under the new profile, or the running
// row lands first and the switch is rejected as executor_running — never a
// switch that both commits and coexists with a running row it never saw.
func TestSwitchTaskRunner_ConcurrentExecutorRunningWriteNeverBlendsWithSwitch(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{
		Metadata: `{"executor_profile_id":"profile-old"}`,
	})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	var wg sync.WaitGroup
	var switchErr, runningErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, switchErr = repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	}()
	go func() {
		defer wg.Done()
		runningErr = repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			SessionID: "session-race", TaskID: "task-1", ExecutorID: "executor-1", Status: "starting",
		})
	}()
	wg.Wait()

	if runningErr != nil {
		t.Fatalf("UpsertExecutorRunning error = %v, want nil", runningErr)
	}

	switchRejectedAsExecutorRunning := false
	if switchErr != nil {
		var conflict *repoerrors.ErrRunnerMutabilityConflict
		if !errors.As(switchErr, &conflict) || conflict.Reason != models.RunnerReasonExecutorRunning {
			t.Fatalf("switch error = %v, want nil or ErrRunnerMutabilityConflict{executor_running}", switchErr)
		}
		switchRejectedAsExecutorRunning = true
	}

	task, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	stored, _ := task.Metadata[models.MetaKeyExecutorProfileID].(string)

	if switchRejectedAsExecutorRunning {
		if stored != "profile-old" {
			t.Fatalf("switch was rejected but stored profile = %q, want unchanged profile-old", stored)
		}
		return
	}
	if stored != "profile-new" {
		t.Fatalf("switch committed but stored profile = %q, want profile-new", stored)
	}
}

// TestSwitchTaskRunner_ConcurrentWorkspaceFolderAttachNeverBlendsWithSwitch is
// the AC-TASKS-RUNNER-SWITCH-002.3a/3b acceptance evidence for the
// guardWorkspaceSourceParentTx retrofit's no-parent (top-level task) branch:
// a runner switch racing a workspace-folder attachment on the same top-level
// task must land as one of exactly two outcomes, never a switch that commits
// with a folder attached it never saw.
func TestSwitchTaskRunner_ConcurrentWorkspaceFolderAttachNeverBlendsWithSwitch(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{
		Metadata: `{"executor_profile_id":"profile-old"}`,
	})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	var wg sync.WaitGroup
	var switchErr, attachErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, switchErr = repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	}()
	go func() {
		defer wg.Done()
		attachErr = repo.CreateWorkspaceSourceBatch(ctx, &models.WorkspaceSourceBatch{
			TaskID: "task-1",
			Sources: []models.WorkspaceSource{
				{Folder: &models.TaskWorkspaceFolder{LocalPath: "/tmp/race-folder", DisplayName: "race-folder"}},
			},
		})
	}()
	wg.Wait()

	if attachErr != nil {
		t.Fatalf("CreateWorkspaceSourceBatch error = %v, want nil", attachErr)
	}

	switchRejectedAsWorkspaceFolderAttached := false
	if switchErr != nil {
		var conflict *repoerrors.ErrRunnerMutabilityConflict
		if !errors.As(switchErr, &conflict) || conflict.Reason != models.RunnerReasonWorkspaceFolderAttached {
			t.Fatalf("switch error = %v, want nil or ErrRunnerMutabilityConflict{workspace_folder_attached}", switchErr)
		}
		switchRejectedAsWorkspaceFolderAttached = true
	}

	task, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	stored, _ := task.Metadata[models.MetaKeyExecutorProfileID].(string)

	if switchRejectedAsWorkspaceFolderAttached {
		if stored != "profile-old" {
			t.Fatalf("switch was rejected but stored profile = %q, want unchanged profile-old", stored)
		}
		return
	}
	if stored != "profile-new" {
		t.Fatalf("switch committed but stored profile = %q, want profile-new", stored)
	}
}

// TestSwitchTaskRunner_ConcurrentTaskRepositoryReparentNeverBlendsWithSwitch
// is the AC-TASKS-RUNNER-SWITCH-002.3a/3b acceptance evidence for the
// UpdateTaskRepository retrofit: re-parenting the task's sole repository
// link away to another task races a runner switch on the vacated task. The
// two outcomes are the switch committing first (against the still-attached
// repository) or the re-parent landing first and the switch being rejected
// as no_repository — never a switch that commits after its task lost its
// only repository.
func TestSwitchTaskRunner_ConcurrentTaskRepositoryReparentNeverBlendsWithSwitch(t *testing.T) {
	repo := newRunnerSwitchTestRepo(t)
	ctx := context.Background()
	seedRunnerSwitchWorkspace(t, repo, "ws-1")
	seedRunnerSwitchTask(t, repo, "task-1", "ws-1", seedRunnerSwitchTaskOpts{
		Metadata: `{"executor_profile_id":"profile-old"}`,
	})
	seedRunnerSwitchTask(t, repo, "task-2", "ws-1", seedRunnerSwitchTaskOpts{})
	seedRunnerSwitchRepository(t, repo, "repo-1", "ws-1")
	taskRepo := seedRunnerSwitchTaskRepository(t, repo, "task-1", "repo-1")

	reparented := *taskRepo
	reparented.TaskID = "task-2"

	var wg sync.WaitGroup
	var switchErr, reparentErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, switchErr = repo.SwitchTaskRunner(ctx, baseRunnerSwitchRequest("task-1", "profile-new", taskRepo))
	}()
	go func() {
		defer wg.Done()
		reparentErr = repo.UpdateTaskRepository(ctx, &reparented)
	}()
	wg.Wait()

	if reparentErr != nil {
		t.Fatalf("UpdateTaskRepository error = %v, want nil", reparentErr)
	}

	switchRejectedAsNoRepository := false
	if switchErr != nil {
		var conflict *repoerrors.ErrRunnerMutabilityConflict
		if !errors.As(switchErr, &conflict) || conflict.Reason != models.RunnerReasonNoRepository {
			t.Fatalf("switch error = %v, want nil or ErrRunnerMutabilityConflict{no_repository}", switchErr)
		}
		switchRejectedAsNoRepository = true
	}

	task, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	stored, _ := task.Metadata[models.MetaKeyExecutorProfileID].(string)

	if switchRejectedAsNoRepository {
		if stored != "profile-old" {
			t.Fatalf("switch was rejected but stored profile = %q, want unchanged profile-old", stored)
		}
		return
	}
	if stored != "profile-new" {
		t.Fatalf("switch committed but stored profile = %q, want profile-new", stored)
	}
}
