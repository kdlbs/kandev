package acp

import (
	"context"
	"strings"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-BUILTIN-MCP-003.3
func TestBuiltinSessionRequestsRejectUnsupportedInjection(t *testing.T) {
	for _, action := range []string{"new", "load", "resume", "reset"} {
		for _, supported := range []bool{false, true} {
			t.Run(action+map[bool]string{false: "/unsupported", true: "/sse fallback"}[supported], func(t *testing.T) {
				a, peer := newSessionResumeAdapter(t, true, true)
				a.capabilities.McpCapabilities = acpsdk.McpCapabilities{Sse: supported}
				servers := []types.McpServer{
					{Name: "kandev", Type: "http", URL: "http://localhost:1234/mcp", BuiltinPackage: p.PackageCore},
					{Name: "kandev", Type: "sse", URL: "http://localhost:1234/sse", BuiltinPackage: p.PackageCore},
					{Name: "kandev-configuration", Type: "http", URL: "http://localhost:1234/mcp/packages/configuration", BuiltinPackage: p.PackageConfiguration},
					{Name: "kandev-configuration", Type: "sse", URL: "http://localhost:1234/sse/packages/configuration", BuiltinPackage: p.PackageConfiguration},
				}
				var err error
				switch action {
				case "new":
					_, err = a.NewSession(context.Background(), servers)
				case "load":
					err = a.LoadSession(context.Background(), "saved", servers)
				case "resume":
					err = a.ResumeSession(context.Background(), "saved", servers)
				case "reset":
					_, err = a.ResetSession(context.Background(), servers)
				}
				if !supported {
					require.ErrorContains(t, err, "built-in MCP")
					require.Zero(t, peer.sessionCounter)
					require.Zero(t, peer.resumeCalls)
					require.Empty(t, peer.loadRequest.SessionId)
					return
				}
				require.NoError(t, err)
				selected := peer.newRequest.McpServers
				if action == "load" || action == "resume" {
					selected = peer.resumeRequest.McpServers
				}
				require.Len(t, selected, 2)
				for _, server := range selected {
					require.NotNil(t, server.Sse)
				}
			})
		}
	}
}

// @covers AC-AGENTS-BUILTIN-MCP-003.6, AC-AGENTS-BUILTIN-MCP-003.5
func TestBuiltinSessionRequestsAcceptPreparedProjectDelivery(t *testing.T) {
	servers := builtinProjectDeliveryServers(p.PackageCore, p.PackageDiagnostics)
	delivery := projectDeliveryRecord(p.PackageCore, p.PackageDiagnostics)
	for _, action := range []string{"new", "load", "resume", "reset"} {
		t.Run(action, func(t *testing.T) {
			a, peer := newProjectDeliveryAdapter(t, delivery)
			var err error
			switch action {
			case "new":
				_, err = a.NewSession(context.Background(), servers)
			case "load":
				err = a.LoadSession(context.Background(), "saved", servers)
			case "resume":
				err = a.ResumeSession(context.Background(), "saved", servers)
			case "reset":
				_, err = a.ResetSession(context.Background(), servers)
			}
			require.NoError(t, err)
			require.Zero(t, peer.loadRequest.SessionId, "load must not run when resume is advertised")
			var wire []acpsdk.McpServer
			switch action {
			case "new", "reset":
				require.Equal(t, 1, peer.sessionCounter)
				wire = peer.newRequest.McpServers
			case "resume":
				require.Equal(t, 1, peer.resumeCalls)
				wire = peer.resumeRequest.McpServers
			case "load":
				require.Equal(t, 1, peer.resumeCalls)
				wire = peer.resumeRequest.McpServers
			}
			require.Empty(t, wire, "project delivery must never add ACP wire entries")
		})
	}
}

// @covers AC-AGENTS-BUILTIN-MCP-002.4, AC-AGENTS-BUILTIN-MCP-003.2
func TestBuiltinSessionRequestsRejectPartialProjectDelivery(t *testing.T) {
	servers := builtinProjectDeliveryServers(p.PackageCore, p.PackageDiagnostics)
	delivery := projectDeliveryRecord(p.PackageCore)
	for _, action := range []string{"new", "load", "resume", "reset"} {
		t.Run(action, func(t *testing.T) {
			a, peer := newProjectDeliveryAdapter(t, delivery)
			var err error
			switch action {
			case "new":
				_, err = a.NewSession(context.Background(), servers)
			case "load":
				err = a.LoadSession(context.Background(), "saved", servers)
			case "resume":
				err = a.ResumeSession(context.Background(), "saved", servers)
			case "reset":
				_, err = a.ResetSession(context.Background(), servers)
			}
			require.ErrorContains(t, err, "built-in MCP")
			require.Zero(t, peer.sessionCounter)
			require.Zero(t, peer.resumeCalls)
			require.Empty(t, peer.loadRequest.SessionId)
		})
	}
}

// @covers AC-AGENTS-BUILTIN-MCP-004.1
func TestBuiltinProjectDeliveryEvidenceIsNotConnectionHealth(t *testing.T) {
	a, _ := newProjectDeliveryAdapter(t, projectDeliveryRecord(p.PackageCore))
	ctx := streams.WithMCPAttachmentContext(context.Background(), streams.MCPAttachmentAttempt{
		AttemptID: "attempt-1",
		TaskID:    "task-1",
		SessionID: "session-1",
		StartedAt: time.Now().UTC(),
	})
	servers := append(builtinProjectDeliveryServers(p.PackageCore),
		types.McpServer{Name: "third-party", Type: "http", URL: "https://mcp.example.test/mcp"})
	_, err := a.NewSession(ctx, servers)
	require.NoError(t, err)

	byServer := map[string][]streams.MCPAttachmentEvidence{}
	for _, event := range drainEvents(a) {
		if event.MCPAttachment == nil {
			continue
		}
		evidence := *event.MCPAttachment
		byServer[evidence.ServerName] = append(byServer[evidence.ServerName], evidence)
		if evidence.Kind == streams.MCPAttachmentEvidenceToolsListObserved ||
			evidence.Kind == streams.MCPAttachmentEvidenceToolCallObserved {
			t.Fatalf("project delivery invented connection health: %+v", evidence)
		}
	}
	core := byServer["kandev"]
	require.Len(t, core, 2, "HTTP/SSE alternatives must deduplicate by package identity")
	require.Equal(t, streams.MCPAttachmentEvidenceDelivered, core[0].Kind)
	require.Equal(t, projectMCPDeliveryReasonCode, core[0].ReasonCode)
	require.Equal(t, streams.MCPAttachmentEvidenceSessionAccepted, core[1].Kind)
	require.Equal(t, projectMCPDeliveryReasonCode, core[1].ReasonCode)
	thirdParty := byServer["third-party"]
	require.Len(t, thirdParty, 1)
	require.Equal(t, streams.MCPAttachmentEvidenceFiltered, thirdParty[0].Kind)
	require.Equal(t, mcpFilterReasonHTTPUnsupported, thirdParty[0].ReasonCode)
}

func TestBuiltinProjectDeliverySkipsDuplicateTransportEvidence(t *testing.T) {
	a, _ := newSessionResumeAdapter(t, true, true)
	a.cfg.ProjectMCPDelivery = projectDeliveryRecord(p.PackageCore)
	servers := builtinProjectDeliveryServers(p.PackageCore)
	ctx := streams.WithMCPAttachmentContext(context.Background(), streams.MCPAttachmentAttempt{
		AttemptID: "attempt-1", TaskID: "task-1", SessionID: "session-1", StartedAt: time.Now().UTC(),
	})
	_, err := a.NewSession(ctx, servers)
	require.NoError(t, err)
	var filtered int
	for _, event := range drainEvents(a) {
		if event.MCPAttachment == nil {
			continue
		}
		if event.MCPAttachment.Kind == streams.MCPAttachmentEvidenceFiltered {
			filtered++
		}
	}
	require.Zero(t, filtered, "a delivered first transport must not emit a duplicate/filtered decision")
}

func newProjectDeliveryAdapter(t *testing.T, delivery *types.ProjectMCPDelivery) (*Adapter, *sessionResumeAgent) {
	t.Helper()
	a, peer := newSessionResumeAdapter(t, true, true)
	a.capabilities.McpCapabilities = acpsdk.McpCapabilities{}
	a.cfg.ProjectMCPDelivery = delivery
	return a, peer
}

func builtinProjectDeliveryServers(packages ...p.PackageID) []types.McpServer {
	servers := make([]types.McpServer, 0, len(packages)*2)
	for _, pkg := range packages {
		name := "kandev"
		if pkg != p.PackageCore {
			name = "kandev-" + string(pkg)
		}
		servers = append(servers,
			types.McpServer{Name: name, Type: "http", URL: "http://localhost:1234/mcp", BuiltinPackage: pkg},
			types.McpServer{Name: name, Type: "sse", URL: "http://localhost:1234/sse", BuiltinPackage: pkg},
		)
	}
	return servers
}

func projectDeliveryRecord(packages ...p.PackageID) *types.ProjectMCPDelivery {
	return &types.ProjectMCPDelivery{
		Version:       types.ProjectMCPDeliveryVersion,
		Strategy:      types.ProjectMCPDeliveryStrategyPi,
		Workspace:     "/workspace",
		TaskID:        "task-1",
		InstanceID:    "instance-1",
		BindingDigest: strings.Repeat("a", 64),
		Packages:      packageNames(packages),
	}
}

func packageNames(packages []p.PackageID) []string {
	names := make([]string, 0, len(packages))
	for _, pkg := range packages {
		names = append(names, string(pkg))
	}
	return names
}
