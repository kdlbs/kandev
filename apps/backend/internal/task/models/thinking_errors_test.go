package models

import (
	"encoding/json"
	"testing"
)

const thinkingProfileSource = "agent_profile"

func TestPiThinkingCauseEvidence(t *testing.T) {
	yes := true
	cause := AgentErrorCause{Operation: "start", Code: "thinking_level_mismatch", Reason: "effective_mismatch", RequestedSetting: "medium", EffectiveSetting: "off", SettingConfigID: "thought_level", SettingSource: thinkingProfileSource, PromptNotSent: &yes, RequestedMode: "unsafe-legacy"}
	normalized := NormalizeAgentErrorCauses([]AgentErrorCause{cause})
	if len(normalized) != 1 {
		t.Fatalf("lost cause: %+v", normalized)
	}
	got := normalized[0]
	if got.RequestedSetting != "medium" || got.EffectiveSetting != "off" || got.SettingSource != thinkingProfileSource || got.RequestedMode != "" || got.PromptNotSent == nil || !*got.PromptNotSent {
		t.Fatalf("wrong evidence: %+v", got)
	}
	cause.RequestedSetting = "/home/private/token"
	cause.SettingSource = "untrusted"
	got = NormalizeAgentErrorCauses([]AgentErrorCause{cause})[0]
	if got.RequestedSetting != "" || got.SettingSource != "" {
		t.Fatalf("unsafe evidence: %+v", got)
	}
	cause.Code = "permission_mode_failed"
	cause.Reason = "application_failed"
	got = NormalizeAgentErrorCauses([]AgentErrorCause{cause})[0]
	if got.EffectiveSetting != "" || got.SettingConfigID != "" {
		t.Fatal("thinking evidence leaked into permission cause")
	}
}

func TestPiThinkingCauseSurvivesStoredErrorDecode(t *testing.T) {
	var raw map[string]interface{}
	err := json.Unmarshal([]byte(`{"message":"startup failed","causes":[{"operation":"start","code":"thinking_level_mismatch","reason":"effective_mismatch","requested_setting":"medium","effective_setting":"off","setting_config_id":"thought_level","setting_source":"agent_profile","prompt_not_sent":true}]}`), &raw)
	if err != nil {
		t.Fatal(err)
	}
	stored, ok := LoadLastAgentError(map[string]interface{}{SessionMetaKeyLastAgentError: raw})
	if !ok || len(stored.Causes) != 1 {
		t.Fatalf("lost cause: %+v", stored)
	}
	cause := NormalizeAgentErrorCauses(stored.Causes)[0]
	if cause.RequestedSetting != "medium" || cause.EffectiveSetting != "off" || cause.SettingConfigID != "thought_level" || cause.SettingSource != thinkingProfileSource {
		t.Fatalf("lost thinking evidence after reload: %+v", cause)
	}
}
