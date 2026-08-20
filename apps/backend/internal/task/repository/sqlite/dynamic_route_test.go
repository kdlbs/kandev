package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDynamicRouteStateAndAttemptsPersistAcrossRepositoryReads(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-dynamic-route", Title: "Dynamic route"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-dynamic-route", TaskID: "task-dynamic-route",
		State: models.TaskSessionStateStarting,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	now := time.Now().UTC()
	state := dynamicruntime.RouteState{
		SessionID: "session-dynamic-route", LogicalProfileID: "dynamic-1",
		ExecutionProfileID: "concrete-1", Generation: 7, ProfileVersion: 3,
		Status: "active", PolicyStateJSON: `{"retry_ordinal":2,"pending_outcome":"skip"}`, UpdatedAt: now,
	}
	if err := repo.SaveRouteState(ctx, state); err != nil {
		t.Fatalf("SaveRouteState: %v", err)
	}
	continuation := dynamicruntime.Continuation{
		TaskDescription: "continue the migration",
		Conversation:    "The predecessor completed the schema scan.",
	}
	if err := repo.SaveRouteContinuation(ctx, dynamicruntime.ContinuationRecord{
		SessionID: state.SessionID, Generation: state.Generation, Continuation: continuation,
	}); err != nil {
		t.Fatalf("SaveRouteContinuation: %v", err)
	}
	if err := repo.SaveRouteContinuation(ctx, dynamicruntime.ContinuationRecord{
		SessionID: state.SessionID, Generation: state.Generation - 1, Continuation: continuation,
	}); !errors.Is(err, dynamicruntime.ErrStaleGeneration) {
		t.Fatalf("stale SaveRouteContinuation error = %v", err)
	}
	if err := repo.AppendRouteAttempt(ctx, dynamicruntime.RouteAttempt{
		SessionID: "session-dynamic-route", LogicalProfileID: "dynamic-1",
		ExecutionProfileID: "concrete-1", Generation: 7, ProfileVersion: 3,
		Reason: "candidate_order", CreatedAt: now,
	}); err != nil {
		t.Fatalf("AppendRouteAttempt: %v", err)
	}
	loaded, err := repo.LoadRouteState(ctx, "session-dynamic-route")
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded state is nil")
		return
	}
	if loaded.Generation != 7 || loaded.ExecutionProfileID != "concrete-1" {
		t.Fatalf("loaded state = %#v", loaded)
	}
	if loaded.PolicyStateJSON != state.PolicyStateJSON {
		t.Fatalf("loaded policy state = %q, want %q", loaded.PolicyStateJSON, state.PolicyStateJSON)
	}
	var loadedContinuation dynamicruntime.Continuation
	if err := json.Unmarshal([]byte(loaded.ContinuationJSON), &loadedContinuation); err != nil {
		t.Fatalf("decode continuation: %v", err)
	}
	if loadedContinuation.TaskDescription != continuation.TaskDescription ||
		loadedContinuation.Conversation != continuation.Conversation {
		t.Fatalf("loaded continuation = %#v", loadedContinuation)
	}
	attempts, err := repo.ListRouteAttempts(ctx, "session-dynamic-route")
	if err != nil {
		t.Fatalf("ListRouteAttempts: %v", err)
	}
	if len(attempts) != 1 || attempts[0].Generation != 7 {
		t.Fatalf("loaded attempts = %#v", attempts)
	}
}

func TestClaimRouteStateDoesNotInsertAfterRestartWithNonZeroExpectation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	state := dynamicruntime.RouteState{
		SessionID: "missing-session", LogicalProfileID: "dynamic-1",
		ExecutionProfileID: "concrete-1", Generation: 2, ProfileVersion: 1,
		Status: "starting", UpdatedAt: time.Now().UTC(),
	}
	claimed, err := repo.ClaimRouteState(ctx, 1, state)
	if err != nil {
		t.Fatalf("ClaimRouteState: %v", err)
	}
	if claimed {
		t.Fatal("claim inserted a route when the expected generation had no durable predecessor")
	}
	loaded, err := repo.LoadRouteState(ctx, state.SessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if loaded != nil {
		t.Fatalf("stale claim created state: %#v", loaded)
	}
}

