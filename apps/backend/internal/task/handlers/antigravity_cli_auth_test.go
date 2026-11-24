package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

type antigravitySessionAccessRepo struct{ mockRepository }

func (*antigravitySessionAccessRepo) GetTaskSession(_ context.Context, id string) (*models.TaskSession, error) {
	return &models.TaskSession{ID: id, TaskID: "task-cli-auth"}, nil
}

func (*antigravitySessionAccessRepo) GetTask(context.Context, string) (*models.Task, error) {
	return &models.Task{ID: "task-cli-auth", WorkspaceID: "workspace-cli-auth"}, nil
}

func (*antigravitySessionAccessRepo) GetWorkspace(context.Context, string) (*models.Workspace, error) {
	return &models.Workspace{ID: "workspace-cli-auth", OwnerID: "session-user"}, nil
}

func TestTaskSessionUserCannotAuthenticateAntigravityCLI(t *testing.T) {
	log := newTestLogger(t)
	repo := &antigravitySessionAccessRepo{}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	manager := newLifecycleManager(t, log)
	if err := manager.ExecutionStoreForTesting().Add(&lifecycle.AgentExecution{
		ID:        "exec-cli-auth",
		SessionID: "sess-b",
		AgentID:   acpcompat.AntigravityAgentID,
		AuthPolicy: acpcompat.AntigravityAuthPolicyForRuntime(
			acpcompat.AntigravityRuntimeCLI, "1.5.15", false,
		),
	}); err != nil {
		t.Fatal(err)
	}
	handler := &ProcessHandlers{service: svc, lifecycleMgr: manager, logger: log}
	ctx, response := processRequestAs(
		t,
		"session-user",
		http.MethodPost,
		"/api/v1/task-sessions/sess-b/authenticate",
		`{"method_id":"oauth-personal"}`,
	)
	handler.httpAuthenticate(ctx)

	if response.Code != http.StatusForbidden {
		t.Fatalf("session-user authentication status = %d, want 403: %s", response.Code, response.Body.String())
	}
}
