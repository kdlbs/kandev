package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-OFFICE-SCHEDULER-002.5 AC-OFFICE-SCHEDULER-002.8
func TestOfficeAssignmentBlockerRecovery(t *testing.T) {
	f := newAssignmentBlockerFixture(t)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("held-%d", i)
		f.task(t, id, "assignment-work", v1.TaskStateTODO, "assignment-worker")
		f.blockers(t, id, v1.TaskStateCompleted, v1.TaskStateInProgress)
	}
	f.task(t, "eligible", "assignment-work", v1.TaskStateTODO, "assignment-worker")
	handler := service.NewOfficeRecoveryHandler(service.NewSchedulerIntegration(f.svc, 0))
	blockerMust(t, handler.Tick(context.Background()))
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("held-%d", i)
		if runs := f.taskRuns(t, id, service.RunReasonTaskAssigned); len(runs) != 0 {
			t.Errorf("%s persisted %d assignment runs, want 0", id, len(runs))
		}
	}
	if runs := f.taskRuns(t, "eligible", service.RunReasonTaskAssigned); len(runs) != 1 {
		t.Errorf("eligible task after five blocked candidates got %d runs, want 1", len(runs))
	}
	blockerMust(t, f.tasks.UpdateTaskState(context.Background(), "held-0-prerequisite-1", v1.TaskStateCompleted))
	blockerMust(t, handler.Tick(context.Background()))
	if runs := f.taskRuns(t, "held-0", service.RunReasonTaskAssigned); len(runs) != 1 {
		t.Fatalf("recovery after completion got %d runs, want 1", len(runs))
	}
}
