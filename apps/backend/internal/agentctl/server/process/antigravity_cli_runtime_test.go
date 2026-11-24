package process

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
)

func TestBuildAdapterConfigForAntigravityCLIWithoutStandaloneBundle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	manager := &Manager{
		cfg: &config.InstanceConfig{
			AgentType: acpcompat.AntigravityAgentID,
			AgentArgs: []string{"/host/bin/agy", "acp"},
			AgentEnv: []string{
				"HOME=" + home,
				"PATH=/host/bin",
				"GEMINI_API_KEY=task-secret",
				"GOOGLE_API_KEY=task-secret",
				"UNRELATED=kept",
			},
			AuthPolicy: acpcompat.AntigravityAuthPolicyForRuntime(
				acpcompat.AntigravityRuntimeCLI, "1.5.15", false,
			),
			Protocol: agent.ProtocolACP,
			WorkDir:  t.TempDir(),
		},
		logger: newTestLogger(t),
	}
	if err := manager.buildAdapterConfig(); err != nil {
		t.Fatalf("buildAdapterConfig without a standalone bundle: %v", err)
	}
	t.Cleanup(func() { _ = manager.adapter.Close() })

	for _, entry := range manager.cfg.AgentEnv {
		if strings.HasPrefix(entry, "GEMINI_API_KEY=") || strings.HasPrefix(entry, "GOOGLE_API_KEY=") {
			t.Fatalf("provider credential survived CLI environment preparation: %q", entry)
		}
	}
	if got := acpcompat.EnvValue(manager.cfg.AgentEnv, "HOME"); got != home {
		t.Fatalf("shared HOME = %q, want %q", got, home)
	}
	if got := acpcompat.EnvValue(manager.cfg.AgentEnv, "ANTIGRAVITY_HARNESS_PATH"); got != "" {
		t.Fatalf("CLI selection unexpectedly configured a standalone harness: %q", got)
	}
	if got := acpcompat.EnvValue(manager.cfg.AgentEnv, "UNRELATED"); got != "kept" {
		t.Fatalf("unrelated environment value = %q, want kept", got)
	}
}
