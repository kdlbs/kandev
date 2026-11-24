package acpv2

import (
	"context"
	"os/exec"
	"path/filepath"
	"time"

	wire "github.com/coder/acp-go-sdk/experimental/v2"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type mcpDelivery struct {
	servers  []wire.MCPServer
	accepted []types.McpServer
	skipped  map[string]types.McpServer
	err      error
}

func (a *Adapter) prepareMCP(servers []types.McpServer) mcpDelivery {
	var caps *wire.MCPCapabilities
	if a.info.Capabilities != nil && a.info.Capabilities.Session != nil {
		caps = a.info.Capabilities.Session.MCP
	}
	delivery := mcpDelivery{skipped: make(map[string]types.McpServer)}
	selected := make(map[string]bool, len(servers))
	for _, server := range servers {
		if selected[server.Name] {
			continue
		}
		config, ok := mcpConfig(caps, server)
		if !ok {
			delivery.skipped[server.Name] = server
			continue
		}
		selected[server.Name] = true
		delete(delivery.skipped, server.Name)
		delivery.accepted = append(delivery.accepted, server)
		delivery.servers = append(delivery.servers, config)
	}
	delivery.err = shared.RequireBuiltinMCPDelivery(servers, delivery.accepted, a.cfg.ProjectMCPDelivery)
	return delivery
}

func mcpConfig(caps *wire.MCPCapabilities, server types.McpServer) (wire.MCPServer, bool) {
	if caps == nil {
		return wire.MCPServer{}, false
	}
	switch server.Type {
	case "http", "streamable_http":
		if caps.HTTP == nil || server.URL == "" {
			return wire.MCPServer{}, false
		}
		config := &wire.MCPServerHTTP{Name: server.Name, URL: server.URL}
		for key, value := range server.Headers {
			config.Headers = append(config.Headers, wire.HTTPHeader{Name: key, Value: value})
		}
		return wire.MCPServer{HTTP: config}, true
	case "", "stdio":
		if caps.Stdio == nil {
			return wire.MCPServer{}, false
		}
		command, err := exec.LookPath(server.Command)
		if err != nil || !filepath.IsAbs(command) {
			return wire.MCPServer{}, false
		}
		config := &wire.MCPServerStdio{Name: server.Name, Command: command, Args: server.Args}
		for key, value := range server.Env {
			config.Env = append(config.Env, wire.EnvVariable{Name: key, Value: value})
		}
		return wire.MCPServer{Stdio: config}, true
	default:
		return wire.MCPServer{}, false
	}
}

func (*Adapter) PublishesMCPAttachmentResults() bool { return true }

func (a *Adapter) publishMCPStart(ctx context.Context, delivery mcpDelivery) {
	info, ok := streams.MCPAttachmentContextFromContext(ctx)
	if !ok {
		return
	}
	a.emit(streams.AgentEvent{Type: streams.EventTypeMCPAttachment, MCPAttachmentAttempt: &info.Attempt})
	for _, evidence := range info.InitialEvidence {
		a.emit(streams.AgentEvent{Type: streams.EventTypeMCPAttachment, MCPAttachment: &evidence})
	}
	for _, server := range delivery.skipped {
		a.publishMCPEvidence(info.Attempt.AttemptID, server, streams.MCPAttachmentEvidenceFiltered, "transport_unsupported")
	}
	if delivery.err == nil {
		for _, server := range delivery.accepted {
			a.publishMCPEvidence(info.Attempt.AttemptID, server, streams.MCPAttachmentEvidenceDelivered, "")
		}
	}
}

func (a *Adapter) publishMCPResult(ctx context.Context, delivery mcpDelivery, err error) {
	info, ok := streams.MCPAttachmentContextFromContext(ctx)
	if !ok {
		return
	}
	kind := streams.MCPAttachmentEvidenceSessionAccepted
	reason := ""
	if err != nil {
		kind, reason = streams.MCPAttachmentEvidenceExplicitError, "session_request_failed"
	}
	for _, server := range delivery.accepted {
		a.publishMCPEvidence(info.Attempt.AttemptID, server, kind, reason)
	}
}

func (a *Adapter) publishMCPEvidence(attempt string, server types.McpServer, kind streams.MCPAttachmentEvidenceKind, reason string) {
	source := streams.MCPServerSourceProfile
	if server.IsInjectedBuiltin() || server.Name == "kandev" {
		source = streams.MCPServerSourceKandev
	}
	target := streams.SanitizeMCPStdioTarget(server.Command)
	if server.URL != "" {
		target = streams.SanitizeMCPNetworkTarget(server.URL)
	}
	a.emit(streams.AgentEvent{Type: streams.EventTypeMCPAttachment, MCPAttachment: &streams.MCPAttachmentEvidence{
		AttemptID: attempt, OccurredAt: time.Now().UTC(), Kind: kind, ReasonCode: reason,
		BuiltinPackage: server.BuiltinPackage, ServerName: server.Name, Source: source,
		Transport: server.Type, Target: target,
	}})
}
