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
	beforeSelection     *streams.SessionModelState
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

func (s *startupModelSelection) captureModelBeforeSelection(client *agentctl.Client, sessionID string) {
	latest, verified := client.GetSessionModelEvidence()
	if latest != nil && latest.SessionID == sessionID {
		s.beforeSelection = latest
	}
	for _, state := range []*streams.SessionModelState{latest, verified} {
		if state != nil && state.SessionID == sessionID {
			s.previousGeneration = max(s.previousGeneration, state.SessionSettingsGeneration)
		}
	}
}

func startupModelCatalog(ctx context.Context, execution *AgentExecution, sessionID string, selection startupModelSelection) *CachedModelState {
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return nil
	}
	modelID := selection.decision.EffectiveModel
	result := selection.configuration
	if result.Confirmed && result.Effective != "" {
		modelID = result.Effective
	}
	latest, verified := client.GetSessionModelEvidence()
	if result.Confirmed && matchesStartupCatalog(result.ModelState, latest, sessionID, modelID, selection.previousGeneration) &&
		(verified == nil || verified.SessionSettingsGeneration <= result.ModelState.SessionSettingsGeneration) {
		return startupCatalogFromReport(result.ModelState)
	}
	if !selection.switchedModel(modelID) {
		return startupCatalogWithoutSwitch(latest, verified, selection.beforeSelection, sessionID, modelID)
	}
	if modelID == "" {
		return nil
	}

	wait, cancel := context.WithTimeout(ctx, freshSessionModelStateWait)
	defer cancel()
	ticker := time.NewTicker(freshSessionModelStatePoll)
	defer ticker.Stop()
	for {
		latest, verified := client.GetSessionModelEvidence()
		if matchesStartupCatalog(verified, latest, sessionID, modelID, selection.previousGeneration) {
			return startupCatalogFromReport(verified)
		}
		select {
		case <-wait.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s startupModelSelection) switchedModel(modelID string) bool {
	return s.decision.SetModelCalled && s.decision.Outcome != ModelSelectionOutcomeProviderDefault &&
		(modelID == "" || s.beforeSelection == nil || modelID != s.beforeSelection.CurrentModelID)
}

func startupCatalogWithoutSwitch(latest, verified, setup *streams.SessionModelState, sessionID, modelID string) *CachedModelState {
	if matchesStartupCatalog(verified, latest, sessionID, modelID, 0) {
		return startupCatalogFromReport(verified)
	}
	if latest != nil && latest.ConfigOptionsDisplayOnly && setup != nil &&
		latest.SessionID == setup.SessionID && latest.CurrentModelID == setup.CurrentModelID &&
		(verified == nil || verified.SessionSettingsGeneration <= setup.SessionSettingsGeneration) {
		latest = setup
	}
	if latest == nil || latest.SessionID != sessionID || latest.ConfigOptionsDisplayOnly ||
		latest.HasConfigOptionEvidence() || (modelID != "" && latest.CurrentModelID != modelID) {
		return nil
	}
	return cachedModelStateFromStream(latest)
}

func matchesStartupCatalog(report, latest *streams.SessionModelState, sessionID, modelID string, afterGeneration uint64) bool {
	if !report.HasConfigOptionEvidence() || report.SessionID != sessionID ||
		report.SessionSettingsGeneration <= afterGeneration || report.CurrentModelID == "" ||
		(modelID != "" && report.CurrentModelID != modelID) {
		return false
	}
	if latest == nil || latest.SessionSettingsGeneration < report.SessionSettingsGeneration {
		return true
	}
	return latest.SessionID == sessionID && latest.CurrentModelID == report.CurrentModelID &&
		(latest.SessionSettingsGeneration == report.SessionSettingsGeneration || latest.ConfigOptionsDisplayOnly)
}

func startupCatalogFromReport(report *streams.SessionModelState) *CachedModelState {
	catalog := cachedModelStateFromStream(report)
	if !report.ConfigOptionsComplete {
		catalog.ConfigOptions = streams.ConfigOptionsForIDs(report.ConfigOptions, report.VerifiedConfigOptionIDs)
	}
	return catalog
}

func verifiedStartupOptions(options map[string]string, state *CachedModelState) map[string]string {
	if _, known := capturedRuntimeConfigOptionCatalog(state); !known {
		return nil
	}
	return sanitizeRuntimeConfigOptionsWithCatalog(options, state)
}
