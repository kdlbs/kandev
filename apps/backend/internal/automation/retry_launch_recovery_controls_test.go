package automation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReconcileUnboundLegacyRetryRetainsVisibleUncertainty(t *testing.T) {
	for _, taskID := range []string{"", "task-before-session"} {
		t.Run(taskID, func(t *testing.T) {
			ctx := context.Background()
			svc := newTestService(t)
			a := &Automation{WorkspaceID: "workspace", Name: "unbound legacy", Enabled: true}
			require.NoError(t, svc.store.CreateAutomation(ctx, a))
			group := &RetryGroup{ID: "legacy-group", AutomationID: a.ID, Generation: 1, State: RetryGroupLive}
			require.NoError(t, svc.store.CreateRetryGroup(ctx, group))
			run := &AutomationRun{ID: "unbound-legacy", AutomationID: a.ID, TaskID: taskID, Status: RunStatusTriggered, RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
			require.NoError(t, svc.store.CreateRun(ctx, run))
			require.NoError(t, svc.ReconcileOpenRuns(ctx))
			retained, err := svc.store.GetRun(ctx, run.ID)
			require.NoError(t, err)
			require.Equal(t, RunStatusTriggered, retained.Status)
			require.Equal(t, ErrRetryLaunchUncertain.Error(), retained.ErrorMessage)
			require.Equal(t, taskID, retained.TaskID)
			require.Empty(t, retained.SessionID)
			require.Empty(t, retained.TurnID)
			require.ErrorIs(t, svc.DeleteRun(ctx, run.ID), ErrRetryLaunchUncertain)
		})
	}
}

func TestDeleteLegacyCommittedTaskWithoutTerminationRetainsOwnership(t *testing.T) {
	for _, all := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "all"}[all], func(t *testing.T) {
			ctx := context.Background()
			svc := newTestService(t)
			deleter := &fakeTaskDeleter{}
			svc.SetTaskDeleter(deleter)
			stopper := &recordingRunStopper{}
			svc.SetRunStopper(stopper)
			a := &Automation{WorkspaceID: "workspace", Name: "unknown committed task", Enabled: true}
			require.NoError(t, svc.store.CreateAutomation(ctx, a))
			group := &RetryGroup{ID: "legacy-group", AutomationID: a.ID, Generation: 1, State: RetryGroupLive}
			require.NoError(t, svc.store.CreateRetryGroup(ctx, group))
			run := &AutomationRun{ID: "legacy-run", AutomationID: a.ID, Status: RunStatusTriggered, RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
			require.NoError(t, svc.store.CreateRun(ctx, run))
			intent := &RetryTaskIntent{ID: "legacy-intent", RunID: run.ID, GroupGeneration: 1, State: retryIntentCreated}
			require.NoError(t, svc.store.CreateRetryIntent(ctx, intent))
			require.NoError(t, svc.store.CreateRetryOperation(ctx, &RetryOperation{
				ID: "legacy-operation", IntentID: intent.ID, RunID: run.ID, GroupGeneration: 1,
				Kind: retryTaskOperationKind, State: retryOperationCommitted, ExternalTaskID: "unknown-task",
			}))
			var err error
			if all {
				err = svc.DeleteAllRuns(ctx, a.ID)
			} else {
				err = svc.DeleteRun(ctx, run.ID)
			}
			require.ErrorIs(t, err, ErrRetryLaunchUncertain)
			require.Empty(t, deleter.deleted)
			require.Empty(t, stopper.taskID, "a task ID alone cannot identify an execution to stop")
			retained, err := svc.store.GetRun(ctx, run.ID)
			require.NoError(t, err)
			require.Equal(t, RunStatusTriggered, retained.Status)
			operation, err := svc.store.GetRetryTaskOperation(ctx, run.ID, 1)
			require.NoError(t, err)
			require.Equal(t, "unknown-task", operation.ExternalTaskID)
			require.Empty(t, operation.ExternalSessionID)
			require.Empty(t, operation.ExternalTurnID)
		})
	}
}
