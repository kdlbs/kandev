package backendapp

import (
	"context"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events/bus"
	mcph "github.com/kandev/kandev/internal/mcp/handlers"
	mcpp "github.com/kandev/kandev/internal/mcp/profile"
	mcps "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"path/filepath"
	"strings"
	"testing"
)

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.6, AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.1, AC-TASKS-ATTACH-WORKSPACE-SOURCES-008.6
func TestManagedBranchAttachmentConsumers(t *testing.T) {
	for _, scenario := range []string{"empty", "missing", "valid", "local-valid", "local-missing", "foreign", "legacy-valid", "legacy-missing", "persistence", "persistence-retry", "pathless-legacy", "active", "inherited", "rebound-session", "changed-environment", "cloner-unavailable", "scoped-missing", "repository-changed", "repository-changed-missing"} {
		t.Run(scenario, func(t *testing.T) { managedAttachment(t, "mcp-branch", scenario) })
	}
	t.Run("prelaunch-child", testManagedBranchPrelaunch)
}

func testManagedBranchPrelaunch(t *testing.T) {
	ctx := context.Background()
	repo := newMaterializerRepo(t)
	log := newTestLogger()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws", Name: "Workspace", OwnerID: "owner"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf", WorkspaceID: "ws", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	for _, task := range []*models.Task{{ID: "caller", WorkspaceID: "ws", WorkflowID: "wf", Title: "Caller"}, {ID: "prelaunch", WorkspaceID: "ws", WorkflowID: "wf", ParentID: "caller", Title: "Child"}} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "caller-session", TaskID: "caller", State: models.TaskSessionStateWaitingForInput}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{ID: "remote", WorkspaceID: "ws", Name: "Remote", SourceType: "provider", Provider: "gitlab", ProviderHost: "https://gitlab.example", ProviderOwner: "team", ProviderName: "repo", DefaultBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	eventBus := bus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	svc := taskservice.NewService(taskservice.Repos{Workspaces: repo, Tasks: repo, TaskRepos: repo, Sessions: repo, RepoEntities: repo, TaskEnvironments: repo, WorkspaceFolders: repo, Turns: repo, Workflows: repo}, eventBus, log, taskservice.RepositoryDiscoveryConfig{})
	cloner := &managedAttachmentCloner{Cloner: repoclone.NewCloner(repoclone.Config{BasePath: canonicalTempDir(t)}, repoclone.ProtocolHTTPS, "", log)}
	exc := executor.NewExecutor(nil, nil, log, executor.ExecutorConfig{})
	exc.SetRepoCloner(cloner, &repoLocalPathUpdater{svc: svc})
	materializer := newWorkspaceSourceMaterializer(repo, newPersistingMaterializerWorktreeMgr(t, filepath.Join(canonicalTempDir(t), "task"), repo), nil, log)
	materializer.SetHostRepositoryCloner(exc)
	svc.SetBranchMaterializer(materializer.branches)
	ctx = authn.WithIdentity(ctx, authn.Identity{UserID: "owner", Role: authn.RoleMember})
	ctx = mcps.WithPrincipal(ctx, mcps.Principal{WorkspaceID: "ws", CallerTaskID: "caller", CallerSessionID: "caller-session", Surface: mcpp.SurfaceKanbanTask})
	dispatcher := ws.NewDispatcher()
	mcph.NewHandlers(svc, nil, nil, nil, nil, repo, repo, eventBus, nil, nil, nil, nil, log).RegisterHandlers(dispatcher)
	msg, err := ws.NewRequest("prelaunch", ws.ActionMCPAddBranchToTask, map[string]any{"task_id": "prelaunch", "repository_id": "remote", "base_branch": "main"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := dispatcher.Dispatch(ctx, msg)
	if err != nil {
		t.Fatal(err)
	}
	if result.Type != ws.MessageTypeResponse || strings.Contains(string(result.Payload), "worktree_path") || len(cloner.requests) != 0 {
		t.Fatalf("prelaunch result=%s requests=%d", result.Payload, len(cloner.requests))
	}
	rows, err := repo.ListTaskRepositories(ctx, "prelaunch")
	if err != nil || len(rows) != 1 {
		t.Fatalf("prelaunch rows=%v error=%v", rows, err)
	}
}
