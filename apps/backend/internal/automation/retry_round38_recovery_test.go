package automation

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/stretchr/testify/require"
)

func TestReplayThenReconcilePreservesAmbiguousContinuation(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	eventBus := bus.NewMemoryEventBus(log)
	eventsSeen := make(chan *AutomationTriggeredEvent, 1)
	_, err = eventBus.Subscribe(events.AutomationTriggered, func(_ context.Context, event *bus.Event) error {
		if evt, ok := event.Data.(*AutomationTriggeredEvent); ok {
			eventsSeen <- evt
		}
		return nil
	})
	require.NoError(t, err)
	svc := NewService(store, eventBus, log)
	a := &Automation{ID: "ambiguous-replay-automation", WorkspaceID: "ambiguous-replay-workspace",
		Name: "ambiguous replay", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, a))
	group := &RetryGroup{ID: "ambiguous-replay-group", AutomationID: a.ID,
		Generation: 1, State: RetryGroupLive}
	require.NoError(t, store.CreateRetryGroup(ctx, group))
	run := &AutomationRun{ID: "ambiguous-replay-run", AutomationID: a.ID,
		TriggerType: TriggerTypeManual, Status: RunStatusTriggered,
		RetryGroupID: group.ID, RetryGroupGeneration: 1, RetryState: RetryStateTriggered}
	require.NoError(t, store.CreateRun(ctx, run))
	intent := &RetryTaskIntent{ID: "ambiguous-replay-intent", RunID: run.ID,
		GroupGeneration: 1, State: retryIntentCreated}
	require.NoError(t, store.CreateRetryIntent(ctx, intent))
	require.NoError(t, store.CreateRetryOperation(ctx, &RetryOperation{
		ID: "ambiguous-replay-operation", IntentID: intent.ID, RunID: run.ID,
		GroupGeneration: 1, Kind: retryTaskOperationKind, State: retryOperationAmbiguous,
		ExternalTaskID: "accepted-task", ExternalSessionID: "accepted-session",
		ExternalTurnID: "accepted-turn",
	}))
	require.NoError(t, store.CreateRetryOutbox(ctx, &RetryOutbox{
		EventID: "ambiguous-replay-event", RunID: run.ID, SnapshotVersion: 1,
		State: retryOutboxPending,
	}))

	require.NoError(t, svc.ReplayPendingRetryEvents(ctx))
	select {
	case event := <-eventsSeen:
		require.True(t, event.RetryAmbiguousRecovery)
	default:
		t.Fatal("expected ambiguous continuation recovery event")
	}
	require.NoError(t, svc.ReconcileOpenRuns(ctx))
	stored, err := store.GetRun(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, RunStatusTriggered, stored.Status)
}
