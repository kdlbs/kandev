package agents

import "testing"

func TestCodexACPDeclaresRuntimeProviderObservation(t *testing.T) {
	descriptor := NewCodexACP().RuntimeProviderObservation()
	if descriptor.Name != "Codex CLI" || descriptor.Package != "@openai/codex" ||
		descriptor.Source != RuntimeComponentBundled || descriptor.Owner != RuntimeComponentOwnerKandev ||
		descriptor.ExternalVersionEnv != "CODEX_PATH" || descriptor.GuidanceURL != "https://github.com/openai/codex" {
		t.Fatalf("Codex provider descriptor = %#v", descriptor)
	}
}

func TestClaudeACPIdentifiesBundledSDKInsteadOfLoginCLI(t *testing.T) {
	descriptor := NewClaudeACP().RuntimeProviderObservation()
	if descriptor.Name != "Claude Agent SDK" || descriptor.Package != "@anthropic-ai/claude-agent-sdk" ||
		descriptor.Source != RuntimeComponentBundled || descriptor.Owner != RuntimeComponentOwnerKandev ||
		descriptor.ExternalVersionEnv != "" {
		t.Fatalf("Claude provider descriptor = %#v", descriptor)
	}
}
