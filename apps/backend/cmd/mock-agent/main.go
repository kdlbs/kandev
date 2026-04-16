// Package main implements a mock agent binary that speaks the ACP
// (Agent Client Protocol) over stdin/stdout. It generates simulated
// responses for rapid feature testing and e2e web app tests.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

	acp "github.com/coder/acp-go-sdk"
)

// logOutput is the writer for log messages (stderr). Tests can override this.
var logOutput io.Writer = os.Stderr

// mcpServerDef describes an MCP server endpoint parsed from --mcp-config.
type mcpServerDef struct {
	URL  string `json:"url"`
	Type string `json:"type"`
}

// mcpServers holds MCP server definitions from --mcp-config.
var mcpServers map[string]mcpServerDef

// mockAgent implements the acp.Agent interface for the mock agent.
type mockAgent struct {
	conn            *acp.AgentSideConnection
	model           string
	sessions        map[acp.SessionId]bool
	commandsEmitted map[acp.SessionId]bool
	mu              sync.Mutex
}

func main() {
	model := parseModelFlag()

	// TUI mode: simple terminal UI for passthrough/PTY testing
	if parseTUIFlag() {
		resumeID := parseResumeFlag()
		resumed := resumeID != "" || parseContinueFlag()
		runTUI(model, parsePromptFlag(), resumed)
		return
	}

	mcpServers = parseMCPConfigFlag()
	defer closeMCPClients()

	ag := &mockAgent{
		model:           model,
		sessions:        make(map[acp.SessionId]bool),
		commandsEmitted: make(map[acp.SessionId]bool),
	}
	asc := acp.NewAgentSideConnection(ag, os.Stdout, os.Stdin)
	ag.conn = asc

	<-asc.Done()
}

// Initialize handles the ACP initialize request, returning agent capabilities.
func (a *mockAgent) Initialize(_ context.Context, _ acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion: acp.ProtocolVersionNumber,
		AgentCapabilities: acp.AgentCapabilities{
			LoadSession:     true,
			McpCapabilities: acp.McpCapabilities{Sse: true},
		},
	}, nil
}

// NewSession creates a new conversation session.
// MCP servers from the ACP request are registered so callMCPTool can use them.
// The Models field advertises the available models so the host utility
// capability probe can populate them in the cache — this is what makes
// the utility-agents settings page show model options for mock-agent in E2E.
func (a *mockAgent) NewSession(_ context.Context, req acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	sid := acp.SessionId(fmt.Sprintf("mock-session-%d", os.Getpid()))
	a.mu.Lock()
	a.sessions[sid] = true
	a.mu.Unlock()

	// Register MCP servers from the ACP session request (SSE servers).
	// This bridges ACP protocol MCP config to the mock agent's MCP client.
	registerACPMcpServers(req.McpServers)

	return acp.NewSessionResponse{
		SessionId: sid,
		Models:    mockSessionModels(),
	}, nil
}

// mockSessionModels returns the mock agent's advertised model list for ACP
// session responses. Two models are exposed so tests can verify both
// selection and default behavior (mock-fast is the default).
func mockSessionModels() *acp.UnstableSessionModelState {
	fastDesc := "Fast mock model for testing"
	smartDesc := "Smart mock model for testing"
	return &acp.UnstableSessionModelState{
		CurrentModelId: "mock-fast",
		AvailableModels: []acp.UnstableModelInfo{
			{ModelId: "mock-fast", Name: "Mock Fast", Description: &fastDesc},
			{ModelId: "mock-smart", Name: "Mock Smart", Description: &smartDesc},
		},
	}
}

// LoadSession restores a previous session for resume.
func (a *mockAgent) LoadSession(_ context.Context, req acp.LoadSessionRequest) (acp.LoadSessionResponse, error) {
	a.mu.Lock()
	a.sessions[req.SessionId] = true
	a.mu.Unlock()
	_, _ = fmt.Fprintf(logOutput, "mock-agent[%d]: resumed session %s\n", os.Getpid(), req.SessionId)
	return acp.LoadSessionResponse{}, nil
}

// Prompt processes a user message and streams responses via SessionUpdate.
func (a *mockAgent) Prompt(ctx context.Context, req acp.PromptRequest) (acp.PromptResponse, error) {
	a.emitAvailableCommandsOnce(ctx, req.SessionId)
	prompt := extractPromptText(req.Prompt)
	e := &emitter{ctx: ctx, conn: a.conn, sid: req.SessionId}
	handlePrompt(e, prompt, a.model)
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil
}

// Cancel handles session cancellation.
func (a *mockAgent) Cancel(_ context.Context, _ acp.CancelNotification) error { return nil }

