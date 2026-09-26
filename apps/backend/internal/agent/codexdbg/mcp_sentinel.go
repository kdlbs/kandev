package codexdbg

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
)

const (
	mcpSentinelName = "codexdbg_sentinel"
	mcpSentinelTool = "codexdbg_probe"
)

// MCPSentinel is an isolated local MCP server used to prove actual app-server
// attachment and tool traffic.
type MCPSentinel struct {
	server *httptest.Server
	mu     sync.Mutex
	state  mcpSentinelState
}

type mcpSentinelState struct {
	InitializeObserved bool
	ToolsListObserved  bool
	ToolCallObserved   bool
	ToolCount          int
}

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// NewMCPSentinel starts a loopback-only Streamable HTTP endpoint with one
// harmless tool.
func NewMCPSentinel() *MCPSentinel {
	sentinel := &MCPSentinel{}
	sentinel.server = httptest.NewServer(http.HandlerFunc(sentinel.serveHTTP))
	return sentinel
}

// ConfigArgs returns codex CLI arguments that add the sentinel to this process.
func (s *MCPSentinel) ConfigArgs() []string {
	return []string{"-c", fmt.Sprintf("mcp_servers.%s.url=%q", mcpSentinelName, s.server.URL+"/mcp"), "-c", fmt.Sprintf("mcp_servers.%s.enabled=true", mcpSentinelName)}
}

// Summary reports endpoint observations. False means the event was not
// observed; it does not prove that Codex rejected the configured server.
func (s *MCPSentinel) Summary() mcpSentinelState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// Close stops the loopback endpoint.
func (s *MCPSentinel) Close() { s.server.Close() }

func (s *MCPSentinel) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost || request.URL.Path != "/mcp" {
		http.NotFound(writer, request)
		return
	}
	defer func() { _ = request.Body.Close() }()
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, 1024*1024))
	if err != nil {
		http.Error(writer, "invalid MCP request", http.StatusBadRequest)
		return
	}
	var message mcpRequest
	if err := json.Unmarshal(body, &message); err != nil || message.JSONRPC != "2.0" || message.Method == "" {
		http.Error(writer, "invalid MCP request", http.StatusBadRequest)
		return
	}
	s.replyHTTP(writer, message)
}

func (s *MCPSentinel) replyHTTP(writer http.ResponseWriter, message mcpRequest) {
	if message.ID == nil || bytes.Equal(message.ID, []byte("null")) {
		if message.Method == "notifications/initialized" {
			s.markInitialize()
			writer.WriteHeader(http.StatusAccepted)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Mcp-Session-Id", "codexdbg-sentinel")
	response := map[string]any{"jsonrpc": "2.0", "id": message.ID}
	switch message.Method {
	case "initialize":
		s.markInitialize()
		response["result"] = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "codexdbg-sentinel", "version": "1"},
		}
	case "tools/list":
		s.markToolsList()
		response["result"] = map[string]any{"tools": []map[string]any{{
			"name":        mcpSentinelTool,
			"description": "Reports that the app-server reached the local debugger sentinel.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		}}}
	case "tools/call":
		var params struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(message.Params, &params) != nil || params.Name != mcpSentinelTool {
			response["error"] = map[string]any{"code": -32602, "message": "unknown sentinel tool"}
			break
		}
		s.markToolCall()
		response["result"] = map[string]any{"content": []map[string]string{{"type": "text", "text": "codexdbg sentinel observed app-server traffic"}}, "isError": false}
	default:
		response["error"] = map[string]any{"code": -32601, "message": "method not found"}
	}
	_ = json.NewEncoder(writer).Encode(response)
}

func (s *MCPSentinel) markInitialize() {
	s.mu.Lock()
	s.state.InitializeObserved = true
	s.mu.Unlock()
}

func (s *MCPSentinel) markToolsList() {
	s.mu.Lock()
	s.state.ToolsListObserved = true
	s.state.ToolCount = 1
	s.mu.Unlock()
}

func (s *MCPSentinel) markToolCall() {
	s.mu.Lock()
	s.state.ToolCallObserved = true
	s.mu.Unlock()
}
