package models

func normalizeThinkingEvidence(cause *AgentErrorCause) bool {
	permissionCode := ""
	switch cause.Code {
	case AgentErrorCauseCodeThinkingLevelFailed:
		permissionCode = AgentErrorCauseCodePermissionModeFailed
	case AgentErrorCauseCodeThinkingLevelUnavailable:
		permissionCode = AgentErrorCauseCodeModelUnavailable
	case AgentErrorCauseCodeThinkingLevelMismatch:
		permissionCode = AgentErrorCauseCodePermissionModeMismatch
	case AgentErrorCauseCodeThinkingLevelUnconfirmed:
		permissionCode = AgentErrorCauseCodePermissionModeUnconfirmed
	default:
		return false
	}
	requested, effective := cause.RequestedSetting, cause.EffectiveSetting
	configID, source := cause.SettingConfigID, cause.SettingSource
	reason, promptNotSent := cause.Reason, cause.PromptNotSent
	clearAgentErrorCauseEvidence(cause)
	cause.RequestedSetting, cause.EffectiveSetting, cause.SettingConfigID, cause.SettingSource = "", "", "", ""
	if !isKnownAgentErrorCauseReason(permissionCode, reason) {
		return true
	}
	cause.Reason, cause.PromptNotSent = reason, promptNotSent
	cause.RequestedSetting, cause.EffectiveSetting = safeAgentErrorSelector(requested), safeAgentErrorSelector(effective)
	cause.SettingConfigID = safeAgentErrorSelector(configID)
	if source == "agent_profile" || source == "session_override" {
		cause.SettingSource = source
	}
	return true
}
