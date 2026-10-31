package lifecycle

import (
	"encoding/json"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// A retained event keeps the child generation that produced it even when a
// replacement stream replays it under a newer lifecycle startup attempt.
func agentEventBelongsToRetiredProcess(execution *AgentExecution, event agentctl.AgentEvent) bool {
	managerID := agentEventDataString(event.Data, "process_manager_id")
	value, ok := event.Data["process_generation"]
	if managerID == "" && (!ok || value == nil) {
		return false
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if managerID != "" {
		if client == nil || client.ProcessManagerID() == "" || managerID != client.ProcessManagerID() {
			return true
		}
	} else if client != nil && client.ProcessManagerID() != "" && ok && value != nil {
		// A manager that reports its identity also stamps every managed exit.
		// An unmarked retained exit therefore belongs to a legacy manager and
		// cannot be attributed safely to the current process.
		return true
	}
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
	return client != nil && generation < client.ProcessGeneration()
}