func TestListPendingRouteStatesExcludesAmbiguousRetryingStates(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, sessionID := range []string{"pending-retry", "pending-reset", "ambiguous-retry"} {
		taskID := "task-" + sessionID
		if err := repo.CreateTask(ctx, &models.Task{ID: taskID, Title: sessionID}); err != nil {
			t.Fatalf("CreateTask(%s): %v", taskID, err)
		}
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput}); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
		}
	}
	for _, state := range []dynamicruntime.RouteState{
		{SessionID: "pending-retry", LogicalProfileID: "dynamic", Generation: 1, Status: "retry_wait", PolicyStateJSON: `{"deadline":"2099-01-01T00:00:00Z"}`, UpdatedAt: now},
		{SessionID: "pending-reset", LogicalProfileID: "dynamic", Generation: 2, Status: "waiting_for_reset", PolicyStateJSON: `{"deadline":"2099-01-01T00:00:00Z"}`, UpdatedAt: now.Add(time.Second)},
		{SessionID: "ambiguous-retry", LogicalProfileID: "dynamic", Generation: 3, Status: "retrying", PolicyStateJSON: `{"deadline":"2099-01-01T00:00:00Z"}`, UpdatedAt: now.Add(2 * time.Second)},
	} {
		if err := repo.SaveRouteState(ctx, state); err != nil {
			t.Fatalf("SaveRouteState(%s): %v", state.SessionID, err)
		}
	}
	states, err := repo.ListPendingRouteStates(ctx)
	if err != nil {
		t.Fatalf("ListPendingRouteStates: %v", err)
	}
	if len(states) != 2 || states[0].SessionID != "pending-retry" || states[1].SessionID != "pending-reset" {
		t.Fatalf("pending states = %#v", states)
	}
}

func TestClaimRouteStateAdvancesGenerationWithFence(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-claim-route", Title: "Claim route"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "claim-session", TaskID: "task-claim-route", State: models.TaskSessionStateStarting,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	now := time.Now().UTC()
	state := dynamicruntime.RouteState{
		SessionID: "claim-session", LogicalProfileID: "dynamic-1",
		ExecutionProfileID: "concrete-1", Generation: 1, ProfileVersion: 1,
		Status: "starting", UpdatedAt: now,
	}
	claimed, err := repo.ClaimRouteState(ctx, 0, state)
	if err != nil || !claimed {
		t.Fatalf("initial ClaimRouteState = %v, %v", claimed, err)
	}
	state.ExecutionProfileID = "concrete-2"
	state.Generation = 2
	state.UpdatedAt = now.Add(time.Second)
	claimed, err = repo.ClaimRouteState(ctx, 1, state)
	if err != nil || !claimed {
		t.Fatalf("successor ClaimRouteState = %v, %v", claimed, err)
	}
	state.ExecutionProfileID = "concrete-3"
	state.Generation = 3
	claimed, err = repo.ClaimRouteState(ctx, 1, state)
	if err != nil {
		t.Fatalf("stale ClaimRouteState: %v", err)
	}
	if claimed {
		t.Fatal("stale claim advanced the route")
	}
	loaded, err := repo.LoadRouteState(ctx, "claim-session")
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if loaded == nil || loaded.Generation != 2 || loaded.ExecutionProfileID != "concrete-2" {
		t.Fatalf("claimed state = %#v", loaded)
	}
}

