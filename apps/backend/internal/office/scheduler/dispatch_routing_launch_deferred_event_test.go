package scheduler_test

// RR19-F3 follow-up: REQ-OFFICE-BACKPRESSURE-003 and
// REQ-OFFICE-LAUNCH-SAFETY-003 both require a durable operator-visible
// record whenever a gate blocks a launch. handleLaunchDeferred in
// service/scheduler_integration.go (the legacy direct-launch fallback)
// already wrote one; its sibling in this file (the routed-dispatch path)
// did not, so a run deferred by the orchestrator's session ceiling was
// silently unrecorded whenever routing was enabled. This test pins that
// both park call sites now write the same durable record.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/scheduler"
	"github.com/kandev/kandev/internal/office/service"
)

func TestDispatchWithRouting_LaunchDeferredByCapacity_RecordsDurableRunEvent(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp"})
	starter := newFakeTaskStarter()
	starter.failFor["claude-acp"] = service.ErrLaunchDeferredByCapacity
	ss := buildScheduler(t, repo, starter)

	run := seedRun(t, repo, `{"task_id":"t-deferred-1"}`)

	launched, parked, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if launched || !parked {
		t.Fatalf("launched=%v parked=%v, want launched=false parked=true", launched, parked)
	}

	events, err := repo.ListRunEvents(context.Background(), run.ID, -1, 0)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	var found bool
	for _, e := range events {
		if string(e.EventType) != "adapter.invoke" {
			continue
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(e.Payload), &payload); err != nil {
			t.Fatalf("unmarshal payload: %v", err)
		}
		if payload["phase"] == "deferred" && payload["reason"] == "session_ceiling" {
			found = true
		}
	}
	if !found {
		t.Fatalf("events = %+v, want a durable adapter.invoke event with phase=deferred reason=session_ceiling", events)
	}
}
