package sqlite

import (
	"context"
	"errors"
	"path/filepath"
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