func TestRecordRouteDecisionClaimsInitialGenerationAndWritesAttemptAtomically(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-atomic-route", Title: "Atomic route"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "atomic-session", TaskID: "task-atomic-route", State: models.TaskSessionStateStarting,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	now := time.Now().UTC()
	decision := dynamicruntime.RouteDecision{
		SessionID: "atomic-session", LogicalProfileID: "dynamic-1",
		ExecutionProfileID: "concrete-1", Generation: 1, ProfileVersion: 2,
		Reason: "candidate_order",
	}
	state := dynamicruntime.RouteState{
		SessionID: decision.SessionID, LogicalProfileID: decision.LogicalProfileID,
		ExecutionProfileID: decision.ExecutionProfileID, Generation: decision.Generation,
		ProfileVersion: decision.ProfileVersion, Status: "starting", UpdatedAt: now,
	}
	if err := repo.RecordRouteDecision(ctx, decision, state); err != nil {
		t.Fatalf("RecordRouteDecision: %v", err)
	}
	loaded, err := repo.LoadRouteState(ctx, decision.SessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if loaded == nil || loaded.Generation != 1 {
		t.Fatalf("loaded state = %#v", loaded)
	}
	attempts, err := repo.ListRouteAttempts(ctx, decision.SessionID)
	if err != nil {
		t.Fatalf("ListRouteAttempts: %v", err)
	}
	if len(attempts) != 1 || attempts[0].ExecutionProfileID != decision.ExecutionProfileID {
		t.Fatalf("loaded attempts = %#v", attempts)
	}
	decision2 := decision
	decision2.ExecutionProfileID = "concrete-2"
	decision2.Generation = 2
	decision2.Reason = "try_next"
	state2 := state
	state2.ExecutionProfileID = decision2.ExecutionProfileID
	state2.Generation = decision2.Generation
	state2.UpdatedAt = now.Add(time.Second)
	if err := repo.RecordRouteDecision(ctx, decision2, state2); err != nil {
		t.Fatalf("RecordRouteDecision successor: %v", err)
	}
	loaded, err = repo.LoadRouteState(ctx, decision.SessionID)
	if err != nil {
		t.Fatalf("LoadRouteState successor: %v", err)
	}
	if loaded == nil || loaded.Generation != 2 || loaded.ExecutionProfileID != "concrete-2" {
		t.Fatalf("successor state = %#v", loaded)
	}
	attempts, err = repo.ListRouteAttempts(ctx, decision.SessionID)
	if err != nil {
		t.Fatalf("ListRouteAttempts successor: %v", err)
	}
	if len(attempts) != 2 || attempts[1].Generation != 2 {
		t.Fatalf("successor attempts = %#v", attempts)
	}
}

func TestUtilityRouteStateIsTransient(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	now := time.Now().UTC()
	decision := dynamicruntime.RouteDecision{
		SessionID: "utility:test", LogicalProfileID: "dynamic-1",
		ExecutionProfileID: "concrete-1", Generation: 1, ProfileVersion: 1,
		Reason: "candidate_order",
	}
	state := dynamicruntime.RouteState{
		SessionID: decision.SessionID, LogicalProfileID: decision.LogicalProfileID,
		ExecutionProfileID: decision.ExecutionProfileID, Generation: 1,
		ProfileVersion: 1, Status: "starting", UpdatedAt: now,
	}
	if err := repo.SaveRouteState(ctx, state); err != nil {
		t.Fatalf("SaveRouteState: %v", err)
	}
	if err := repo.AppendRouteAttempt(ctx, dynamicruntime.RouteAttempt{SessionID: decision.SessionID}); err != nil {
		t.Fatalf("AppendRouteAttempt: %v", err)
	}
	if err := repo.RecordRouteDecision(ctx, decision, state); err != nil {
		t.Fatalf("RecordRouteDecision: %v", err)
	}
	claimed, err := repo.ClaimRouteState(ctx, 0, state)
	if err != nil || !claimed {
		t.Fatalf("ClaimRouteState = %v, %v", claimed, err)
	}
	loaded, err := repo.LoadRouteState(ctx, decision.SessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if loaded != nil {
		t.Fatalf("utility route state persisted: %#v", loaded)
	}
	attempts, err := repo.ListRouteAttempts(ctx, decision.SessionID)
	if err != nil {
		t.Fatalf("ListRouteAttempts: %v", err)
	}
	if len(attempts) != 0 {
		t.Fatalf("utility route attempts = %#v", attempts)
	}
}
