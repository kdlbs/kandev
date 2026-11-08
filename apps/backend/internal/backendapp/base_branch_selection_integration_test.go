package backendapp

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	agentexecutor "github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

var errBaseSelectionRuntimeBoundary = errors.New("base selection runtime boundary")

// baseSelectionExecutorBackend is the external agent/transport fake. It
// counts admission attempts so preparation and rollback remain observable.
type baseSelectionExecutorBackend struct {
	created        int
	workspacePaths []string
}

func (b *baseSelectionExecutorBackend) Name() agentexecutor.Name { return agentexecutor.NameStandalone }
func (b *baseSelectionExecutorBackend) HealthCheck(context.Context) error {
	return nil
}
func (b *baseSelectionExecutorBackend) CreateInstance(_ context.Context, req *lifecycle.ExecutorCreateRequest) (*lifecycle.ExecutorInstance, error) {
	b.created++
	b.workspacePaths = append(b.workspacePaths, req.WorkspacePath)
	return nil, errBaseSelectionRuntimeBoundary
}
func (b *baseSelectionExecutorBackend) StopInstance(context.Context, *lifecycle.ExecutorInstance, bool) error {
	return nil
}
func (b *baseSelectionExecutorBackend) RecoverInstances(context.Context, []*models.ExecutorRunning) ([]*lifecycle.ExecutorInstance, error) {
	return nil, nil
}
func (b *baseSelectionExecutorBackend) GetInteractiveRunner() *process.InteractiveRunner {
	return nil
}
func (b *baseSelectionExecutorBackend) RequiresCloneURL() bool { return false }
func (b *baseSelectionExecutorBackend) ShouldApplyPreferredShell() bool {
	return false
}
func (b *baseSelectionExecutorBackend) IsAlwaysResumable() bool { return true }

type baseSelectionFixture struct {
	harness    bootStateTestHarness
	executor   *executor.Executor
	backend    *baseSelectionExecutorBackend
	manager    *lifecycle.Manager
	store      *worktree.SQLiteStore
	workspace  string
	remotePath string
	cachePath  string
	sourcePath string
	selected   string
	main       string
}

