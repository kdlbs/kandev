package lifecycle

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
)

// TestToolTimeoutCoversOfflineBudget pins the launch-check half of
// AC-PLATFORM-DETACHED-AGENT-CONTINUITY-003.1's "Harness tool timeout"
// contract: the harness's own tool-call timeout must outlive the resolved
// offline budget, so a waiting Kandev MCP call is never aborted by the
// harness before ErrOfflineBudgetExhausted would fire. See
// docs/specs/platform/system-design/detached-agent-continuity-02.md.
func TestToolTimeoutCoversOfflineBudget(t *testing.T) {
	t.Run("raises the managed default to budget plus overhead", func(t *testing.T) {
		mgr := newTestManager(t)
		agentConfig, ok := newTestRegistry().Get("claude-acp")
		require.True(t, ok)

		req := &LaunchRequest{
			TaskID: "task-1", SessionID: "session-1",
			EnvironmentResolutionRequired: true,
			Metadata: map[string]interface{}{
				MetadataKeyOfflineBudgetMinutes: "1440",
			},
		}

		env, err := mgr.buildEnvForExecution(context.Background(), "exec-1", req, agentConfig, nil)
		require.NoError(t, err)
		require.Equal(t, strconv.Itoa((1440+10)*60000), env["MCP_TOOL_TIMEOUT"],
			"a 1440-minute budget must raise the 2h default to budget+10m")
	})

	t.Run("keeps the declared default when it already covers the budget", func(t *testing.T) {
		mgr := newTestManager(t)
		agentConfig, ok := newTestRegistry().Get("claude-acp")
		require.True(t, ok)

		req := &LaunchRequest{
			TaskID: "task-1", SessionID: "session-1",
			EnvironmentResolutionRequired: true,
			Metadata: map[string]interface{}{
				MetadataKeyOfflineBudgetMinutes: "15",
			},
		}

		env, err := mgr.buildEnvForExecution(context.Background(), "exec-1", req, agentConfig, nil)
		require.NoError(t, err)
		require.Equal(t, "7200000", env["MCP_TOOL_TIMEOUT"],
			"the declared 2h default already covers a 15m budget and must not be lowered")
	})

	t.Run("fails when a higher-precedence source undercuts the budget", func(t *testing.T) {
		mgr := newTestManager(t)
		agentConfig, ok := newTestRegistry().Get("claude-acp")
		require.True(t, ok)

		req := &LaunchRequest{
			TaskID: "task-1", SessionID: "session-1",
			EnvironmentResolutionRequired: true,
			EnvironmentDefinitions: []runtimeenv.Definition{
				{Key: "MCP_TOOL_TIMEOUT", Literal: "60000", Origin: runtimeenv.OriginExecutorProfile},
			},
			Metadata: map[string]interface{}{
				MetadataKeyOfflineBudgetMinutes: "15",
			},
		}

		_, err := mgr.buildEnvForExecution(context.Background(), "exec-1", req, agentConfig, nil)
		require.ErrorIs(t, err, ErrToolTimeoutBelowOfflineBudget)
	})

	t.Run("accepts a higher-precedence override that meets the floor", func(t *testing.T) {
		mgr := newTestManager(t)
		agentConfig, ok := newTestRegistry().Get("claude-acp")
		require.True(t, ok)

		req := &LaunchRequest{
			TaskID: "task-1", SessionID: "session-1",
			EnvironmentResolutionRequired: true,
			EnvironmentDefinitions: []runtimeenv.Definition{
				{Key: "MCP_TOOL_TIMEOUT", Literal: "960000", Origin: runtimeenv.OriginExecutorProfile},
			},
			Metadata: map[string]interface{}{
				MetadataKeyOfflineBudgetMinutes: "15",
			},
		}

		env, err := mgr.buildEnvForExecution(context.Background(), "exec-1", req, agentConfig, nil)
		require.NoError(t, err)
		require.Equal(t, "960000", env["MCP_TOOL_TIMEOUT"], "an override that clears the floor is left untouched")
	})
}
