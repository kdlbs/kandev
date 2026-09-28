# Cursor plugin MCP compatibility and ownership evidence

## Tested environment and binary

- **Cursor Agent version:** `2026.09.26-dd393fe`
- **Platform:** Darwin arm64 (macOS)
- **Protocol:** ACP v1 (`cursor-agent acp`) and CLI (`cursor-agent`)

## Observed behavior & gap reproduction

1. **Interactive CLI (`cursor-agent`)**:
   - Reads `~/.cursor/plugins/` dynamically on startup.
   - Discovers plugin tools on the fly via `GetDynamicTools` / `CallDynamicTool`.
2. **ACP Server (`cursor-agent acp`)**:
   - Does not dynamically scan `~/.cursor/plugins/` into the ACP protocol tool definitions.
   - Connects to MCP servers declared in the project's `.cursor/mcp.json` (or `~/.cursor/mcp.json`).
   - When `.cursor/mcp.json` is populated with plugin server definitions (e.g. `atlassian`, `figma`), `cursor-agent acp` connects and exposes the tools.

## Supported manifest shapes

### 1. Standalone MCP files (`mcp.json` / `.mcp.json`)
```json
{
  "mcpServers": {
    "atlassian": {
      "type": "streamable-http",
      "url": "https://mcp.atlassian.com/v2/mcp"
    }
  }
}
```

### 2. Plugin descriptor manifests (`.cursor-plugin/plugin.json` / `plugin.json`)
```json
{
  "name": "harness",
  "mcpServers": {
    "figma": {
      "type": "http",
      "url": "https://mcp.figma.com/mcp"
    }
  }
}
```

### 3. Global user configuration (`~/.cursor/mcp.json`)
```json
{
  "mcpServers": {
    "custom-server": {
      "command": "node",
      "args": ["server.js"]
    }
  }
}
```

## Discovery, Policy, Precedence and Reconciliation Rules

1. **Eligible Discovery Roots & Selection**:
   - Central registry: checks `<cursorHome>/plugins/installed.json` or `<cursorHome>/plugins/plugins.json` when present to filter out disabled or uninstalled plugins.
   - Local plugins: `<cursorHome>/plugins/local/*/{mcp.json,.mcp.json,.cursor-plugin/plugin.json,plugin.json}`.
   - Cache & marketplaces: `<cursorHome>/plugins/cache/*/*/*` and `<cursorHome>/plugins/marketplaces/*/*/*`. Only completed versions containing valid manifests are considered, grouped by plugin name to pick only the single newest version per plugin.
   - User global config: `<cursorHome>/mcp.json`.

2. **Descriptor & Manifest Support**:
   - Inline `mcpServers` object and relative file references (e.g. `"mcpServers": "config/servers.json"`).
   - Relative references are strictly contained within `pluginRoot` after symlink evaluation with 1MB bounded I/O.
   - Supported variables (`${CURSOR_PLUGIN_ROOT}`, `${PLUGIN_ROOT}`) are expanded in `command`, `args`, and `env` without shell evaluation.
   - Entries with unsupported execution context (such as required `cwd`) or unresolved variables `${...}` are skipped.

3. **Transport Validation**:
   - Validates mutually exclusive command (stdio) vs URL (network) shapes.
   - Normalizes network transports (`http`, `sse`, `streamable-http`).
   - Rejects conflicting or unknown transports without masking valid siblings.

4. **Executor Policy Enforcement**:
   - All imported candidate servers are resolved against the executor's `mcpconfig.Policy`.
   - Transport allow/deny, server allowlist/denylist, URL rewrites, and environment injection apply to imported servers.
   - Explicit profile server names (including any denied by policy) and `kandev` are strictly reserved, preventing plugins from resurrecting denied profile servers.

5. **Precedence Order**:
   `Internal kandev > Explicit Profile MCP > User-Owned Project Servers > Global User ~/.cursor/mcp.json > Plugin Manifests (newest mtime)`

6. **Persistent Ownership & Reconciliation**:
   - Backend-private ownership state is tracked per workspace in `.cursor/.kandev-mcp-imports.json` recording imported server names and sha256 JSON fingerprints.
   - On each launch/preparation, previously imported servers whose fingerprints match (unmodified by user) are removed if they are no longer in the allowed import set (e.g. preference disabled or plugin deleted).
   - User-edited entries (fingerprint mismatch) and pre-existing user project servers are preserved as user-owned.
   - Writes to `.cursor/mcp.json` and `.cursor/.kandev-mcp-imports.json` are atomic with 0600 permissions. Symlinked or malformed project files are left untouched.
