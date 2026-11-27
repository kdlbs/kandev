package automation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnavailableCredentialChoiceExhaustsRetry(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "workspace-credential-retry", Name: "credential retry", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "credential-retry-group", AutomationID: a.ID, Generation: 1, State: RetryGroupLive}
	run := &AutomationRun{
		ID: "credential-retry-parent", AutomationID: a.ID, TriggerType: TriggerTypeManual,
		Status: RunStatusTriggered, RetryGroupID: group.ID, RetryGroupGeneration: 1,
		AttemptNumber: 1, RetryState: RetryStateTriggered,
		RetryPolicySnapshot: `{"mode":"infinite","delay_seconds":"0","backoff":"fixed"}`,
	}
	require.NoError(t, store.CreateRetryAdmission(ctx, run, group))

	child, err := store.FinalizeRetryFailure(ctx, run.ID, 1, ErrCredentialChoiceUnavailable, "launch")
	require.NoError(t, err)
	require.Nil(t, child)

	parent, err := store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, RunStatusFailed, parent.Status)
	require.Equal(t, RetryStateExhausted, parent.RetryState)
	groups, err := store.GetRetryGroup(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, RetryGroupCompleted, groups.State)
}
