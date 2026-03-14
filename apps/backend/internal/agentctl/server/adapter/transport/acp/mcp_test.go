package acp

import (
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
)

// --- mapToEnvVars ---

func TestMapToEnvVars_Empty(t *testing.T) {
	result := mapToEnvVars(nil)
	if result == nil {
		t.Fatal("expected non-nil empty slice for nil input")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 vars, got %d", len(result))
	}

	result = mapToEnvVars(map[string]string{})
	if result == nil {
		t.Fatal("expected non-nil empty slice for empty input")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 vars, got %d", len(result))
	}
}

func TestMapToEnvVars_WithValues(t *testing.T) {
	env := map[string]string{
		"GITHUB_TOKEN": "secret",
		"API_KEY":      "key123",
	}

	result := mapToEnvVars(env)

	if len(result) != 2 {
		t.Fatalf("expected 2 vars, got %d", len(result))
	}

	found := map[string]string{}
	for _, v := range result {
		found[v.Name] = v.Value
	}
	if found["GITHUB_TOKEN"] != "secret" {
		t.Errorf("GITHUB_TOKEN = %q, want %q", found["GITHUB_TOKEN"], "secret")
	}
	if found["API_KEY"] != "key123" {
		t.Errorf("API_KEY = %q, want %q", found["API_KEY"], "key123")
	}
}

// --- mapToHTTPHeaders ---

func TestMapToHTTPHeaders_Empty(t *testing.T) {
	result := mapToHTTPHeaders(nil)
	if result == nil {
		t.Fatal("expected non-nil empty slice for nil input")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 headers, got %d", len(result))
	}
}

func TestMapToHTTPHeaders_WithValues(t *testing.T) {
	headers := map[string]string{
		"Authorization": "Bearer tok",
		"X-Api-Key":     "key",
	}

	result := mapToHTTPHeaders(headers)

	if len(result) != 2 {
		t.Fatalf("expected 2 headers, got %d", len(result))
	}

	found := map[string]string{}
	for _, h := range result {
		found[h.Name] = h.Value
	}
	if found["Authorization"] != "Bearer tok" {
		t.Errorf("Authorization = %q, want %q", found["Authorization"], "Bearer tok")
	}
	if found["X-Api-Key"] != "key" {
		t.Errorf("X-Api-Key = %q, want %q", found["X-Api-Key"], "key")
	}
}

// --- toACPMcpServers ---

func TestToACPMcpServers_Empty(t *testing.T) {
	result := toACPMcpServers(nil)
	if result == nil {
		t.Fatal("expected non-nil empty slice for nil input")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 servers, got %d", len(result))
	}
}

func TestToACPMcpServers_Stdio(t *testing.T) {
	servers := []types.McpServer{
		{
			Name:    "github",
			Type:    "stdio",
			Command: "npx",
			Args:    []string{"-y", "@mcp/github"},
			Env:     map[string]string{"TOKEN": "val"},
		},
	}

	result := toACPMcpServers(servers)

	if len(result) != 1 {
		t.Fatalf("expected 1 server, got %d", len(result))
	}
	if result[0].Stdio == nil {
		t.Fatal("expected Stdio variant, got nil")
	}
	if result[0].Sse != nil || result[0].Http != nil {
		t.Error("expected only Stdio variant to be set")
	}
	s := result[0].Stdio
	if s.Name != "github" {
		t.Errorf("Name = %q, want %q", s.Name, "github")
	}
	if s.Command != "npx" {
		t.Errorf("Command = %q, want %q", s.Command, "npx")
	}
	if len(s.Args) != 2 || s.Args[0] != "-y" {
		t.Errorf("Args = %v, want [-y @mcp/github]", s.Args)
	}
	if len(s.Env) != 1 || s.Env[0].Name != "TOKEN" || s.Env[0].Value != "val" {
		t.Errorf("Env = %v, want [{TOKEN val}]", s.Env)
	}
}

func TestToACPMcpServers_StdioEmptyEnv(t *testing.T) {
	servers := []types.McpServer{
		{Name: "srv", Type: "stdio", Command: "cmd"},
	}

	result := toACPMcpServers(servers)

	s := result[0].Stdio
	if s.Env == nil {
		t.Fatal("Env should be empty slice, not nil (ACP SDK non-omitempty)")
	}
	if len(s.Env) != 0 {
		t.Errorf("expected 0 env vars, got %d", len(s.Env))
	}
}

func TestToACPMcpServers_SSE(t *testing.T) {
	servers := []types.McpServer{
		{
			Name:    "remote",
			Type:    "sse",
			URL:     "https://mcp.example.com/sse",
			Headers: map[string]string{"Authorization": "Bearer tok"},
		},
	}

	result := toACPMcpServers(servers)

	if len(result) != 1 {
		t.Fatalf("expected 1 server, got %d", len(result))
	}
	if result[0].Sse == nil {
		t.Fatal("expected Sse variant, got nil")
	}
	s := result[0].Sse
	if s.Name != "remote" {
		t.Errorf("Name = %q, want %q", s.Name, "remote")
	}
	if s.Url != "https://mcp.example.com/sse" {
		t.Errorf("Url = %q, want %q", s.Url, "https://mcp.example.com/sse")
	}
	if s.Type != "sse" {
		t.Errorf("Type = %q, want %q", s.Type, "sse")
	}
	if len(s.Headers) != 1 || s.Headers[0].Name != "Authorization" {
		t.Errorf("Headers = %v, want [{Authorization Bearer tok}]", s.Headers)
	}
}

