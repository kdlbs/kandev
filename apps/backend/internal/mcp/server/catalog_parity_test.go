package mcp

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcp "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A task role is an additive policy choice: it must not alter the base Kanban
// catalog that session peers receive from the same server revision.
func TestKanbanCatalogParityRetainsMessageTaskAcrossAdditiveCapabilities(t *testing.T) {
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)

	base := NewWithProfile(backend, "parent", "task-parent", 10005, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, nil))
	peer := NewWithProfile(backend, "peer", "task-peer", 10006, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, nil))
	child := NewWithProfile(backend, "child", "task-child", 10006, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, []mcpprofile.Capability{
			mcpprofile.CapabilityParentQuestion,
			mcpprofile.CapabilityTaskTitle,
			mcpprofile.CapabilityCanvas,
		}, []string{"github"}))

	baseTools := toolNameSet(base)
	peerTools := toolNameSet(peer)
	childTools := toolNameSet(child)

	assert.Equal(t, baseTools, peerTools, "same profile revisions serve the same base catalog")
	require.Contains(t, baseTools, "message_task_kandev")
	require.Contains(t, childTools, "message_task_kandev")
	for name := range baseTools {
		assert.Contains(t, childTools, name, "additive capabilities must retain base tool %q", name)
	}

	assert.Contains(t, childTools, "ask_parent_question_kandev")
	assert.Contains(t, childTools, "set_task_title_kandev")
	assert.Contains(t, childTools, "create_canvas_kandev")
	assert.Contains(t, childTools, "report_change_request_auto_fix_outcome_kandev")
}

func TestKanbanCatalogDigestIncludesMessageTaskBeforeBoundedPublication(t *testing.T) {
	log := newTestLogger(t)
	backend := &testBackend{}
	server := NewWithProfile(backend, "session", "task", 10005, log, "", false,
		mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, nil))
	evidenceEvents := make(chan streams.MCPAttachmentEvidence, 2)
	server.SetAttachmentReporter(func(evidence streams.MCPAttachmentEvidence) { evidenceEvents <- evidence })
	server.SetAttachmentAttempt(streams.MCPAttachmentAttempt{AttemptID: "attempt-1"})

	server.registerMCPConnection("connection-1")
	<-evidenceEvents
	tools := registeredTools(server)
	server.observeMCPToolsList("connection-1", tools)
	evidence := <-evidenceEvents

	require.Equal(t, streams.MCPAttachmentEvidenceToolsListObserved, evidence.Kind)
	require.Equal(t, len(tools), evidence.ToolCount)
	require.NotEmpty(t, evidence.ToolCatalogHash)
	require.Equal(t, streams.MCPToolCatalogDigestAlgorithm, evidence.ToolCatalogHashAlgorithm)
	require.Contains(t, evidenceToolNames(evidence.Tools), "message_task_kandev")
	assert.Empty(t, backend.lastAction, "catalog observation is evidence only; it must not invoke message delivery")

	summaries := make([]streams.MCPToolSummary, 0, len(tools))
	for _, tool := range tools {
		summaries = append(summaries, summarizeMCPTool(tool))
	}
	wantDigest, wantAlgorithm := streams.MCPToolCatalogDigest(summaries)
	assert.Equal(t, wantDigest, evidence.ToolCatalogHash)
	assert.Equal(t, wantAlgorithm, evidence.ToolCatalogHashAlgorithm)
}

func toolNameSet(server *Server) map[string]struct{} {
	names := make(map[string]struct{})
	for _, name := range getRegisteredToolNames(server) {
		names[name] = struct{}{}
	}
	return names
}

func registeredTools(server *Server) []mcp.Tool {
	toolsByName := server.mcpServer.ListTools()
	tools := make([]mcp.Tool, 0, len(toolsByName))
	for _, registered := range toolsByName {
		tools = append(tools, registered.Tool)
	}
	return tools
}

func evidenceToolNames(tools []streams.MCPToolSummary) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}
