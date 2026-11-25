package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/events/bus"
)

type permissionRequestEventPayload struct {
	TaskID               string `json:"task_id"`
	SessionID            string `json:"session_id"`
	PendingID            string `json:"pending_id"`
	AutoApprovedOptionID string `json:"auto_approved_option_id,omitempty"`
	AutoApprovalPending  bool   `json:"auto_approval_pending,omitempty"`
}

func TestPermissionNotificationOccurrenceAcceptsTypedAndMapPayloads(t *testing.T) {
	typed := permissionRequestEventPayload{TaskID: "t1", SessionID: "s1", PendingID: "p1"}
	asMap := map[string]interface{}{"task_id": "t1", "session_id": "s1", "pending_id": "p1"}
	for name, data := range map[string]any{"struct": typed, "map": asMap} {
		taskID, sessionID, pendingID, ok := permissionNotificationOccurrence(data)
		if !ok || taskID != "t1" || sessionID != "s1" || pendingID != "p1" {
			t.Errorf("%s: got %q %q %q %v", name, taskID, sessionID, pendingID, ok)
		}
	}
}

func TestPermissionNotificationOccurrenceSkipsAutoAnsweredAndIncomplete(t *testing.T) {
	cases := map[string]permissionRequestEventPayload{
		"auto approved": {TaskID: "t1", SessionID: "s1", PendingID: "p1", AutoApprovedOptionID: "allow"},
		"auto pending":  {TaskID: "t1", SessionID: "s1", PendingID: "p1", AutoApprovalPending: true},
		"no pending id": {TaskID: "t1", SessionID: "s1"},
		"no session":    {TaskID: "t1", PendingID: "p1"},
	}
	for name, payload := range cases {
		if _, _, _, ok := permissionNotificationOccurrence(payload); ok {
			t.Errorf("%s: must not notify", name)
		}
	}
}

func TestFailedSessionOccurrenceFiresOnlyOnTransitionIntoFailed(t *testing.T) {
	event := func(old, next, updated string) *bus.Event {
		return &bus.Event{ID: "evt-1", Data: map[string]interface{}{
			"task_id": "t1", "session_id": "s1", "old_state": old, "new_state": next, "updated_at": updated,
		}}
	}
	taskID, sessionID, occurrence, ok := failedSessionOccurrence(event("RUNNING", "FAILED", "2026-01-01T00:00:00Z"))
	if !ok || taskID != "t1" || sessionID != "s1" || occurrence != "s1:failed:2026-01-01T00:00:00Z" {
		t.Fatalf("got %q %q %q %v", taskID, sessionID, occurrence, ok)
	}
	if _, _, occ, _ := failedSessionOccurrence(event("RUNNING", "FAILED", "2026-02-02T00:00:00Z")); occ == occurrence {
		t.Error("separate failures of one session must produce distinct occurrence IDs")
	}
	if _, _, occ, ok := failedSessionOccurrence(event("RUNNING", "FAILED", "")); !ok || occ != "s1:failed:evt-1" {
		t.Errorf("missing updated_at should fall back to the event id, got %q %v", occ, ok)
	}
	for name, e := range map[string]*bus.Event{
		"already failed": event("FAILED", "FAILED", "x"),
		"other state":    event("RUNNING", "COMPLETED", "x"),
	} {
		if _, _, _, ok := failedSessionOccurrence(e); ok {
			t.Errorf("%s: must not notify", name)
		}
	}
}
