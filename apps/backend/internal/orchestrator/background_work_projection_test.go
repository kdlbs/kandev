package orchestrator

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	eventbus "github.com/kandev/kandev/internal/events/bus"
)

type mockBackgroundWorkObserver struct {
	mu           sync.Mutex
	observations []streams.WorkloadRunObservation
	outputs      []streams.WorkloadOutputChunk
}

func (m *mockBackgroundWorkObserver) RecordBackgroundWorkloadObservation(
	_ context.Context,
	obs streams.WorkloadRunObservation,
	_, _ string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observations = append(m.observations, obs)
	return nil
}

func (m *mockBackgroundWorkObserver) AppendBackgroundWorkloadOutput(
	_ context.Context,
	chunk streams.WorkloadOutputChunk,
	_ string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outputs = append(m.outputs, chunk)
	return nil
}

func (m *mockBackgroundWorkObserver) allObservations() []streams.WorkloadRunObservation {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]streams.WorkloadRunObservation, len(m.observations))
	copy(out, m.observations)
	return out
}

func TestBackgroundWorkProjectionReplayAndSuccessor(t *testing.T) {
	observer := &mockBackgroundWorkObserver{}
	log := testLogger()
	bus := eventbus.NewMemoryEventBus(log)

	svc := &Service{
		logger:                 log,
		eventBus:               bus,
		backgroundWorkObserver: observer,
	}

	sessionID := "session-proj-1"
	taskID := "task-proj-1"
	workID1 := "work-proj-1"
	workID2 := "work-proj-2"

	// 1. Initial running observation
	obs1 := streams.WorkloadRunObservation{
		SessionID: sessionID,
		WorkID:    workID1,
		Kind:      streams.WorkloadKindShell,
		Title:     "build",
		State:     streams.RunStateRunning,
		Revision:  1,
	}
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:           streams.EventTypeBackgroundWorkUpdated,
			BackgroundWork: &obs1,
		},
	})

	// 2. Terminal observation (completed, revision 2)
	obs2 := obs1
	obs2.State = streams.RunStateCompleted
	obs2.Revision = 2
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:           streams.EventTypeBackgroundWorkUpdated,
			BackgroundWork: &obs2,
		},
	})

	// 3. Replay of older running observation (revision 1)
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:           streams.EventTypeBackgroundWorkUpdated,
			BackgroundWork: &obs1,
		},
	})

	// 4. Successor workload (workID2)
	obs3 := streams.WorkloadRunObservation{
		SessionID: sessionID,
		WorkID:    workID2,
		Kind:      streams.WorkloadKindSubagent,
		Title:     "test reviewer",
		State:     streams.RunStateRunning,
		Revision:  1,
	}
	svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		Data: &lifecycle.AgentStreamEventData{
			Type:           streams.EventTypeBackgroundWorkUpdated,
			BackgroundWork: &obs3,
		},
	})

	obsList := observer.allObservations()
	require.Len(t, obsList, 4)
	require.Equal(t, workID1, obsList[0].WorkID)
	require.Equal(t, streams.RunStateRunning, obsList[0].State)
	require.Equal(t, streams.RunStateCompleted, obsList[1].State)
	require.Equal(t, workID2, obsList[3].WorkID)
	require.Equal(t, "", obsList[3].OriginTurnID, "unknown origin turn must remain empty/unknown")
}

func TestBackgroundWorkSnapshotRace(t *testing.T) {
	observer := &mockBackgroundWorkObserver{}
	log := testLogger()
	bus := eventbus.NewMemoryEventBus(log)

	svc := &Service{
		logger:                 log,
		eventBus:               bus,
		backgroundWorkObserver: observer,
	}

	sessionID := "session-race-1"
	taskID := "task-race-1"

	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	const goroutines = 10
	const eventsPerGoroutine = 20

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			<-startBarrier

			for j := 0; j < eventsPerGoroutine; j++ {
				obs := streams.WorkloadRunObservation{
					SessionID: sessionID,
					WorkID:    "work-race-common",
					Kind:      streams.WorkloadKindShell,
					Title:     "concurrent build",
					State:     streams.RunStateRunning,
					Revision:  int64(j + 1),
				}
				svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
					TaskID:    taskID,
					SessionID: sessionID,
					Data: &lifecycle.AgentStreamEventData{
						Type:           streams.EventTypeBackgroundWorkUpdated,
						BackgroundWork: &obs,
					},
				})

				chunk := streams.WorkloadOutputChunk{
					WorkID: "work-race-common",
					Chunk:  "output log\n",
					Offset: int64(j * 10),
				}
				svc.handleAgentStreamEvent(context.Background(), &lifecycle.AgentStreamEventPayload{
					TaskID:    taskID,
					SessionID: sessionID,
					Data: &lifecycle.AgentStreamEventData{
						Type:                 streams.EventTypeBackgroundWorkOutput,
						BackgroundWorkOutput: &chunk,
					},
				})
			}
		}(i)
	}

	close(startBarrier)
	wg.Wait()

	obsList := observer.allObservations()
	require.NotEmpty(t, obsList)
}
