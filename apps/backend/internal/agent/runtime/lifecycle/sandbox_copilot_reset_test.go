package lifecycle

import (
	"testing"

	policy "github.com/kandev/kandev/internal/agent/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSandboxCopilotResetKeepsPreflightEnvironment(t *testing.T) {
	manager, execution, runner, inventory, _, configDir := newCopilotPassthroughResetFixture(t, "1.0.104", "local")
	execution.SandboxSelection = &policy.SelectionContext{OrgID: "org", ExecutorID: "local"}
	inventory.onRun = func() {
		execution.setRuntimeEnvironment(map[string]string{"COPILOT_HOME": t.TempDir()})
	}

	prepared, err := manager.preparePassthroughReset(t.Context(), execution, runner)
	require.NoError(t, err)
	require.Len(t, inventory.envs, 1)
	require.Equal(t, configDir, inventory.envs[0]["COPILOT_HOME"])
	require.Equal(t, inventory.envs[0]["COPILOT_HOME"], prepared.startRequest.Env["COPILOT_HOME"],
		"replacement must use the environment that selected its plugin mounts")
	require.Equal(t, 1, runner.stopCalls)
}
