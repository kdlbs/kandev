package orchestrator

import (
	"context"
	"testing"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// A dynamic route moved from candidate-a to candidate-b, but the candidate-b
// launch has not replaced the candidate-a execution yet, so that execution
// still serves the session. A later failure of the candidate-a execution must
// not open candidate-b's circuit or advance the route past it: candidate-b
// never ran.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.10
func TestDynamicFailureOfSupersededCandidateDoesNotChargeCurrentCandidate(t *testing.T) {
	ctx := context.Background()
	const (
		taskID      = "task-dynamic-superseded-failure"
		sessionID   = "session-dynamic-superseded-failure"
		executionID = "execution-candidate-a-still-serving"
		dynamicID   = "dynamic-superseded-failure"
		candidateA  = "candidate-a"
		candidateB  = "candidate-b"
		candidateC  = "candidate-c"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	circuits := dynamicruntime.NewCircuitRegistry()
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, dynamicID, []workflowDynamicCandidate{
		{executionProfileID: candidateA, enabled: true, rulesJSON: `{"on_provider_error":"try_next"}`},
		{executionProfileID: candidateB, enabled: true, rulesJSON: `{"on_provider_error":"try_next"}`},
		{executionProfileID: candidateC, enabled: true},
	}, dynamicruntime.WithPersistence(repo), dynamicruntime.WithCircuitRegistry(circuits))
	svc.SetProfileExecutionResolver(resolver)

	initial, err := resolver.Resolve(ctx, sessionID, dynamicID, 0, "")
	if err != nil || initial.ExecutionProfileID != candidateA {
		t.Fatalf("initial resolve = %#v, %v; want %s", initial, err, candidateA)
	}
	rateLimited := &routingerr.Error{
		Code: routingerr.CodeRateLimited, Class: routingerr.ClassTransient,
		Confidence: routingerr.ConfHigh, FallbackAllowed: true, AutoRetryable: true,
	}
	moved, err := resolver.ResolveExecutionAfterFailure(ctx, sessionID, dynamicID, candidateA, initial.Generation, rateLimited)
	if err != nil || moved.ExecutionProfileID != candidateB {
		t.Fatalf("route after the first candidate-a failure = %#v, %v; want %s", moved, err, candidateB)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.AgentProfileID = dynamicID
	session.ExecutionProfileID = moved.ExecutionProfileID
	session.RouteGeneration = moved.Generation
	session.RouteState = dynamicRouteStatusActive
	session.AgentExecutionID = executionID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("project route: %v", err)
	}
	// The next prompt still reaches the candidate-a execution.
	svc.beginDynamicAttempt(sessionID)
	svc.bindDynamicAttemptExecution(sessionID, executionID)

	svc.routeDynamicAgentFailure(ctx, watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID,
		AgentProfileID: dynamicID, ExecutionProfileID: candidateA, PromptGeneration: 1,
	}, rateLimited)

	candidateBKey := dynamicruntime.ResourceKey(dynamicruntime.ScopeProfile, candidateB)
	if circuits.IsOpen(candidateBKey, time.Now()) {
		t.Errorf("a candidate-a failure opened the circuit of %s", candidateB)
	}
	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Generation != moved.Generation || state.ExecutionProfileID != candidateB {
		t.Fatalf("route state = %#v, want generation %d still on %s", state, moved.Generation, candidateB)
	}
}