// Authenticate handles auth requests (no-op for mock).
func (a *mockAgent) Authenticate(_ context.Context, _ acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

// SetSessionMode handles mode changes (no-op for mock).
func (a *mockAgent) SetSessionMode(_ context.Context, _ acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}

func (a *mockAgent) SetSessionConfigOption(_ context.Context, _ acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}

// emitAvailableCommandsOnce sends the available commands list once per session,
// on the first prompt. Emitting during Prompt (not NewSession) avoids writing
// notifications to stdout before the JSON-RPC session response.
func (a *mockAgent) emitAvailableCommandsOnce(ctx context.Context, sid acp.SessionId) {
	a.mu.Lock()
	if a.commandsEmitted[sid] {
		a.mu.Unlock()
		return
	}
	a.commandsEmitted[sid] = true
	a.mu.Unlock()

	_ = a.conn.SessionUpdate(ctx, acp.SessionNotification{
		SessionId: sid,
		Update: acp.SessionUpdate{
			AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{
				AvailableCommands: mockAvailableCommands(),
			},
		},
	})
}

// mockAvailableCommands returns the slash commands supported by the mock agent.
func mockAvailableCommands() []acp.AvailableCommand {
	hint := func(h string) *acp.AvailableCommandInput {
		return &acp.AvailableCommandInput{Unstructured: &acp.UnstructuredCommandInput{Hint: h}}
	}
	return []acp.AvailableCommand{
		{Name: "slow", Description: "Run a slow response (default 5s)", Input: hint("duration (e.g. 10s)")},
		{Name: "error", Description: "Simulate an error"},
		{Name: "thinking", Description: "Emit thinking/reasoning blocks"},
		{Name: "crash", Description: "Simulate agent crash"},
		{Name: "all", Description: "Demonstrate all message types"},
		{Name: "todo", Description: "Emit a todo list"},
		{Name: "mermaid", Description: "Emit a mermaid diagram"},
		{Name: "subagent", Description: "Emit a subagent sequence"},
		{Name: "tool:read", Description: "Emit a read file tool call"},
		{Name: "tool:edit", Description: "Emit an edit file tool call"},
		{Name: "tool:exec", Description: "Emit a shell exec tool call"},
		{Name: "tool:search", Description: "Emit a search tool call"},
	}
}

// extractPromptText concatenates text content blocks from the prompt.
func extractPromptText(blocks []acp.ContentBlock) string {
	var parts []string
	for _, b := range blocks {
		if b.Text != nil {
			parts = append(parts, b.Text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// --- Flag parsing (unchanged) ---

// parseModelFlag extracts --model value from command line args.
func parseModelFlag() string {
	return parseModelFromArgs(os.Args)
}

// parseModelFromArgs extracts --model value from the given args slice.
func parseModelFromArgs(args []string) string {
	for i, arg := range args[1:] {
		if arg == "--model" && i+1 < len(args)-1 {
			return args[i+2]
		}
		if v, ok := strings.CutPrefix(arg, "--model="); ok {
			return v
		}
	}
	return "mock-default"
}

// parseResumeFlag extracts --resume value from command line args.
func parseResumeFlag() string {
	return parseResumeFromArgs(os.Args)
}

// parseResumeFromArgs extracts --resume value from the given args slice.
func parseResumeFromArgs(args []string) string {
	for i, arg := range args[1:] {
		if arg == "--resume" && i+1 < len(args)-1 {
			return args[i+2]
		}
		if v, ok := strings.CutPrefix(arg, "--resume="); ok {
			return v
		}
	}
	return ""
}

// parseContinueFlag checks if -c is present in the command line args.
// Used by TUI mode for generic "continue last session" resume.
func parseContinueFlag() bool {
	return slices.Contains(os.Args[1:], "-c")
}

// mcpConfigPayload is the JSON structure for --mcp-config.
type mcpConfigPayload struct {
	MCPServers map[string]mcpServerDef `json:"mcpServers"`
}

// parseMCPConfigFlag extracts --mcp-config value from command line args
// and returns the parsed MCP server definitions.
func parseMCPConfigFlag() map[string]mcpServerDef {
	raw := parseMCPConfigFromArgs(os.Args)
	if raw == "" {
		return nil
	}
	var payload mcpConfigPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		_, _ = fmt.Fprintf(logOutput, "mock-agent: failed to parse --mcp-config: %v\n", err)
		return nil
	}
	return payload.MCPServers
}

// parseMCPConfigFromArgs extracts --mcp-config value from the given args slice.
func parseMCPConfigFromArgs(args []string) string {
	for i, arg := range args[1:] {
		if arg == "--mcp-config" && i+1 < len(args)-1 {
			return args[i+2]
		}
		if v, ok := strings.CutPrefix(arg, "--mcp-config="); ok {
			return v
		}
	}
	return ""
}