func newBaseSelectionFixture(t *testing.T) *baseSelectionFixture {
	t.Helper()
	ctx := context.Background()
	harness := newBootStateTestHarness(t)
	log := newTestLogger()
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)

	agentRegistry := registry.NewRegistry(log)
	mockAgent := agents.NewMockAgentWithID("mock-agent", "Mock", "Mock")
	mockAgent.SetEnabled(true)
	mockAgent.SetSupportsMCP(false)
	require.NoError(t, agentRegistry.Register(mockAgent))

	backend := &baseSelectionExecutorBackend{}
	executorRegistry := lifecycle.NewExecutorRegistry(log)
	executorRegistry.Register(backend)
	manager := lifecycle.NewManager(
		agentRegistry, eventBus, executorRegistry, nil,
		baseSelectionProfileResolver{}, nil, lifecycle.ExecutorFallbackWarn, t.TempDir(), log,
	)
	manager.SetExecutorProfileReader(harness.taskRepo)
	manager.SetExecutorRunningWriter(harness.taskRepo)
	manager.SetWorkspaceInfoProvider(harness.taskSvc)
	preparers := lifecycle.NewPreparerRegistry(log)
	preparers.Register(models.ExecutorTypeLocal, lifecycle.NewLocalPreparer(log))
	manager.SetPreparerRegistry(preparers)
	store, err := worktree.NewSQLiteStore(harness.db, harness.db)
	require.NoError(t, err)
	tasksBase := t.TempDir()
	worktreeManager, err := worktree.NewManager(worktree.Config{
		Enabled: true, TasksBasePath: tasksBase, BranchPrefix: "kandev/",
	}, store, log)
	require.NoError(t, err)
	manager.SetWorktreeManager(worktreeManager)
	t.Cleanup(func() {
		_ = manager.StopAllAgents(ctx)
		_ = manager.Stop()
	})

	adapter := newLifecycleAdapter(manager, agentRegistry, log)
	orchestratorExecutor := executor.NewExecutor(
		adapter, harness.taskRepo, log, executor.ExecutorConfig{},
	)
	require.NoError(t, harness.taskRepo.CreateExecutor(ctx, &models.Executor{
		ID: "base-selection-worktree", Name: "Worktree", Type: models.ExecutorTypeWorktree,
		Status: models.ExecutorStatusActive,
	}))
	require.NoError(t, harness.taskRepo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: "base-selection-profile", ExecutorID: "base-selection-worktree", Name: "Worktree profile",
	}))

	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, workspaces)

	remotePath, sourcePath := seedBaseSelectionRemote(t)
	cachePath := filepath.Join(t.TempDir(), "cache")
	runSelectedBaseGit(t, filepath.Dir(cachePath), "clone", remotePath, cachePath)
	mainHead := strings.TrimSpace(runSelectedBaseGit(t, cachePath, "rev-parse", "HEAD"))
	runSelectedBaseGit(t, sourcePath, "checkout", "-b", "feature/qualified-base")
	require.NoError(t, os.WriteFile(filepath.Join(sourcePath, "selected.txt"), []byte("selected\n"), 0o644))
	runSelectedBaseGit(t, sourcePath, "add", "selected.txt")
	runSelectedBaseGit(t, sourcePath, "commit", "-m", "selected base")
	selectedHead := strings.TrimSpace(runSelectedBaseGit(t, sourcePath, "rev-parse", "HEAD"))
	require.NotEqual(t, mainHead, selectedHead)
	runSelectedBaseGit(t, sourcePath, "push", "origin", "feature/qualified-base")

	return &baseSelectionFixture{
		harness: harness, executor: orchestratorExecutor, backend: backend, manager: manager, store: store,
		workspace: workspaces[0].ID, remotePath: remotePath, cachePath: cachePath, sourcePath: sourcePath,
		selected: selectedHead, main: mainHead,
	}
}

type baseSelectionProfileResolver struct{}

func (baseSelectionProfileResolver) ResolveProfile(_ context.Context, profileID string) (*lifecycle.AgentProfileInfo, error) {
	return &lifecycle.AgentProfileInfo{
		ProfileID: profileID, ProfileName: "Mock profile", AgentID: "mock-agent", AgentName: "mock-agent",
		NativeSessionResume: true,
	}, nil
}

func (f *baseSelectionFixture) createTask(t *testing.T, attachments ...taskservice.TaskRepositoryInput) *models.Task {
	t.Helper()
	result, err := f.harness.taskSvc.CreateTask(context.Background(), &taskservice.CreateTaskRequest{
		WorkspaceID: f.workspace, WorkflowID: f.workflowID(t), Title: "Base selection", Repositories: attachments,
	})
	require.NoError(t, err)
	return result.Task
}

func (f *baseSelectionFixture) workflowID(t *testing.T) string {
	t.Helper()
	workflows, err := f.harness.taskSvc.ListWorkflows(context.Background(), f.workspace, true)
	require.NoError(t, err)
	require.NotEmpty(t, workflows)
	return workflows[0].ID
}

func (f *baseSelectionFixture) addRepository(t *testing.T, id, path string, name string) {
	t.Helper()
	require.NoError(t, f.harness.taskRepo.CreateRepository(context.Background(), &models.Repository{
		ID: id, WorkspaceID: f.workspace, Name: name, SourceType: "local", LocalPath: path, DefaultBranch: "main",
	}))
}

func (f *baseSelectionFixture) createSession(t *testing.T, taskID, sessionID string) *models.TaskSession {
	t.Helper()
	session := &models.TaskSession{
		ID: sessionID, TaskID: taskID, AgentProfileID: "mock-profile", ExecutionProfileID: "mock-profile",
		ExecutorID: "base-selection-worktree", ExecutorProfileID: "base-selection-profile",
		State: models.TaskSessionStateCreated, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, f.harness.taskRepo.CreateTaskSession(context.Background(), session))
	return session
}

