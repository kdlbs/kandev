package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
)

// AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.14
func TestDockerSelectedSettingsKeepRequestedStartMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	settingsDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		t.Fatalf("create selected config directory: %v", err)
	}
	selectedSettings := `{"model":"opus","permissions":{"defaultMode":"default"},"env":{"SELECTED":"value"}}`
	if err := os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(selectedSettings), 0o600); err != nil {
		t.Fatalf("seed selected settings: %v", err)
	}

	m := initialModeManager(t)
	outcome := m.applyInitialMode(map[string]string{}, "exec-mode", agents.NewClaudeACP(), "bypassPermissions", "local_docker")
	if outcome.Request == nil || outcome.Delivered {
		t.Fatalf("initial mode outcome = %+v, want a pending executor installation", outcome)
	}
	executor := &DockerExecutor{kandevHomeDir: m.dataDir, logger: newTestLogger()}
	request := &ExecutorCreateRequest{
		InstanceID:  "exec-mode",
		AgentConfig: agents.NewClaudeACP(),
		InitialMode: outcome.Request,
		Metadata: map[string]interface{}{
			MetadataKeyAgentConfigBundles: []string{"claude.settings"},
		},
	}
	if err := executor.seedSessionDir(context.Background(), request); err != nil {
		t.Fatalf("seed session directory: %v", err)
	}

	finalDir := SessionDirHostPath(m.dataDir, request.InstanceID, request.AgentConfig.Runtime().SessionConfig.SessionDirTemplate)
	settings := deliveredSettings(t, finalDir)
	if got := deliveredSettingsMode(t, finalDir); got != "bypassPermissions" {
		t.Fatalf("final defaultMode = %q, want bypassPermissions", got)
	}
	if settings["model"] != "opus" || settings["env"].(map[string]any)["SELECTED"] != "value" {
		t.Fatalf("selected bundle settings were not preserved: %+v", settings)
	}
	if !request.InitialMode.Delivered || request.InitialMode.ConfigDir != request.AgentConfig.Runtime().SessionConfig.SessionDirTarget {
		t.Fatalf("initial mode delivery = %+v, want verified container config directory", request.InitialMode)
	}
}
