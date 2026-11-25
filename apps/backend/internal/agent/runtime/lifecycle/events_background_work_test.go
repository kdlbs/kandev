package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestBackgroundWorkObservationOriginTurn(t *testing.T) {
	manager, _ := createTestManagerWithTracking()
	execution := createTestExecution("execution", "task", "session")
	execution.promptLifecycleMu.Lock()
	execution.promptGeneration = 8
	execution.promptTurnID = "turn-successor"
	execution.promptTurnIDs = map[uint64]string{7: "turn-launch"}
	execution.promptLifecycleMu.Unlock()

	for _, test := range []struct {
		name       string
		generation uint64
		origin     string
		want       string
	}{
		{name: "saved launch generation", generation: 7, want: "turn-launch"},
		{name: "unknown generation", generation: 6},
		{name: "explicit origin", generation: 7, origin: "turn-explicit", want: "turn-explicit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			observation := &streams.WorkloadRunObservation{
				WorkID:       "watch-work",
				Kind:         streams.WorkloadKindMonitor,
				State:        streams.RunStateRunning,
				OriginTurnID: test.origin,
			}
			result := manager.handleAgentEventState(execution, agentctl.AgentEvent{
				Type:             streams.EventTypeBackgroundWorkUpdated,
				PromptGeneration: test.generation,
				BackgroundWork:   observation,
			})
			require.Equal(t, test.want, result.BackgroundWork.OriginTurnID)
			require.Empty(t, result.TurnID, "background work ownership stays on the observation")
		})
	}
}
