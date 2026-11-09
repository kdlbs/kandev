package models

import (
	"testing"
	"time"
)

func TestWorkspaceInventoryRepairFenceEqualComparesEveryField(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	fence := func() *WorkspaceInventoryRepairFence {
		return &WorkspaceInventoryRepairFence{
			TaskUpdatedAt: base, WorkflowStepID: "step", SessionUpdatedAt: base.Add(time.Second),
			SessionState: string(TaskSessionStateFailed), RouteKnown: true, RouteGeneration: 2,
			RouteState: "active", RouteUpdatedAt: base.Add(2 * time.Second),
		}
	}
	tests := []struct {
		name   string
		mutate func(*WorkspaceInventoryRepairFence)
	}{
		{name: "task updated", mutate: func(f *WorkspaceInventoryRepairFence) { f.TaskUpdatedAt = f.TaskUpdatedAt.Add(time.Nanosecond) }},
		{name: "workflow step", mutate: func(f *WorkspaceInventoryRepairFence) { f.WorkflowStepID = "other-step" }},
		{name: "session updated", mutate: func(f *WorkspaceInventoryRepairFence) { f.SessionUpdatedAt = f.SessionUpdatedAt.Add(time.Nanosecond) }},
		{name: "session state", mutate: func(f *WorkspaceInventoryRepairFence) { f.SessionState = string(TaskSessionStateCancelled) }},
		{name: "route known", mutate: func(f *WorkspaceInventoryRepairFence) { f.RouteKnown = false }},
		{name: "route generation", mutate: func(f *WorkspaceInventoryRepairFence) { f.RouteGeneration++ }},
		{name: "route state", mutate: func(f *WorkspaceInventoryRepairFence) { f.RouteState = "selecting" }},
		{name: "route updated", mutate: func(f *WorkspaceInventoryRepairFence) { f.RouteUpdatedAt = f.RouteUpdatedAt.Add(time.Nanosecond) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := fence()
			tt.mutate(changed)
			if fence().Equal(changed) || changed.Equal(fence()) {
				t.Fatalf("fences differing in %s compared equal", tt.name)
			}
		})
	}

	if !fence().Equal(fence()) {
		t.Fatal("identical fences must compare equal")
	}
	sameInstant := fence()
	sameInstant.TaskUpdatedAt = base.In(time.FixedZone("offset", 3600))
	if !fence().Equal(sameInstant) {
		t.Fatal("the same instant in another location must compare equal")
	}
	var missing *WorkspaceInventoryRepairFence
	if !missing.Equal(nil) {
		t.Fatal("two absent fences must compare equal")
	}
	if missing.Equal(fence()) || fence().Equal(nil) {
		t.Fatal("an absent fence must never equal a present one")
	}
}
