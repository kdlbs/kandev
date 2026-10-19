package lifecycle

import (
	"encoding/json"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// A retained event keeps the child generation that produced it even when a
// replacement stream replays it under a newer lifecycle startup attempt.
func agentEventBelongsToRetiredProcess(execution *AgentExecution, event agentctl.AgentEvent) bool {
	value, ok := event.Data["process_generation"]
	if !ok || value == nil {
		return false
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return false
	}
	var generation uint64
	if err := json.Unmarshal(raw, &generation); err != nil || generation == 0 {
		return false
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	return client != nil && generation < client.ProcessGeneration()
}
