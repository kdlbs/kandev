package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
)

func taskStartOwner(t *testing.T, f *prFixture, taskID string) string {
	t.Helper()
	task, err := f.repo.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	owner, _ := task.Metadata[models.MetaKeyAutomationStartOwned].(string)
	return owner
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.3
func TestPRContinuationMovesToStepWithoutWorkflowAutoStart(t *testing.T) {
	a := perPRAutomation()
	a.PRContinuationStepID = "step-review"
	f := newPRFixture(t, a)
	seedPRTask(t, f.repo, "pr-task", "step-done", models.TaskSessionStateWaitingForInput)
	f.auto.mappings[prTestSubject] = "pr-task"
	evt := perPREvent(prTestSubject)
	evt.RunID = "run-17"

	f.svc.createAutomationTask(context.Background(), evt)

	require.Equal(t, []string{"step-review"}, f.lifecycle.moves)
	require.Equal(t, "run-17", f.lifecycle.ownerAtMove,
		"the run owns the agent start while the move publishes task.moved")
	require.Empty(t, taskStartOwner(t, f, "pr-task"), "ownership is released after dispatch")
	require.Nil(t, f.creator.got)
}

func TestPRContinuationSkipsMoveWhenAlreadyInStepOrUnset(t *testing.T) {
	for name, step := range map[string]string{"already in step": "step-done", "unset": ""} {
		t.Run(name, func(t *testing.T) {
			a := perPRAutomation()
			a.PRContinuationStepID = step
			f := newPRFixture(t, a)
			seedPRTask(t, f.repo, "pr-task", "step-done", models.TaskSessionStateWaitingForInput)
			f.auto.mappings[prTestSubject] = "pr-task"

			f.svc.createAutomationTask(context.Background(), perPREvent(prTestSubject))

			require.Empty(t, f.lifecycle.moves)
		})
	}
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.4
func TestPRContinuationStepMoveFailureFailsRun(t *testing.T) {
	a := perPRAutomation()
	a.PRContinuationStepID = "step-deleted"
	f := newPRFixture(t, a)
	f.lifecycle.moveErr = errors.New("workflow step not found")
	seedPRTask(t, f.repo, "pr-task", "step-done", models.TaskSessionStateWaitingForInput)
	f.auto.mappings[prTestSubject] = "pr-task"
	evt := perPREvent(prTestSubject)
	evt.RunID = "run-17"

	f.svc.createAutomationTask(context.Background(), evt)

	require.Contains(t, f.auto.terminal["run-17"], "continuation step unavailable")
	require.Contains(t, f.auto.terminal["run-17"], string(automation.RunStatusFailed))
	require.Empty(t, taskStartOwner(t, f, "pr-task"), "a failed move releases ownership")
	require.Equal(t, "pr-task", f.auto.mappings[prTestSubject], "the mapping is kept")
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-005.5
func TestNewPRTaskStartsInConfiguredStep(t *testing.T) {
	a := perPRAutomation()
	a.PRContinuationStepID = "step-review"
	f := newPRFixture(t, a)

	f.svc.createAutomationTask(context.Background(), perPREvent(prTestSubject))

	require.NotNil(t, f.creator.got)
	require.Equal(t, "step-start", f.creator.got.WorkflowStepID)
	require.Empty(t, f.lifecycle.moves, "a new task is not moved to the continuation step")
}

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-009.5
func TestAutomationPRTaskArchiveAndDelete(t *testing.T) {
	f := newPRFixture(t, perPRAutomation())
	ctx := context.Background()
	seedPRTask(t, f.repo, "live", "step-done", models.TaskSessionStateWaitingForInput)
	seedPRTask(t, f.repo, "archived", "step-done", models.TaskSessionStateWaitingForInput)
	require.NoError(t, f.repo.ArchiveTask(ctx, "archived"))

	require.NoError(t, f.svc.ArchiveAutomationPRTask(ctx, "live"))
	require.NoError(t, f.svc.ArchiveAutomationPRTask(ctx, "archived"))
	require.NoError(t, f.svc.ArchiveAutomationPRTask(ctx, "missing"))
	require.Equal(t, []string{"live"}, f.lifecycle.archived, "archived and missing tasks count as done")

	require.NoError(t, f.svc.DeleteAutomationPRTask(ctx, "live", "automation: PR merged"))
	require.NoError(t, f.svc.DeleteAutomationPRTask(ctx, "missing", "automation: PR merged"))
	require.Equal(t, []string{"live"}, f.lifecycle.deleted)
	require.Equal(t, []string{"automation: PR merged"}, f.lifecycle.deleteReasons)

	f.svc.SetAutomationPRTaskLifecycle(nil)
	require.Error(t, f.svc.ArchiveAutomationPRTask(ctx, "live"), "an unwired lifecycle fails closed")
}
