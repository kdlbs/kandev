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

const accountBlockedProviderError = `Error from provider example-gateway: 403 Forbidden {"error":{"code":"user_blocked",` +
	`"message":"Your access has been restricted due to repeated policy violations."}}`

const (
	accountBlockedProfile = "candidate-blocked"
	accountBlockNext      = "candidate-next"
	accountBlockDynamicID = "dynamic-account-blocked"
)

type accountBlockRoute struct {
	svc        *Service
	repo       *sqliterepo.Repository
	circuits   *dynamicruntime.CircuitRegistry
	generation int64
}

// setupAccountBlockRoute projects a dynamic route whose current candidate is
// sessionProfile onto a running session served by executionID.
func setupAccountBlockRoute(t *testing.T, taskID, sessionID, executionID, sessionProfile string) accountBlockRoute {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateRunning)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, taskID, v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	circuits := dynamicruntime.NewCircuitRegistry()
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, accountBlockDynamicID, []workflowDynamicCandidate{
		{executionProfileID: accountBlockedProfile, enabled: true, rulesJSON: `{"on_provider_error":"try_next"}`},
		{executionProfileID: accountBlockNext, enabled: true},
	}, dynamicruntime.WithPersistence(repo), dynamicruntime.WithCircuitRegistry(circuits))
	svc.SetProfileExecutionResolver(resolver)
	initial, err := resolver.Resolve(ctx, sessionID, accountBlockDynamicID, 0, "")
	if err != nil || initial.ExecutionProfileID != accountBlockedProfile {
		t.Fatalf("initial resolve = %#v, %v; want %s", initial, err, accountBlockedProfile)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.AgentProfileID = accountBlockDynamicID
	session.ExecutionProfileID = sessionProfile
	session.RouteGeneration = initial.Generation
	session.RouteState = dynamicRouteStatusActive
	session.AgentExecutionID = executionID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("project route: %v", err)
	}
	return accountBlockRoute{svc: svc, repo: repo, circuits: circuits, generation: initial.Generation}
}

func accountBlockedEvent(taskID, sessionID, executionID string, outputObserved bool) watcher.AgentEventData {
	return watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID, AgentExecutionID: executionID,
		AgentProfileID: accountBlockDynamicID, ExecutionProfileID: accountBlockedProfile, PromptGeneration: 1,
		ErrorMessage: accountBlockedProviderError, DynamicRouteAttempt: true,
		EvidenceKnown: true, OutputObserved: outputObserved,
	}
}

func classifyAccountBlocked(t *testing.T, data watcher.AgentEventData) *routingerr.Error {
	t.Helper()
	classified := classifyKanbanFailure(data)
	if classified.Code != routingerr.CodeAccountBlocked {
		t.Fatalf("classified = %s (%s), want account_blocked", classified.Code, classified.ClassifierRule)
	}
	return classified
}

func (r accountBlockRoute) held(profileID string) bool {
	return r.circuits.IsHeld(dynamicruntime.ResourceKey(dynamicruntime.ScopeAccountBlock, profileID))
}

// An account restriction reported after the attempt already produced output
// stays with the operator: the output is preserved and no successor launches.
// The restricted profile must still be held, or the next selection would send
// requests to the blocked account again.
func TestAccountBlockedAfterOutputHoldsProfile(t *testing.T) {
	const (
		taskID      = "task-account-blocked-output"
		sessionID   = "session-account-blocked-output"
		executionID = "execution-blocked"
	)
	route := setupAccountBlockRoute(t, taskID, sessionID, executionID, accountBlockedProfile)
	data := accountBlockedEvent(taskID, sessionID, executionID, true)

	route.svc.routeDynamicAgentFailure(context.Background(), data, classifyAccountBlocked(t, data))

	if !route.held(accountBlockedProfile) {
		t.Fatal("the restricted profile was not held")
	}
	if route.held(accountBlockNext) {
		t.Fatal("the next candidate was held")
	}
	state, err := route.repo.LoadRouteState(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Generation != route.generation || state.ExecutionProfileID != accountBlockedProfile {
		t.Fatalf("route state = %#v, want generation %d still on %s", state, route.generation, accountBlockedProfile)
	}
}

// A restriction reported before any output may move the route on: the
// restricted profile is held and the unrestricted sibling takes over.
func TestAccountBlockedBeforeOutputHoldsProfileAndAdvances(t *testing.T) {
	const (
		taskID      = "task-account-blocked-pre-output"
		sessionID   = "session-account-blocked-pre-output"
		executionID = "execution-blocked-pre-output"
	)
	route := setupAccountBlockRoute(t, taskID, sessionID, executionID, accountBlockedProfile)
	route.svc.lastTurnPrompt.Store(sessionID, capturedPrompt{text: "retry the task"})
	route.svc.beginDynamicAttempt(sessionID)
	route.svc.bindDynamicAttemptExecution(sessionID, executionID)
	data := accountBlockedEvent(taskID, sessionID, executionID, false)

	handled := route.svc.routeDynamicAgentFailure(context.Background(), data, classifyAccountBlocked(t, data))

	if !route.held(accountBlockedProfile) {
		t.Fatal("the restricted profile was not held")
	}
	if route.held(accountBlockNext) {
		t.Fatal("the next candidate was held")
	}
	// The successor launch can complete in this call or on a detached worker.
	deadline := time.Now().Add(2 * time.Second)
	for {
		state, err := route.repo.LoadRouteState(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("LoadRouteState: %v", err)
		}
		if state != nil && state.Generation == route.generation+1 && state.ExecutionProfileID == accountBlockNext {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("route state = %#v (handled=%v), want generation %d on %s",
				state, handled, route.generation+1, accountBlockNext)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A predecessor can keep serving the session after the route already points
// to its successor. Its restriction must still hold its own profile, even
// though its failure may not advance the current route.
func TestAccountBlockedPredecessorHoldsItsProfile(t *testing.T) {
	const (
		taskID      = "task-account-blocked-predecessor"
		sessionID   = "session-account-blocked-predecessor"
		executionID = "execution-predecessor"
	)
	route := setupAccountBlockRoute(t, taskID, sessionID, executionID, accountBlockNext)
	data := accountBlockedEvent(taskID, sessionID, executionID, true)

	route.svc.routeDynamicAgentFailure(context.Background(), data, classifyAccountBlocked(t, data))

	if !route.held(accountBlockedProfile) {
		t.Fatal("the predecessor's restricted profile was not held")
	}
	if route.held(accountBlockNext) {
		t.Fatal("the current candidate was held for its predecessor's failure")
	}
}