func (f *baseSelectionFixture) launch(t *testing.T, task *models.Task, sessionID string) error {
	t.Helper()
	_, err := f.executor.LaunchPreparedSession(context.Background(), task.ToAPI(), sessionID,
		executor.LaunchOptions{AgentProfileID: "mock-profile", StartAgent: true})
	return err
}

func (f *baseSelectionFixture) worktrees(t *testing.T, taskID string) []*worktree.Worktree {
	t.Helper()
	rows, err := f.store.GetWorktreesByTaskID(context.Background(), taskID)
	require.NoError(t, err)
	return rows
}

func (f *baseSelectionFixture) requireNoMaterialization(t *testing.T, taskID string) {
	t.Helper()
	require.Empty(t, f.worktrees(t, taskID), "strict failure must not persist a worktree")
	environment, err := f.harness.taskRepo.GetTaskEnvironmentByTaskID(context.Background(), taskID)
	require.NoError(t, err)
	require.Nil(t, environment, "strict failure must not publish a ready environment")
	require.Zero(t, f.backend.created, "strict failure must not start an agent runtime")
	if list := runSelectedBaseGit(t, f.cachePath, "worktree", "list", "--porcelain"); strings.Count(list, "worktree ") != 1 {
		t.Fatalf("git worktrees = %q, want only the cache checkout", list)
	}
}

func TestBaseBranchSelection_LaunchRejectsWrongBase(t *testing.T) {
	f := newBaseSelectionFixture(t)
	f.addRepository(t, "repo-explicit", f.cachePath, "explicit")
	task := f.createTask(t, taskservice.TaskRepositoryInput{
		RepositoryID: "repo-explicit", BaseBranch: "feature/qualified-base",
	})
	f.createSession(t, task.ID, "session-explicit")

	err := f.launch(t, task, "session-explicit")
	require.Error(t, err)
	require.Contains(t, err.Error(), "feature/qualified-base")
	require.Contains(t, err.Error(), "explicit", "preparation error must name the repository")
	require.False(t, errors.Is(err, errBaseSelectionRuntimeBoundary), "runtime must not be reached")
	f.requireNoMaterialization(t, task.ID)

	rows, listErr := f.harness.taskRepo.ListTaskRepositories(context.Background(), task.ID)
	require.NoError(t, listErr)
	require.Len(t, rows, 1)
	require.Equal(t, "feature/qualified-base", rows[0].BaseBranch, "failed attempt must preserve the attachment selection")
	selection, ok, loadErr := models.LoadBaseBranchSelection(rows[0].Metadata)
	require.NoError(t, loadErr)
	require.True(t, ok)
	require.Equal(t, models.BaseBranchSelectionSourceExplicit, selection.Source)

	require.Equal(t, f.main, strings.TrimSpace(runSelectedBaseGit(t, f.cachePath, "rev-parse", "main")))
}

