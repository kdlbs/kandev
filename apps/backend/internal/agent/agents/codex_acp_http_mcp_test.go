package agents

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agentctl/types"
)

// TestCodexACP_AssumeMcpHttpEnabled pins the CodexACP runtime config to
// forcing HTTP MCP support. The Codex agent's bridge package advertises
// mcpCapabilities.http = true at initialize time, so the filter alone keeps
// HTTP servers, but a future bridge downgrade (or a forked build) would
// silently strip every HTTP-only server. AssumeMcpHttp is the belt-and-
// suspenders override that keeps HTTP servers attached even when the
// advertised capability drops to false. Removing it re-introduces the
// tool_count=0 regression observed in the Codex ACP profile.
func TestCodexACP_AssumeMcpHttpEnabled(t *testing.T) {
	a := NewCodexACP()
	rt := a.Runtime()
	if rt == nil {
		t.Fatal("Runtime() returned nil")
	}
	if !rt.AssumeMcpHttp {
		t.Fatal("CodexACP.Runtime().AssumeMcpHttp must be true so HTTP MCP servers are not silently filtered")
	}
}

// TestCodexACP_AssumeMcpSseDisabled confirms SSE is intentionally NOT forced
// on for Codex. codex-acp rejects SSE transport with invalidRequest, so
// pretending it works would surface that error per-server rather than
// letting the capability filter drop the duplicate SSE entry cleanly.
func TestCodexACP_AssumeMcpSseDisabled(t *testing.T) {
	rt := NewCodexACP().Runtime()
	if rt.AssumeMcpSse {
		t.Fatal("CodexACP.Runtime().AssumeMcpSse must remain false; codex-acp rejects SSE transport with invalidRequest")
	}
}

// TestCodexACP_PassthroughStrategyPreservesHTTPKandevServer walks the full
// MCP shape the ACP runtime path produces (an HTTP kandev server with a /mcp
// URL plus a stdio user server) through CodexStrategy's -c argv emit and
// asserts (a) the HTTP server's URL lands in mcp_servers.<name>.url= without
// being silently downgraded, (b) the stdio server still emits
// command/args/env (stdio MCP behavior must not regress), and (c) header
// values that look like secrets/cookies never appear in argv in a way that
// would be visible to a sibling host process. The HTTP kandev server has no
// headers in this scenario; if a future injection site ever starts adding
// headers to the localhost kandev URL, that change must be reviewed against
// this test (the argv will visibly grow).
func TestCodexACP_PassthroughStrategyPreservesHTTPKandevServer(t *testing.T) {
	strategy := mcpconfig.CodexStrategy{}
	servers := []types.McpServer{
		{Name: "kandev", Type: "http", URL: "http://localhost:10005/mcp"},
		{Name: "github", Type: "stdio", Command: "npx", Args: []string{"-y", "@mcp/github"}, Env: map[string]string{"GITHUB_TOKEN": "tok"}},
	}
	art, err := strategy.BuildPassthroughMCP(servers, mcpconfig.PassthroughPaths{})
	if err != nil {
		t.Fatalf("BuildPassthroughMCP: %v", err)
	}
	joined := strings.Join(art.Args, " ")
	if !strings.Contains(joined, `mcp_servers.kandev.url="http://localhost:10005/mcp"`) {
		t.Errorf("HTTP kandev server missing from -c argv; got: %s", joined)
	}
	if !strings.Contains(joined, `mcp_servers.github.command="npx"`) {
		t.Errorf("stdio github command missing from -c argv; got: %s", joined)
	}
	if !strings.Contains(joined, `mcp_servers.github.args=["-y","@mcp/github"]`) {
		t.Errorf("stdio github args missing from -c argv; got: %s", joined)
	}
	if !strings.Contains(joined, `mcp_servers.github.env={"GITHUB_TOKEN":"tok"}`) {
		t.Errorf("stdio github env missing from -c argv; got: %s", joined)
	}
	if strings.Contains(joined, `mcp_servers.kandev.http_headers`) {
		t.Errorf("kandev should not emit http_headers when none are configured; got: %s", joined)
	}
}

