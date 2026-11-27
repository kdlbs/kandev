package orchestrator

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.10
func TestStreamErrorFailureAttributionAfterRouteChange(t *testing.T) {
	for _, tc := range []struct {
		name        string
		profileID   string
		wantCircuit bool
	}{
		{name: "predecessor", profileID: "candidate-a", wantCircuit: false},
		{name: "current candidate", profileID: "candidate-b", wantCircuit: true},
		{name: "legacy event without profile", wantCircuit: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, circuits, generation := newStreamFailureRouteFixture(t)
			before, err := repo.LoadRouteState(context.Background(), "session-stream-failure")
			if err != nil || before == nil {
				t.Fatalf("load route before failure: %#v, %v", before, err)
			}
			wire := map[string]interface{}{
				"type": "agent/event", "agent_type": "codex-acp",
				"task_id": "task-stream-failure", "session_id": "session-stream-failure",
				"execution_id": "execution-stream-failure", "agent_id": "execution-stream-failure",
				"agent_profile_id": "dynamic-stream-failure",
				"owner_kind":       "task",
				"data": map[string]interface{}{
					"type": "error", "prompt_generation": 1,
					"error": "API Error: Repeated 529 Overloaded errors. The API is at capacity.",
				},
			}
			if tc.profileID != "" {
				wire["execution_profile_id"] = tc.profileID
			}
			encoded, err := json.Marshal(wire)
			if err != nil {
				t.Fatalf("marshal stream event: %v", err)
			}
			var payload agentruntime.AgentStreamEventPayload
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatalf("unmarshal stream event: %v", err)
			}
			svc.handleAgentErrorEvent(context.Background(), &payload)

			key := dynamicruntime.ResourceKey(dynamicruntime.ScopeProfile, "candidate-b")
			if got := circuits.IsOpen(key, time.Now()); got != tc.wantCircuit {
				t.Fatalf("candidate-b circuit open = %v, want %v", got, tc.wantCircuit)
			}
			state, err := repo.LoadRouteState(context.Background(), "session-stream-failure")
			if err != nil || state == nil || state.Generation != generation || state.ExecutionProfileID != "candidate-b" {
				t.Fatalf("route state = %#v, %v, want generation %d on candidate-b", state, err, generation)
			}
			if !tc.wantCircuit && *state != *before {
				t.Fatalf("predecessor error changed the successor route: before=%#v, after=%#v", before, state)
			}
		})
	}
}

func newStreamFailureRouteFixture(t *testing.T) (*Service, *sqliterepo.Repository, *dynamicruntime.CircuitRegistry, int64) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-stream-failure", "session-stream-failure", models.TaskSessionStateRunning)
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "task-stream-failure", v1.TaskStateInProgress)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, &mockAgentManager{})
	circuits := dynamicruntime.NewCircuitRegistry()
	resolver := newWorkflowDynamicProfileResolverWithCandidates(t, "dynamic-stream-failure", []workflowDynamicCandidate{
		{executionProfileID: "candidate-a", enabled: true, rulesJSON: `{"on_provider_error":"try_next"}`},
		{executionProfileID: "candidate-b", enabled: true, rulesJSON: `{"on_provider_error":"stop"}`},
		{executionProfileID: "candidate-c", enabled: true},
	}, dynamicruntime.WithPersistence(repo), dynamicruntime.WithCircuitRegistry(circuits))
	svc.SetProfileExecutionResolver(resolver)
	initial, err := resolver.Resolve(ctx, "session-stream-failure", "dynamic-stream-failure", 0, "")
	if err != nil {
		t.Fatalf("initial resolve: %v", err)
	}
	moved, err := resolver.ResolveExecutionAfterFailure(ctx, "session-stream-failure", "dynamic-stream-failure", "candidate-a", initial.Generation, &routingerr.Error{
		Code: routingerr.CodeRateLimited, Class: routingerr.ClassTransient,
		Confidence: routingerr.ConfHigh, FallbackAllowed: true, AutoRetryable: true,
	})
	if err != nil || moved.ExecutionProfileID != "candidate-b" {
		t.Fatalf("successor route = %#v, %v, want candidate-b", moved, err)
	}
	session, err := repo.GetTaskSession(ctx, "session-stream-failure")
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	session.AgentProfileID = "dynamic-stream-failure"
	session.ExecutionProfileID = moved.ExecutionProfileID
	session.RouteGeneration = moved.Generation
	session.RouteState = dynamicRouteStatusActive
	session.AgentExecutionID = "execution-stream-failure"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("project successor route: %v", err)
	}
	svc.beginPromptAttempt(session.ID, session.AgentExecutionID, 1, true)
	return svc, repo, circuits, moved.Generation
}
