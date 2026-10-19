package orchestrator

import (
	"context"
	"errors"
	"testing"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.12
func TestResumeCallerReportsActivationFailureAndRecoversSameExecution(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task1", "session1", models.TaskSessionStateWaitingForInput)
	seedExecutorRunning(t, repo, "session1", "task1", "old-execution")
	persistence := &failingRouteActivationRepo{Repository: repo}
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, "dynamic-logical",
		[]workflowDynamicCandidate{{executionProfileID: "candidate-1", enabled: true}},
		dynamicruntime.WithPersistence(persistence), dynamicruntime.WithStateLoader(repo))
	engine := dynamicruntime.NewEngine(dynamicruntime.WithPersistence(repo))
	decision := seedClaimedDynamicRoute(t, ctx, repo, engine, "session1", "old-execution")
	launches := 0
	live := false
	manager := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunningFn:       func(context.Context, string) bool { return live },
		isAgentReadyFn:         func(context.Context, string) bool { return live },
		launchAgentFunc: func(launchCtx context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launches++
			current, err := repo.GetTaskSession(launchCtx, request.SessionID)
			if err != nil {
				return nil, err
			}
			current.State = models.TaskSessionStateWaitingForInput
			if err := repo.UpdateTaskSession(launchCtx, current); err != nil {
				return nil, err
			}
			if err := repo.UpsertExecutorRunning(launchCtx, &models.ExecutorRunning{
				ID: request.SessionID, SessionID: request.SessionID, TaskID: "task1",
				AgentExecutionID: "ready-execution", ExecutionProfileID: "candidate-1", Status: "ready",
			}); err != nil {
				return nil, err
			}
			live = true
			persistence.failActivation = true
			return &executor.LaunchAgentResponse{AgentExecutionID: "ready-execution"}, nil
		},
	}
	tasks := newMockTaskRepo()
	seedMockTaskState(tasks, "task1", v1.TaskStateInProgress)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), tasks, manager)
	svc.SetProfileExecutionResolver(resolver)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	result, err := svc.resumeTaskSessionWithContinuation(ctx, "task1", "session1",
		executor.ResumeOptions{RecoveryAction: recoveryActionResume}, nil)
	if result != nil || !errors.Is(err, errInjectedRouteActivation) {
		t.Fatalf("actual resume after failed activation = %#v, %v; want the activation error", result, err)
	}
	current := mustTaskSession(t, repo, ctx, "session1")
	if current.State != models.TaskSessionStateWaitingForInput || current.AgentExecutionID != "ready-execution" {
		t.Fatalf("failed activation replaced or failed the ready runtime: %#v", current)
	}
	persistence.failActivation = false
	result, err = svc.ResumeTaskSession(ctx, "task1", "session1")
	if err != nil || result == nil || result.AgentExecutionID != "ready-execution" {
		t.Fatalf("actual resume after store recovery = %#v, %v; want the same execution", result, err)
	}
	state, err := repo.LoadRouteState(ctx, "session1")
	if err != nil || state == nil || state.Generation != decision.Generation || state.Status != dynamicRouteStatusActive {
		t.Fatalf("recovered route = %#v, %v; want the original generation active", state, err)
	}
	if launches != 1 || len(manager.stopAgentArgs) != 0 || len(manager.stopAgentWithReasonArgs) != 0 {
		t.Fatal("activation recovery stopped or relaunched the ready runtime")
	}
}
