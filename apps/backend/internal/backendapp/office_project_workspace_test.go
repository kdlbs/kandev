package backendapp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officeservice "github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/worktree"
)

const officeProjectSourceSentinel = "office-project-source-sentinel.txt"

// @covers AC-TASKS-PROJECT-REPOSITORIES-001.1, AC-TASKS-PROJECT-REPOSITORIES-001.2, AC-TASKS-PROJECT-REPOSITORIES-001.5, AC-TASKS-PROJECT-REPOSITORIES-001.8
func TestOfficeProjectFirstLaunchSources(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}

	for _, tc := range []struct {
		name         string
		executorType models.ExecutorType
		executorID   string
	}{
		{name: "local_pc", executorType: models.ExecutorTypeLocal, executorID: "exec-office-project-local"},
		{name: "worktree", executorType: models.ExecutorTypeWorktree, executorID: "exec-office-project-worktree"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			h := newRoutineCronHarness(t)
			projectSource := initOfficeProjectSourceRepo(t)
			encoded, err := officemodels.EncodeRepositories([]string{projectSource})
			if err != nil {
				t.Fatalf("encode project sources: %v", err)
			}
			const projectID = "project-first-launch-sources"
			if err := h.officeRepo.CreateProject(ctx, &officemodels.Project{
				ID: projectID, WorkspaceID: h.workspaceID, Name: "First launch sources",
				Status: officemodels.ProjectStatusActive, Repositories: encoded,
			}); err != nil {
				t.Fatalf("create Office project: %v", err)
			}
			wireOfficeProjectRepositorySources(h.taskSvc, h.officeRepo)

			executorProfileID := registerOfficeProjectExecutor(t, ctx, h, tc.executorID, tc.executorType)
			tasksBase := filepath.Join(t.TempDir(), "tasks")
			_, adapter := newOfficeProjectLifecycleAdapter(t, ctx, h, tasksBase)
			h.agentMgr.launchDelegate = adapter
			handoff := taskservice.NewHandoffService(h.taskRepo, h.taskRepo, nil, h.officeRepo, h.officeRepo, nil)
			handoff.SetSessionReader(h.taskRepo)
			h.taskSvc.SetWorkspacePolicyAttacher(handoff)
			h.orchestrator.SetWorkspaceMaterializer(handoff)

			workflows, err := h.taskSvc.ListWorkflows(ctx, h.workspaceID, true)
			if err != nil || len(workflows) == 0 {
				t.Fatalf("list workspace workflows: %v (count=%d)", err, len(workflows))
			}
			root, err := h.taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
				WorkspaceID: h.workspaceID, WorkflowID: workflows[0].ID,
				Title: "Project source parent", ProjectID: projectID,
			})
			if err != nil {
				t.Fatalf("create project-backed root task: %v", err)
			}
			rootRepo, _ := assertOfficeProjectTaskRepository(t, ctx, h, root.Task.ID, projectSource)
			childID, err := h.taskSvc.CreateChildTask(ctx, root.Task, taskservice.ChildTaskSpec{Title: "Inheriting child"})
			if err != nil {
				t.Fatalf("create inheriting child: %v", err)
			}
			assertChildProjectRepository(t, ctx, h, childID, rootRepo.ID)
			child, err := h.taskRepo.GetTask(ctx, childID)
			if err != nil || child == nil || child.ParentID != root.Task.ID {
				t.Fatalf("load inheriting child: task=%#v err=%v", child, err)
			}
			childWorkspace, ok := child.Metadata["workspace"].(map[string]interface{})
			if !ok || childWorkspace["mode"] != "inherit_parent" {
				t.Fatalf("child workspace policy = %#v, want inherit_parent", child.Metadata["workspace"])
			}

			starter, ok := newOfficeTaskStarter(h.orchestrator).(officeservice.TaskStarterWithLaunchContextSession)
			if !ok {
				t.Fatal("production Office task starter does not return the launched session")
			}
			rootSessionID := startOfficeProjectSession(t, ctx, starter, h.workspaceID, root.Task.ID, tc.executorID, executorProfileID, "root")
			rootLaunch := h.agentMgr.awaitLaunch(t)
			awaitOfficeProjectSessionRunning(t, ctx, h, rootSessionID)
			assertOfficeProjectLaunchRequest(t, rootLaunch, root.Task.ID, rootRepo)
			rootEnvironment := assertOfficeProjectTaskEnvironment(t, ctx, h, root.Task.ID, rootRepo.ID)
			workspaceGroup, err := h.officeRepo.GetWorkspaceGroupForTask(ctx, root.Task.ID)
			if err != nil || workspaceGroup == nil {
				t.Fatalf("load materialized root workspace group: group=%#v err=%v", workspaceGroup, err)
			}
			if groupEnvironmentID := handoff.GetSharedGroupEnvironment(ctx, root.Task.ID); groupEnvironmentID != rootEnvironment.ID {
				t.Fatalf("materialized workspace group environment = %q, want root task environment %q", groupEnvironmentID, rootEnvironment.ID)
			}
			if tc.executorType == models.ExecutorTypeWorktree && workspaceGroup.MaterializedPath != rootEnvironment.WorkspacePath {
				t.Fatalf("materialized Worktree group path = %q, want prepared environment path %q", workspaceGroup.MaterializedPath, rootEnvironment.WorkspacePath)
			}
			assertOfficeProjectSentinel(t, rootEnvironment.WorkspacePath)
			assertOfficeProjectExecutorWorkspace(t, tc.executorType, tasksBase, rootRepo, rootEnvironment)
			if tc.executorType == models.ExecutorTypeWorktree {
				rows, err := h.taskRepo.ListTaskEnvironmentRepos(ctx, rootEnvironment.ID)
				if err != nil {
					t.Fatalf("list root task_environment_repos: %v", err)
				}
				if len(rows) != 1 || rows[0].WorktreeID == "" || rows[0].WorktreePath != rootEnvironment.WorkspacePath {
					t.Fatalf("Worktree task_environment_repos = %#v, want the prepared repository worktree", rows)
				}
			}

			childSessionID := startOfficeProjectSession(t, ctx, starter, h.workspaceID, child.ID, tc.executorID, executorProfileID, "child")
			childLaunch := h.agentMgr.awaitLaunch(t)
			awaitOfficeProjectSessionRunning(t, ctx, h, childSessionID)
			assertOfficeProjectLaunchRequest(t, childLaunch, child.ID, rootRepo)
			childSession, err := h.taskRepo.GetTaskSession(ctx, childSessionID)
			if err != nil || childSession == nil {
				t.Fatalf("load inheriting child session: session=%#v err=%v", childSession, err)
			}
			if childSession.TaskEnvironmentID != rootEnvironment.ID {
				t.Fatalf("child task_environment_id = %q, want inherited parent environment %q", childSession.TaskEnvironmentID, rootEnvironment.ID)
			}
			childEnvironment, err := h.taskRepo.GetTaskEnvironment(ctx, childSession.TaskEnvironmentID)
			if err != nil || childEnvironment == nil {
				t.Fatalf("load inherited child environment: environment=%#v err=%v", childEnvironment, err)
			}
			assertOfficeProjectSentinel(t, childEnvironment.WorkspacePath)
			assertOfficeProjectTaskEnvironmentInventory(t, ctx, h, childEnvironment.ID, rootRepo.ID)
			if rootSessionID == childSessionID {
				t.Fatalf("root and child launches reused session ID %q", rootSessionID)
			}
		})
	}
}

