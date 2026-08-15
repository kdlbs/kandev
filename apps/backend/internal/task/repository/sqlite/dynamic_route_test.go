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

func TestRecordRouteDecisionClaimsInitialGenerationAndWritesAttemptAtomically(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
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
}
