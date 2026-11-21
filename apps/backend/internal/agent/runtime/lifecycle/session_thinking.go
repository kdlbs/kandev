package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/task/models"
)

func classifyThinkingFailure(failure *BootstrapFailure) {
	switch failure.Code {
	case models.AgentErrorCauseCodePermissionModeFailed:
		failure.Code = models.AgentErrorCauseCodeThinkingLevelFailed
	case models.AgentErrorCauseCodePermissionModeMismatch:
		failure.Code = models.AgentErrorCauseCodeThinkingLevelMismatch
	case models.AgentErrorCauseCodePermissionModeUnconfirmed:
		failure.Code = models.AgentErrorCauseCodeThinkingLevelUnconfirmed
	}
	failure.Detail = bootstrapFailureDetail(failure.Code)
	failure.RequestedSetting, failure.EffectiveSetting = failure.RequestedMode, failure.EffectiveMode
	failure.RequestedMode, failure.EffectiveMode = "", ""
}

func piThinkingConfigID(state *CachedModelState) string {
	if state != nil {
		if option, ok := acpcompat.ThinkingOption(state.ConfigOptions); ok {
			return option.ID
		}
	}
	// The Pi bridge used this canonical ID before capability projection existed.
	return "thought_level"
}

func (sm *SessionManager) applyStartupSessionMode(ctx context.Context, execution *AgentExecution, sessionID, value, configID string, runtimeOverride bool) error {
	if configID == "" || value == "" {
		return sm.applyExplicitSessionMode(ctx, execution, sessionID, value)
	}
	var err error
	state := execution.GetModelState()
	if state != nil && len(state.ConfigOptions) > 0 {
		option, ok := acpcompat.ThinkingOption(state.ConfigOptions)
		available := false
		if ok {
			for _, choice := range option.Options {
				if choice.Value == value {
					available = true
				}
			}
		}
		if !available {
			failure := permissionModeBootstrapFailure(execution, value, "", models.AgentErrorCauseCodePermissionModeFailed,
				models.AgentErrorCauseReasonRequestedNotAdvertised, fmt.Errorf("thinking level %q is unavailable for the selected model", value))
			failure.Code = models.AgentErrorCauseCodeThinkingLevelUnavailable
			failure.Detail = bootstrapFailureDetail(failure.Code)
			err = failure
		}
	}
	if err == nil {
		err = sm.applyExplicitSessionMode(ctx, execution, sessionID, value)
	}
	var failure *BootstrapFailure
	if errors.As(err, &failure) {
		if failure.RequestedSetting == "" {
			classifyThinkingFailure(failure)
		}
		failure.SettingConfigID = configID
		failure.SettingSource = string(ModeSourceAgentProfile)
		if runtimeOverride {
			failure.SettingSource = string(ModeSourceSessionOverride)
		}
	}
	return err
}