func registerOfficeProjectExecutor(
	t *testing.T,
	ctx context.Context,
	h *routineCronHarness,
	executorID string,
	executorType models.ExecutorType,
) string {
	t.Helper()
	executorProfileID := executorID + "-profile"
	if err := h.taskRepo.CreateExecutor(ctx, &models.Executor{
		ID: executorID, Name: string(executorType), Type: executorType,
		Status: models.ExecutorStatusActive, IsSystem: true,
	}); err != nil {
		t.Fatalf("create %s executor: %v", executorType, err)
	}
	if err := h.taskRepo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: executorProfileID, ExecutorID: executorID, Name: string(executorType) + " profile",
	}); err != nil {
		t.Fatalf("create %s executor profile: %v", executorType, err)
	}
	return executorProfileID
}

func newOfficeProjectLifecycleAdapter(
	t *testing.T,
	ctx context.Context,
	h *routineCronHarness,
	tasksBase string,
) (*lifecycle.Manager, *lifecycleAdapter) {
	t.Helper()
	log := newTestLogger()
	agentRegistry := registry.NewRegistry(log)
	mockAgent := agents.NewMockAgentWithID("mock-agent", "Mock", "Mock")
	mockAgent.SetEnabled(true)
	mockAgent.SetSupportsMCP(false)
	if err := agentRegistry.Register(mockAgent); err != nil {
		t.Fatalf("register mock ACP agent: %v", err)
	}
	server := newManagedGoCacheAgentCtlServer(t)
	backend := &managedGoCacheExecutorBackend{serverURL: server.URL(), log: log}
	executorRegistry := lifecycle.NewExecutorRegistry(log)
	executorRegistry.Register(backend)
	manager := lifecycle.NewManager(
		agentRegistry, h.eventBus, executorRegistry, nil,
		managedGoCacheProfileResolver{}, nil, lifecycle.ExecutorFallbackWarn, t.TempDir(), log,
	)
	manager.SetExecutorProfileReader(h.taskRepo)
	manager.SetExecutorRunningWriter(h.taskRepo)
	manager.SetWorkspaceInfoProvider(h.taskSvc)
	preparers := lifecycle.NewPreparerRegistry(log)
	preparers.Register(models.ExecutorTypeLocal, lifecycle.NewLocalPreparer(log))
	manager.SetPreparerRegistry(preparers)
	store, err := worktree.NewSQLiteStore(h.db, h.db)
	if err != nil {
		t.Fatalf("create persisted Worktree store: %v", err)
	}
	worktreeManager, err := worktree.NewManager(worktree.Config{
		Enabled: true, TasksBasePath: tasksBase, BranchPrefix: "kandev/",
	}, store, log)
	if err != nil {
		t.Fatalf("create Worktree manager: %v", err)
	}
	manager.SetWorktreeManager(worktreeManager)
	t.Cleanup(func() {
		_ = manager.StopAllAgents(ctx)
		_ = manager.Stop()
	})
	return manager, newLifecycleAdapter(manager, agentRegistry, log)
}

