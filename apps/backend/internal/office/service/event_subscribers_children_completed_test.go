package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestTaskMovedTerminalToTerminalDoesNotWakeParent(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	event := bus.NewEvent(events.TaskMoved, "test", map[string]string{
		"task_id":                   "parent-1-child-0",
		"workspace_id":              "ws-1",
		"from_step_name":            "Cancelled",
		"to_step_name":              "Done",
		"assignee_agent_profile_id": "worker-1",
		"parent_id":                 "parent-1",
	})
	if err := eb.Publish(context.Background(), events.TaskMoved, event); err != nil {
		t.Fatalf("publish task.moved: %v", err)
	}

	if calls := dispatcher.Calls(); len(calls) != 0 {
		t.Fatalf("terminal-to-terminal move dispatched a parent wake: %#v", calls)
	}
}