func TestBaseBranchSelection_RepairAndRetry(t *testing.T) {
	f := newBaseSelectionFixture(t)
	f.addRepository(t, "repo-retry", f.cachePath, "retry")
	task := f.createTask(t, taskservice.TaskRepositoryInput{
		RepositoryID: "repo-retry", BaseBranch: "feature/qualified-base",
	})
	f.createSession(t, task.ID, "session-retry-first")

	require.Error(t, f.launch(t, task, "session-retry-first"))
	f.requireNoMaterialization(t, task.ID)

	runSelectedBaseGit(t, f.cachePath, "fetch", "origin", "feature/qualified-base:refs/heads/feature/qualified-base")
	f.createSession(t, task.ID, "session-retry-second")

	err := f.launch(t, task, "session-retry-second")
	require.ErrorIs(t, err, errBaseSelectionRuntimeBoundary, "repaired preparation must reach the runtime boundary")
	require.Equal(t, 1, f.backend.created)

	require.Len(t, f.backend.workspacePaths, 1)
	worktreePath := f.backend.workspacePaths[0]
	require.Contains(t, worktreePath, "base-selection", "runtime must receive the prepared worktree path")
	require.Equal(t, f.selected, strings.TrimSpace(runSelectedBaseGit(t, worktreePath, "rev-parse", "HEAD")))
	sentinel, readErr := os.ReadFile(filepath.Join(worktreePath, "selected.txt"))
	require.NoError(t, readErr)
	require.Equal(t, "selected\n", string(sentinel))

	rows, listErr := f.harness.taskRepo.ListTaskRepositories(context.Background(), task.ID)
	require.NoError(t, listErr)
	require.Len(t, rows, 1)
	require.Equal(t, "feature/qualified-base", rows[0].BaseBranch)
}

func TestBaseBranchSelection_MultiAttachmentAdmission(t *testing.T) {
	t.Run("separate repositories", func(t *testing.T) {
		f := newBaseSelectionFixture(t)
		secondRemote, secondSource := seedBaseSelectionRemote(t)
		secondCache := filepath.Join(t.TempDir(), "second-cache")
		runSelectedBaseGit(t, filepath.Dir(secondCache), "clone", secondRemote, secondCache)
		runSelectedBaseGit(t, secondSource, "checkout", "-b", "feature/qualified-base")
		require.NoError(t, os.WriteFile(filepath.Join(secondSource, "selected.txt"), []byte("selected\n"), 0o644))
		runSelectedBaseGit(t, secondSource, "add", "selected.txt")
		runSelectedBaseGit(t, secondSource, "commit", "-m", "selected base")
		runSelectedBaseGit(t, secondSource, "push", "origin", "feature/qualified-base")

		f.addRepository(t, "repo-valid", f.cachePath, "valid")
		f.addRepository(t, "repo-missing", secondCache, "missing")
		task := f.createTask(t,
			taskservice.TaskRepositoryInput{RepositoryID: "repo-valid", BaseBranch: "main"},
			taskservice.TaskRepositoryInput{RepositoryID: "repo-missing", BaseBranch: "feature/qualified-base"},
		)
		f.createSession(t, task.ID, "session-separate")
		err := f.launch(t, task, "session-separate")
		require.Error(t, err)
		require.Contains(t, err.Error(), "feature/qualified-base")
		require.Zero(t, f.backend.created)
		require.Empty(t, f.worktrees(t, task.ID), "rollback must remove the valid attachment worktree")
		if list := runSelectedBaseGit(t, f.cachePath, "worktree", "list", "--porcelain"); strings.Count(list, "worktree ") != 1 {
			t.Fatalf("valid repository kept a rolled-back worktree: %q", list)
		}
	})

	t.Run("one repository twice", func(t *testing.T) {
		f := newBaseSelectionFixture(t)
		f.addRepository(t, "repo-double", f.cachePath, "double")
		task := f.createTask(t,
			taskservice.TaskRepositoryInput{RepositoryID: "repo-double", BaseBranch: "main"},
			taskservice.TaskRepositoryInput{RepositoryID: "repo-double", BaseBranch: "feature/qualified-base"},
		)
		f.createSession(t, task.ID, "session-double")
		err := f.launch(t, task, "session-double")
		require.Error(t, err)
		require.Contains(t, err.Error(), "feature/qualified-base")
		require.Zero(t, f.backend.created)
		require.Empty(t, f.worktrees(t, task.ID), "rollback must remove both same-repository slots")
		if list := runSelectedBaseGit(t, f.cachePath, "worktree", "list", "--porcelain"); strings.Count(list, "worktree ") != 1 {
			t.Fatalf("same-repository failure kept a worktree: %q", list)
		}
	})
}

