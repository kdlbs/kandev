package utility

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

// TestProbeAllowlist_CoversEveryEnabledInferenceAgent guards against the
// `command "<bin>" is not an allowed ACP probe command` regression: every
// agent the registry exposes as an InferenceAgent must have its primary
// command (filepath.Base of cfg.Command.Args()[0]) listed in
// allowedProbeCommands, otherwise the host-utility probe rejects it the
// moment a user installs that CLI on PATH.
func TestProbeAllowlist_CoversEveryEnabledInferenceAgent(t *testing.T) {
	t.Parallel()

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger.NewLogger: %v", err)
	}

	reg := registry.NewRegistry(log)
	reg.LoadDefaults()

	for _, ag := range reg.ListEnabled() {
		ia, ok := ag.(agents.InferenceAgent)
		if !ok {
			continue
		}
		cfg := ia.InferenceConfig()
		if cfg == nil || !cfg.Supported || cfg.Command.IsEmpty() {
			continue
		}
		primary := cfg.Command.Args()[0]
		if resolveProbeCommand(primary) == "" {
			t.Errorf("agent %q: probe binary %q missing from allowedProbeCommands", ag.ID(), primary)
		}
	}
}
