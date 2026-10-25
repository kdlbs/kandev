package process

import (
	"slices"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/mcpmode"
	"github.com/kandev/kandev/internal/mcp/profile"
	"go.uber.org/zap"
)

// injectedKandevMCPServerName is the reserved server identifier used by the
// host-injected Kandev MCP entry. This must match kandevMCPServerName in
// adapter/transport/acp/adapter_session.go and kandevMcpServerName in
// server/config/config.go.
const injectedKandevMCPServerName = "kandev"

// coordinatorAutoApprovedNames is the tool list a coordinator session
// auto-approves, regardless of the profile's auto_approve flag or
// AGENTCTL_AUTO_APPROVE_PERMISSIONS
// (docs/specs/coordinator/system-design/permissions.md#auto-approval): the
// names bound at conversation open, or the phase-1 seven when the session has
// no binding. An instance without a coordinator profile approves nothing.
func (m *Manager) coordinatorAutoApprovedNames() []string {
	if m.cfg == nil || m.cfg.McpProfile == nil || m.cfg.McpProfile.Surface != profile.SurfaceCoordinator {
		return nil
	}
	return profile.BoundCoordinatorToolNames(*m.cfg.McpProfile)
}

// autoApproveCoordinatorPermission auto-approves a permission request from a
// coordinator session's agent only when it carries the same host-injected
// MCP provenance as autoApproveInjectedKandevPermission and its tool name
// parses to server "kandev" and one of the session's bound tool names, each
// compared as the full string, never by prefix. It is the only auto-approval
// path consulted in coordinator mode: the blanket AutoApprovePermissions
// flag and the generic "any kandev tool" injected-MCP approval are both
// bypassed for this mode by the caller.
func (m *Manager) autoApproveCoordinatorPermission(req *adapter.PermissionRequest) (*adapter.PermissionResponse, bool) {
	if m.cfg == nil || m.cfg.McpMode != mcpmode.Coordinator {
		return nil, false
	}
	server, tool, option, ok := m.resolveInjectedKandevPermission(req)
	if !ok || server != injectedKandevMCPServerName || !slices.Contains(m.coordinatorAutoApprovedNames(), tool) {
		return nil, false
	}
	m.logger.Info("auto-approving coordinator MCP permission",
		zap.String("reason", "coordinator_tool_allowlist"),
		zap.String("tool", tool),
		zap.String("option_kind", string(option.Kind)))
	return &adapter.PermissionResponse{OptionID: option.OptionID}, true
}

func (m *Manager) autoApproveInjectedKandevPermission(req *adapter.PermissionRequest) (*adapter.PermissionResponse, bool) {
	server, tool, option, ok := m.resolveInjectedKandevPermission(req)
	if !ok || server != injectedKandevMCPServerName {
		return nil, false
	}
	m.logger.Info("auto-approving injected Kandev MCP permission",
		zap.String("reason", "injected_kandev_mcp"),
		zap.String("tool", tool),
		zap.String("option_kind", string(option.Kind)))
	return &adapter.PermissionResponse{OptionID: option.OptionID}, true
}

// resolveInjectedKandevPermission verifies the request targets the genuine
// host-injected Kandev MCP entry (internal construction provenance: exact
// current-port HTTP or SSE server entry, no command/args/env/headers) and
// carries a qualified tool name with an offered allow option. Shared by both
// auto-approval paths so the provenance check can never drift between them.
func (m *Manager) resolveInjectedKandevPermission(req *adapter.PermissionRequest) (server, tool string, option adapter.PermissionOption, ok bool) {
	if req == nil || m.cfg == nil || !m.cfg.InjectedKandevMCP || m.cfg.Port <= 0 {
		return "", "", adapter.PermissionOption{}, false
	}
	if !injectedKandevMCPConfigured(m.cfg) {
		return "", "", adapter.PermissionOption{}, false
	}
	if req.ToolName == nil {
		return "", "", adapter.PermissionOption{}, false
	}
	server, tool, parsed := types.ParseQualifiedMCPToolName(*req.ToolName)
	if !parsed {
		return "", "", adapter.PermissionOption{}, false
	}
	option, ok = injectedKandevPermissionOption(req.Options)
	if !ok {
		return "", "", adapter.PermissionOption{}, false
	}
	return server, tool, option, true
}

func injectedKandevMCPConfigured(cfg *config.InstanceConfig) bool {
	for _, server := range cfg.McpServers {
		if server.Name != injectedKandevMCPServerName || server.Command != "" || len(server.Args) != 0 || len(server.Env) != 0 || len(server.Headers) != 0 {
			continue
		}
		switch server.Type {
		case "http":
			if server.URL == canonicalKandevMCPURL(cfg, "http") && server.URL != "" {
				return true
			}
		case "sse":
			if server.URL == canonicalKandevMCPURL(cfg, "sse") && server.URL != "" {
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

// guardedSession reports whether this instance runs under a sandbox launch. A
// guarded session never receives the blanket or generic injected-server
// approval for Kandev tools.
func (m *Manager) guardedSession() bool {
	return m.cfg != nil && (m.cfg.Sandbox != nil || m.cfg.SandboxRequired)
}

// guardedKandevApproval decides a permission request of a guarded session that
// resolves to the injected Kandev server: only tools the classification table
// lists as read or self are approved, matched by the whole bare name after the
// `_kandev` suffix, never by prefix. Every other tool of that server takes the
// normal prompt without standing approvals. resolved reports that the request targeted
// that server, so neither the blanket flag nor the generic injected-server
// approval applies to it.
func (m *Manager) guardedKandevApproval(req *adapter.PermissionRequest) (resolved bool, response *adapter.PermissionResponse) {
	server, tool, option, ok := m.resolveInjectedKandevPermission(req)
	if !ok || server != injectedKandevMCPServerName {
		return false, nil
	}
	name, hasSuffix := strings.CutSuffix(tool, "_"+injectedKandevMCPServerName)
	entry, listed := profile.GuardedToolByName(name)
	if !hasSuffix || !listed || (entry.Class != profile.ClassRead && entry.Class != profile.ClassSelf) {
		dropStandingApprovals(req)
		return true, nil
	}
	m.logger.Info("auto-approving guarded Kandev MCP permission",
		zap.String("reason", "guarded_tool_class"),
		zap.String("tool", tool),
		zap.String("class", string(entry.Class)),
		zap.String("option_kind", string(option.Kind)))
	return true, &adapter.PermissionResponse{OptionID: option.OptionID}
}

// dropStandingApprovals removes allow_always options from a prompt a person
// will answer, so approving one guarded Kandev call never approves later ones.
func dropStandingApprovals(req *adapter.PermissionRequest) {
	req.Options = slices.DeleteFunc(req.Options, func(option adapter.PermissionOption) bool {
		return normalizePermissionOptionKind(option.Kind) == streams.PermissionOptionKindAllowAlways
	})
}
