package mcpconfig

import (
	"encoding/json"
	"path/filepath"

	"github.com/kandev/kandev/internal/agentctl/types"
)

// PassthroughPaths carries filesystem locations a strategy may use to
// materialize MCP config for a passthrough (raw CLI) launch without writing to
// the user's global config.
type PassthroughPaths struct {
	// TempConfigPath is a kandev-owned temp file path a strategy may write a
	// config file to (used by Claude and OpenCode). Empty when unavailable.
	TempConfigPath string
	// WorkspaceDir is the agent's working directory (worktree root). Used by
	// strategies that write a project-local config file (Cursor). Empty when
	// unavailable.
	WorkspaceDir string
}

// PassthroughConfigFile is a config file a strategy wants materialized on disk.
type PassthroughConfigFile struct {
	Path    string
	Content []byte
	// SkipIfExists, when true, means the file must NOT be written if a file
	// already exists at Path (Cursor: never clobber a user's existing mcp.json).
	SkipIfExists bool
}

// PassthroughArtifacts is what a strategy produces for a single passthrough launch.
type PassthroughArtifacts struct {
	Files []PassthroughConfigFile // config files to materialize
	Args  []string                // extra argv tokens appended to the passthrough command
	Env   map[string]string       // extra environment variables for the agent process
}

// PassthroughMCPStrategy materializes resolved MCP servers into the CLI-specific
// shape (config files, CLI args, and/or env vars) for a passthrough launch,
// without writing to the user's global config. Each passthrough-capable agent
// declares the strategy that matches how its CLI loads MCP servers.
type PassthroughMCPStrategy interface {
	BuildPassthroughMCP(servers []types.McpServer, paths PassthroughPaths) (PassthroughArtifacts, error)
}

// isStdioServer reports whether the server is a stdio (command-based) server.
// Type is authoritative when set (ToACPServers always sets it); otherwise a
// server with a command and no URL is treated as stdio.
func isStdioServer(s types.McpServer) bool {
	if s.Type != "" {
		return s.Type == string(ServerTypeStdio)
	}
	return s.Command != "" && s.URL == ""
}

