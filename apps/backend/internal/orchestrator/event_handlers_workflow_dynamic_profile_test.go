package orchestrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentsettingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.1
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.3
func TestCreateNewSessionForStep_ResolvesDynamicProfileBeforeWorkspaceAttach(t *testing.T) {
	ctx := context.Background()
	taskRepo, current := seedDynamicWorkflowSwitch(t)
	const dynamicProfileID = "profile-dynamic"
	const concreteProfileID = "profile-concrete"

	resolver := newWorkflowDynamicProfileResolver(t, dynamicProfileID, concreteProfileID)
	agentManager := &mockAgentManager{
		repoForExecutionLookup: taskRepo,
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			if req.AgentProfileID != concreteProfileID {
				return nil, fmt.Errorf("failed to resolve agent profile: profile %s: %w", req.AgentProfileID, lifecycle.ErrVirtualProfile)
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: "replacement-execution"}, nil
		},
	}
	schedulerRepo := newMockTaskRepo()
	schedulerRepo.tasks[current.TaskID] = &v1.Task{ID: current.TaskID, WorkspaceID: "ws1", Title: "Test Task"}
	svc := createTestServiceWithScheduler(taskRepo, newMockStepGetter(), schedulerRepo, agentManager)
	svc.SetProfileExecutionResolver(resolver)

	created, err := svc.createNewSessionForStep(ctx, current.TaskID, current, dynamicProfileID)
	if err != nil {
		t.Fatalf("createNewSessionForStep: %v", err)
	}
	initialGeneration := created.RouteGeneration
	resolvedProfileID, err := svc.resolveDynamicLaunchExecution(ctx, created, created.AgentProfileID, true)
	if err != nil {
		t.Fatalf("resolve persisted route: %v", err)
	}
	if created.AgentProfileID != dynamicProfileID ||
		created.ExecutionProfileID != concreteProfileID ||
		resolvedProfileID != concreteProfileID ||
		created.RouteGeneration != initialGeneration || initialGeneration == 0 {
		t.Fatalf("resolved session = logical %q concrete %q returned %q generation %d→%d",
			created.AgentProfileID, created.ExecutionProfileID, resolvedProfileID,
			initialGeneration, created.RouteGeneration)
	}
}

func seedDynamicWorkflowSwitch(t *testing.T) (*sqliterepo.Repository, *models.TaskSession) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-dynamic-switch", "session-current", "step-work")
	current, err := repo.GetTaskSession(ctx, "session-current")
	if err != nil {
		t.Fatal(err)
	}
	current.State = models.TaskSessionStateRunning
	current.AgentProfileID = "profile-old"
	current.ExecutorID = models.ExecutorIDWorktree
	current.TaskEnvironmentID = "environment-dynamic-switch"
	if err := repo.UpdateTaskSession(ctx, current); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: current.TaskEnvironmentID, TaskID: current.TaskID,
		ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatal(err)
	}
	return repo, current
}

func newWorkflowDynamicProfileResolver(
	t *testing.T,
	dynamicProfileID, concreteProfileID string,
) *agentruntime.ProfileExecutionResolver {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, cleanup, err := agentsettingsstore.Provide(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanup() })
	ctx := context.Background()
	for _, agent := range []*agentsettingsmodels.Agent{
		{ID: "dynamic", Name: "dynamic"},
		{ID: "concrete-agent", Name: "concrete-agent"},
	} {
		if err := repo.CreateAgent(ctx, agent); err != nil {
			t.Fatal(err)
		}
	}
	for _, profile := range []*agentsettingsmodels.AgentProfile{
		{ID: dynamicProfileID, AgentID: "dynamic", Name: "Cascade", Enabled: true},
		{ID: concreteProfileID, AgentID: "concrete-agent", Name: "Concrete", Enabled: true},
	} {
		if err := repo.CreateAgentProfile(ctx, profile); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CreateDynamicAgentProfile(ctx,
		&agentsettingsmodels.DynamicAgentProfile{ProfileID: dynamicProfileID, Version: 1},
		[]agentsettingsmodels.DynamicAgentRoute{{
			DynamicProfileID:   dynamicProfileID,
			ExecutionProfileID: concreteProfileID,
			Enabled:            true,
		}},
	); err != nil {
		t.Fatal(err)
	}
	return agentruntime.NewProfileExecutionResolver(repo, dynamicruntime.NewEngine(), true)
}
