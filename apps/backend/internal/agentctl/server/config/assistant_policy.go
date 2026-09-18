package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

const AssistantToolPolicy = "claude-broker-v1"

func (c *InstanceConfig) AssistantRestricted() bool {
	return c.McpProfile != nil && c.McpProfile.Surface == mcpprofile.SurfaceAssistantBroker
}

func (c *InstanceConfig) BrokerRestricted() bool {
	return c.McpProfile != nil && (c.McpProfile.Surface == mcpprofile.SurfaceAssistantBroker || c.McpProfile.Surface == mcpprofile.SurfaceOrchestratorBroker)
}

// applyAssistantPolicy replaces every ambient MCP attachment. The executable
// is this managed agentctl binary, never a profile-supplied command.
func applyAssistantPolicy(c *InstanceConfig) {
	if !c.BrokerRestricted() {
		return
	}
	c.AutoApprovePermissions = false
	c.ShellEnabled = false
	c.DisableAskQuestion = true
	executable, err := os.Executable()
	if err != nil {
		c.McpServers = nil
		return
	}
	brokerEnv := map[string]string{}
	kept := []string{}
	for _, entry := range c.AgentEnv {
		key, value, _ := strings.Cut(entry, "=")
		switch key {
		case "CLAUDE_CODE_EXECUTABLE", "NODE_OPTIONS", "NODE_PATH":
			continue
		case "KANDEV_API_URL", "KANDEV_API_KEY", "KANDEV_RUN_ID", "KANDEV_TASK_ID", "KANDEV_AGENT_ID", "KANDEV_WORKSPACE_ID", "KANDEV_RUNTIME_API_PREFIX", "KANDEV_PERSONAL_ASSISTANT_ENABLED":
			brokerEnv[key] = value
		}
		kept = append(kept, entry)
	}
	c.AgentEnv = kept
	c.McpServers = []McpServerConfig{{Name: "kandev_assistant", Type: "stdio", Command: executable,
		Args: []string{"kandev", "assistant-mcp"}, Env: brokerEnv}}
}

// RefreshAssistantPolicy reapplies the immutable restriction after runtime
// credentials are refreshed or a managed process is configured for restart.
func RefreshAssistantPolicy(c *InstanceConfig) { applyAssistantPolicy(c) }

func ValidateAssistantCommand(c *InstanceConfig, args []string) error {
	if !c.AssistantRestricted() {
		return nil
	}
	if len(args) != 4 || filepath.Base(args[0]) != "npx" || args[1] != "--yes" ||
		(args[2] != "--prefer-offline" && args[2] != "--prefer-online") || args[3] != "@agentclientprotocol/claude-agent-acp@0.75.1" {
		return fmt.Errorf("assistant policy requires the qualified managed Claude ACP runtime")
	}
	return nil
}
