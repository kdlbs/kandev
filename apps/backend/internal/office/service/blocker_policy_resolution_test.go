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

const (
	policyFirst  = "dependent-prerequisite-0"
	policySecond = "dependent-prerequisite-1"
)

// policyResolutionSource completes one blocker through a real resolution
// producer after its persisted state has been written.
type policyResolutionSource func(t *testing.T, f *assignmentBlockerFixture, id string)

func policyStateEvent(t *testing.T, f *assignmentBlockerFixture, id string) {
	t.Helper()
	f.completedStateEvent(t, id)
}

func policyReactivity(t *testing.T, f *assignmentBlockerFixture, id string) {
	t.Helper()
	done := "done"
	_, err := f.scheduler.ApplyTaskMutation(context.Background(),
		&scheduler.TaskSnapshot{ID: id, WorkspaceID: "assignment-ws", State: "IN_PROGRESS"},
		scheduler.TaskMutation{NewStatus: &done, ActorType: "user", ActorID: "operator"})
	blockerMust(t, err)
}

func policyStepMove(toStep string) policyResolutionSource {
	return func(t *testing.T, f *assignmentBlockerFixture, id string) {
		t.Helper()
		data := service.TaskMovedData{TaskID: id, WorkspaceID: "assignment-ws",
			AssigneeAgentProfileID: "assignment-worker", FromStepName: "Work", ToStepName: toStep}
		blockerMust(t, f.eb.Publish(context.Background(), events.TaskMoved, bus.NewEvent(events.TaskMoved, "policy-test", data)))
	}
}

func newPolicyResolutionFixture(t *testing.T) *assignmentBlockerFixture {
	t.Helper()
	f := newAssignmentBlockerFixture(t)
	blockerMust(t, f.scheduler.RegisterTaskCompletionSubscriber(f.eb))
	f.task(t, "dependent", "assignment-work", v1.TaskStateTODO, "assignment-worker")
	f.blockers(t, "dependent", v1.TaskStateInProgress, v1.TaskStateInProgress)
	for _, id := range []string{policyFirst, policySecond} {
		_, err := f.repo.UpdateTaskAssignee(context.Background(), id, "assignment-worker")
		blockerMust(t, err)
	}
	return f
}

func (f *assignmentBlockerFixture) complete(t *testing.T, source policyResolutionSource, id string) {
	t.Helper()
	blockerMust(t, f.tasks.UpdateTaskState(context.Background(), id, v1.TaskStateCompleted))
	source(t, f, id)
}

func (f *assignmentBlockerFixture) resolutionRuns(t *testing.T, want int, when string) {
	t.Helper()
	if got := len(f.taskRuns(t, "dependent", service.RunReasonTaskBlockersResolved)); got != want {
		t.Fatalf("%s: resolution runs=%d, want %d", when, got, want)
	}
}

var policyResolutionSources = map[string]policyResolutionSource{
	"state_event": policyStateEvent,
	"reactivity":  policyReactivity,
}

// @covers AC-OFFICE-BLOCKER-POLICY-001.2 AC-OFFICE-BLOCKER-POLICY-001.3
func TestOfficeBlockerPolicyResolutionOrder(t *testing.T) {
	for name, source := range policyResolutionSources {
		for _, failure := range []v1.TaskState{v1.TaskStateFailed, v1.TaskStateCancelled} {
			t.Run(name+"/"+string(failure)+"/fail_then_complete", func(t *testing.T) {
				f := newPolicyResolutionFixture(t)
				blockerMust(t, f.tasks.UpdateTaskState(context.Background(), policySecond, failure))
				f.complete(t, source, policyFirst)
				f.resolutionRuns(t, 0, "completion beside a failed blocker")
				f.complete(t, source, policySecond)
				f.resolutionRuns(t, 1, "failed blocker retried to completion")
			})
			t.Run(name+"/"+string(failure)+"/complete_then_fail", func(t *testing.T) {
				f := newPolicyResolutionFixture(t)
				f.complete(t, source, policyFirst)
				f.resolutionRuns(t, 0, "partial completion")
				blockerMust(t, f.tasks.UpdateTaskState(context.Background(), policySecond, failure))
				f.resolutionRuns(t, 0, "final blocker failed")
				f.complete(t, source, policySecond)
				f.resolutionRuns(t, 1, "failed blocker retried to completion")
			})
		}
	}
}

// @covers AC-OFFICE-BLOCKER-POLICY-001.4
func TestOfficeBlockerPolicyStepNameDoesNotSatisfy(t *testing.T) {
	cases := []struct {
		name, step string
		state      v1.TaskState
	}{
		{"cancelled_step_cancelled_state", "Cancelled", v1.TaskStateCancelled},
		{"cancelled_step_failed_state", "cancelled", v1.TaskStateFailed},
		{"done_step_in_progress_state", "Done", v1.TaskStateInProgress},
		{"done_step_completed_state", "Done", v1.TaskStateCompleted},
		{"cancelled_step_completed_state", "Cancelled", v1.TaskStateCompleted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newPolicyResolutionFixture(t)
			blockerMust(t, f.tasks.UpdateTaskState(context.Background(), policyFirst, v1.TaskStateCompleted))
			blockerMust(t, f.tasks.UpdateTaskState(context.Background(), policySecond, tc.state))
			policyStepMove(tc.step)(t, f, policySecond)
			f.resolutionRuns(t, 0, "task.moved without task.state_changed")
		})
	}
}
