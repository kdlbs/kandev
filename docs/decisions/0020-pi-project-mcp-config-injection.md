# 0020: Pi project MCP config injection

**Status:** accepted
**Date:** 2026-06-16
**Area:** backend

## Context

Kandev injects its session-scoped MCP server into ACP `session/new` and
`session/load` requests. The Pi ACP adapter accepts those MCP parameters but
does not wire them through to the underlying Pi CLI, so session tools such as
`create_task_plan_kandev` and `ask_user_question_kandev` are unavailable in Pi
sessions. Pi MCP extensions read project-local config from `.pi/mcp.json`, which
gives Kandev a non-global injection point.

## Decision

Pi uses a dedicated `mcpconfig.PiStrategy` that writes or merges
`<workspace>/.pi/mcp.json` under the `mcpServers` key. The strategy emits Pi's
remote transport spelling (`streamable-http`) and preserves existing user
settings and servers when the file already exists.

For passthrough sessions, Pi declares the strategy on `PassthroughConfig`, using
the existing passthrough MCP materialization path. For ACP sessions, Pi declares
the same strategy on `RuntimeConfig.ProjectMCPStrategy`; the lifecycle manager
materializes the file after the agentctl instance port is known and before the
agent subprocess starts.

## Consequences

Pi sessions can discover Kandev's internal session MCP server through the Pi MCP
adapter or extension without modifying global Pi configuration. Existing
project-level `.pi/mcp.json` files are preserved and merged, while Kandev-owned
files are tracked for cleanup.

Remote executors whose agentctl-local port cannot be derived skip project-file
materialization with a warning; in that case Pi protocol-mode sessions will not
receive the project-local MCP tools. ACP `session/new` MCP injection remains in
place for agents that consume it directly.

No spec update is needed: this is an internal agent-runtime integration detail,
not a new user-invocable product surface.

## Alternatives Considered

- Wait for Pi ACP to wire `session/new` MCP servers through to Pi. This would be
  cleaner but leaves Kandev's planning and question tools unavailable today.
- Point Pi at Kandev's external MCP endpoint. Rejected because the external
  endpoint intentionally does not expose session-scoped tools.
- Reuse Cursor's project-file strategy. Rejected because Pi's preferred remote
  transport shape uses `transport: "streamable-http"` and a different file path.
