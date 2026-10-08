package main

import (
	"context"
	"encoding/json"
	"testing"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestAgentProjectScenarioCreatesWorkerThroughMCP(t *testing.T) {
	var received map[string]interface{}
	server := mcpserver.NewMCPServer("test", "1.0", mcpserver.WithToolCapabilities(true))
	server.AddTool(mcp.NewTool("create_agent_project_worker_kandev"), func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		received = req.GetArguments()
		return mcp.NewToolResultText(`{"id":"worker-1","title":"Worker created by the coordinator"}`), nil
	})
	testServer := mcpserver.NewTestServer(server)
	previousServers := mcpServers
	previousClients := mcpClients
	previousCancels := mcpClientCancels
	mcpServers = map[string]mcpServerDef{"kandev": {URL: testServer.URL + "/sse", Type: "sse"}}
	mcpClients = make(map[string]*mcpclient.Client)
	mcpClientCancels = make(map[string]context.CancelFunc)
	t.Cleanup(func() {
		closeMCPClients()
		testServer.Close()
		mcpServers = previousServers
		mcpClients = previousClients
		mcpClientCancels = previousCancels
	})

	emitter, updates := newTestEmitter()
	emitPredefinedScenario(emitter, "agent-project-create-worker")

	if received["tier"] != "economy" || received["title"] != "Worker created by the coordinator" || received["prompt"] != `e2e:message("Project worker completed.")` {
		data, _ := json.Marshal(received)
		t.Fatalf("worker tool arguments = %s", data)
	}
	var messages []string
	for _, update := range updates.getUpdates() {
		if update.notification.Update.AgentMessageChunk != nil {
			messages = append(messages, getTextContent(update))
		}
	}
	if len(messages) == 0 || messages[len(messages)-1] != "The project worker was created." {
		t.Fatalf("agent messages = %v, want project worker success", messages)
	}
}
