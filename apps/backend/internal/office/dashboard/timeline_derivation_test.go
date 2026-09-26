package dashboard

import "testing"

// TestDeriveTaskTimestamps locks down the Started/Completed sidebar
// derivation described in docs/specs/office/requirements/tasks.md: startedAt is the
// earliest transition into in_progress, completedAt is the latest
// transition into done. A regression here silently reverts the Office
// task detail sidebar to permanently blank Started/Completed rows.
func TestDeriveTaskTimestamps(t *testing.T) {
	cases := []struct {
		name          string
		changes       []TimelineEvent
		wantStarted   string
		wantCompleted string
	}{
		{
			name:          "no status changes",
			changes:       nil,
			wantStarted:   "",
			wantCompleted: "",
		},
		{
			name: "todo to in_progress only",
			changes: []TimelineEvent{
				{From: "todo", To: "in_progress", At: "2026-08-01T10:00:00Z"},
			},
			wantStarted:   "2026-08-01T10:00:00Z",
			wantCompleted: "",
		},
		{
			name: "full todo -> in_progress -> done",
			changes: []TimelineEvent{
				// ListActivityEntriesByTarget returns rows DESC by created_at.
				{From: "in_progress", To: "done", At: "2026-08-01T12:00:00Z"},
				{From: "todo", To: "in_progress", At: "2026-08-01T10:00:00Z"},
			},
			wantStarted:   "2026-08-01T10:00:00Z",
			wantCompleted: "2026-08-01T12:00:00Z",
		},
		{
			name: "reopened: done -> in_progress -> done, expect latest done",
			changes: []TimelineEvent{
				{From: "in_progress", To: "done", At: "2026-08-03T09:00:00Z"},
				{From: "done", To: "in_progress", At: "2026-08-02T08:00:00Z"},
				{From: "in_progress", To: "done", At: "2026-08-01T12:00:00Z"},
				{From: "todo", To: "in_progress", At: "2026-08-01T10:00:00Z"},
			},
			wantStarted:   "2026-08-01T10:00:00Z",
			wantCompleted: "2026-08-03T09:00:00Z",
		},
		{
			name: "out-of-order input still resolves min/max",
			changes: []TimelineEvent{
				{From: "todo", To: "in_progress", At: "2026-08-05T10:00:00Z"},
				{From: "in_progress", To: "done", At: "2026-08-01T12:00:00Z"},
				{From: "todo", To: "in_progress", At: "2026-08-01T09:00:00Z"},
			},
			wantStarted:   "2026-08-01T09:00:00Z",
			wantCompleted: "2026-08-01T12:00:00Z",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotStarted, gotCompleted := deriveTaskTimestamps(c.changes)
			if gotStarted != c.wantStarted {
				t.Errorf("startedAt = %q, want %q", gotStarted, c.wantStarted)
			}
			if gotCompleted != c.wantCompleted {
				t.Errorf("completedAt = %q, want %q", gotCompleted, c.wantCompleted)
			}
		})
	}
}

// TestDeriveTaskTimestampsStepNameVariants covers the workflow-engine
// TaskMoved producer, whose activity entries carry raw step names ("In
// Progress", "Done") rather than the already-normalized lowercase status
// strings the direct Office status-update path writes. Both producers
// write the same "task_status_changed" activity action.
func TestDeriveTaskTimestampsStepNameVariants(t *testing.T) {
	changes := []TimelineEvent{
		{From: "In Progress", To: "Done", At: "2026-08-01T12:00:00Z"},
		{From: "Todo", To: "In Progress", At: "2026-08-01T10:00:00Z"},
	}
	gotStarted, gotCompleted := deriveTaskTimestamps(changes)
	if gotStarted != "2026-08-01T10:00:00Z" {
		t.Errorf("startedAt = %q, want 2026-08-01T10:00:00Z", gotStarted)
	}
	if gotCompleted != "2026-08-01T12:00:00Z" {
		t.Errorf("completedAt = %q, want 2026-08-01T12:00:00Z", gotCompleted)
	}
}
