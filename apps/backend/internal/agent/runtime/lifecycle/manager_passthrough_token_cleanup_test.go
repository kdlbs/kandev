package lifecycle

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

// A per-session config holding the Bearer token is claimed when written and
// removed with the execution, so the token does not outlive the instance.
func TestRemoveExecutionDeletesTokenBearingPassthroughConfig(t *testing.T) {
	mgr := newTestManager(t)
	mgr.dataDir = t.TempDir()
	execution := &AgentExecution{ID: "exec-token", SessionID: "session-token"}
	require.NoError(t, mgr.executionStore.Add(execution))

	paths := mgr.passthroughMCPPaths(execution)
	content := []byte(`{"mcp":{"kandev":{"headers":{"Authorization":"Bearer secret-token"}}}}`)
	require.NoError(t, mgr.writePassthroughMCPFiles(context.Background(), execution,
		[]mcpconfig.PassthroughConfigFile{{Path: paths.TempConfigPath, Content: content}}))
	written, err := os.ReadFile(paths.TempConfigPath)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(written), "secret-token"))

	mgr.RemoveExecution(execution.ID)

	_, err = os.Stat(paths.TempConfigPath)
	require.True(t, os.IsNotExist(err), "token-bearing config must be removed with the execution")
}
