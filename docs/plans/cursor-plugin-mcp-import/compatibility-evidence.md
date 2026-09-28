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

## Discovery and Precedence Rules

1. **Discovery roots**:
   - `<cursorHome>/plugins/cache/*/*/*/{mcp.json,.mcp.json,.cursor-plugin/plugin.json,plugin.json}`
   - `<cursorHome>/plugins/local/*/{mcp.json,.mcp.json,.cursor-plugin/plugin.json,plugin.json}`
   - `<cursorHome>/plugins/marketplaces/*/*/*/{mcp.json,.mcp.json,.cursor-plugin/plugin.json,plugin.json}`
   - `<cursorHome>/mcp.json`
2. **Precedence order**:
   `Explicit Profile MCP > Existing Project User Entries > Global ~/.cursor/mcp.json > Plugin Manifests (newest mtime)`
3. **Reserved Names**:
   `kandev` is strictly reserved for Kandev internal tools and cannot be overridden by plugin imports.
4. **Ownership & Reconciliation**:
   When writing `.cursor/mcp.json`, Kandev merges plugin servers under `mcpServers`. If `cursor_plugins_mcp_enabled` is later disabled, Kandev removes only previously imported plugin servers that have not been modified by the user, preserving all user-added configuration.
