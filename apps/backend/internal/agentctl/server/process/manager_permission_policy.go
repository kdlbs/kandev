package process

import (
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// injectedKandevMCPServerName is the reserved server identifier used by the
// host-injected Kandev MCP entry. This must match kandevMCPServerName in
// adapter/transport/acp/adapter_session.go and kandevMcpServerName in
// server/config/config.go.
const injectedKandevMCPServerName = "kandev"

func (m *Manager) autoApproveInjectedKandevPermission(req *adapter.PermissionRequest) (*adapter.PermissionResponse, bool) {
	if req == nil || m.cfg == nil || !m.cfg.InjectedKandevMCP || m.cfg.Port <= 0 {
		return nil, false
	}
	if !injectedKandevMCPConfigured(m.cfg) {
		return nil, false
	}
	if req.ToolName == nil {
		return nil, false
	}
	server, tool, ok := types.ParseQualifiedMCPToolName(*req.ToolName)
	if !ok || server != injectedKandevMCPServerName {
		return nil, false
	}

	option, ok := injectedKandevPermissionOption(req.Options)
	if !ok {
		return nil, false
	}
	m.logger.Info("auto-approving injected Kandev MCP permission",
		zap.String("reason", "injected_kandev_mcp"),
		zap.String("tool", tool),
		zap.String("option_kind", string(option.Kind)))
	return &adapter.PermissionResponse{OptionID: option.OptionID}, true
}

func injectedKandevMCPConfigured(cfg *config.InstanceConfig) bool {
	for _, server := range cfg.McpServers {
		if server.Name != injectedKandevMCPServerName || server.Command != "" || len(server.Args) != 0 || len(server.Env) != 0 || len(server.Headers) != 0 {
			continue
		}
		switch server.Type {
		case "http":
			if server.URL == fmt.Sprintf("http://localhost:%d/mcp", cfg.Port) {
				return true
			}
		case "sse":
			if server.URL == fmt.Sprintf("http://localhost:%d/sse", cfg.Port) {
				return true
			}
		}
	}
	return false
}

func injectedKandevPermissionOption(options []adapter.PermissionOption) (adapter.PermissionOption, bool) {
	for _, allowedKind := range []streams.PermissionOptionKind{
		streams.PermissionOptionKindAllowOnce,
		streams.PermissionOptionKindAllowAlways,
	} {
		for _, option := range options {
			if normalizePermissionOptionKind(option.Kind) == allowedKind {
				return option, true
			}
		}
	}
	return adapter.PermissionOption{}, false
}

// normalizePermissionOptionKind folds the surface spelling differences that
// separate providers produce. A kind Kandev fails to recognize is treated as
// "not an allow", which turns an approval into a refusal, so the comparison
// must not depend on casing or surrounding whitespace.
func normalizePermissionOptionKind(kind streams.PermissionOptionKind) streams.PermissionOptionKind {
	return streams.PermissionOptionKind(strings.ToLower(strings.TrimSpace(string(kind))))
}

// isAllowPermissionKind reports whether the provider marked this option as an
// approval rather than a refusal.
func isAllowPermissionKind(kind streams.PermissionOptionKind) bool {
	switch normalizePermissionOptionKind(kind) {
	case streams.PermissionOptionKindAllowOnce, streams.PermissionOptionKindAllowAlways:
		return true
	default:
		return false
	}
}