func startOfficeProjectSession(
	t *testing.T,
	ctx context.Context,
	starter officeservice.TaskStarterWithLaunchContextSession,
	workspaceID, taskID, executorID, executorProfileID, suffix string,
) string {
	t.Helper()
	sessionID, err := starter.StartTaskWithLaunchContextReturningSession(ctx, taskID, "mock-profile", officeservice.LaunchContext{
		ExecutorID: executorID, ExecutorProfileID: executorProfileID,
		Prompt: "Read the project source file.",
		Env: map[string]string{
			"KANDEV_CLI":          "kandev",
			"KANDEV_API_URL":      "http://localhost:7400/api/v1",
			"KANDEV_API_KEY":      "test-api-key",
			"KANDEV_AGENT_ID":     "office-agent-test",
			"KANDEV_WORKSPACE_ID": workspaceID,
			"KANDEV_RUN_ID":       "office-project-" + suffix,
			"KANDEV_TASK_ID":      taskID,
		},
	})
	if err != nil {
		t.Fatalf("launch Office task %q through production starter: %v", taskID, err)
	}
	if sessionID == "" {
		t.Fatalf("Office task %q launch returned no session ID", taskID)
	}
	return sessionID
}

func awaitOfficeProjectSessionRunning(t *testing.T, ctx context.Context, h *routineCronHarness, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		session, err := h.taskRepo.GetTaskSession(ctx, sessionID)
		if err != nil {
			t.Fatalf("load Office project session %q: %v", sessionID, err)
		}
		if session != nil && session.State == models.TaskSessionStateRunning {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for Office project session %q to reach RUNNING: %#v", sessionID, session)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func assertOfficeProjectLaunchRequest(t *testing.T, req *executor.LaunchAgentRequest, taskID string, repository *models.Repository) {
	t.Helper()
	if req == nil || req.TaskID != taskID {
		t.Fatalf("LaunchAgent request = %#v, want task %q", req, taskID)
	}
	if req.RepositoryID != repository.ID || req.RepositoryPath != repository.LocalPath {
		t.Fatalf("launch repository identity/path = %q/%q, want %q/%q", req.RepositoryID, req.RepositoryPath, repository.ID, repository.LocalPath)
	}
}

func assertOfficeProjectTaskRepository(
	t *testing.T,
	ctx context.Context,
	h *routineCronHarness,
	taskID, sourcePath string,
) (*models.Repository, *models.TaskRepository) {
	t.Helper()
	attachments, err := h.taskRepo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		t.Fatalf("list task repository inventory: %v", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("task repository inventory = %#v, want one project source", attachments)
	}
	repository, err := h.taskRepo.GetRepository(ctx, attachments[0].RepositoryID)
	if err != nil || repository == nil {
		t.Fatalf("load attached repository %q: repository=%#v err=%v", attachments[0].RepositoryID, repository, err)
	}
	resolvedPath, err := filepath.EvalSymlinks(sourcePath)
	if err != nil {
		t.Fatalf("resolve project source path: %v", err)
	}
	if repository.LocalPath != resolvedPath {
		t.Fatalf("attached repository path = %q, want %q", repository.LocalPath, resolvedPath)
	}
	if attachments[0].BaseBranch != "main" {
		t.Fatalf("attached base branch = %q, want normal main default", attachments[0].BaseBranch)
	}
	return repository, attachments[0]
}

func assertChildProjectRepository(t *testing.T, ctx context.Context, h *routineCronHarness, childID, repositoryID string) {
	t.Helper()
	attachments, err := h.taskRepo.ListTaskRepositories(ctx, childID)
	if err != nil {
		t.Fatalf("list child repository inventory: %v", err)
	}
	if len(attachments) != 1 || attachments[0].RepositoryID != repositoryID {
		t.Fatalf("child repository inventory = %#v, want inherited repository %q", attachments, repositoryID)
	}
}

func assertOfficeProjectTaskEnvironment(t *testing.T, ctx context.Context, h *routineCronHarness, taskID, repositoryID string) *models.TaskEnvironment {
	t.Helper()
	environment, err := h.taskRepo.GetTaskEnvironmentByTaskID(ctx, taskID)
	if err != nil || environment == nil {
		t.Fatalf("load task environment for %q: environment=%#v err=%v", taskID, environment, err)
	}
	assertOfficeProjectTaskEnvironmentInventory(t, ctx, h, environment.ID, repositoryID)
	return environment
}

func assertOfficeProjectTaskEnvironmentInventory(t *testing.T, ctx context.Context, h *routineCronHarness, environmentID, repositoryID string) {
	t.Helper()
	rows, err := h.taskRepo.ListTaskEnvironmentRepos(ctx, environmentID)
	if err != nil {
		t.Fatalf("list task_environment_repos for %q: %v", environmentID, err)
	}
	if len(rows) != 1 || rows[0].RepositoryID != repositoryID {
		t.Fatalf("task_environment_repos = %#v, want one row for repository %q", rows, repositoryID)
	}
}

func assertOfficeProjectExecutorWorkspace(
	t *testing.T,
	executorType models.ExecutorType,
	tasksBase string,
	repository *models.Repository,
	environment *models.TaskEnvironment,
) {
	t.Helper()
	if executorType == models.ExecutorTypeLocal {
		if environment.WorkspacePath != repository.LocalPath {
			t.Fatalf("local_pc workspace = %q, want source checkout %q", environment.WorkspacePath, repository.LocalPath)
		}
		return
	}
	if environment.WorkspacePath == repository.LocalPath {
		t.Fatalf("Worktree environment workspace = %q, want isolated checkout", environment.WorkspacePath)
	}
	relativePath, err := filepath.Rel(tasksBase, environment.WorkspacePath)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		t.Fatalf("Worktree path %q is outside task workspace root %q (relative=%q, err=%v)", environment.WorkspacePath, tasksBase, relativePath, err)
	}
}

func initOfficeProjectSourceRepo(t *testing.T) string {
	t.Helper()
	repositoryPath := t.TempDir()
	mustGit(t, repositoryPath, "init", "-b", "main")
	mustGit(t, repositoryPath, "config", "core.hooksPath", "/dev/null")
	if err := os.WriteFile(filepath.Join(repositoryPath, officeProjectSourceSentinel), []byte("project source is available\n"), 0o644); err != nil {
		t.Fatalf("write project source sentinel: %v", err)
	}
	mustGit(t, repositoryPath, "add", officeProjectSourceSentinel)
	mustGit(t, repositoryPath, "commit", "-m", "add project source sentinel")
	return repositoryPath
}

func assertOfficeProjectSentinel(t *testing.T, workspacePath string) {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(workspacePath, officeProjectSourceSentinel))
	if err != nil || string(contents) != "project source is available\n" {
		t.Fatalf("read prepared project source at %q: contents=%q err=%v", workspacePath, contents, err)
	}
}
