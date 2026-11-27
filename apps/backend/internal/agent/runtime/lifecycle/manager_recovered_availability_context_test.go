package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type recoveredAvailabilityContextBus struct {
	*MockEventBus
	publishedContextStates chan recoveredAvailabilityContextState
}

type recoveredAvailabilityContextState struct {
	err         error
	hasDeadline bool
}

func (b *recoveredAvailabilityContextBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	_, hasDeadline := ctx.Deadline()
	b.publishedContextStates <- recoveredAvailabilityContextState{err: ctx.Err(), hasDeadline: hasDeadline}
	return b.MockEventBus.Publish(ctx, subject, event)
}

func newRecoveredAvailabilityContextTestManager(t *testing.T) (*Manager, *recoveredAvailabilityContextBus) {
	t.Helper()
	log := newTestLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{name: executor.NameSSH})
	ctxBus := &recoveredAvailabilityContextBus{
		MockEventBus:           &MockEventBus{},
		publishedContextStates: make(chan recoveredAvailabilityContextState, 1),
	}
	mgr := NewManager(newTestRegistry(), ctxBus, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	return mgr, ctxBus
}

func recoveredAvailabilityTestPayload(t *testing.T, mgr *Manager) (*AgentExecution, AgentEventPayload) {
	t.Helper()
	execution := &AgentExecution{
		ID:        "exec-recovered-availability",
		SessionID: "session-recovered-availability",
		TaskID:    "task-recovered-availability",
		Status:    v1.AgentStatusReady,
	}
	require.NoError(t, mgr.executionStore.Add(execution))
	payload := newAgentEventPayload(execution)
	payload.Status = string(v1.AgentStatusReady)
	payload.RecoveredAvailabilityOnly = true
	return execution, payload
}

func TestRecoveredAvailabilityPublicationSurvivesAttemptCancellation(t *testing.T) {
	mgr, eventBus := newRecoveredAvailabilityContextTestManager(t)
	execution, payload := recoveredAvailabilityTestPayload(t, mgr)
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	mgr.publishRecoveredAvailabilityAfterTransfer(parent, execution, &payload)

	select {
	case state := <-eventBus.publishedContextStates:
		require.NoError(t, state.err, "availability publication must outlive the completed recovery attempt")
		require.True(t, state.hasDeadline, "the manager-owned publication context must remain bounded")
	default:
		t.Fatal("expected recovered availability publication")
	}
	require.Len(t, eventBus.PublishedEvents, 1)
}

func TestRecoveredAvailabilityPublicationStopsWithManager(t *testing.T) {
	tests := []struct {
		name     string
		shutdown func(*Manager)
	}{
		{
			name: "stop context",
			shutdown: func(mgr *Manager) {
				mgr.closeStopCh()
			},
		},
		{
			name: "shutdown flag",
			shutdown: func(mgr *Manager) {
				mgr.shuttingDown.Store(true)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mgr, eventBus := newRecoveredAvailabilityContextTestManager(t)
			execution, payload := recoveredAvailabilityTestPayload(t, mgr)
			test.shutdown(mgr)

			mgr.publishRecoveredAvailabilityAfterTransfer(context.Background(), execution, &payload)

			require.Empty(t, eventBus.PublishedEvents)
			select {
			case <-eventBus.publishedContextStates:
				t.Fatal("manager shutdown must suppress recovered availability publication")
			default:
			}
		})
	}
}
