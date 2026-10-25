package acp

import (
	"context"
	"fmt"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/sessionmodel"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type sessionModeRequest struct {
	conn                 *acp.ClientSideConnection
	sessionID, modeID    string
	option               streams.ConfigOption
	legacyModes          []streams.SessionModeInfo
	models               []modelInfo
	generation, baseline uint64
	uncertain            bool
}

func selectSessionModeOption(config []streams.ConfigOption, modes []streams.SessionModeInfo, modeID, configID string) (streams.ConfigOption, bool, error) {
	option, found := modeConfigOption(config)
	if configID != "" {
		found = false
		for _, candidate := range config {
			if candidate.ID == configID && isModeConfigOption(candidate) {
				option, found = candidate, true
				break
			}
		}
		if !found {
			return option, false, fmt.Errorf("config option %q is not an advertised mode option", configID)
		}
	}
	if found && !configOptionAdvertisesValue(option, modeID) {
		return option, true, fmt.Errorf("requested mode %q is not advertised by config option %q", modeID, option.ID)
	}
	if !found && !legacyModesAdvertise(modes, modeID) {
		return option, false, fmt.Errorf("requested mode %q is not advertised by the active session", modeID)
	}
	return option, found, nil
}

func (a *Adapter) setConfigSessionMode(ctx context.Context, request sessionModeRequest) (streams.ModeResult, bool, error) {
	unknown := streams.ModeResult{Requested: request.modeID}
	if err := ctx.Err(); err != nil {
		return unknown, false, err
	}
	response, err := request.conn.SetSessionConfigOption(ctx, acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: acp.SessionId(request.sessionID), ConfigId: acp.SessionConfigId(request.option.ID),
			Value: acp.SessionConfigValueId(request.modeID),
		},
	})
	if err != nil {
		if !sessionmodel.IsMethodNotFound(err) {
			return unknown, true, fmt.Errorf("set session mode config option failed: %w", err)
		}
		if !legacyModesAdvertise(request.legacyModes, request.modeID) {
			return unknown, false, fmt.Errorf("set session mode config option failed: %w", err)
		}
		// Notifications from the failed config RPC cannot confirm the legacy RPC.
		baseline := a.currentModeSnapshot().generation
		return a.setLegacySessionMode(ctx, request.conn, request.sessionID, request.modeID, baseline, request.uncertain)
	}
	if !a.isActiveSession(request.sessionID) {
		return unknown, true, nil
	}
	if len(response.ConfigOptions) > 0 {
		a.emitAuthoritativeConfigOptions(request.sessionID, request.option.ID, response.ConfigOptions, request.models, true, request.generation)
		if effective := currentModeFromConfig(convertACPConfigOptions(response.ConfigOptions)); effective != "" {
			return streams.ModeResult{Requested: request.modeID, Effective: effective, Confirmed: true}, true, nil
		}
	}
	result := a.awaitModeSettle(ctx, request.sessionID, request.modeID, request.baseline)
	// Uncorrelated notifications can belong to an earlier timed-out request.
	if request.uncertain {
		return unknown, true, nil
	}
	return result, true, nil
}

func (a *Adapter) alreadySatisfiedLegacyMode(
	ctx context.Context,
	conn *acp.ClientSideConnection,
	sessionID, modeID string,
) (uint64, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	if a.closed || a.acpConn != conn || a.sessionID != sessionID || a.modeSessionID != sessionID ||
		a.currentModeID != modeID || a.modeOutcomeUncertain {
		return 0, false, nil
	}
	if _, hasModeOption := modeConfigOption(a.availableConfigOptions); hasModeOption {
		return 0, false, nil
	}
	if !legacyModesAdvertise(a.availableModes, modeID) {
		return 0, false, nil
	}
	a.sessionSettingsGeneration++
	return a.sessionSettingsGeneration, true, nil
}

func (a *Adapter) emitSessionModeResult(sessionID, modeID string, result streams.ModeResult, settingsGeneration uint64) {
	a.mu.RLock()
	if a.sessionID != sessionID {
		a.mu.RUnlock()
		return
	}
	cachedModes := append([]streams.SessionModeInfo(nil), a.availableModes...)
	a.mu.RUnlock()

	reported, requested := sessionModeEventFields(modeID, result)
	event := AgentEvent{
		Type:           streams.EventTypeSessionMode,
		SessionID:      sessionID,
		CurrentModeID:  reported,
		AvailableModes: cachedModes,
	}
	if settingsGeneration == 0 {
		settingsGeneration = a.nextSessionSettingsGeneration(sessionID)
	}
	event.SessionSettingsGeneration = settingsGeneration
	event.RequestedModeID = requested
	a.sendUpdate(event)
}
