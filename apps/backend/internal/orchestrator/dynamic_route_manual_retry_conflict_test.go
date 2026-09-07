package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingpolicy"
	"github.com/kandev/kandev/internal/task/models"
)

// seedDueDynamicRetryWaitRoute claims generation 1 for a two-candidate dynamic
// profile through the real resolver, then persists a durable retry_wait state
// with an already-elapsed deadline - the same shape a policy-classified
// transient failure leaves behind, and the precondition both
// ProfileExecutionResolver.ResumePendingRoute (the recovery-timer path) and
// ProfileExecutionResolver.ResolveRouteAction "retry" (the manual path) act on.
func seedDueDynamicRetryWaitRoute(
	t *testing.T,
	ctx context.Context,
	repo interface {
		GetTaskSession(context.Context, string) (*models.TaskSession, error)
		UpdateTaskSession(context.Context, *models.TaskSession) error
		SaveRouteState(context.Context, dynamicruntime.RouteState) error
	},
	dynamicProfileID, sessionID string,
) {
	t.Helper()
	deadline := time.Now().Add(-time.Minute)
	policyState := dynamicruntime.PolicyState{Deadline: &deadline}
	raw, err := json.Marshal(policyState)
	if err != nil {
		t.Fatalf("marshal policy state: %v", err)
	}
	if err := repo.SaveRouteState(ctx, dynamicruntime.RouteState{
		SessionID:          sessionID,
		LogicalProfileID:   dynamicProfileID,
		ExecutionProfileID: "candidate-1",
		Generation:         1,
		ProfileVersion:     1,
		Status:             string(routingpolicy.DecisionRetry),
		PolicyStateJSON:    string(raw),
		UpdatedAt:          time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed retry_wait route state: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.AgentProfileID = dynamicProfileID
	session.ExecutionProfileID = "candidate-1"
	session.RouteGeneration = 1
	session.RouteState = string(routingpolicy.DecisionRetry)
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}
}

// TestResolveRetryRouteAction_TimerResumeThenManualRetrySameGeneration is the
// regression test for the manual-retry fall-through: once the recovery timer
// (ProfileExecutionResolver.ResumePendingRoute, as called from
// runDynamicPolicyRecovery) has resumed a due retry_wait route to "retrying",
// a manual Retry click observing the same expected generation must be told
// its view is stale instead of claiming a second, competing successor
// generation via the unfenced-by-status resolve() fallback.
func TestResolveRetryRouteAction_TimerResumeThenManualRetrySameGeneration(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-manual-retry-after-timer"
		sessionID = "session-manual-retry-after-timer"
		dynamicID = "dynamic-manual-retry-after-timer"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, dynamicID, []workflowDynamicCandidate{
		{executionProfileID: "candidate-1", enabled: true},
		{executionProfileID: "candidate-2", enabled: true},
	}, dynamicruntime.WithPersistence(repo), dynamicruntime.WithStateLoader(repo))
	seedDueDynamicRetryWaitRoute(t, ctx, repo, dynamicID, sessionID)

	// The recovery timer resumes generation 1 first.
	timerResult, err := resolver.ResumePendingRoute(ctx, sessionID, 1)
	if err != nil {
		t.Fatalf("timer ResumePendingRoute: %v", err)
	}
	if timerResult.Generation != 1 || timerResult.Decision.Status != "retrying" {
		t.Fatalf("timer result = %+v, want generation 1 status retrying", timerResult)
	}

	// A manual retry click still observing generation 1 must not claim a
	// second, independent generation.
	_, err = resolver.ResolveRouteAction(ctx, sessionID, dynamicID, "candidate-1", 1, "retry")
	if !errors.Is(err, dynamicruntime.ErrRecoveryPending) {
		t.Fatalf("manual retry after timer resume error = %v, want %v", err, dynamicruntime.ErrRecoveryPending)
	}

	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Generation != 1 || state.Status != "retrying" {
		t.Fatalf("durable route state = %#v, want unchanged generation 1 status retrying (exactly one decision)", state)
	}
}

// TestResolveRetryRouteAction_DoubleManualRetrySameGeneration is the
// regression test for the more likely real-world trigger: two Retry clicks
// racing (or simply double-clicked) with no recovery timer involved at all.
// The first claims the resume; the second, still observing the pre-resume
// generation, must be rejected rather than launching a second successor.
func TestResolveRetryRouteAction_DoubleManualRetrySameGeneration(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-double-manual-retry"
		sessionID = "session-double-manual-retry"
		dynamicID = "dynamic-double-manual-retry"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, dynamicID, []workflowDynamicCandidate{
		{executionProfileID: "candidate-1", enabled: true},
		{executionProfileID: "candidate-2", enabled: true},
	}, dynamicruntime.WithPersistence(repo), dynamicruntime.WithStateLoader(repo))
	seedDueDynamicRetryWaitRoute(t, ctx, repo, dynamicID, sessionID)

	first, err := resolver.ResolveRouteAction(ctx, sessionID, dynamicID, "candidate-1", 1, "retry")
	if err != nil {
		t.Fatalf("first manual retry: %v", err)
	}
	if first.Generation != 1 || first.ExecutionProfileID != "candidate-1" {
		t.Fatalf("first manual retry result = %+v, want generation 1 candidate-1", first)
	}

	_, err = resolver.ResolveRouteAction(ctx, sessionID, dynamicID, "candidate-1", 1, "retry")
	if !errors.Is(err, dynamicruntime.ErrRecoveryPending) {
		t.Fatalf("second manual retry error = %v, want %v", err, dynamicruntime.ErrRecoveryPending)
	}

	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Generation != 1 || state.Status != "retrying" {
		t.Fatalf("durable route state = %#v, want unchanged generation 1 status retrying (exactly one decision)", state)
	}
}

