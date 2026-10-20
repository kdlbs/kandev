package lifecycle

import (
	"errors"
	"fmt"

	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (m *Manager) prepareReadyEventForExecution(execution *AgentExecution, attemptID string, completion *agentctlclient.AgentEvent) (*AgentExecution, AgentEventPayload, error) {
	executionID := execution.ID
	var payload AgentEventPayload
	var updated *AgentExecution
	var alreadyReady bool
	// Capture prompt identity under promptLifecycleMu before the store lock.
	if err := m.executionStore.WithPromptLifecycleLock(executionID, func(execution *AgentExecution) {
		if execution.Status == v1.AgentStatusReady {
			alreadyReady = true
			return
		}
		execution.Status = v1.AgentStatusReady
		payload = newAgentEventPayloadPromptLocked(execution)
		payload.AttemptID = attemptID
		if completion != nil {
			payload.TurnID = completion.TurnID
			payload.PromptGeneration = completion.PromptGeneration
		}
		updated = execution
	}); err != nil {
		if errors.Is(err, ErrExecutionNotFound) {
			return nil, AgentEventPayload{}, fmt.Errorf("execution %q not found", executionID)
		}
		return nil, AgentEventPayload{}, err
	}
	if alreadyReady {
		return nil, AgentEventPayload{}, nil
	}
	return updated, payload, nil
}
