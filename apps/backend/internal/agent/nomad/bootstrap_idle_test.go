package nomad

import (
	"strings"
	"testing"
)

func TestBootstrapScriptDisablesAgentctlIdleReaper(t *testing.T) {
	state := validTestState()
	script, err := BuildBootstrapScript(state, state.AllocationRoot)
	if err != nil {
		t.Fatal(err)
	}
	const export = "export KANDEV_ACP_IDLE_TIMEOUT=0\n"
	idx := strings.Index(script, export)
	if idx < 0 {
		t.Fatalf("bootstrap must disable the agentctl idle reaper for a retained environment:\n%s", script)
	}
	if src := strings.Index(script, `. "$bootstrap_env_file"`); idx < src {
		t.Fatal("idle timeout must be exported after the private environment is sourced so it cannot be overridden")
	}
	if idx > strings.Index(script, "exec \"$agentctl_path\"") {
		t.Fatal("idle timeout must be exported before agentctl is exec'd")
	}
}
