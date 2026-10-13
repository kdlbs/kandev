package automation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetryBindingPreservesIdentityAndHistoryOnReplay(t *testing.T) {
	for _, action := range []ThreadAction{ThreadActionCreated, ThreadActionReplaced, ThreadActionResumed} {
		t.Run(string(action), func(t *testing.T) {
			store := setupTestStore(t)
			ctx := context.Background()
			a := &Automation{ID: "binding-automation", WorkspaceID: "binding-workspace", Name: "binding", Enabled: true}
			require.NoError(t, store.CreateAutomation(ctx, a))
			run := &AutomationRun{ID: "binding-run", AutomationID: a.ID, Status: RunStatusTriggered,
				RetryGroupID: "binding-group", RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
			require.NoError(t, store.CreateRetryAdmission(ctx, run, &RetryGroup{
				ID: run.RetryGroupID, AutomationID: a.ID, Generation: 1, State: RetryGroupLive,
			}))
			require.NoError(t, store.BindRunTask(ctx, run.ID, "task", "repository selected"))
			require.ErrorIs(t, store.BindRun(ctx, run.ID, "conflicting-task", "session", "turn",
				action, "conflicting task"), ErrRetryGenerationMismatch)
			require.NoError(t, store.BindRun(ctx, run.ID, "task", "session", "turn", action, "original disposition"))
			require.NoError(t, store.BindRun(ctx, run.ID, "task", "session", "turn", ThreadActionResumed, "recovery disposition"))
			for _, identity := range []RunDispatch{
				{TaskID: "other-task", SessionID: "session", TurnID: "turn"},
				{TaskID: "task", SessionID: "other-session", TurnID: "turn"},
				{TaskID: "task", SessionID: "session", TurnID: "other-turn"},
			} {
				require.ErrorIs(t, store.BindRun(ctx, run.ID, identity.TaskID, identity.SessionID,
					identity.TurnID, ThreadActionResumed, "conflicting disposition"), ErrRetryGenerationMismatch)
			}
			bound, err := store.GetRun(ctx, run.ID)
			require.NoError(t, err)
			require.Equal(t, "task", bound.TaskID)
			require.Equal(t, "session", bound.SessionID)
			require.Equal(t, "turn", bound.TurnID)
			require.Equal(t, action, bound.ThreadAction)
			require.Equal(t, "original disposition", bound.ThreadReason)
		})
	}
}