// TestCodexACP_PassthroughStrategyEmitsHTTPHeadersAsHttpHeadersOnly asserts
// that an HTTP MCP server carrying Authorization / Cookie / X-API-Key
// headers emits them under the http_headers key Codex actually consumes.
// The test uses values that are not real secrets; the assertion is purely
// on argv shape, not on whether the values are valid tokens. The no-secret
// invariant is the argv never grows an unrelated leak: the only Authorization-
// shaped token in argv is the one we explicitly fed in.
func TestCodexACP_PassthroughStrategyEmitsHTTPHeadersAsHttpHeadersOnly(t *testing.T) {
	strategy := mcpconfig.CodexStrategy{}
	servers := []types.McpServer{
		{Name: "remote", Type: "http", URL: "https://mcp.example.com/mcp",
			Headers: map[string]string{"Authorization": "Bearer test-token-sh"}},
	}
	art, err := strategy.BuildPassthroughMCP(servers, mcpconfig.PassthroughPaths{})
	if err != nil {
		t.Fatalf("BuildPassthroughMCP: %v", err)
	}
	joined := strings.Join(art.Args, " ")
	if !strings.Contains(joined, `mcp_servers.remote.url="https://mcp.example.com/mcp"`) {
		t.Errorf("HTTP remote url missing from -c argv; got: %s", joined)
	}
	if !strings.Contains(joined, `mcp_servers.remote.http_headers={"Authorization":"Bearer test-token-sh"}`) {
		t.Errorf("HTTP remote http_headers missing from -c argv; got: %s", joined)
	}
	if !strings.Contains(joined, "test-token-sh") {
		t.Errorf("expected the configured header value to appear exactly as supplied; got: %s", joined)
	}
}

// TestCodexACP_PassthroughStrategyDedupesSameNameHTTPAndSSE exercises the
// real dual-injection scenario: agentctl injects both kandev/http and
// kandev/sse under the same name, the ACP-side capability filter (with
// caps.Http=true, caps.Sse=false — the codex-acp 1.6.0 shape) keeps only
// kandev/http, and the surviving entry's URL is the /mcp endpoint. SSE
// must not survive, otherwise the Codex subprocess will reject it with
// invalidRequest at startup.
func TestCodexACP_PassthroughStrategyDedupesSameNameHTTPAndSSE(t *testing.T) {
	strategy := mcpconfig.CodexStrategy{}
	servers := []types.McpServer{
		{Name: "kandev", Type: "http", URL: "http://localhost:10005/mcp"},
		{Name: "kandev", Type: "sse", URL: "http://localhost:10005/sse"},
	}
	art, err := strategy.BuildPassthroughMCP(servers, mcpconfig.PassthroughPaths{})
	if err != nil {
		t.Fatalf("BuildPassthroughMCP: %v", err)
	}
	joined := strings.Join(art.Args, " ")
	if !strings.Contains(joined, `mcp_servers.kandev.url="http://localhost:10005/mcp"`) {
		t.Errorf("dual-injection: HTTP /mcp url missing from -c argv; got: %s", joined)
	}
	if strings.Contains(joined, `mcp_servers.kandev.url="http://localhost:10005/sse"`) {
		t.Errorf("dual-injection: SSE /sse url leaked into -c argv; got: %s", joined)
	}
}

// TestCodexACP_PassthroughStrategyDedupesSameNameSSEFirstThenHTTP covers the
// same dual-injection scenario with the entries reversed: kandev/sse arrives
// before kandev/http. HTTP-beats-SSE is a documented contract and must hold
// regardless of input order; the surviving entry must be the /mcp endpoint,
// otherwise codex-acp rejects the SSE transport with invalidRequest.
func TestCodexACP_PassthroughStrategyDedupesSameNameSSEFirstThenHTTP(t *testing.T) {
	strategy := mcpconfig.CodexStrategy{}
	servers := []types.McpServer{
		{Name: "kandev", Type: "sse", URL: "http://localhost:10005/sse"},
		{Name: "kandev", Type: "http", URL: "http://localhost:10005/mcp"},
	}
	art, err := strategy.BuildPassthroughMCP(servers, mcpconfig.PassthroughPaths{})
	if err != nil {
		t.Fatalf("BuildPassthroughMCP: %v", err)
	}
	joined := strings.Join(art.Args, " ")
	if !strings.Contains(joined, `mcp_servers.kandev.url="http://localhost:10005/mcp"`) {
		t.Errorf("dual-injection (sse first): HTTP /mcp url missing from -c argv; got: %s", joined)
	}
	if strings.Contains(joined, `mcp_servers.kandev.url="http://localhost:10005/sse"`) {
		t.Errorf("dual-injection (sse first): SSE /sse url leaked into -c argv; got: %s", joined)
	}
}
