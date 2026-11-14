package orchestrator

import (
	"context"
	"testing"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

func midTurnProviderUnavailable() *routingerr.Error {
	return &routingerr.Error{
		Code: routingerr.CodeProviderUnavailable, Class: routingerr.ClassTransient,
		Confidence: routingerr.ConfHigh, FallbackAllowed: true, AutoRetryable: true,
	}
}

// A mid-turn failure counts against the candidate, and a later turn on the same route that completes cleanly resets the count.
// A completion reported for a different candidate leaves it unchanged.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.20
func TestDynamicCleanTurnCompletionResetsCircuitFailures(t *testing.T) {
	const (
		taskID      = "task-circuit-completion-reset"
		sessionID   = "session-circuit-completion-reset"
		executionID = "execution-circuit-completion-reset"
	)
	ctx := context.Background()
	route := setupMidTurnCircuitRoute(t, taskID, sessionID, executionID, midTurnFailing)
	key := dynamicruntime.ResourceKey(dynamicruntime.ScopeProfile, midTurnFailing)

	route.svc.routeDynamicAgentFailure(ctx,
		midTurnFailureEvent(taskID, sessionID, executionID, midTurnFailing), midTurnProviderUnavailable())
	if got := route.failures(key); got != 1 {
		t.Fatalf("failures after a mid-turn failure = %d, want 1", got)
	}
	session, err := route.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}

	route.svc.resetDynamicCircuitFailuresForCompletion(ctx, watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID, ExecutionProfileID: midTurnNext,
	}, session)
	if got := route.failures(key); got != 1 {
		t.Fatalf("a completion of another candidate changed the count to %d", got)
	}

	route.svc.resetDynamicCircuitFailuresForCompletion(ctx, watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID, ExecutionProfileID: midTurnFailing,
	}, session)
	if got := route.failures(key); got != 0 {
		t.Fatalf("failures after a clean turn = %d, want 0", got)
	}
}

func (r midTurnCircuitRoute) failures(key string) int {
	for _, snapshot := range r.circuits.Snapshots() {
		if snapshot.Key == key {
			return snapshot.Failures
		}
	}
	return 0
}
