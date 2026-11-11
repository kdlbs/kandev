package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/scheduler"
	"github.com/kandev/kandev/internal/office/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// humanDoneBlockerWakeCase describes a blocker that reaches its final step
// through the task.moved path (a human Done on a task that is not yet on its
// final step). The blocked task must get a blockers-resolved run.
type humanDoneBlockerWakeCase struct {
	blockerAssignee string
	// payload is the task.moved body. It is merged over the blocker ID.
	payload map[string]any
}

// runHumanDoneTaskMovedWake drives a blocker through a task.moved event and
// returns how many blockers-resolved runs were queued for the blocked task.
func runHumanDoneTaskMovedWake(t *testing.T, c humanDoneBlockerWakeCase) int {
	t.Helper()
	ctx := context.Background()
	f := newAssignmentBlockerFixture(t)
	f.task(t, "dependent", "assignment-work", v1.TaskStateTODO, "assignment-worker")
	f.blockers(t, "dependent", v1.TaskStateInProgress)
	f.wireResolutionEngine(t)

	blocker := "dependent-prerequisite-0"
	if c.blockerAssignee != "" {
		_, err := f.repo.UpdateTaskAssignee(ctx, blocker, c.blockerAssignee)
		blockerMust(t, err)
	}
	blockerMust(t, f.tasks.UpdateTaskState(ctx, blocker, v1.TaskStateCompleted))

	data := map[string]any{"task_id": blocker}
	for k, v := range c.payload {
		data[k] = v
	}
	blockerMust(t, f.eb.Publish(ctx, events.TaskMoved, bus.NewEvent(events.TaskMoved, "human-done-test", data)))
	return len(f.taskRuns(t, "dependent", service.RunReasonTaskBlockersResolved))
}

// productionTaskMovedPayload mirrors the fields task/service needs to publish
// for Office completion handling. Step names are not required.
func productionTaskMovedPayload(assignee string) map[string]any {
	return map[string]any{
		"workspace_id":              "assignment-ws",
		"old_state":                 string(v1.TaskStateInProgress),
		"new_state":                 string(v1.TaskStateCompleted),
		"from_workflow_id":          "assignment-workflow",
		"to_workflow_id":            "assignment-workflow",
		"workflow_id":               "assignment-workflow",
		"from_step_id":              "assignment-work",
		"to_step_id":                "assignment-done",
		"session_id":                "",
		"move_id":                   "move-1",
		"step_transition_id":        "transition-1",
		"assignee_agent_profile_id": assignee,
		"queue_promotion":           false,
	}
}

// The event's committed state transition identifies completion even when the
// final step has a custom name and the payload has no step names.
func TestHumanDoneFinalStep_WakesBlockedTasks_ProductionTaskMovedPayload(t *testing.T) {
	got := runHumanDoneTaskMovedWake(t, humanDoneBlockerWakeCase{
		blockerAssignee: "assignment-worker",
		payload:         productionTaskMovedPayload("assignment-worker"),
	})
	if got != 1 {
		t.Fatalf("blockers-resolved runs = %d, want 1 (blocked task must wake when its blocker completes)", got)
	}
}

// Human completion also wakes dependents when the blocker has no assignee.
func TestHumanDoneFinalStep_WakesBlockedTasks_UnassignedBlockerOnDoneStep(t *testing.T) {
	got := runHumanDoneTaskMovedWake(t, humanDoneBlockerWakeCase{
		blockerAssignee: "",
		payload: map[string]any{
			"workspace_id":   "assignment-ws",
			"old_state":      string(v1.TaskStateInProgress),
			"new_state":      string(v1.TaskStateCompleted),
			"from_step_name": "In Progress",
			"to_step_name":   "Done",
		},
	})
	if got != 1 {
		t.Fatalf("blockers-resolved runs = %d, want 1 (unassigned blocker must wake blocked tasks)", got)
	}
}

// The committed state transition works with a custom final-step name.
func TestHumanDoneFinalStep_WakesBlockedTasks_CustomFinalStepName(t *testing.T) {
	got := runHumanDoneTaskMovedWake(t, humanDoneBlockerWakeCase{
		blockerAssignee: "assignment-worker",
		payload: map[string]any{
			"workspace_id":              "assignment-ws",
			"old_state":                 string(v1.TaskStateInProgress),
			"new_state":                 string(v1.TaskStateCompleted),
			"assignee_agent_profile_id": "assignment-worker",
			"from_step_name":            "In Progress",
			"to_step_name":              "Shipped",
		},
	})
	if got != 1 {
		t.Fatalf("blockers-resolved runs = %d, want 1 (custom final step must wake blocked tasks)", got)
	}
}

// Control: an assigned blocker moving to a step named "Done" with names in the
// payload. This passes, so the harness itself can queue a blockers-resolved run.
func TestHumanDoneFinalStep_WakesBlockedTasks_ControlAssignedDoneStepName(t *testing.T) {
	got := runHumanDoneTaskMovedWake(t, humanDoneBlockerWakeCase{
		blockerAssignee: "assignment-worker",
		payload: map[string]any{
			"assignee_agent_profile_id": "assignment-worker",
			"from_step_name":            "In Progress",
			"to_step_name":              "Done",
		},
	})
	if got != 1 {
		t.Fatalf("control: blockers-resolved runs = %d, want 1", got)
	}
}

// Control for path A: an unassigned blocker completed through the reactivity
// pipeline (NewStatus done) wakes the blocked task. This is the behavior the
// task.moved path should match.
func TestHumanDoneFinalStep_WakesBlockedTasks_ReactivityDoneControl(t *testing.T) {
	ctx := context.Background()
	f := newAssignmentBlockerFixture(t)
	f.task(t, "dependent", "assignment-work", v1.TaskStateTODO, "assignment-worker")
	f.blockers(t, "dependent", v1.TaskStateInProgress)
	f.wireResolutionEngine(t)

	blocker := "dependent-prerequisite-0"
	blockerMust(t, f.tasks.UpdateTaskState(ctx, blocker, v1.TaskStateCompleted))
	done := "done"
	_, err := f.scheduler.ApplyTaskMutation(ctx,
		&scheduler.TaskSnapshot{ID: blocker, WorkspaceID: "assignment-ws", State: "IN_PROGRESS"},
		scheduler.TaskMutation{NewStatus: &done, ActorType: "user", ActorID: "operator"})
	blockerMust(t, err)

	if got := len(f.taskRuns(t, "dependent", service.RunReasonTaskBlockersResolved)); got != 1 {
		t.Fatalf("reactivity control: blockers-resolved runs = %d, want 1", got)
	}
}
