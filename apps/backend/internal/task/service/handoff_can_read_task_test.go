package service

import (
	"context"
	"testing"

	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// CanReadTask is the exported read-relation guard the Office runtime uses to
// scope blocked_by. It must answer exactly as the document and comment read
// guard does, so this pins the cases blocker scope relies on.
//
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.2
func TestHandoffServiceCanReadTask(t *testing.T) {
	f := newChildRelationsFixture(t, "ws-can-read")
	ctx := context.Background()
	taskRepo, ok := f.svc.tasks.(*sqliterepo.Repository)
	if !ok {
		t.Fatalf("task repository type = %T", f.svc.tasks)
	}
	handoff := NewHandoffService(taskRepo, nil, nil, f.officeRep, f.officeRep, nil)

	parent := f.createRoot(t, "ws-can-read", "Parent", nil)
	unrelated := f.createRoot(t, "ws-can-read", "Unrelated", nil)
	childA, err := f.svc.CreateChildTask(ctx, parent, ChildTaskSpec{Title: "A"})
	if err != nil {
		t.Fatalf("create child A: %v", err)
	}
	childB, err := f.svc.CreateChildTask(ctx, parent, ChildTaskSpec{Title: "B"})
	if err != nil {
		t.Fatalf("create child B: %v", err)
	}

	cases := []struct {
		name            string
		caller, target  string
		wantReadAllowed bool
	}{
		{"self", parent.ID, parent.ID, true},
		{"descendant", parent.ID, childA, true},
		{"ancestor", childA, parent.ID, true},
		{"sibling", childA, childB, true},
		{"unrelated root", parent.ID, unrelated.ID, false},
		{"missing target", parent.ID, "missing-task", false},
		{"missing caller", "", parent.ID, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := handoff.CanReadTask(ctx, tc.caller, tc.target)
			if err != nil {
				t.Fatalf("CanReadTask: %v", err)
			}
			if got != tc.wantReadAllowed {
				t.Fatalf("CanReadTask(%s, %s) = %v, want %v", tc.caller, tc.target, got, tc.wantReadAllowed)
			}
		})
	}
}
