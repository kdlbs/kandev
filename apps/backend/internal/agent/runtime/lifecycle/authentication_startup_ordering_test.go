package lifecycle

import (
	"errors"
	"fmt"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestAuthenticationStartupOrdering(t *testing.T) {
	for _, test := range []struct {
		name          string
		eventPosition string
		eventCount    int
		stale         bool
	}{
		{name: "event before response", eventPosition: "before", eventCount: 1},
		{name: "event after response", eventPosition: "after", eventCount: 1},
		{name: "response without event", eventPosition: "absent", eventCount: 0},
		{name: "duplicate event", eventPosition: "duplicate", eventCount: 2},
		{name: "late event after successor attempt", eventPosition: "late", eventCount: 0, stale: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager, eventBus := createTestManagerWithTracking()
			execution := createTestExecution("execution-auth", "task-auth", "session-auth")
			generation := execution.beginStartupAttemptWithID("attempt-auth")
			require.NoError(t, manager.executionStore.Add(execution))

			authEvent := agentctl.AgentEvent{
				Type:        streams.EventTypeAuthRequired,
				SessionID:   "native-session-not-created",
				AuthMethods: []streams.AuthMethodInfo{{ID: "cursor_login", Name: "Cursor login"}},
			}
			publishEvent := func(startupGeneration uint64) {
				manager.handleAgentEventWithStartupGeneration(execution, authEvent, startupGeneration)
			}
			classifyResponse := func() *BootstrapFailure {
				return bootstrapFailureFor(execution, fmt.Errorf("start failed: %w", &testSessionAuthenticationError{
					failure: agentctltypesSessionAuthenticationFailure(),
					cause:   errors.New("provider diagnostic token=private-token"),
				}))
			}

			var failure *BootstrapFailure
			switch test.eventPosition {
			case "before":
				publishEvent(generation)
				failure = classifyResponse()
			case "after":
				failure = classifyResponse()
				publishEvent(generation)
			case "duplicate":
				publishEvent(generation)
				publishEvent(generation)
				failure = classifyResponse()
			case "late":
				failure = classifyResponse()
				execution.beginStartupAttemptWithID("attempt-successor")
				publishEvent(generation)
			case "absent":
				failure = classifyResponse()
			}

			require.NotNil(t, failure)
			require.Equal(t, models.AgentErrorCauseCodeAuthenticationRequired, failure.SafeCode())
			require.Equal(t, "Agent authentication is required.", failure.SafeDetail())
			require.Len(t, failure.SessionFailure.AuthMethods, 1)
			events := eventBus.getStreamEvents()
			require.Len(t, events, test.eventCount)
			for _, event := range events {
				require.Equal(t, execution.ID, event.ExecutionID)
				require.Equal(t, "attempt-auth", event.AttemptID)
				require.Equal(t, streams.EventTypeAuthRequired, event.Data.Type)
			}
			if test.stale {
				require.Equal(t, "attempt-successor", execution.currentStartupAttemptID())
			}
		})
	}
}

func agentctltypesSessionAuthenticationFailure() agentctltypes.SessionOperationFailure {
	return agentctltypes.SessionOperationFailure{
		Code: agentctltypes.SessionOperationFailureCodeAuthenticationRequired,
		AuthMethods: []streams.AuthMethodInfo{{
			ID: "cursor_login", Name: "Cursor login",
		}},
	}
}
