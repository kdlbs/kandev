package sqlite

import (
	"context"
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
		Status: "active", UpdatedAt: now,
	}
	if err := repo.SaveRouteState(ctx, state); err != nil {
		t.Fatalf("SaveRouteState: %v", err)
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
	}
	if loaded.Generation != 7 || loaded.ExecutionProfileID != "concrete-1" {
		t.Fatalf("loaded state = %#v", loaded)
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