func TestToACPMcpServers_HTTP(t *testing.T) {
	servers := []types.McpServer{
		{
			Name: "http-srv",
			Type: "http",
			URL:  "https://mcp.example.com/http",
		},
	}

	result := toACPMcpServers(servers)

	if result[0].Http == nil {
		t.Fatal("expected Http variant for http type")
	}
	if result[0].Http.Type != "http" {
		t.Errorf("Type = %q, want %q", result[0].Http.Type, "http")
	}
	if result[0].Http.Url != "https://mcp.example.com/http" {
		t.Errorf("Url = %q, want %q", result[0].Http.Url, "https://mcp.example.com/http")
	}
}

func TestToACPMcpServers_StreamableHTTP(t *testing.T) {
	servers := []types.McpServer{
		{
			Name: "stream-srv",
			Type: "streamable_http",
			URL:  "https://mcp.example.com/stream",
		},
	}

	result := toACPMcpServers(servers)

	if result[0].Http == nil {
		t.Fatal("expected Http variant for streamable_http type")
	}
	if result[0].Http.Type != "streamable_http" {
		t.Errorf("Type = %q, want %q", result[0].Http.Type, "streamable_http")
	}
}

func TestToACPMcpServers_Mixed(t *testing.T) {
	servers := []types.McpServer{
		{Name: "s1", Type: "stdio", Command: "cmd"},
		{Name: "s2", Type: "sse", URL: "https://sse"},
		{Name: "s3", Type: "http", URL: "https://http"},
		{Name: "s4", Type: "streamable_http", URL: "https://stream"},
	}

	result := toACPMcpServers(servers)

	if len(result) != 4 {
		t.Fatalf("expected 4 servers, got %d", len(result))
	}
	if result[0].Stdio == nil {
		t.Error("result[0] should be Stdio")
	}
	if result[1].Sse == nil {
		t.Error("result[1] should be Sse")
	}
	if result[2].Http == nil {
		t.Error("result[2] should be Http")
	}
	if result[3].Http == nil {
		t.Error("result[3] should be Http (streamable_http)")
	}
}

// --- filterMcpServersByCapabilities ---

func TestFilterMcpServersByCapabilities_StdioAlwaysAllowed(t *testing.T) {
	log := newTestLoggerForMcp()
	servers := []types.McpServer{
		{Name: "s1", Type: "stdio", Command: "cmd"},
	}
	caps := acp.McpCapabilities{Sse: false, Http: false}

	result := filterMcpServersByCapabilities(servers, caps, log)

	if len(result) != 1 {
		t.Fatalf("stdio should always pass, got %d servers", len(result))
	}
}

func TestFilterMcpServersByCapabilities_SSEFiltered(t *testing.T) {
	log := newTestLoggerForMcp()
	servers := []types.McpServer{
		{Name: "s1", Type: "sse", URL: "https://sse"},
	}

	result := filterMcpServersByCapabilities(servers, acp.McpCapabilities{Sse: false}, log)
	if len(result) != 0 {
		t.Error("SSE should be filtered when Sse=false")
	}

	result = filterMcpServersByCapabilities(servers, acp.McpCapabilities{Sse: true}, log)
	if len(result) != 1 {
		t.Error("SSE should pass when Sse=true")
	}
}

func TestFilterMcpServersByCapabilities_HTTPFiltered(t *testing.T) {
	log := newTestLoggerForMcp()
	servers := []types.McpServer{
		{Name: "s1", Type: "http", URL: "https://http"},
	}

	result := filterMcpServersByCapabilities(servers, acp.McpCapabilities{Http: false}, log)
	if len(result) != 0 {
		t.Error("HTTP should be filtered when Http=false")
	}

	result = filterMcpServersByCapabilities(servers, acp.McpCapabilities{Http: true}, log)
	if len(result) != 1 {
		t.Error("HTTP should pass when Http=true")
	}
}

func TestFilterMcpServersByCapabilities_StreamableHTTPFiltered(t *testing.T) {
	log := newTestLoggerForMcp()
	servers := []types.McpServer{
		{Name: "s1", Type: "streamable_http", URL: "https://stream"},
	}

	result := filterMcpServersByCapabilities(servers, acp.McpCapabilities{Http: false}, log)
	if len(result) != 0 {
		t.Error("streamable_http should be filtered when Http=false")
	}

	result = filterMcpServersByCapabilities(servers, acp.McpCapabilities{Http: true}, log)
	if len(result) != 1 {
		t.Error("streamable_http should pass when Http=true")
	}
}

func TestFilterMcpServersByCapabilities_Mixed(t *testing.T) {
	log := newTestLoggerForMcp()
	servers := []types.McpServer{
		{Name: "stdio", Type: "stdio", Command: "cmd"},
		{Name: "sse", Type: "sse", URL: "https://sse"},
		{Name: "http", Type: "http", URL: "https://http"},
		{Name: "stream", Type: "streamable_http", URL: "https://stream"},
	}

	// Only SSE capability
	result := filterMcpServersByCapabilities(servers, acp.McpCapabilities{Sse: true, Http: false}, log)
	if len(result) != 2 {
		t.Fatalf("expected 2 (stdio + sse), got %d", len(result))
	}
	if result[0].Name != "stdio" || result[1].Name != "sse" {
		t.Errorf("expected [stdio, sse], got [%s, %s]", result[0].Name, result[1].Name)
	}

	// All capabilities
	result = filterMcpServersByCapabilities(servers, acp.McpCapabilities{Sse: true, Http: true}, log)
	if len(result) != 4 {
		t.Errorf("expected 4 with all caps, got %d", len(result))
	}
}

// newTestLoggerForMcp creates a logger for MCP tests.
func newTestLoggerForMcp() *logger.Logger {
	return newTestAdapter().logger
}
