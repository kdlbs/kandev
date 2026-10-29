package process

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSandboxStartupDoesNotExposeArgumentsOrPrivatePaths(t *testing.T) {
	launch, _ := processSandboxFixture(t)
	core, logs := observer.New(zap.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	manager := NewManager(&config.InstanceConfig{WorkDir: t.TempDir(), Sandbox: launch, AgentArgs: []string{"/bin/sh", "private-mcp-capability"}}, log)
	manager.adapter = newStubAdapter()
	require.NoError(t, manager.buildFinalCommand())
	for _, entry := range logs.All() {
		raw, e := json.Marshal(entry.ContextMap())
		require.NoError(t, e)
		require.NotContains(t, string(raw), "private-mcp-capability")
		require.NotContains(t, string(raw), launch.Root)
	}
	manager.cmd.Path = filepath.Join(launch.Root, "missing-executable")
	_, err = manager.startManagedProcess(nil)
	require.Error(t, err)
	require.NotContains(t, err.Error(), launch.Root)
}
