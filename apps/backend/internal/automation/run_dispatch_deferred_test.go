package automation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// A deferred launch leaves the admitted run open and bound to its task, so a
// later dispatch can still bind the launch it makes; any other dispatch error
// still fails the run. Either way the callback's error is returned unchanged.
func TestDispatchRunDeferredLaunchLeavesTheRunOpen(t *testing.T) {
	cases := []struct {
		name        string
		dispatchErr error
		wantStatus  RunStatus
	}{
		{name: "deferred", dispatchErr: ErrRunDeferred, wantStatus: RunStatusTriggered},
		{name: "failed", dispatchErr: errors.New("executor unavailable"), wantStatus: RunStatusFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			ctx := context.Background()
			a := &Automation{WorkspaceID: "ws-1", Name: "deferred", Enabled: true, MaxConcurrentRuns: 1}
			require.NoError(t, svc.store.CreateAutomation(ctx, a))
			run := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTriggered}
			require.NoError(t, svc.store.CreateRun(ctx, run))
			require.NoError(t, svc.store.BindRunTask(ctx, run.ID, "task-1", ""))

			err := svc.DispatchRun(ctx, run.ID, ThreadActionCreated, "created", func() (RunDispatch, error) {
				return RunDispatch{}, tc.dispatchErr
			})
			require.Same(t, tc.dispatchErr, err)

			got, err := svc.store.GetRun(ctx, run.ID)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, got.Status)
			require.Equal(t, "task-1", got.TaskID)
			if tc.wantStatus != RunStatusTriggered {
				return
			}
			require.NoError(t, svc.DispatchRun(ctx, run.ID, ThreadActionCreated, "created", func() (RunDispatch, error) {
				return RunDispatch{TaskID: "task-1", SessionID: "session-1", TurnID: "turn-1"}, nil
			}))
			bound, err := svc.store.GetRun(ctx, run.ID)
			require.NoError(t, err)
			require.Equal(t, RunStatusTaskCreated, bound.Status)
			require.Equal(t, "turn-1", bound.TurnID)
		})
	}
}
