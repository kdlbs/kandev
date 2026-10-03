package registry

import (
	"github.com/kandev/kandev/internal/agent/agents"
	"testing"
)

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.1, AC-AGENTS-RUNTIME-NOTIFY-002.2
func TestEveryDefaultRegistrationHasAnExplicitRuntimeUpdateCapability(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	reg := NewRegistry(newTestLogger())
	reg.LoadDefaults()
	expected := map[string]string{
		"claude-acp": "managed", "codex-acp": "managed", "codex-app-server": "managed", "copilot-acp": "managed", "gemini": "managed", "opencode-acp": "managed", "pi-acp": "managed", "muse-acp": "managed",
		"auggie": "manual", "amp-acp": "manual", "qwen-acp": "manual", "iflow-acp": "manual", "droid-acp": "manual", "kilocode-acp": "manual", "cursor-acp": "manual", "kimi-acp": "manual", "minimax-acp": "manual", "kiro-acp": "manual", "qoder-acp": "manual", "trae-acp": "manual", "omp-acp": "manual", "devin-acp": "manual", "grok-acp": "manual", "hermes-acp": "manual", "goose-acp": "manual", "antigravity-acp": "manual",
		"dynamic": "unsupported", "mock-agent": "unsupported", "cursor_cloud": "unsupported",
	}
	all := reg.List()
	if len(all) != len(expected) {
		t.Fatalf("registrations=%d coverage=%d: update the capability matrix", len(all), len(expected))
	}
	for _, ag := range all {
		cap := agents.RuntimeUpdateCapabilities(ag)
		if cap.Management != expected[ag.ID()] || cap.RuntimeID == "" || cap.Owner == "" || cap.Mechanism == "" {
			t.Errorf("%s: %+v; expected %s", ag.ID(), cap, expected[ag.ID()])
		}
		if cap.Management == "managed" && (cap.Managed == nil || cap.Source.NPM == "") {
			t.Errorf("managed registration %s has no trusted recipe/source", ag.ID())
		}
		if cap.Management != "managed" && cap.Managed != nil {
			t.Errorf("external registration %s authorizes mutation", ag.ID())
		}
	}
}
