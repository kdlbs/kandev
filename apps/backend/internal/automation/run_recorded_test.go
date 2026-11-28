package automation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

type recordedRun struct {
	automationID, runID string
	status              RunStatus
}

func observeRecordedRuns(store *Store) *[]recordedRun {
	var mu sync.Mutex
	calls := &[]recordedRun{}
	store.runRecorded = func(_ context.Context, automationID, runID string, status RunStatus) {
		mu.Lock()
		defer mu.Unlock()
		*calls = append(*calls, recordedRun{automationID, runID, status})
	}
	return calls
}

// @covers AC-OFFICE-AUTOMATION-RUNS-001.23
func TestRunRecordedObserverSeesEachCommittedInsert(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	a := &Automation{WorkspaceID: "ws-recorded", Name: "recorded", Enabled: true,
		RetryPolicy: RetryPolicy{Mode: RetryModeFinite, MaxRetries: "3", DelaySeconds: "60", Backoff: RetryBackoffFixed}}
	require.NoError(t, store.CreateAutomation(ctx, a))
	calls := observeRecordedRuns(store)

	plain := &AutomationRun{ID: "run-plain", AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusSkipped}
	require.NoError(t, store.CreateRun(ctx, plain))
	require.Error(t, store.CreateRun(ctx, &AutomationRun{ID: "run-plain", AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusSkipped}))

	group := &RetryGroup{ID: "group-recorded", AutomationID: a.ID, Generation: 1, State: RetryGroupLive}
	retry := &AutomationRun{ID: "run-retry", AutomationID: a.ID, TriggerType: TriggerTypeManual,
		Status: RunStatusTriggered, RetryGroupID: group.ID, AttemptNumber: 1,
		RetryState: RetryStateTriggered, RetryGroupGeneration: 1,
		RetryPolicySnapshot: `{"mode":"finite","max_retries":"3","delay_seconds":"60","backoff":"fixed"}`}
	require.NoError(t, store.CreateRetryAdmission(ctx, retry, group))
	child, err := store.FinalizeRetryFailure(ctx, retry.ID, 1, errors.New("attempt failed"), "launch")
	require.NoError(t, err)
	require.NotNil(t, child)

	require.Equal(t, []recordedRun{
		{a.ID, "run-plain", RunStatusSkipped},
		{a.ID, "run-retry", RunStatusTriggered},
		{a.ID, child.ID, RunStatusScheduledRetry},
	}, *calls)
}

// @covers AC-OFFICE-AUTOMATION-RUNS-001.23
func TestRunRecordedPublishesWorkspaceScopedEvent(t *testing.T) {
	svc := newTestService(t)
	t.Cleanup(svc.eventBus.Close)
	a := createTestAutomation(t, svc, "ws-live")
	received := make(chan *bus.Event, 1)
	_, err := svc.eventBus.Subscribe(events.AutomationRunCreated, func(_ context.Context, event *bus.Event) error {
		received <- event
		return nil
	})
	require.NoError(t, err)

	require.NoError(t, svc.Store().CreateRun(context.Background(), &AutomationRun{
		ID: "run-live", AutomationID: a.ID, TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
	}))
	select {
	case event := <-received:
		require.Equal(t, map[string]any{
			"workspace_id": "ws-live", "automation_id": a.ID, "run_id": "run-live", "status": string(RunStatusTriggered),
		}, event.Data)
	case <-time.After(2 * time.Second):
		t.Fatal("no automation.run.created event")
	}
}
