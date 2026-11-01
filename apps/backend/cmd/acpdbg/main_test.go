package main

import (
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/acpdbg"
)

func TestProbeRunConfigAdvertisesConfiguredAIRCapabilities(t *testing.T) {
	cfg := probeRunConfig(&sharedFlags{
		workdir:         "/tmp/worktree",
		stderr:          true,
		airCapabilities: "asyncTasks, promptHandoff",
	}, acpdbg.AgentSpec{ID: "claude", Command: []string{"claude", "acp"}})

	if !reflect.DeepEqual(cfg.AIRCapabilities, []string{"asyncTasks", "promptHandoff"}) {
		t.Fatalf("AIR capabilities = %v, want [asyncTasks promptHandoff]", cfg.AIRCapabilities)
	}
	if cfg.AgentID != "claude" || !reflect.DeepEqual(cfg.Command, []string{"claude", "acp"}) {
		t.Fatalf("probe run config lost agent command: %+v", cfg)
	}
}
