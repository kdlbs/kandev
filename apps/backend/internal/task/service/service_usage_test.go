package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// Service-layer authorization coverage for the task-cost-ledger read surface
// (docs/specs/task-cost-ledger/spec.md AC-18): GetTaskUsageTotals and
// GetTaskSessionUsageTotals must apply the same per-user workspace scoping as
// every other task/session-keyed entry point (see service_access_test.go).

func seedUsageEvent(t *testing.T, repo interface {
	CreateTaskUsageEvent(context.Context, *models.TaskUsageEvent) error
}, usageEventID, taskID, sessionID string) {
	t.Helper()
	tokensCachedRead := int64(20)
	tokensCachedWrite := int64(5)
	tokensOut := int64(30)
	event := &models.TaskUsageEvent{
		UsageEventID:      usageEventID,
		TaskID:            taskID,
		SessionID:         sessionID,
		AgentType:         "claude",
		Model:             "claude-sonnet-5",
		Provider:          "anthropic",
		TokensIn:          100,
		TokensCachedRead:  &tokensCachedRead,
		TokensCachedWrite: &tokensCachedWrite,
		TokensOut:         &tokensOut,
		TokensTotal:       155,
		CostSubcents:      42,
		CostSource:        "actual",
		ContractVersion:   1,
	}
	if err := repo.CreateTaskUsageEvent(context.Background(), event); err != nil {
		t.Fatalf("seed usage event %s: %v", usageEventID, err)
	}
}

func TestGetTaskUsageTotals_ScopingAndAggregation(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	seedUsageEvent(t, repo, "evt-task-usage-1", "task-b", "sess-b")

	if _, err := svc.GetTaskUsageTotals(ctxAs("user-a"), "task-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign GetTaskUsageTotals: %v", err)
	}

	for name, ctx := range map[string]context.Context{
		"owner":    ctxAs("user-b"),
		"internal": context.Background(),
	} {
		totals, err := svc.GetTaskUsageTotals(ctx, "task-b")
		if err != nil {
			t.Fatalf("%s GetTaskUsageTotals: %v", name, err)
		}
		if totals.EventCount != 1 {
			t.Fatalf("%s EventCount = %d, want 1", name, totals.EventCount)
		}
		if totals.TokensIn != 100 {
			t.Fatalf("%s TokensIn = %d, want 100", name, totals.TokensIn)
		}
	}
}

func TestGetTaskUsageTotals_UnknownTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)

	for name, ctx := range map[string]context.Context{
		"scoped":   ctxAs("user-b"),
		"internal": context.Background(),
	} {
		if _, err := svc.GetTaskUsageTotals(ctx, "task-does-not-exist"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
			t.Fatalf("%s unknown task: %v", name, err)
		}
	}
}

func TestGetTaskSessionUsageTotals_ScopingAndAggregation(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	seedUsageEvent(t, repo, "evt-session-usage-1", "task-b", "sess-b")

	if _, err := svc.GetTaskSessionUsageTotals(ctxAs("user-a"), "task-b", "sess-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign GetTaskSessionUsageTotals: %v", err)
	}

	for name, ctx := range map[string]context.Context{
		"owner":    ctxAs("user-b"),
		"internal": context.Background(),
	} {
		totals, err := svc.GetTaskSessionUsageTotals(ctx, "task-b", "sess-b")
		if err != nil {
			t.Fatalf("%s GetTaskSessionUsageTotals: %v", name, err)
		}
		if totals.EventCount != 1 {
			t.Fatalf("%s EventCount = %d, want 1", name, totals.EventCount)
		}
	}
}

func TestGetTaskSessionUsageTotals_MismatchedPairAndUnknownSession(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	if err := repo.CreateTask(context.Background(), &models.Task{
		ID: "task-b-other", WorkspaceID: "ws-b", WorkflowID: "wf-b", WorkflowStepID: "step-1",
		Title: "B's other task", State: "created", Priority: "medium",
	}); err != nil {
		t.Fatalf("create other task: %v", err)
	}

	for name, ctx := range map[string]context.Context{
		"owner":    ctxAs("user-b"),
		"internal": context.Background(),
	} {
		if _, err := svc.GetTaskSessionUsageTotals(ctx, "task-b-other", "sess-b"); !errors.Is(err, repoerrors.ErrTaskNotFound) {
			t.Fatalf("%s mismatched pair: %v", name, err)
		}
		// An outright unknown session ID surfaces models.ErrTaskSessionNotFound
		// (not repoerrors.ErrTaskNotFound) - AuthorizeTaskSessionAccess only
		// substitutes the task sentinel for a mismatched real session/task pair,
		// per its own doc comment. handlers.isNotFound's "not found" substring
		// fallback still maps this to HTTP 404.
		if _, err := svc.GetTaskSessionUsageTotals(ctx, "task-b", "sess-does-not-exist"); !errors.Is(err, models.ErrTaskSessionNotFound) {
			t.Fatalf("%s unknown session: %v", name, err)
		}
	}
}

func TestGetTaskUsageTotals_ZeroUsageReturnsZeroedTotals(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)

	totals, err := svc.GetTaskUsageTotals(ctxAs("user-b"), "task-b")
	if err != nil {
		t.Fatalf("GetTaskUsageTotals: %v", err)
	}
	if totals.EventCount != 0 {
		t.Fatalf("EventCount = %d, want 0", totals.EventCount)
	}
	if !totals.OutputTokensComplete {
		t.Error("OutputTokensComplete = false, want true for zero usage")
	}
	if totals.FirstEventAt != nil || totals.LastEventAt != nil {
		t.Errorf("timestamps = (%v, %v), want (nil, nil)", totals.FirstEventAt, totals.LastEventAt)
	}
}
