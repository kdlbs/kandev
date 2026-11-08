package orchestrator

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	eventbus "github.com/kandev/kandev/internal/events/bus"
)

// committingBackgroundWorkObserver records observations and reports the durable
// projection, mirroring task/service's committed-row behaviour.
type committingBackgroundWorkObserver struct {
	mu       sync.Mutex
	observed []streams.WorkloadRunObservation
	commit   streams.WorkloadRunObservation
}

func (o *committingBackgroundWorkObserver) RecordBackgroundWorkloadObservation(
	_ context.Context,
	obs streams.WorkloadRunObservation,
	_, _ string,
) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.observed = append(o.observed, obs)
	return nil
}

func (o *committingBackgroundWorkObserver) AppendBackgroundWorkloadOutput(
	_ context.Context,
	_ streams.WorkloadOutputChunk,
	_ string,
) error {
	return nil
}

func (o *committingBackgroundWorkObserver) RecordBackgroundWorkloadObservationWithResult(
	_ context.Context,
	obs streams.WorkloadRunObservation,
	_, _ string,
) (streams.WorkloadRunObservation, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.observed = append(o.observed, obs)
	committed := o.commit
	committed.SessionID = obs.SessionID
	committed.WorkID = obs.WorkID
	return committed, nil
}

// TestBackgroundWorkPublicationCarriesCommittedProjection pins the publisher
// contract: the browser must receive the durable revision, retained output, and
// exit status, not the adapter's raw frame (which carries none of them) and not
// the adapter's empty capability declaration.
func TestBackgroundWorkPublicationCarriesCommittedProjection(t *testing.T) {
	log := testLogger()
	bus := eventbus.NewMemoryEventBus(log)
	observer := &committingBackgroundWorkObserver{
		commit: streams.WorkloadRunObservation{
			State:    streams.RunStateFailed,
			Revision: 7,
			ExitCode: exitCodePtr(2),
			Output:   "checks failed",
		},
	}
	svc := &Service{logger: log, eventBus: bus, backgroundWorkObserver: observer}

	const sessionID = "session-committed"
	var published *streams.WorkloadRunObservation
	_, err := bus.Subscribe(
		events.BuildBackgroundWorkUpdatedSubject(sessionID),
		func(_ context.Context, event *eventbus.Event) error {
			payload, ok := event.Data.(*streams.WorkloadRunObservation)
			if ok {
				published = payload
			}
			return nil
		},
	)
	require.NoError(t, err)

	incoming := streams.WorkloadRunObservation{
		SessionID: sessionID,
		WorkID:    "shell-1",
		Kind:      streams.WorkloadKindShell,
		Title:     "CI poller",
		State:     streams.RunStateFailed,
		Capabilities: streams.WorkloadCapabilities{
			Discovery: "events_only",
			Output:    "snapshot",
		},
	}
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    "task-committed",
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:           streams.EventTypeBackgroundWorkUpdated,
			BackgroundWork: &incoming,
		},
	})

	require.NotNil(t, published)
	require.Equal(t, streams.RunStateFailed, published.State)
	require.EqualValues(t, 7, published.Revision, "durable revision must survive publication")
	require.NotNil(t, published.ExitCode)
	require.Equal(t, 2, *published.ExitCode)
	require.Equal(t, "checks failed", published.Output)
	require.Equal(t, "snapshot", published.Capabilities.Output,
		"adapter-declared capabilities are not persisted and must travel with the frame")
	require.Len(t, observer.observed, 1)
}

func exitCodePtr(code int) *int { return &code }
