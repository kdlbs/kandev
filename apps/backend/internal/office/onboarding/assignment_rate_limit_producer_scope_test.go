package onboarding

import (
	"context"
	"encoding/json"
	"testing"

	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// fakeOnboardingRunQueuer is a minimal shared.RunQueuer fake that captures
// the payload maybeCreateOnboardingTask enqueues, so the test can inspect
// it directly instead of inferring shape from a persisted row.
type fakeOnboardingRunQueuer struct {
	reason, payload string
	called          bool
}

func (f *fakeOnboardingRunQueuer) QueueRun(
	_ context.Context, _, reason, payload, _ string,
) (runsservice.QueueOutcome, error) {
	f.called = true
	f.reason = reason
	f.payload = payload
	return runsservice.QueueOutcomeQueued, nil
}

// TestMaybeCreateOnboardingTask_NeverCarriesAgentActorType pins
// AC-OFFICE-ASSIGN-RATE-001.12 for the onboarding task_assigned producer
// (documented in service.go as "a third task_assigned producer"): its
// payload is fmt.Sprintf(`{"task_id":%q}`, taskID), with no actor_type
// field at all, so it can never fall into the assignment-wake-rate-limit's
// scope.
func TestMaybeCreateOnboardingTask_NeverCarriesAgentActorType(t *testing.T) {
	svc, _, _ := newTestOnboardingService(t)
	svc.taskCreator = &mockTaskCreatorOnboarding{}
	queuer := &fakeOnboardingRunQueuer{}
	svc.runQueuer = queuer

	taskID := svc.maybeCreateOnboardingTask(context.Background(), "ws-1", "agent-1", CompleteRequest{
		TaskTitle:       "Explore the codebase",
		TaskDescription: "Create an engineering roadmap",
	})
	if taskID == "" {
		t.Fatal("expected a created task ID")
	}
	if !queuer.called {
		t.Fatal("expected QueueRun to be called")
	}
	if queuer.reason != runReasonTaskAssigned {
		t.Fatalf("reason = %q, want %q", queuer.reason, runReasonTaskAssigned)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(queuer.payload), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if actorType, present := payload["actor_type"]; present && actorType == "agent" {
		t.Fatalf("onboarding task_assigned payload carries actor_type=agent: %v", payload)
	}
}