func TestResolveRetryRouteAction_ReclaimsPersistedRetryingAfterRestart(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-retry-after-restart"
		sessionID = "session-retry-after-restart"
		dynamicID = "dynamic-retry-after-restart"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	seedDueDynamicRetryWaitRoute(t, ctx, repo, dynamicID, sessionID)

	firstResolver := newWorkflowDynamicProfileResolverWithCandidates(t, dynamicID, []workflowDynamicCandidate{
		{executionProfileID: "candidate-1", enabled: true},
		{executionProfileID: "candidate-2", enabled: true},
	}, dynamicruntime.WithPersistence(repo), dynamicruntime.WithStateLoader(repo))
	if _, err := firstResolver.ResumePendingRoute(ctx, sessionID, 1); err != nil {
		t.Fatalf("timer resume: %v", err)
	}

	restartedResolver := newWorkflowDynamicProfileResolverWithCandidates(t, dynamicID, []workflowDynamicCandidate{
		{executionProfileID: "candidate-1", enabled: true},
		{executionProfileID: "candidate-2", enabled: true},
	}, dynamicruntime.WithPersistence(repo), dynamicruntime.WithStateLoader(repo))
	result, err := restartedResolver.ResolveRouteAction(ctx, sessionID, dynamicID, "candidate-1", 1, "retry")
	if err != nil {
		t.Fatalf("manual retry after restart: %v", err)
	}
	if result.Generation != 2 || result.ExecutionProfileID != "candidate-1" || result.Decision.Status != "starting" {
		t.Fatalf("result = %+v, want generation 2 candidate-1 starting", result)
	}
	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Generation != 2 || state.Status != "starting" {
		t.Fatalf("durable route state = %#v, want fenced successor", state)
	}
}

// TestResolveRetryRouteAction_NoExistingRouteStateStillResolvesFresh proves
// the fall-through is preserved for its legitimate case: a retry action on a
// session with no durable route state yet (first launch via a retry action).
func TestResolveRetryRouteAction_NoExistingRouteStateStillResolvesFresh(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-manual-retry-fresh"
		sessionID = "session-manual-retry-fresh"
		dynamicID = "dynamic-manual-retry-fresh"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, dynamicID, []workflowDynamicCandidate{
		{executionProfileID: "candidate-1", enabled: true},
	}, dynamicruntime.WithPersistence(repo), dynamicruntime.WithStateLoader(repo))
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.AgentProfileID = dynamicID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}

	result, err := resolver.ResolveRouteAction(ctx, sessionID, dynamicID, "", 0, "retry")
	if err != nil {
		t.Fatalf("manual retry with no existing route state: %v", err)
	}
	if result.ExecutionProfileID != "candidate-1" || result.Generation != 1 {
		t.Fatalf("result = %+v, want fresh generation 1 candidate-1", result)
	}
}

// TestResolveRetryRouteAction_FromActionRequiredStillResumes is the
// not-regressed case: a route already at durable action_required must still
// resume at the same generation and execution profile through the manual
// retry path, since force=true never gates on that status.
func TestResolveRetryRouteAction_FromActionRequiredStillResumes(t *testing.T) {
	ctx := context.Background()
	const (
		taskID    = "task-manual-retry-action-required"
		sessionID = "session-manual-retry-action-required"
		dynamicID = "dynamic-manual-retry-action-required"
	)
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateWaitingForInput)
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, dynamicID, []workflowDynamicCandidate{
		{executionProfileID: "candidate-1", enabled: true},
		{executionProfileID: "candidate-2", enabled: true},
	}, dynamicruntime.WithPersistence(repo), dynamicruntime.WithStateLoader(repo))
	if err := repo.SaveRouteState(ctx, dynamicruntime.RouteState{
		SessionID: sessionID, LogicalProfileID: dynamicID,
		ExecutionProfileID: "candidate-1", Generation: 1, ProfileVersion: 1,
		Status: "action_required", UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed action_required route state: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.AgentProfileID = dynamicID
	session.ExecutionProfileID = "candidate-1"
	session.RouteGeneration = 1
	session.RouteState = "action_required"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("UpdateTaskSession: %v", err)
	}

	result, err := resolver.ResolveRouteAction(ctx, sessionID, dynamicID, "candidate-1", 1, "retry")
	if err != nil {
		t.Fatalf("manual retry from action_required: %v", err)
	}
	if result.Generation != 1 || result.ExecutionProfileID != "candidate-1" {
		t.Fatalf("result = %+v, want unchanged generation 1 candidate-1", result)
	}

	state, err := repo.LoadRouteState(ctx, sessionID)
	if err != nil {
		t.Fatalf("LoadRouteState: %v", err)
	}
	if state == nil || state.Status != "retrying" || state.Generation != 1 {
		t.Fatalf("durable route state = %#v, want generation 1 status retrying", state)
	}
}
