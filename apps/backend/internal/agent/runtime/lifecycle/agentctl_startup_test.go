package lifecycle

import (
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	commonconfig "github.com/kandev/kandev/internal/common/config"
)

func testAgentctlStartupConfig() commonconfig.AgentctlStartupConfig {
	return commonconfig.AgentctlStartupConfig{
		Configured:                true,
		IdleTimeout:               2 * time.Hour,
		IdleReaperInterval:        3 * time.Minute,
		NotificationQueueCapacity: 4096,
		OTLPEndpoint:              "http://collector:4318",
	}
}

func TestAgentctlStartupContractReachesContainerSpriteAndSSHEnvironments(t *testing.T) {
	startup := testAgentctlStartupConfig()
	encoded, err := commonconfig.EncodeAgentctlStartupConfig(startup)
	if err != nil {
		t.Fatalf("EncodeAgentctlStartupConfig: %v", err)
	}

	containerEnv, err := (&ContainerManager{}).buildEnvVars(ContainerConfig{
		AgentConfig:           agents.NewMockAgent(),
		AgentctlStartupConfig: startup,
	})
	if err != nil {
		t.Fatalf("buildEnvVars: %v", err)
	}
	if !containsEnvValue(containerEnv, commonconfig.InternalAgentctlStartupConfigEnv, encoded) {
		t.Fatalf("container environment did not carry the resolved contract: %v", containerEnv)
	}
	if got := countEnvKey(containerEnv, commonconfig.InternalAgentctlStartupConfigEnv); got != 1 {
		t.Fatalf("container environment contains %d startup contracts, want exactly one: %v", got, containerEnv)
	}

	spriteEnv := (&SpritesExecutor{}).buildSpriteEnv(map[string]string{
		commonconfig.InternalAgentctlStartupConfigEnv: "host-value",
	}, startup)
	if !containsEnvValue(spriteEnv, commonconfig.InternalAgentctlStartupConfigEnv, encoded) {
		t.Fatalf("sprite environment did not override the host contract: %v", spriteEnv)
	}

	sshEnv := sshAgentctlLaunchEnv(map[string]string{
		commonconfig.InternalAgentctlStartupConfigEnv: "host-value",
	}, "nonce", startup)
	if sshEnv[commonconfig.InternalAgentctlStartupConfigEnv] != encoded {
		t.Fatalf("SSH environment contract = %q, want %q", sshEnv[commonconfig.InternalAgentctlStartupConfigEnv], encoded)
	}
}

func TestAgentctlStartupContractRejectsInvalidLaunchValues(t *testing.T) {
	startup := testAgentctlStartupConfig()
	startup.NotificationQueueCapacity = 1
	if err := validateAgentctlStartupConfig(startup); err == nil {
		t.Fatal("validateAgentctlStartupConfig accepted an invalid queue capacity")
	}
}

func containsEnvValue(env []string, key, want string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) && strings.TrimPrefix(entry, prefix) == want {
			return true
		}
	}
	return false
}

func countEnvKey(env []string, key string) int {
	prefix := key + "="
	count := 0
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			count++
		}
	}
	return count
}
