package lifecycle

import (
	"context"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/constants"
)

func TestCoalescedExecutionContextDoesNotConsumeLaunchBudgetDuringPreparation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		manager := &Manager{stopCh: make(chan struct{})}
		sharedCtx, cancel := manager.coalescedExecutionContext(context.Background())
		defer cancel()

		if _, ok := sharedCtx.Deadline(); ok {
			t.Fatal("coalesced execution context must not have a launch deadline")
		}

		// Simulate environment creation before the runtime launch phase begins.
		<-time.After(constants.SetupScriptTimeout)
		launchCtx, launchCancel := withLaunchPhaseTimeout(sharedCtx)
		defer launchCancel()

		deadline, ok := launchCtx.Deadline()
		if !ok {
			t.Fatal("launch phase context must have a deadline")
		}
		if got := time.Until(deadline); got != constants.AgentLaunchTimeout {
			t.Fatalf("launch phase budget = %s, want %s", got, constants.AgentLaunchTimeout)
		}
	})
}

func TestPreparationContextRetainsItsIndependentBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		base := context.Background()
		launchCtx, launchCancel := withLaunchPhaseTimeout(base)
		defer launchCancel()

		prepareParent := preparationContext(launchCtx)
		if _, ok := prepareParent.Deadline(); ok {
			t.Fatal("preparation parent must not inherit the launch deadline")
		}

		prepareCtx, prepareCancel := context.WithTimeout(prepareParent, constants.SetupScriptTimeout)
		defer prepareCancel()
		deadline, ok := prepareCtx.Deadline()
		if !ok {
			t.Fatal("preparation context must have a deadline")
		}
		if got := time.Until(deadline); got != constants.SetupScriptTimeout {
			t.Fatalf("preparation budget = %s, want %s", got, constants.SetupScriptTimeout)
		}
	})
}

func TestFinishContextResetReturnsEventsBufferedDuringFinalization(t *testing.T) {
	execution := &AgentExecution{}
	require.NoError(t, execution.beginContextReset())

	buffered := agentctl.AgentEvent{
		Type:      streams.EventTypeSessionModels,
		SessionID: "new-session",
	}
	stale := agentctl.AgentEvent{
		Type:      streams.EventTypeSessionStatus,
		SessionID: "old-session",
	}
	require.True(t, execution.bufferOrDropContextResetEvent(buffered))
	require.True(t, execution.bufferOrDropContextResetEvent(stale))

	final := execution.finishContextReset("new-session")
	require.Equal(t, []agentctl.AgentEvent{buffered}, final)
	require.False(t, execution.bufferOrDropContextResetEvent(agentctl.AgentEvent{
		Type:      streams.EventTypeSessionStatus,
		SessionID: "new-session",
	}))
}

// @covers AC-AGENTS-BACKGROUND-WORK-011.8
func TestContextResetAcceptsRetiredWatchProjectionForCommittedSession(t *testing.T) {
	execution := &AgentExecution{}
	require.NoError(t, execution.beginContextReset())
	retired := agentctl.AgentEvent{
		Type:      streams.EventTypeBackgroundWorkUpdated,
		SessionID: "new-session",
		BackgroundWork: &streams.WorkloadRunObservation{
			WorkID: "watch-work", RunID: "watch-run", Kind: streams.WorkloadKindMonitor,
			State: streams.RunStateUnknown, SourceCallID: "watch-call",
		},
	}
	stale := retired
	stale.SessionID = "old-session"
	stale.BackgroundWork = &streams.WorkloadRunObservation{
		WorkID: "stale-watch-work", RunID: "stale-watch-run", Kind: streams.WorkloadKindMonitor,
		State: streams.RunStateUnknown, SourceCallID: "watch-call",
	}
	require.True(t, execution.bufferOrDropContextResetEvent(retired))
	require.True(t, execution.bufferOrDropContextResetEvent(stale))

	accepted := execution.drainContextResetEvents("new-session")
	require.Equal(t, []agentctl.AgentEvent{retired}, accepted,
		"the reset commit accepts the retirement update and excludes stale old-session events")
	execution.finishContextReset("new-session")
}

func TestContextResetRetainsWatchRetirementsBeyondSetupBufferLimit(t *testing.T) {
	execution := &AgentExecution{}
	require.NoError(t, execution.beginContextReset())

	for i := 0; i < maxContextResetSetupEvents; i++ {
		require.True(t, execution.bufferOrDropContextResetEvent(agentctl.AgentEvent{
			Type: streams.EventTypeSessionModels, SessionID: "new-session", OperationID: strconv.Itoa(i),
		}))
	}
	require.True(t, execution.bufferOrDropContextResetEvent(agentctl.AgentEvent{
		Type: streams.EventTypeSessionModels, SessionID: "new-session", OperationID: "setup-overflow",
	}))

	const watchCount = maxContextResetSetupEvents + 7
	for i := 0; i < watchCount; i++ {
		id := strconv.Itoa(i)
		require.True(t, execution.bufferOrDropContextResetEvent(agentctl.AgentEvent{
			Type: streams.EventTypeBackgroundWorkUpdated, SessionID: "new-session",
			BackgroundWork: &streams.WorkloadRunObservation{
				WorkID: "watch-work-" + id, RunID: "watch-run-" + id,
				Kind: streams.WorkloadKindMonitor, State: streams.RunStateUnknown,
				SourceCallID: "watch-call-" + id,
			},
		}))
	}

	accepted := execution.drainContextResetEvents("new-session")
	require.Equal(t, maxContextResetSetupEvents+watchCount, len(accepted))
	setupCount := 0
	retiredWorkIDs := make(map[string]struct{}, watchCount)
	for _, event := range accepted {
		switch event.Type {
		case streams.EventTypeSessionModels:
			setupCount++
			require.NotEqual(t, "setup-overflow", event.OperationID)
		case streams.EventTypeBackgroundWorkUpdated:
			require.NotNil(t, event.BackgroundWork)
			retiredWorkIDs[event.BackgroundWork.WorkID] = struct{}{}
		}
	}
	require.Equal(t, maxContextResetSetupEvents, setupCount)
	require.Len(t, retiredWorkIDs, watchCount)

	execution.finishContextReset("new-session")
}
