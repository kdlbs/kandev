package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
)

func TestDockerSessionSeedDoesNotApplyPermissionModeToSettings(t *testing.T) {
	for _, selected := range []bool{false, true} {
		name := "unselected settings stay out of session"
		if selected {
			name = "selected settings are copied unchanged"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			sourceDir := filepath.Join(home, ".claude")
			require.NoError(t, os.MkdirAll(sourceDir, 0o700))
			const source = `{"model":"opus","permissions":{"defaultMode":"default"},"env":{"SELECTED":"value"},"hooks":{"PreToolUse":[{"command":"sentinel-hook"}]}}`
			require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "settings.json"), []byte(source), 0o600))

			metadata := map[string]interface{}{}
			if selected {
				metadata[MetadataKeyAgentConfigBundles] = []string{"claude.settings"}
			}
			configDir := filepath.Join(t.TempDir(), "explicit-claude-config")
			request := &ExecutorCreateRequest{
				InstanceID:  "exec-mode",
				AgentConfig: agents.NewClaudeACP(),
				Metadata:    metadata,
				Env:         map[string]string{"CLAUDE_CONFIG_DIR": configDir},
			}
			dataDir := t.TempDir()
			executor := &DockerExecutor{kandevHomeDir: dataDir, logger: newTestLogger()}
			require.NoError(t, executor.seedSessionDir(context.Background(), request))

			sessionDir := SessionDirHostPath(dataDir, request.InstanceID, request.AgentConfig.Runtime().SessionConfig.SessionDirTemplate)
			settings, err := os.ReadFile(filepath.Join(sessionDir, "settings.json"))
			if selected {
				require.NoError(t, err)
				require.JSONEq(t, source, string(settings))
			} else {
				require.ErrorIs(t, err, os.ErrNotExist)
			}
			require.Equal(t, configDir, request.Env["CLAUDE_CONFIG_DIR"], "permission mode preparation must preserve the explicit settings directory")
		})
	}
}
