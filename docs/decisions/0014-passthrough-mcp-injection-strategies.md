# 0014: Per-CLI MCP server injection for passthrough mode

**Status:** accepted
**Date:** 2026-05-29
**Area:** backend

## Context

Kandev runs agent CLIs in two modes: **ACP** (kandev speaks the Agent Client Protocol to the CLI and passes MCP servers via `session/new`) and **passthrough** (the raw CLI runs directly in a PTY and the user interacts with its native TUI). MCP servers configured on an agent profile's settings page were only wired into ACP mode. In passthrough mode the single `writePassthroughMCPConfig` helper hardcoded one server — kandev's own HTTP tools server — into a Claude-shaped JSON file, gated on a `MCPConfigFlag Param` that only `claude-acp` declared. Codex, Cursor, and OpenCode passthrough sessions received **no** MCP servers at all, and even Claude never received the profile's configured servers.

The naive fix — "write a JSON file and pass `--mcp-config`" — only works for Claude. Each CLI loads MCP servers differently, uses a different config schema, and has a different "don't touch the user's global config" escape hatch:

- **Claude Code** — `--mcp-config <file>` flag (variadic, additive); `{"mcpServers": {...}}` schema.
- **Codex** — no MCP-config-file flag at all; servers come from `~/.codex/config.toml` or repeatable `-c key=value` CLI overrides.
- **Cursor (`cursor-agent`)** — no MCP flag and no reliable MCP env var; only auto-discovers a project-local `.cursor/mcp.json`.
- **OpenCode** — no config CLI flag; config file is selected via the `OPENCODE_CONFIG` env var; schema uses an `mcp` block with `local`/`remote` types.

A hard requirement was that kandev must **never modify the user's global CLI config** (`~/.claude.json`, `~/.codex/config.toml`, `~/.cursor/mcp.json`, `~/.config/opencode`).

## Decision

**Each passthrough-capable agent declares a `PassthroughMCPStrategy` that materializes the resolved MCP server list into that CLI's native shape (config files, CLI args, and/or env vars), without writing to the user's global config.** The runtime resolves servers once and delegates emission to the strategy.

The strategy interface (`internal/agent/mcpconfig/passthrough.go`) is pure — it returns descriptors; the runtime performs all I/O:

```go
type PassthroughMCPStrategy interface {
    BuildPassthroughMCP(servers []types.McpServer, paths PassthroughPaths) (PassthroughArtifacts, error)
}
// PassthroughArtifacts: Files (to write, with SkipIfExists), Args (appended to cmd), Env (merged into process env)
```

Four implementations:

| CLI | Mechanism | Schema | Global config touched |
|-----|-----------|--------|----------------------|
| Claude | temp file + `--mcp-config <path>` args | `{"mcpServers":{…}}`, `streamable_http`→`streamable-http` | No |
| Codex | repeated `-c mcp_servers.<name>.<key>=<json>` args | transport inferred from `command` vs `url`; no `type` key | No |
| Cursor | project-local `<workspace>/.cursor/mcp.json`, `SkipIfExists` | `{"mcpServers":{…}}` | No |
| OpenCode | temp file + `OPENCODE_CONFIG` env var | `{"mcp":{name:{type:"local"\|"remote",…}}}` | No |

Runtime wiring (`internal/agent/runtime/lifecycle/manager_passthrough.go`, `applyPassthroughMCP`):

1. **Resolve once, reuse the ACP path.** Profile servers come from the same `resolveMcpServersWithParams` ACP already uses (transport policy, allow/deny, URL rewrite, env injection all apply identically). Kandev's own HTTP tools server is prepended; a profile server named `kandev` is dropped so it cannot shadow the real one.
2. **Strategy emits, runtime writes.** Files are written 0600 under a kandev-owned temp dir (Claude/OpenCode) or the worktree (Cursor); written paths are tracked (unioned across relaunch/resume/restart) in `execution.Metadata` for cleanup. `SkipIfExists` files that already exist are never overwritten and never tracked (so a user's own file is neither clobbered nor deleted). Strategy env is merged in `buildPassthroughEnv`; strategy args are appended **last** to the command.
3. **Cleanup** removes only kandev-written files when the execution is removed.

The old `MCPConfigFlag Param` / `CmdBuilder.MCPConfig` / `PassthroughOptions.MCPConfigPath` mechanism was removed in favor of `MCPStrategy` and `PassthroughOptions.MCPArgs`.

### Key sub-decisions

- **MCP args are appended *last* in the built command.** Claude's `--mcp-config` is variadic and would otherwise swallow a positional prompt as an extra config path. Codex's `-c` overrides are order-insensitive, so trailing placement is safe for both. (Only Claude and Codex emit args; Cursor/OpenCode use files/env.)
- **Claude injection is additive (no `--strict-mcp-config`).** Kandev's servers merge with the user's `~/.claude.json` and project `.mcp.json` rather than replacing them. `ClaudeStrategy{Strict: true}` is available if isolation is ever wanted, but the friendlier default preserves the user's own servers.
- **Cursor only creates `.cursor/mcp.json` when absent.** If the user already has one, kandev injects nothing for Cursor that session — including its own tools server. This honors "never touch the user's files" at the cost of kandev tools being unavailable when a project file pre-exists.

## Consequences

- All four CLIs receive kandev's tools server plus the profile's MCP servers in passthrough mode; previously three received nothing.
- Adding a passthrough CLI with a new MCP mechanism = implement one `PassthroughMCPStrategy` and set it on the agent's `PassthroughConfig`. No runtime changes.
- The kandev HTTP server requires the standalone port; passthrough launch still errors when the port is unavailable (preserved behavior).
- `execution.Metadata` stores the written-file list and env map; getters decode both in-memory (`[]string`, `map[string]string`) and JSON-rehydrated (`[]interface{}`, `map[string]interface{}`) shapes so cleanup survives a backend restart.
- Cursor's "skip if exists" means kandev tools can be silently absent for Cursor when a user `.cursor/mcp.json` exists — an accepted trade-off, documented at the strategy.

## Alternatives considered

- **`CODEX_HOME` pointing at a kandev temp dir (rejected).** It relocates Codex's *entire* config dir — including `auth.json` — so a bare temp dir leaves Codex unauthenticated, forcing kandev to seed/symlink the user's credentials. Repeatable `-c` overrides inject only the MCP servers, touch nothing else, and never write to disk.
- **Keep the single Claude-shaped writer and bolt on per-CLI special cases (rejected).** The output schemas and delivery mechanisms diverge enough (TOML-via-flags vs JSON file vs env-selected JSON vs project file) that a strategy per agent is clearer than branching inside one writer.
- **A `kind` enum dispatched by the runtime (rejected).** A true strategy object per agent keeps the per-CLI emission logic with its schema and avoids a growing switch in the runtime.
- **Emit Codex's `experimental_use_rmcp_client` for remote servers (deferred).** Some intermediate Codex versions gated streamable-HTTP MCP behind that flag; current Codex loads `url` servers natively, and kandev launches the latest via `npx`. Emitting a global experimental flag risks affecting the user's other servers or erroring on versions where the key changed. Documented at the call site instead.
