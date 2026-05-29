package mcpconfig

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

var sampleServers = []types.McpServer{
	{Name: "kandev", Type: "http", URL: "http://localhost:1234/mcp"},
	{Name: "github", Type: "stdio", Command: "npx", Args: []string{"-y", "@mcp/github"}, Env: map[string]string{"GITHUB_TOKEN": "tok"}},
	{Name: "stream", Type: "streamable_http", URL: "https://x/mcp", Headers: map[string]string{"Authorization": "Bearer t"}},
}

// --- Claude ------------------------------------------------------------------

func TestClaudeStrategy_FileAndFlags(t *testing.T) {
	art, err := ClaudeStrategy{}.BuildPassthroughMCP(sampleServers, PassthroughPaths{TempConfigPath: "/tmp/cfg.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(art.Files) != 1 || art.Files[0].Path != "/tmp/cfg.json" {
		t.Fatalf("expected one file at /tmp/cfg.json, got %+v", art.Files)
	}
	if art.Files[0].SkipIfExists {
		t.Error("claude temp file should overwrite, not SkipIfExists")
	}
	wantArgs := []string{"--mcp-config", "/tmp/cfg.json"}
	if !reflect.DeepEqual(art.Args, wantArgs) {
		t.Errorf("Args = %v, want %v", art.Args, wantArgs)
	}

	var got claudeMCPFile
	if err := json.Unmarshal(art.Files[0].Content, &got); err != nil {
		t.Fatalf("content not valid JSON: %v", err)
	}
	if got.MCPServers["kandev"].Type != "http" || got.MCPServers["kandev"].URL != "http://localhost:1234/mcp" {
		t.Errorf("kandev entry = %+v", got.MCPServers["kandev"])
	}
	gh := got.MCPServers["github"]
	if gh.Type != "stdio" || gh.Command != "npx" || gh.Env["GITHUB_TOKEN"] != "tok" {
		t.Errorf("github entry = %+v", gh)
	}
	// kandev's streamable_http must normalize to the hyphenated claude spelling.
	if got.MCPServers["stream"].Type != "streamable-http" {
		t.Errorf("stream Type = %q, want streamable-http", got.MCPServers["stream"].Type)
	}
}

func TestClaudeStrategy_RemoteTransportMapping(t *testing.T) {
	servers := []types.McpServer{
		{Name: "sse", Type: "sse", URL: "https://x/sse"},
		{Name: "http", Type: "http", URL: "https://x/http"},
		{Name: "stream", Type: "streamable_http", URL: "https://x/stream"},
	}
	art, _ := ClaudeStrategy{}.BuildPassthroughMCP(servers, PassthroughPaths{TempConfigPath: "/tmp/c.json"})
	var got claudeMCPFile
	if err := json.Unmarshal(art.Files[0].Content, &got); err != nil {
		t.Fatalf("content not valid JSON: %v", err)
	}
	if got.MCPServers["sse"].Type != "sse" {
		t.Errorf("sse Type = %q, want sse", got.MCPServers["sse"].Type)
	}
	if got.MCPServers["http"].Type != "http" {
		t.Errorf("http Type = %q, want http", got.MCPServers["http"].Type)
	}
	// kandev spells streamable HTTP with a hyphen and treats it as an alias of http.
	if got.MCPServers["stream"].Type != "streamable-http" {
		t.Errorf("stream Type = %q, want streamable-http", got.MCPServers["stream"].Type)
	}
}

func TestClaudeStrategy_Strict(t *testing.T) {
	art, _ := ClaudeStrategy{Strict: true}.BuildPassthroughMCP(sampleServers, PassthroughPaths{TempConfigPath: "/tmp/cfg.json"})
	if got := strings.Join(art.Args, " "); got != "--mcp-config /tmp/cfg.json --strict-mcp-config" {
		t.Errorf("Args = %q", got)
	}
}

func TestClaudeStrategy_SkipsReservedAndEmpty(t *testing.T) {
	servers := []types.McpServer{
		{Name: "workspace", Type: "stdio", Command: "x"},
		{Name: "", Type: "stdio", Command: "y"},
		{Name: "ok", Type: "stdio", Command: "z"},
	}
	art, _ := ClaudeStrategy{}.BuildPassthroughMCP(servers, PassthroughPaths{TempConfigPath: "/tmp/c.json"})
	var got claudeMCPFile
	_ = json.Unmarshal(art.Files[0].Content, &got)
	if _, ok := got.MCPServers["workspace"]; ok {
		t.Error("reserved name 'workspace' must be skipped")
	}
	if len(got.MCPServers) != 1 {
		t.Errorf("expected only 'ok', got %v", got.MCPServers)
	}
}

func TestClaudeStrategy_NoPathOrNoServers(t *testing.T) {
	if art, _ := (ClaudeStrategy{}).BuildPassthroughMCP(sampleServers, PassthroughPaths{}); len(art.Files) != 0 || len(art.Args) != 0 {
		t.Error("no temp path should produce no artifacts")
	}
	if art, _ := (ClaudeStrategy{}).BuildPassthroughMCP(nil, PassthroughPaths{TempConfigPath: "/tmp/c.json"}); len(art.Files) != 0 {
		t.Error("no servers should produce no artifacts")
	}
}

func TestIsStdioServer_TypeFallback(t *testing.T) {
	cases := []struct {
		name string
		srv  types.McpServer
		want bool
	}{
		{"explicit stdio", types.McpServer{Type: "stdio", Command: "x"}, true},
		{"explicit http", types.McpServer{Type: "http", URL: "https://x"}, false},
		{"untyped command only", types.McpServer{Command: "x"}, true},
		{"untyped url only", types.McpServer{URL: "https://x"}, false},
		{"untyped command+url treated as remote", types.McpServer{Command: "x", URL: "https://x"}, false},
	}
	for _, tc := range cases {
		if got := isStdioServer(tc.srv); got != tc.want {
			t.Errorf("%s: isStdioServer = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// --- Codex -------------------------------------------------------------------

func TestCodexStrategy_Args(t *testing.T) {
	art, err := CodexStrategy{}.BuildPassthroughMCP(sampleServers, PassthroughPaths{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(art.Files) != 0 || len(art.Env) != 0 {
		t.Error("codex strategy must not write files or env")
	}
	joined := strings.Join(art.Args, " ")
	// stdio server: command + args + env
	for _, want := range []string{
		`-c mcp_servers.kandev.url="http://localhost:1234/mcp"`,
		`-c mcp_servers.github.command="npx"`,
		`-c mcp_servers.github.args=["-y","@mcp/github"]`,
		`-c mcp_servers.github.env={"GITHUB_TOKEN":"tok"}`,
		`-c mcp_servers.stream.url="https://x/mcp"`,
		`-c mcp_servers.stream.http_headers={"Authorization":"Bearer t"}`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing arg %q in:\n%s", want, joined)
		}
	}
	// Every value token must be preceded by a -c flag.
	for i := 0; i < len(art.Args); i += 2 {
		if art.Args[i] != "-c" {
			t.Fatalf("arg %d = %q, want -c (args: %v)", i, art.Args[i], art.Args)
		}
	}
}

func TestCodexStrategy_OmitsEmptyArgsAndEnv(t *testing.T) {
	servers := []types.McpServer{{Name: "bare", Type: "stdio", Command: "run"}}
	art, _ := CodexStrategy{}.BuildPassthroughMCP(servers, PassthroughPaths{})
	if got := strings.Join(art.Args, " "); got != `-c mcp_servers.bare.command="run"` {
		t.Errorf("Args = %q", got)
	}
}

func TestCodexStrategy_QuotesServerNamesWithDots(t *testing.T) {
	servers := []types.McpServer{{Name: "my.server", Type: "stdio", Command: "run"}}
	art, _ := CodexStrategy{}.BuildPassthroughMCP(servers, PassthroughPaths{})
	// A dotted name must be TOML-quoted so codex doesn't read the dot as nesting.
	if got := strings.Join(art.Args, " "); got != `-c mcp_servers."my.server".command="run"` {
		t.Errorf("Args = %q", got)
	}
}

// --- Cursor ------------------------------------------------------------------

func TestCursorStrategy_ProjectFileSkipIfExists(t *testing.T) {
	art, err := CursorStrategy{}.BuildPassthroughMCP(sampleServers, PassthroughPaths{WorkspaceDir: "/work"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(art.Args) != 0 || len(art.Env) != 0 {
		t.Error("cursor strategy must not produce args or env")
	}
	if len(art.Files) != 1 {
		t.Fatalf("expected one file, got %d", len(art.Files))
	}
	f := art.Files[0]
	if f.Path != filepath.Join("/work", ".cursor", "mcp.json") {
		t.Errorf("Path = %q", f.Path)
	}
	if !f.SkipIfExists {
		t.Error("cursor file must be SkipIfExists (never clobber user's file)")
	}
	var got cursorMCPFile
	if err := json.Unmarshal(f.Content, &got); err != nil {
		t.Fatalf("content not valid JSON: %v", err)
	}
	if got.MCPServers["github"].Type != "stdio" || got.MCPServers["github"].Command != "npx" {
		t.Errorf("github entry = %+v", got.MCPServers["github"])
	}
	// Remote entries carry no type, just url+headers.
	if got.MCPServers["stream"].Type != "" || got.MCPServers["stream"].URL != "https://x/mcp" {
		t.Errorf("stream entry = %+v", got.MCPServers["stream"])
	}
}

func TestCursorStrategy_NoWorkspace(t *testing.T) {
	if art, _ := (CursorStrategy{}).BuildPassthroughMCP(sampleServers, PassthroughPaths{}); len(art.Files) != 0 {
		t.Error("no workspace dir should produce no artifacts")
	}
}

// --- OpenCode ----------------------------------------------------------------

func TestOpenCodeStrategy_FileAndEnv(t *testing.T) {
	art, err := OpenCodeStrategy{}.BuildPassthroughMCP(sampleServers, PassthroughPaths{TempConfigPath: "/tmp/oc.json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(art.Args) != 0 {
		t.Error("opencode strategy uses env, not args")
	}
	if art.Env[opencodeConfigEnvVar] != "/tmp/oc.json" {
		t.Errorf("OPENCODE_CONFIG = %q, want /tmp/oc.json", art.Env[opencodeConfigEnvVar])
	}
	if len(art.Files) != 1 || art.Files[0].Path != "/tmp/oc.json" {
		t.Fatalf("expected one file at /tmp/oc.json, got %+v", art.Files)
	}
	var got opencodeMCPFile
	if err := json.Unmarshal(art.Files[0].Content, &got); err != nil {
		t.Fatalf("content not valid JSON: %v", err)
	}
	if got.Schema != opencodeConfigSchema {
		t.Errorf("$schema = %q", got.Schema)
	}
	// remote http server -> type "remote"
	if got.MCP["kandev"].Type != "remote" || got.MCP["kandev"].URL != "http://localhost:1234/mcp" || !got.MCP["kandev"].Enabled {
		t.Errorf("kandev entry = %+v", got.MCP["kandev"])
	}
	// stdio -> type "local", command is [cmd, ...args]
	gh := got.MCP["github"]
	if gh.Type != "local" || !reflect.DeepEqual(gh.Command, []string{"npx", "-y", "@mcp/github"}) {
		t.Errorf("github entry = %+v", gh)
	}
	if gh.Environment["GITHUB_TOKEN"] != "tok" {
		t.Errorf("github env = %+v", gh.Environment)
	}
}

func TestOpenCodeStrategy_NoPath(t *testing.T) {
	if art, _ := (OpenCodeStrategy{}).BuildPassthroughMCP(sampleServers, PassthroughPaths{}); len(art.Files) != 0 || len(art.Env) != 0 {
		t.Error("no temp path should produce no artifacts")
	}
}

// All four strategies satisfy the interface.
func TestStrategiesImplementInterface(t *testing.T) {
	var _ PassthroughMCPStrategy = ClaudeStrategy{}
	var _ PassthroughMCPStrategy = CodexStrategy{}
	var _ PassthroughMCPStrategy = CursorStrategy{}
	var _ PassthroughMCPStrategy = OpenCodeStrategy{}
}
