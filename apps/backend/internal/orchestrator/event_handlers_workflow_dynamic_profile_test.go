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
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
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
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-work"] = &wfmodels.WorkflowStep{ID: "step-work", WorkflowID: "wf1", Name: "Work"}
	svc := createTestServiceWithScheduler(taskRepo, stepGetter, schedulerRepo, agentManager)
	svc.SetProfileExecutionResolver(resolver)

	created, err := svc.createNewSessionForStep(ctx, current.TaskID, current, dynamicProfileID)
	if err != nil {
		t.Fatalf("createNewSessionForStep: %v", err)
	}
	initialGeneration := created.RouteGeneration
	if _, err := svc.StartCreatedSession(ctx, current.TaskID, created.ID, dynamicProfileID, "start", true, false, true, nil, nil); err != nil {
		t.Fatalf("StartCreatedSession: %v", err)
	}
	persisted, err := taskRepo.GetTaskSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("re-read session: %v", err)
	}
	resolvedProfileID, err := svc.resolveDynamicLaunchExecution(ctx, persisted, persisted.AgentProfileID, true)
	if err != nil {
		t.Fatalf("resolve persisted route: %v", err)
	}
	if persisted.AgentProfileID != dynamicProfileID ||
		persisted.ExecutionProfileID != concreteProfileID ||
		resolvedProfileID != concreteProfileID ||
		persisted.RouteGeneration != initialGeneration || initialGeneration == 0 {
		t.Fatalf("resolved session = logical %q concrete %q returned %q generation %d→%d",
			persisted.AgentProfileID, persisted.ExecutionProfileID, resolvedProfileID,
			initialGeneration, persisted.RouteGeneration)
	}
}

// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.1
func TestCreateNewSessionForStep_RemovesPreparedSessionWhenDynamicResolutionFails(t *testing.T) {
	ctx := context.Background()
	taskRepo, current := seedDynamicWorkflowSwitch(t)
	const dynamicProfileID = "profile-dynamic"
	const concreteProfileID = "profile-concrete"

	resolver := newWorkflowNoCandidateProfileResolver(t, dynamicProfileID, concreteProfileID)
	launchCalled := false
	agentManager := &mockAgentManager{
		repoForExecutionLookup: taskRepo,
		launchAgentFunc: func(_ context.Context, _ *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCalled = true
			return &executor.LaunchAgentResponse{AgentExecutionID: "unexpected-launch"}, nil
		},
	}
	schedulerRepo := newMockTaskRepo()
	schedulerRepo.tasks[current.TaskID] = &v1.Task{ID: current.TaskID, WorkspaceID: "ws1", Title: "Test Task"}
	svc := createTestServiceWithScheduler(taskRepo, newMockStepGetter(), schedulerRepo, agentManager)
	svc.SetProfileExecutionResolver(resolver)

	if _, err := svc.createNewSessionForStep(ctx, current.TaskID, current, dynamicProfileID); err == nil {
		t.Fatal("createNewSessionForStep succeeded without an eligible dynamic candidate")
	}
	if launchCalled {
		t.Fatal("lifecycle launch ran after dynamic profile resolution failed")
	}
	sessions, err := taskRepo.ListTaskSessions(ctx, current.TaskID)
	if err != nil {
		t.Fatalf("list task sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != current.ID {
		t.Fatalf("sessions after failed resolution = %+v, want only %q", sessions, current.ID)
	}
	active, err := taskRepo.GetActiveTaskSessionByTaskID(ctx, current.TaskID)
	if err != nil {
		t.Fatalf("get active session: %v", err)
	}
	if active == nil || active.ID != current.ID || active.State != models.TaskSessionStateRunning {
		t.Fatalf("active session after failed resolution = %+v, want running %q", active, current.ID)
	}
	if reusable, err := svc.findReusableSessionForProfile(ctx, current.TaskID, dynamicProfileID, current.ID); err != nil {
		t.Fatalf("find reusable session: %v", err)
	} else if reusable != nil {
		t.Fatalf("stale replacement session remained reusable: %+v", reusable)
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
	return newWorkflowDynamicProfileResolverWithCandidate(t, dynamicProfileID, concreteProfileID, true)
}

func newWorkflowNoCandidateProfileResolver(
	t *testing.T,
	dynamicProfileID, concreteProfileID string,
) *agentruntime.ProfileExecutionResolver {
	return newWorkflowDynamicProfileResolverWithCandidate(t, dynamicProfileID, concreteProfileID, false)
}

func newWorkflowDynamicProfileResolverWithCandidate(
	t *testing.T,
	dynamicProfileID, concreteProfileID string,
	candidateEnabled bool,
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
			Enabled:            candidateEnabled,
		}},
	); err != nil {
		t.Fatal(err)
	}
	return agentruntime.NewProfileExecutionResolver(repo, dynamicruntime.NewEngine(), true)
}
