package lifecycle

import (
	"context"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type startupModelSelection struct {
	model, appliedModel string
	handled             bool
	decision            ModelSelectionDecision
	err                 error
	configuration       streams.ConfigurationResult
	previousGeneration  uint64
}

type startupModelApplier struct {
	client *agentctl.Client
	result streams.ConfigurationResult
}

func (a *startupModelApplier) SetModel(ctx context.Context, model string) error {
	var err error
	a.result, err = a.client.SetModelWithResult(ctx, model)
	return err
}

func cursorCatalogGeneration(client *agentctl.Client, sessionID string) uint64 {
	var generation uint64
	for _, state := range []*streams.SessionModelState{client.GetLastSessionModelState(), client.GetLastCompleteModelState()} {
		if state != nil && state.SessionID == sessionID {
			generation = max(generation, state.SessionSettingsGeneration)
		}
	}
	return generation
}

func cursorStartupModelCatalog(ctx context.Context, execution *AgentExecution, sessionID string, selection startupModelSelection) *CachedModelState {
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return nil
	}
	modelID := selection.decision.EffectiveModel
	latest := client.GetLastCompleteModelState()
	if !selection.decision.SetModelCalled {
		if matchesCursorCatalog(latest, sessionID, modelID, 0) {
			return cachedModelStateFromStream(latest)
		}
		return nil
	}
	if modelID == "" {
		return nil
	}
	if result := selection.configuration; result.Confirmed && result.Effective == modelID &&
		matchesCursorCatalog(result.ModelState, sessionID, modelID, selection.previousGeneration) &&
		(latest == nil || latest.SessionSettingsGeneration <= result.ModelState.SessionSettingsGeneration) {
		return cachedModelStateFromStream(result.ModelState)
	}

	wait, cancel := context.WithTimeout(ctx, freshSessionModelStateWait)
	defer cancel()
	ticker := time.NewTicker(freshSessionModelStatePoll)
	defer ticker.Stop()
	for {
		state := client.GetLastCompleteModelState()
		if matchesCursorCatalog(state, sessionID, modelID, selection.previousGeneration) {
			return cachedModelStateFromStream(state)
		}
		select {
		case <-wait.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func matchesCursorCatalog(state *streams.SessionModelState, sessionID, modelID string, afterGeneration uint64) bool {
	return state != nil && state.ConfigOptionsComplete && state.SessionID == sessionID &&
		state.SessionSettingsGeneration > afterGeneration && state.CurrentModelID != "" &&
		(modelID == "" || state.CurrentModelID == modelID)
}

func verifiedStartupOptions(options map[string]string, state *CachedModelState) map[string]string {
	if _, known := capturedRuntimeConfigOptionCatalog(state); !known {
		return nil
	}
	return sanitizeRuntimeConfigOptionsWithCatalog(options, state)
}
