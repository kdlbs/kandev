package orchestrator

import (
	"context"
	"testing"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	midTurnDynamicID = "dynamic-midturn-circuit"
	midTurnFailing   = "candidate-failing"
	midTurnNext      = "candidate-next"
)

type midTurnCircuitRoute struct {
	svc        *Service
	repo       *sqliterepo.Repository
	circuits   *dynamicruntime.CircuitRegistry
	generation int64
}

// setupMidTurnCircuitRoute projects an active dynamic route on midTurnFailing
// onto a running session whose current execution is executionID, and records
// sessionProfile as the route's current candidate on the session.
func setupMidTurnCircuitRoute(t *testing.T, taskID, sessionID, executionID, sessionProfile string, circuitOptions ...dynamicruntime.CircuitRegistryOption) midTurnCircuitRoute {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	circuits := dynamicruntime.NewCircuitRegistry(circuitOptions...)
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, midTurnDynamicID, []workflowDynamicCandidate{
		{executionProfileID: midTurnFailing, enabled: true, rulesJSON: `{"on_provider_error":"try_next"}`},
		{executionProfileID: midTurnNext, enabled: true},
	}, dynamicruntime.WithPersistence(repo), dynamicruntime.WithCircuitRegistry(circuits))
	svc.SetProfileExecutionResolver(resolver)
	initial, err := resolver.Resolve(ctx, sessionID, midTurnDynamicID, 0, "")
	if err != nil || initial.ExecutionProfileID != midTurnFailing {
		t.Fatalf("initial resolve = %#v, %v; want %s", initial, err, midTurnFailing)
	}
	// The launch succeeded and the turn is running on the selected candidate.
	if err := resolver.MarkRouteActive(ctx, sessionID, initial.Generation); err != nil {
		t.Fatalf("MarkRouteActive: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.AgentProfileID = midTurnDynamicID
	session.ExecutionProfileID = sessionProfile
	session.RouteGeneration = initial.Generation
	session.RouteState = dynamicRouteStatusActive
	session.AgentExecutionID = executionID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("project route: %v", err)
	}
	return midTurnCircuitRoute{svc: svc, repo: repo, circuits: circuits, generation: initial.Generation}
}

func midTurnFailureEvent(taskID, sessionID, executionID, executionProfileID string) watcher.AgentEventData {
	return watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID,
		AgentProfileID: midTurnDynamicID, ExecutionProfileID: executionProfileID, PromptGeneration: 1,
		DynamicRouteAttempt: true, EvidenceKnown: true, OutputObserved: true, EffectObserved: true,
	}
}

func (r midTurnCircuitRoute) open(profileID string) bool {
	return r.circuits.IsOpen(dynamicruntime.ResourceKey(dynamicruntime.ScopeProfile, profileID), time.Now())
}

func (r midTurnCircuitRoute) requireRouteUnchanged(t *testing.T, sessionID, profileID string) {
	t.Helper()
	state, err := r.repo.LoadRouteState(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Generation != r.generation || state.ExecutionProfileID != profileID ||
		state.Status != dynamicRouteStatusActive {
		t.Fatalf("route state = %#v, want active generation %d still on %s", state, r.generation, profileID)
	}
}

// A provider failure reported after the attempt already produced output and
// effects may not replay or advance the turn, but it is still evidence that
// the candidate is unhealthy. Its circuit opens, the route stays where it is,
// and the next selection skips the candidate.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.19
func TestDynamicMidTurnProviderFailureOpensCandidateCircuit(t *testing.T) {
	codes := []routingerr.Code{routingerr.CodeProviderUnavailable, routingerr.CodeRateLimited}
	for _, code := range codes {
		t.Run(string(code), func(t *testing.T) {
			ctx := context.Background()
			taskID := "task-midturn-" + string(code)
			sessionID := "session-midturn-" + string(code)
			const executionID = "execution-midturn"
			route := setupMidTurnCircuitRoute(t, taskID, sessionID, executionID, midTurnFailing)
			failure := &routingerr.Error{
				Code: code, Class: routingerr.ClassTransient,
				Confidence: routingerr.ConfHigh, FallbackAllowed: true, AutoRetryable: true,
			}

			handled := route.svc.routeDynamicAgentFailure(ctx,
				midTurnFailureEvent(taskID, sessionID, executionID, midTurnFailing), failure)

			if handled {
				t.Fatal("a mid-turn failure was routed; it must keep the normal recovery surface")
			}
			if !route.open(midTurnFailing) {
				t.Fatalf("the circuit of %s stayed closed after a mid-turn %s failure", midTurnFailing, code)
			}
			if route.open(midTurnNext) {
				t.Fatalf("the circuit of %s opened for another candidate's failure", midTurnNext)
			}
			route.requireRouteUnchanged(t, sessionID, midTurnFailing)

			nextSessionID := "session-after-" + string(code)
			seedTaskAndSession(t, route.repo, "task-after-"+string(code), nextSessionID, models.TaskSessionStateRunning)
			next, err := route.svc.profileExecutionResolver.Resolve(ctx, nextSessionID, midTurnDynamicID, 0, "")
			if err != nil || next.ExecutionProfileID != midTurnNext {
				t.Fatalf("next selection = %#v, %v; want %s", next, err, midTurnNext)
			}
		})
	}
}

// A predecessor execution can keep serving the session after the route moved
// to its successor. Its late mid-turn failure must not open the successor's
// circuit or touch the route.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.10
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.19
func TestDynamicMidTurnFailureOfPredecessorDoesNotOpenSuccessorCircuit(t *testing.T) {
	const (
		taskID      = "task-midturn-predecessor"
		sessionID   = "session-midturn-predecessor"
		executionID = "execution-predecessor"
	)
	route := setupMidTurnCircuitRoute(t, taskID, sessionID, executionID, midTurnNext)
	failure := &routingerr.Error{
		Code: routingerr.CodeProviderUnavailable, Class: routingerr.ClassTransient,
		Confidence: routingerr.ConfHigh, FallbackAllowed: true, AutoRetryable: true,
	}

	route.svc.routeDynamicAgentFailure(context.Background(),
		midTurnFailureEvent(taskID, sessionID, executionID, midTurnFailing), failure)

	if route.open(midTurnNext) {
		t.Fatalf("a predecessor failure opened the circuit of the successor %s", midTurnNext)
	}
}

// An unclassified mid-turn failure is not evidence about the provider, so no
// circuit opens and the route is unchanged.
//
// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.19
func TestDynamicMidTurnUnclassifiedFailureDoesNotOpenCircuit(t *testing.T) {
	const (
		taskID      = "task-midturn-unclassified"
		sessionID   = "session-midturn-unclassified"
		executionID = "execution-midturn-unclassified"
	)
	route := setupMidTurnCircuitRoute(t, taskID, sessionID, executionID, midTurnFailing)
	failure := &routingerr.Error{
		Code: routingerr.CodeUnknownProvider, Class: routingerr.ClassUnclassified,
		Confidence: routingerr.ConfLow,
	}

	route.svc.routeDynamicAgentFailure(context.Background(),
		midTurnFailureEvent(taskID, sessionID, executionID, midTurnFailing), failure)

	if route.open(midTurnFailing) {
		t.Fatal("an unclassified mid-turn failure opened the candidate circuit")
	}
	route.requireRouteUnchanged(t, sessionID, midTurnFailing)
}
