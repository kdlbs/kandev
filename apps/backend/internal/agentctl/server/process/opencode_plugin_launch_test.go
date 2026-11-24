package process

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

func newOpenCodePluginManager(t *testing.T, mutate func(*config.InstanceConfig)) *Manager {
	t.Helper()
	home := t.TempDir()
	work := t.TempDir()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	require.NoError(t, err)
	cfg := &config.InstanceConfig{
		HomeDir:                    home,
		WorkDir:                    work,
		SessionID:                  "session-1",
		TaskID:                     "task-1",
		Protocol:                   agent.ProtocolACP,
		AgentType:                  agents.OpenCodeACPAgentID,
		AgentEnv:                   []string{"PATH=/usr/bin", "OPENCODE_CONFIG=/home/user/opencode.json"},
		OpenCodePluginEnabled:      true,
		AgentBackgroundWorkEnabled: true,
		OpenCodePluginRuntime: &agentctltypes.OpenCodePluginRuntime{
			Family: "v2", Source: "managed", Version: "2.2.0",
		},
	}
	if mutate != nil {
		mutate(cfg)
	}
	manager := NewManager(cfg, log)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		manager.closeOpenCodePlugin(ctx)
	})
	return manager
}

func TestOpenCodePluginLaunchPreparesHostAndInlineConfiguration(t *testing.T) {
	manager := newOpenCodePluginManager(t, nil)
	manager.prepareOpenCodePlugin()
	host := manager.openCodePlugin()
	require.NotNil(t, host)
	bootstrap := host.BootstrapPath()
	require.FileExists(t, bootstrap)
	info, err := os.Stat(bootstrap)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	value := processEnvValue(manager.cfg.AgentEnv, openCodeConfigContentEnv)
	require.NotEmpty(t, value)
	var document map[string]any
	require.NoError(t, json.Unmarshal([]byte(value), &document))
	plugins := document["plugins"].([]any)
	require.Len(t, plugins, 1)
	entry := plugins[0].(map[string]any)
	require.Equal(t, host.ArtifactDir(), entry["package"])
	require.Equal(t, bootstrap, entry["options"].(map[string]any)["bootstrapFile"])
	require.Equal(t, "/home/user/opencode.json", processEnvValue(manager.cfg.AgentEnv, "OPENCODE_CONFIG"))

	first := host
	manager.prepareOpenCodePlugin()
	require.Same(t, first, manager.openCodePlugin(), "repeated preparation must keep one host")
	require.Equal(t, value, processEnvValue(manager.cfg.AgentEnv, openCodeConfigContentEnv),
		"repeated preparation must not duplicate the plugin entry")
}

func TestOpenCodePluginLaunchStaysOffForEveryDisabledPath(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*config.InstanceConfig)
	}{
		{name: "flag off", mutate: func(cfg *config.InstanceConfig) { cfg.OpenCodePluginEnabled = false }},
		{name: "shared background off", mutate: func(cfg *config.InstanceConfig) { cfg.AgentBackgroundWorkEnabled = false }},
		{name: "runtime evidence missing", mutate: func(cfg *config.InstanceConfig) { cfg.OpenCodePluginRuntime = nil }},
		{name: "v1 runtime", mutate: func(cfg *config.InstanceConfig) { cfg.OpenCodePluginRuntime.Family = "v1" }},
		{name: "unsupported version", mutate: func(cfg *config.InstanceConfig) { cfg.OpenCodePluginRuntime.Version = "2.3.0" }},
		{name: "other provider", mutate: func(cfg *config.InstanceConfig) { cfg.AgentType = "claude-acp" }},
		{name: "non-ACP protocol", mutate: func(cfg *config.InstanceConfig) { cfg.Protocol = "pty" }},
		{name: "utility session", mutate: func(cfg *config.InstanceConfig) { cfg.TaskID = "" }},
		{name: "stripped variable", mutate: func(cfg *config.InstanceConfig) { cfg.StripEnv = []string{openCodeConfigContentEnv} }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			manager := newOpenCodePluginManager(t, testCase.mutate)
			manager.prepareOpenCodePlugin()
			require.Nil(t, manager.openCodePlugin())
			require.Empty(t, processEnvValue(manager.cfg.AgentEnv, openCodeConfigContentEnv))
			require.Equal(t, "/home/user/opencode.json", processEnvValue(manager.cfg.AgentEnv, "OPENCODE_CONFIG"))
		})
	}
}

func TestOpenCodePluginLaunchPreservesMalformedUserConfiguration(t *testing.T) {
	manager := newOpenCodePluginManager(t, func(cfg *config.InstanceConfig) {
		cfg.AgentEnv = append(cfg.AgentEnv, openCodeConfigContentEnv+"={not json")
	})
	manager.prepareOpenCodePlugin()
	require.Nil(t, manager.openCodePlugin(), "a conflict must not activate the integration")
	require.Equal(t, "{not json", processEnvValue(manager.cfg.AgentEnv, openCodeConfigContentEnv),
		"user configuration must not be erased or repaired")
}

func TestOpenCodePluginLaunchCleanupRemovesPrivateFiles(t *testing.T) {
	manager := newOpenCodePluginManager(t, nil)
	manager.prepareOpenCodePlugin()
	host := manager.openCodePlugin()
	require.NotNil(t, host)
	bootstrap := host.BootstrapPath()
	manager.closeOpenCodePlugin(context.Background())
	require.Nil(t, manager.openCodePlugin())
	require.NoFileExists(t, bootstrap)
}
