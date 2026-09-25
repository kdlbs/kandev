package agents

import "testing"

func TestCodexACP_OpenAICompatibleProvider(t *testing.T) {
	spec := NewCodexACP().OpenAICompatibleProvider()
	if spec == nil {
		t.Fatal("codex-acp should advertise OpenAI-compatible provider support")
	}
	if spec.AuthMethodID != "gateway" {
		t.Errorf("AuthMethodID = %q, want gateway", spec.AuthMethodID)
	}
	if spec.KeyEnvVar != "OPENAI_API_KEY" {
		t.Errorf("KeyEnvVar = %q, want OPENAI_API_KEY", spec.KeyEnvVar)
	}
	if spec.ProviderName == "" {
		t.Error("ProviderName should be set")
	}
}

func TestOpenAICompatibleProviderSpecFor_UnsupportedAgentReturnsNil(t *testing.T) {
	if spec := OpenAICompatibleProviderSpecFor(NewClaudeACP()); spec != nil {
		t.Errorf("claude-acp should not advertise provider support, got %+v", spec)
	}
}
