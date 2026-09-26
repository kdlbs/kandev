package dashboard_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// stubQuorumDispatcher satisfies both shared.WorkflowEngineDispatcher
// (HandleTrigger, unused here) and the package-local quorumEvaluatingDispatcher
// capability, letting these tests control EvaluateStepQuorum's return value
// directly instead of routing through a real engine/session setup.
type stubQuorumDispatcher struct {
	snapshot engine.QuorumSnapshot
	err      error
}

func (s *stubQuorumDispatcher) HandleTrigger(
	_ context.Context, _ string, _ engine.Trigger, _ any, _ string,
) error {
	return nil
}

func (s *stubQuorumDispatcher) EvaluateStepQuorum(
	_ context.Context, _ string,
) (engine.QuorumSnapshot, error) {
	return s.snapshot, s.err
}

// TestGetTaskQuorum_DispatcherNotWired is AC-57c/57d's "no office-side
// fallback" contract: when the engine dispatcher isn't wired, the AC-24b
// read side must fail closed exactly like the write side
// (decisionRecordingDispatcher in decisions.go), never report a healthy
// empty result. newTestDeps' fixture wires a real dispatcher, so this test
// swaps in one that satisfies only the base HandleTrigger capability, not
// quorumEvaluatingDispatcher.
type baseOnlyDispatcher struct{}

func (baseOnlyDispatcher) HandleTrigger(
	_ context.Context, _ string, _ engine.Trigger, _ any, _ string,
) error {
	return nil
}

func TestGetTaskQuorum_DispatcherNotWired(t *testing.T) {
	deps := newTestDeps(t)
	deps.svc.SetWorkflowEngineDispatcher(baseOnlyDispatcher{})

	_, err := deps.svc.GetTaskQuorum(context.Background(), "any-task")
	if err == nil {
		t.Fatalf("GetTaskQuorum: got nil error, want an error (AC-57c: no office-side fallback)")
	}
}

func TestGetTaskQuorum_PropagatesEvaluationError(t *testing.T) {
	deps := newTestDeps(t)
	wantErr := errors.New("boom")
	deps.svc.SetWorkflowEngineDispatcher(&stubQuorumDispatcher{err: wantErr})

	_, err := deps.svc.GetTaskQuorum(context.Background(), "any-task")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestGetTaskQuorum_ProjectsSnapshotGuards(t *testing.T) {
	deps := newTestDeps(t)
	deps.svc.SetWorkflowEngineDispatcher(&stubQuorumDispatcher{
		snapshot: engine.QuorumSnapshot{
			StepID:              "step-1",
			ReevaluationBlocked: true,
			Guards: []engine.QuorumGuardState{
				{
					TargetStepID:  "step-2",
					Role:          "approver",
					Threshold:     "all_approve",
					RequiredCount: 2,
					ReceivedCount: 1,
					Satisfied:     false,
					Reason:        "threshold_not_met",
				},
			},
		},
	})

	resp, err := deps.svc.GetTaskQuorum(context.Background(), "any-task")
	if err != nil {
		t.Fatalf("GetTaskQuorum: %v", err)
	}
	if !resp.ReevaluationBlocked {
		t.Fatalf("ReevaluationBlocked = false, want true")
	}
	if len(resp.Guards) != 1 {
		t.Fatalf("Guards = %d, want 1", len(resp.Guards))
	}
	g := resp.Guards[0]
	if g.TargetStepID != "step-2" || g.Role != "approver" || g.Threshold != "all_approve" ||
		g.RequiredCount != 2 || g.ReceivedCount != 1 || g.Satisfied || g.Reason != "threshold_not_met" {
		t.Fatalf("Guards[0] = %#v, unexpected projection", g)
	}
}

// TestGetTaskQuorumEndpoint_NotFound is AC-24b/F37: task existence is
// resolved at the handler layer (mirroring getTask), 404ing before
// EvaluateStepQuorum is ever reached.
func TestGetTaskQuorumEndpoint_NotFound(t *testing.T) {
	deps := newTestDeps(t)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/office/workspaces/ws-q/tasks/does-not-exist/quorum", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// TestGetTaskQuorumEndpoint_EmptyListForNoGuardedTransition is AC-24b's
// baseline: a real task with a bound step and no active session in this
// fixture (fakeSessionResolver) returns 200 with an empty guard list rather
// than an error.
func TestGetTaskQuorumEndpoint_EmptyListForNoGuardedTransition(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "q1", "ws-q", "Q", "in_review", 2)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/office/workspaces/ws-q/tasks/q1/quorum", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp dashboard.QuorumResponseDTO
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Guards) != 0 {
		t.Fatalf("Guards = %d, want 0", len(resp.Guards))
	}
	if resp.ReevaluationBlocked {
		t.Fatalf("ReevaluationBlocked = true, want false")
	}
}

// TestGetTaskQuorumEndpoint_EmptyListForUnboundStep is AC-24c: a task with
// no workflow_step_id bound at all returns 200 with an empty list, not an
// error.
func TestGetTaskQuorumEndpoint_EmptyListForUnboundStep(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTaskNoStep(t, deps, "q2", "ws-q", "Q2")

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/office/workspaces/ws-q/tasks/q2/quorum", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp dashboard.QuorumResponseDTO
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Guards) != 0 {
		t.Fatalf("Guards = %d, want 0", len(resp.Guards))
	}
}

func TestGetTaskQuorumEndpoint_RejectsWrongWorkspace(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "q3", "ws-q", "Q3", "in_review", 2)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/office/workspaces/ws-other/tasks/q3/quorum", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