// TestBaseBranchSelection_MultiAttachmentSameSessionRetry reproduces the
// partial multi-repository failure followed by a repair and a retry on the
// same task and session. The valid attachment's first-attempt worktree is
// rolled back before persistence, so its in-memory cache aliases must not
// survive either.
func TestBaseBranchSelection_MultiAttachmentSameSessionRetry(t *testing.T) {
	f := newBaseSelectionFixture(t)
	secondRemote, secondSource := seedBaseSelectionRemote(t)
	secondCache := filepath.Join(t.TempDir(), "second-cache")
	runSelectedBaseGit(t, filepath.Dir(secondCache), "clone", secondRemote, secondCache)
	runSelectedBaseGit(t, secondSource, "checkout", "-b", "feature/qualified-base")
	require.NoError(t, os.WriteFile(filepath.Join(secondSource, "selected.txt"), []byte("selected\n"), 0o644))
	runSelectedBaseGit(t, secondSource, "add", "selected.txt")
	runSelectedBaseGit(t, secondSource, "commit", "-m", "selected base")
	runSelectedBaseGit(t, secondSource, "push", "origin", "feature/qualified-base")

	f.addRepository(t, "repo-valid", f.cachePath, "valid")
	f.addRepository(t, "repo-missing", secondCache, "missing")
	task := f.createTask(t,
		taskservice.TaskRepositoryInput{RepositoryID: "repo-valid", BaseBranch: "main"},
		taskservice.TaskRepositoryInput{RepositoryID: "repo-missing", BaseBranch: "feature/qualified-base"},
	)
	f.createSession(t, task.ID, "session-multi-retry")

	require.Error(t, f.launch(t, task, "session-multi-retry"))
	require.Zero(t, f.backend.created)
	require.Equal(t, 1, worktreeCount(t, f.cachePath), "valid attachment must be rolled back")
	require.Equal(t, 1, worktreeCount(t, secondCache), "failed attachment created no checkout")

	runSelectedBaseGit(t, secondCache, "fetch", "origin",
		"feature/qualified-base:refs/heads/feature/qualified-base")

	err := f.launch(t, task, "session-multi-retry")
	require.ErrorIs(t, err, errBaseSelectionRuntimeBoundary,
		"same-session retry must admit both attachments after repair: %v", err)
	require.Equal(t, 1, f.backend.created)
	require.Equal(t, 2, worktreeCount(t, f.cachePath), "valid attachment was admitted on retry")
	require.Equal(t, 2, worktreeCount(t, secondCache), "repaired attachment was admitted on retry")
}

func worktreeCount(t *testing.T, repositoryPath string) int {
	t.Helper()
	list := runSelectedBaseGit(t, repositoryPath, "worktree", "list", "--porcelain")
	return strings.Count(list, "worktree ")
}

func seedBaseSelectionRemote(t *testing.T) (remotePath, sourcePath string) {
	t.Helper()
	root := t.TempDir()
	remotePath = filepath.Join(root, "origin.git")
	runSelectedBaseGit(t, root, "init", "--bare", "-b", "main", remotePath)
	sourcePath = filepath.Join(root, "source")
	runSelectedBaseGit(t, root, "clone", remotePath, sourcePath)
	runSelectedBaseGit(t, sourcePath, "config", "user.email", "test@example.com")
	runSelectedBaseGit(t, sourcePath, "config", "user.name", "Test User")
	runSelectedBaseGit(t, sourcePath, "config", "commit.gpgsign", "false")
	require.NoError(t, os.WriteFile(filepath.Join(sourcePath, "main.txt"), []byte("main\n"), 0o644))
	runSelectedBaseGit(t, sourcePath, "add", "main.txt")
	runSelectedBaseGit(t, sourcePath, "commit", "-m", "main")
	runSelectedBaseGit(t, sourcePath, "push", "-u", "origin", "main")
	return remotePath, sourcePath
}

func runSelectedBaseGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2020-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2020-01-01T00:00:00Z")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return string(output)
}