// marshalMCPFile renders a config struct as pretty-printed JSON with a trailing
// newline, matching the existing passthrough config writer.
func marshalMCPFile(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// --- Claude ------------------------------------------------------------------

// claudeReservedServerName is skipped by Claude Code at load time (it warns and
// ignores it), so we never emit it.
const claudeReservedServerName = "workspace"

// claudeStreamableHTTPType is Claude Code's spelling of streamable HTTP (hyphen,
// not underscore); Claude treats it as an alias of "http".
const claudeStreamableHTTPType = "streamable-http"

// ClaudeStrategy writes an `mcpServers` JSON file and points Claude Code at it
// via --mcp-config. Without Strict the servers are merged additively with the
// user's ~/.claude.json and project .mcp.json. With Strict, --strict-mcp-config
// is added so ONLY these servers load (the user's other MCP sources are ignored).
type ClaudeStrategy struct {
	Strict bool
}

type claudeMCPFile struct {
	MCPServers map[string]claudeServerEntry `json:"mcpServers"`
}

type claudeServerEntry struct {
	Type    string            `json:"type"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (s ClaudeStrategy) BuildPassthroughMCP(servers []types.McpServer, paths PassthroughPaths) (PassthroughArtifacts, error) {
	if len(servers) == 0 || paths.TempConfigPath == "" {
		return PassthroughArtifacts{}, nil
	}
	entries := make(map[string]claudeServerEntry, len(servers))
	for _, srv := range servers {
		if srv.Name == "" || srv.Name == claudeReservedServerName {
			continue
		}
		entries[srv.Name] = claudeServerEntryFromServer(srv)
	}
	if len(entries) == 0 {
		// Every server was filtered out (blank/reserved names); emit nothing
		// rather than a `--mcp-config` flag pointing at an empty config.
		return PassthroughArtifacts{}, nil
	}
	content, err := marshalMCPFile(claudeMCPFile{MCPServers: entries})
	if err != nil {
		return PassthroughArtifacts{}, err
	}
	args := []string{"--mcp-config", paths.TempConfigPath}
	if s.Strict {
		args = append(args, "--strict-mcp-config")
	}
	return PassthroughArtifacts{
		Files: []PassthroughConfigFile{{Path: paths.TempConfigPath, Content: content}},
		Args:  args,
	}, nil
}

func claudeServerEntryFromServer(srv types.McpServer) claudeServerEntry {
	if isStdioServer(srv) {
		return claudeServerEntry{Type: string(ServerTypeStdio), Command: srv.Command, Args: srv.Args, Env: srv.Env}
	}
	return claudeServerEntry{Type: claudeRemoteType(srv.Type), URL: srv.URL, Headers: srv.Headers}
}

// claudeRemoteType maps kandev's transport names onto Claude Code's. Claude
// spells streamable HTTP with a hyphen ("streamable-http") and treats it as an
// alias of "http".
func claudeRemoteType(t string) string {
	switch t {
	case string(ServerTypeSSE):
		return string(ServerTypeSSE)
	case string(ServerTypeStreamableHTTP):
		return claudeStreamableHTTPType
	default:
		return string(ServerTypeHTTP)
	}
}

// --- Codex -------------------------------------------------------------------

// CodexStrategy injects MCP servers via repeated `-c mcp_servers.<name>.<key>=<json>`
// CLI overrides. Codex has no MCP-config-file flag; `-c` overrides sit at the top
// of the config precedence stack and never modify config.toml. Codex parses each
// value as JSON when possible, so values are emitted as JSON. Transport is
// inferred from the presence of `command` vs `url` (there is no `type` key).
type CodexStrategy struct{}

func (CodexStrategy) BuildPassthroughMCP(servers []types.McpServer, _ PassthroughPaths) (PassthroughArtifacts, error) {
	args := make([]string, 0, len(servers)*4)
	for _, srv := range servers {
		if srv.Name == "" {
			continue
		}
		serverArgs, err := codexServerArgs(srv)
		if err != nil {
			return PassthroughArtifacts{}, err
		}
		args = append(args, serverArgs...)
	}
	if len(args) == 0 {
		return PassthroughArtifacts{}, nil
	}
	return PassthroughArtifacts{Args: args}, nil
}

// codexServerArgs returns the flat ["-c", "key=json", ...] tokens for one server.
func codexServerArgs(srv types.McpServer) ([]string, error) {
	type pair struct {
		key   string
		value any
	}
	var pairs []pair
	if isStdioServer(srv) {
		pairs = append(pairs, pair{"command", srv.Command})
		if len(srv.Args) > 0 {
			pairs = append(pairs, pair{"args", srv.Args})
		}
		if len(srv.Env) > 0 {
			pairs = append(pairs, pair{"env", srv.Env})
		}
	} else {
		// Remote (url-based) server. Current Codex loads streamable-HTTP MCP
		// servers from `url` natively. Some intermediate Codex versions gated
		// this behind `experimental_use_rmcp_client = true`; if a pinned Codex
		// build ever stops loading the kandev server, emit that override here.
		pairs = append(pairs, pair{"url", srv.URL})
		if len(srv.Headers) > 0 {
			pairs = append(pairs, pair{"http_headers", srv.Headers})
		}
	}
	// NOTE: env vars and headers are JSON-encoded into `-c` overrides, which
	// land in the process argument list (visible via `ps aux`) — a local user
	// on the host could read tokens this way. Codex has no file-based MCP config
	// injection (unlike Claude/OpenCode), so the `-c` path is the only option.
	args := make([]string, 0, len(pairs)*2)
	for _, p := range pairs {
		enc, err := json.Marshal(p.value)
		if err != nil {
			return nil, err
		}
		args = append(args, "-c", "mcp_servers."+codexKeyName(srv.Name)+"."+p.key+"="+string(enc))
	}
	return args, nil
}

// codexKeyName renders an MCP server name as a single TOML key segment for a
// `-c mcp_servers.<name>.<key>` override. Names that are valid TOML bare keys
// are emitted as-is; names containing dots (or other non-bare characters) are
// quoted so Codex does not misread an embedded dot as extra key nesting (which
// would silently inject the server under the wrong path).
func codexKeyName(name string) string {
	if isTOMLBareKey(name) {
		return name
	}
	// Non-bare key: emit a quoted key. JSON string encoding produces
	// TOML-compatible escaping (\", \\, \n, \t, \uXXXX, …), covering dots and
	// control characters alike.
	enc, err := json.Marshal(name)
	if err != nil {
		return `""`
	}
	return string(enc)
}

func isTOMLBareKey(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// --- Cursor ------------------------------------------------------------------

// CursorStrategy writes a project-local .cursor/mcp.json into the workspace.
// cursor-agent auto-discovers it from the working directory. cursor-agent has no
// MCP-config flag and no reliable MCP env var, so a project file is the only
// non-global mechanism. The file is created only when absent — an existing user
// file is never overwritten (when one exists, kandev injects nothing for Cursor).
type CursorStrategy struct{}

type cursorMCPFile struct {
	MCPServers map[string]cursorServerEntry `json:"mcpServers"`
}

type cursorServerEntry struct {
	Type    string            `json:"type,omitempty"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

func (CursorStrategy) BuildPassthroughMCP(servers []types.McpServer, paths PassthroughPaths) (PassthroughArtifacts, error) {
	if len(servers) == 0 || paths.WorkspaceDir == "" {
		return PassthroughArtifacts{}, nil
	}
	entries := make(map[string]cursorServerEntry, len(servers))
	for _, srv := range servers {
		if srv.Name == "" {
			continue
		}
		if isStdioServer(srv) {
			entries[srv.Name] = cursorServerEntry{Type: string(ServerTypeStdio), Command: srv.Command, Args: srv.Args, Env: srv.Env}
		} else {
			entries[srv.Name] = cursorServerEntry{URL: srv.URL, Headers: srv.Headers}
		}
	}
	content, err := marshalMCPFile(cursorMCPFile{MCPServers: entries})
	if err != nil {
		return PassthroughArtifacts{}, err
	}
	return PassthroughArtifacts{
		Files: []PassthroughConfigFile{{
			Path:         filepath.Join(paths.WorkspaceDir, ".cursor", "mcp.json"),
			Content:      content,
			SkipIfExists: true,
		}},
	}, nil
}

// --- OpenCode ----------------------------------------------------------------

const (
	opencodeConfigSchema = "https://opencode.ai/config.json"
	opencodeConfigEnvVar = "OPENCODE_CONFIG"
	// opencode's MCP server discriminator values (distinct from kandev's
	// transport names): stdio servers are "local", all remote transports "remote".
	opencodeServerTypeLocal  = "local"
	opencodeServerTypeRemote = "remote"
)

// OpenCodeStrategy writes an opencode JSON config (an `mcp` block) to a temp file
// and points opencode at it via the OPENCODE_CONFIG env var. opencode has no
// config CLI flag; it merges OPENCODE_CONFIG over the global config without
// modifying ~/.config/opencode. opencode collapses all remote transports into a
// single "remote" type and uses "local" for stdio (command is a string array).
type OpenCodeStrategy struct{}

type opencodeMCPFile struct {
	Schema string                         `json:"$schema"`
	MCP    map[string]opencodeServerEntry `json:"mcp"`
}

type opencodeServerEntry struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Enabled     bool              `json:"enabled"`
}

func (OpenCodeStrategy) BuildPassthroughMCP(servers []types.McpServer, paths PassthroughPaths) (PassthroughArtifacts, error) {
	if len(servers) == 0 || paths.TempConfigPath == "" {
		return PassthroughArtifacts{}, nil
	}
	entries := make(map[string]opencodeServerEntry, len(servers))
	for _, srv := range servers {
		if srv.Name == "" {
			continue
		}
		if isStdioServer(srv) {
			command := append([]string{srv.Command}, srv.Args...)
			entries[srv.Name] = opencodeServerEntry{Type: opencodeServerTypeLocal, Command: command, Environment: srv.Env, Enabled: true}
		} else {
			entries[srv.Name] = opencodeServerEntry{Type: opencodeServerTypeRemote, URL: srv.URL, Headers: srv.Headers, Enabled: true}
		}
	}
	content, err := marshalMCPFile(opencodeMCPFile{Schema: opencodeConfigSchema, MCP: entries})
	if err != nil {
		return PassthroughArtifacts{}, err
	}
	return PassthroughArtifacts{
		Files: []PassthroughConfigFile{{Path: paths.TempConfigPath, Content: content}},
		Env:   map[string]string{opencodeConfigEnvVar: paths.TempConfigPath},
	}, nil
}
