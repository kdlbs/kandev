package lifecycle

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRunIdentitySurvivesCredentialTeardown(t *testing.T) {
	execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session"}
	execution.setRuntimeEnvironment(map[string]string{"KANDEV_RUN_ID": "first", "KANDEV_RUN_TOKEN": "secret"})
	first := newAgentEventPayload(execution)
	execution.setRuntimeEnvironment(map[string]string{"KANDEV_RUN_ID": "second", "KANDEV_RUN_TOKEN": "another"})
	require.Equal(t, "first", first.RunID)
	execution.clearRuntimeEnvironment()
	require.Empty(t, execution.RuntimeEnvironment())
	require.Equal(t, "second", newAgentEventPayload(execution).RunID)
}
