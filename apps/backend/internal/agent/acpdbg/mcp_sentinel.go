package acpdbg

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

const (
	mcpInitializeMethod = "initialize"
	mcpToolsListMethod  = "tools/list"
	mcpToolsCallMethod  = "tools/call"
)

// MCPSentinel is a deliberately tiny streamable-HTTP MCP server used only by
// acpdbg. It records protocol milestones without recording MCP payloads.
// Absence of a milestone is an unobserved result, not an agent failure.
type MCPSentinel struct {
	recorder *Recorder
	server   *httptest.Server
	// credential is a random per-probe value advertised as an Authorization
	// header in session/new. Only whether it arrived is recorded.
	credential string

	mu          sync.Mutex
	connections map[string]*sentinelConnection
}

type sentinelConnection struct {
	initialized bool
	toolsListed bool
	toolCalled  bool
	toolCount   int
	// headerSeen and headerMissing record whether the configured
	// Authorization header arrived on this connection's requests.
	headerSeen    bool
	headerMissing bool
}

// MCPSentinelSummary separates actual endpoint observations from ACP delivery.
type MCPSentinelSummary struct {
	InitializeObserved bool
	ToolsListObserved  bool
	ToolCallObserved   bool
	ToolCount          int
	// HeaderObserved reports that at least one request carried the configured
	// Authorization header; HeaderMissing that at least one request lacked it.
	// Both false means the agent never contacted the sentinel.
	HeaderObserved bool
	HeaderMissing  bool
}

// NewMCPSentinel starts an in-process sentinel. Call Close after session/new
// has returned; the server never exposes a host project endpoint or credentials.
func NewMCPSentinel(recorder *Recorder) *MCPSentinel {
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	s := &MCPSentinel{recorder: recorder, connections: make(map[string]*sentinelConnection), credential: hex.EncodeToString(raw)}
	s.server = httptest.NewServer(http.HandlerFunc(s.serveHTTP))
	_ = recorder.Meta("mcp_sentinel_started", map[string]any{"target": s.server.URL + "/mcp"})
	return s
}

func (s *MCPSentinel) URL() string { return s.server.URL + "/mcp" }

// Headers is the ACP header list advertised for the sentinel server entry.
func (s *MCPSentinel) Headers() []map[string]string {
	return []map[string]string{{"name": "Authorization", "value": "Bearer " + s.credential}}
}

func (s *MCPSentinel) Close() {
	if s.server != nil {
		s.server.Close()
	}
}

func (s *MCPSentinel) Summary() MCPSentinelSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result MCPSentinelSummary
	for _, connection := range s.connections {
		result.InitializeObserved = result.InitializeObserved || connection.initialized
		result.ToolsListObserved = result.ToolsListObserved || connection.toolsListed
		result.ToolCallObserved = result.ToolCallObserved || connection.toolCalled
		result.HeaderObserved = result.HeaderObserved || connection.headerSeen
		result.HeaderMissing = result.HeaderMissing || connection.headerMissing
		if connection.toolCount > result.ToolCount {
			result.ToolCount = connection.toolCount
		}
	}
	return result
}

func (s *MCPSentinel) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/mcp" {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = r.Body.Close() }()
	var request struct {
		ID     any    `json:"id"`
		Method string `json:"method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid JSON-RPC request", http.StatusBadRequest)
		return
	}

	connectionID := s.connectionID(r)
	s.record(connectionID, request.Method, r.Header.Get("Authorization") == "Bearer "+s.credential)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Mcp-Session-Id", connectionID)
	response := map[string]any{"jsonrpc": "2.0", "id": request.ID}
	switch request.Method {
	case mcpInitializeMethod:
		response["result"] = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "acpdbg-sentinel", "version": "1"},
		}
	case mcpToolsListMethod:
		response["result"] = map[string]any{"tools": []map[string]any{{
			"name": "acpdbg_sentinel", "description": "ACP debug sentinel", "inputSchema": map[string]any{"type": "object"},
		}}}
	case mcpToolsCallMethod:
		response["result"] = map[string]any{"content": []map[string]string{{"type": "text", "text": "sentinel"}}}
	default:
		response["error"] = map[string]any{"code": -32601, "message": "method not found"}
	}
	_ = json.NewEncoder(w).Encode(response)
}

func (s *MCPSentinel) connectionID(r *http.Request) string {
	if id := r.Header.Get("Mcp-Session-Id"); id != "" {
		return opaqueConnectionID(id)
	}
	return opaqueConnectionID(r.RemoteAddr)
}

func opaqueConnectionID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sentinel-" + hex.EncodeToString(sum[:4])
}

func (s *MCPSentinel) record(connectionID, method string, headerSeen bool) {
	s.mu.Lock()
	connection := s.connections[connectionID]
	if connection == nil {
		connection = &sentinelConnection{}
		s.connections[connectionID] = connection
	}
	if headerSeen {
		connection.headerSeen = true
	} else {
		connection.headerMissing = true
	}
	switch method {
	case mcpInitializeMethod:
		connection.initialized = true
	case mcpToolsListMethod:
		connection.toolsListed = true
		connection.toolCount = 1
	case mcpToolsCallMethod:
		connection.toolCalled = true
	}
	s.mu.Unlock()
	_ = s.recorder.Meta("mcp_sentinel_"+method, map[string]any{
		"connection_id":   connectionID,
		"header_observed": headerSeen,
		"timestamp":       time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func (s *MCPSentinel) String() string {
	return fmt.Sprintf("MCP sentinel at %s", s.URL())
}
