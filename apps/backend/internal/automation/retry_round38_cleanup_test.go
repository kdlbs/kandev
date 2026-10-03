package automation

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func ambiguousCleanupFixture(t *testing.T, name string) (*Service, *Automation, *fakeTaskDeleter) {
	t.Helper()
	svc := newTestService(t)
	deleter := &fakeTaskDeleter{}
	svc.SetTaskDeleter(deleter)
	svc.SetTaskOriginLookup(&fakeTaskOriginLookup{results: map[string]fakeOriginResult{
		"accepted-bulk-task": {isAutomationRun: true, ok: true},
	}})
	ctx := context.Background()
	a := &Automation{ID: "ambiguous-cleanup-" + name, WorkspaceID: "ws-ambiguous-cleanup-" + name,
		Name: name, Enabled: true}
	require.NoError(t, svc.store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "ambiguous-cleanup-group-" + name, AutomationID: a.ID,
		Generation: 1, State: RetryGroupLive}
	require.NoError(t, svc.store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "ambiguous-cleanup-run-" + name, AutomationID: a.ID,
		Status: RunStatusTriggered, TriggerType: TriggerTypeManual,
		RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
	require.NoError(t, svc.store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{ID: "ambiguous-cleanup-intent-" + name, RunID: run.ID,
		GroupGeneration: 1, State: retryIntentCreated}
	require.NoError(t, svc.store.CreateRetryIntent(ctx, intent))
	require.NoError(t, svc.store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "ambiguous-cleanup-operation-" + name, IntentID: intent.ID, RunID: run.ID,
		GroupGeneration: 1, Kind: retryTaskOperationKind, State: retryOperationAmbiguous,
		ExternalTaskID: "accepted-bulk-task", ExternalSessionID: "accepted-session",
		ExternalTurnID: "accepted-turn",
	}))
	return svc, a, deleter
}

func TestDeleteAutomationRemovesAmbiguousAcceptedTask(t *testing.T) {
	svc, a, deleter := ambiguousCleanupFixture(t, "delete-automation")
	require.NoError(t, svc.DeleteAutomation(context.Background(), a.ID))
	require.Equal(t, []string{"accepted-bulk-task"}, deleter.deleted)
}

func TestDeleteAllRunsRemovesAmbiguousAcceptedTask(t *testing.T) {
	svc, a, deleter := ambiguousCleanupFixture(t, "delete-all-runs")
	require.NoError(t, svc.DeleteAllRuns(context.Background(), a.ID))
	require.Equal(t, []string{"accepted-bulk-task"}, deleter.deleted)
}

func TestClaimDueRetryTerminalizesPoisonGroup(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	a := &Automation{ID: "poison-automation", WorkspaceID: "poison-workspace",
		Name: "poison", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "poison-group", AutomationID: a.ID, Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "poison-run", AutomationID: a.ID, TriggerType: TriggerTypeManual,
		Status: RunStatusScheduledRetry, RetryGroupID: group.ID, RetryGroupGeneration: 1,
		RetryState: RetryStateScheduled, RetryScheduledAt: &now, AttemptNumber: 2}
	require.NoError(t, store.CreateRun(ctx, run))
	require.NoError(t, store.CreateRetryIntent(ctx, &RetryTaskIntent{
		ID: "poison-intent", RunID: run.ID, GroupGeneration: 1, State: retryIntentAdmitted,
	}))

	_, _, err := store.ClaimDueRetry(ctx, now, time.Second)
	require.ErrorIs(t, err, ErrNoDueRetry)
	storedGroup, err := store.GetRetryGroup(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, RetryGroupCompleted, storedGroup.State)
	history, err := store.ListRetryHistory(ctx, a.ID, "", 10)
	require.NoError(t, err)
	require.Len(t, history.Items, 1)
	require.True(t, history.Items[0].Completed)
}

func TestDeferRetryClaimForCapacityReschedulesFutureAttempt(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	a := &Automation{ID: "capacity-backoff-automation", WorkspaceID: "capacity-backoff-workspace",
		Name: "capacity backoff", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "capacity-backoff-group", AutomationID: a.ID,
		Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "capacity-backoff-run", AutomationID: a.ID,
		Status: RunStatusScheduledRetry, RetryGroupID: group.ID, RetryGroupGeneration: 1,
		RetryState: RetryStateScheduled, RetryScheduledAt: &now, AttemptNumber: 2}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{ID: "capacity-backoff-intent", RunID: run.ID,
		GroupGeneration: 1, State: retryIntentAdmitted}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "capacity-backoff-operation", IntentID: intent.ID, RunID: run.ID,
		GroupGeneration: 1, Kind: retryTaskOperationKind, State: retryOperationRequested,
	}))
	claimed, token, err := store.ClaimDueRetry(ctx, now, time.Second)
	require.NoError(t, err)
	require.Equal(t, run.ID, claimed.ID)
	require.NoError(t, store.DeferRetryClaimForCapacity(ctx, run.ID, token, 1))

	_, _, err = store.ClaimDueRetry(ctx, time.Now().UTC(), time.Second)
	require.ErrorIs(t, err, ErrNoDueRetry)
	stored, err := store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, RetryStateScheduled, stored.RetryState)
	require.NotNil(t, stored.RetryScheduledAt)
	require.True(t, stored.RetryScheduledAt.After(time.Now().UTC()))
}
