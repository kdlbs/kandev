package lifecycle

import (
	"context"
	"testing"

	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSandboxRestartReviewIsConsumedBeforeAutomaticReplacement(t *testing.T) {
	selection := &policy.SelectionContext{ExecutorID: "local", RestartReview: &contract.RestartPolicyReview{AttemptID: "previous", SelectionDigest: "review"}}
	execution := &AgentExecution{SandboxSelection: selection}
	manager := &Manager{}
	calls := 0
	manager.SetSandboxRuntime(func(_ context.Context, current *AgentExecution, generation uint64, previous string, _ map[string]string, _ *client.Client) (SandboxAttempt, error) {
		calls++
		if calls == 1 {
			require.NotNil(t, current.SandboxSelection.RestartReview)
		} else {
			require.Nil(t, current.SandboxSelection.RestartReview, "automatic replacement cannot reuse operator review")
			require.Equal(t, "reviewed", previous)
		}
		return &startSandboxFixture{launch: &contract.Launch{AttemptID: "reviewed", Generation: generation}}, nil
	})
	_, err := manager.prepareExecutionSandbox(t.Context(), execution, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, selection.RestartReview, "do not mutate the caller's admission context")
	execution.sandboxDispatched = true
	_, err = manager.prepareExecutionSandbox(t.Context(), execution, nil, nil)
	require.NoError(t, err)
	require.Equal(t, 2, calls)
}
