package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/task/models"
)

func TestContainerAgentEnvironmentIsIndependentOfSessionMode(t *testing.T) {
	for _, tc := range []struct {
		executor models.ExecutorType
		want     bool
	}{
		{executor: models.ExecutorTypeLocalDocker, want: true},
		{executor: models.ExecutorTypeRemoteDocker, want: true},
		{executor: models.ExecutorTypeSprites, want: true},
		{executor: models.ExecutorTypeKubernetes, want: true},
		{executor: models.ExecutorTypeWorktree},
		{executor: models.ExecutorTypeLocal},
		{executor: models.ExecutorTypeSSH},
	} {
		t.Run(string(tc.executor), func(t *testing.T) {
			env := map[string]string{"CLAUDE_CONFIG_DIR": "/user/selected/config"}
			applyContainerAgentEnvironment(env, agents.NewClaudeACP(), tc.executor)
			if got := env["IS_SANDBOX"] == "1"; got != tc.want {
				t.Fatalf("IS_SANDBOX enabled = %v, want %v for %q", got, tc.want, tc.executor)
			}
			if got := env["CLAUDE_CONFIG_DIR"]; got != "/user/selected/config" {
				t.Fatalf("selected settings directory = %q, want unchanged", got)
			}
		})
	}
}

func TestContainerAgentEnvironmentPreservesExplicitAvailabilityValue(t *testing.T) {
	env := map[string]string{"IS_SANDBOX": "0"}
	applyContainerAgentEnvironment(env, agents.NewClaudeACP(), models.ExecutorTypeLocalDocker)
	if got := env["IS_SANDBOX"]; got != "0" {
		t.Fatalf("explicit sandbox availability = %q, want preserved value", got)
	}
}
