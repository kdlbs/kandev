package process

import (
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/pkg/agent"
)

func TestClaudeMonitorEvidenceLaunchIdentity(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	configDir := t.TempDir()
	manager := &Manager{
		cfg: &config.InstanceConfig{
			AgentType: claudeACPAgentType,
			AgentArgs: []string{"cat"},
			AgentEnv: []string{
				"PATH=/usr/bin",
				"HOME=" + homeDir,
				"CLAUDE_CONFIG_DIR=" + configDir,
			},
			WorkDir:  workDir,
			Protocol: agent.ProtocolACP,
		},
		logger: newTestLogger(t),
	}
	if err := manager.buildAdapterConfig(); err != nil {
		t.Fatalf("buildAdapterConfig: %v", err)
	}
	t.Cleanup(func() { _ = manager.adapter.Close() })
	if got := manager.adapterCfg.ClaudeConfigDir; got != configDir {
		t.Fatalf("ClaudeConfigDir = %q, want explicit launch config dir %q", got, configDir)
	}
	if got := manager.adapterCfg.WorkDir; got != filepath.Clean(workDir) {
		t.Fatalf("WorkDir = %q, want canonical launch directory %q", got, filepath.Clean(workDir))
	}

	if err := manager.adapter.Close(); err != nil {
		t.Fatalf("close first adapter: %v", err)
	}
	manager.cfg.AgentEnv = []string{"PATH=/usr/bin", "HOME=" + homeDir}
	if err := manager.buildAdapterConfig(); err != nil {
		t.Fatalf("buildAdapterConfig without explicit Claude config dir: %v", err)
	}
	if got, want := manager.adapterCfg.ClaudeConfigDir, filepath.Join(homeDir, ".claude"); got != want {
		t.Fatalf("default ClaudeConfigDir = %q, want %q", got, want)
	}
	if got := claudeConfigDirFromLaunch(claudeACPAgentType, []string{"HOME=" + homeDir}, []string{"HOME"}); got != "" {
		t.Fatalf("ClaudeConfigDir with stripped HOME = %q, want empty", got)
	}
	if got := claudeConfigDirFromLaunch(claudeACPAgentType, []string{"CLAUDE_CONFIG_DIR=" + configDir}, []string{"CLAUDE_CONFIG_DIR"}); got != "" {
		t.Fatalf("ClaudeConfigDir with stripped override = %q, want empty", got)
	}
}
