package acp

import (
	"fmt"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func (a *Adapter) assistantRestricted() bool { return a.cfg.ToolPolicy != "" }

func (a *Adapter) assistantSessionMeta(servers []types.McpServer) (map[string]any, error) {
	if !a.assistantRestricted() {
		return nil, nil
	}
	if a.cfg.ToolPolicy != "claude-broker-v1" || a.agentID != "claude-acp" || a.agentInfo == nil ||
		a.agentInfo.Name != "@agentclientprotocol/claude-agent-acp" || a.agentInfo.Version != "0.75.1" {
		return nil, fmt.Errorf("assistant policy unsupported by this provider version")
	}
	if len(servers) != 1 || servers[0].Name != "kandev_assistant" || servers[0].Command == "" || servers[0].URL != "" {
		return nil, fmt.Errorf("assistant attachment policy requires only the managed broker")
	}
	return map[string]any{
		"disableBuiltInTools": true,
		"claudeCode": map[string]any{"options": map[string]any{
			"tools": []string{}, "settingSources": []string{}, "plugins": []any{},
			"strictMcpConfig": true, "allowedTools": []string{"mcp__kandev_assistant__*"},
			"settings":                map[string]any{"disableAllHooks": true},
			"enableFileCheckpointing": false,
			"extraArgs":               map[string]any{"disable-slash-commands": nil, "permission-mode": "dontAsk"},
		}},
	}, nil
}

func (a *Adapter) clientCapabilities() acpsdk.ClientCapabilities {
	if a.assistantRestricted() {
		return acpsdk.ClientCapabilities{}
	}
	return clientCapabilitiesForAgent(a.agentID)
}
