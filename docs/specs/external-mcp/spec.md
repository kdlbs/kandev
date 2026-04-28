---
status: draft
created: 2026-04-28
owner: tbd
---

# External MCP Endpoint

## Why

Users want to operate on Kandev workspaces, workflows, agents, and tasks from coding agents (Claude Code, Cursor, Codex, …) running outside Kandev. Today the Kandev MCP is embedded inside each `agentctl` instance, scoped to a single session, and reachable only on a container-internal `localhost` port — there is no way to add Kandev as an MCP server in an external agent's global config.

## What

- The Kandev backend exposes an MCP server on its existing HTTP port (default `38429`) bound to `127.0.0.1`.
- Users register Kandev as an MCP server in their external coding agent using one of:
  - Streamable HTTP: `http://localhost:38429/mcp`
  - SSE: `http://localhost:38429/mcp/sse`
- The external endpoint exposes the **config-mode tool surface** plus `create_task_kandev` (no plan tools, no `ask_user_question_kandev`).
- The endpoint requires no authentication in v1.
- The Settings UI shows the URL and ready-to-paste config snippets for popular agents.
- The existing per-`agentctl` MCP behavior is unchanged — the same tool definitions back both endpoints.

## Scenarios

- **GIVEN** a running Kandev backend, **WHEN** a user adds `http://localhost:38429/mcp` to their Claude Code MCP config and asks "list my Kandev workspaces", **THEN** Claude Code calls `list_workspaces_kandev` and returns the workspaces that appear in the Kandev UI.
- **GIVEN** the user is in a Cursor session outside Kandev, **WHEN** they ask Cursor to "create a task to fix the login bug in workspace X", **THEN** the task appears in the Kandev kanban board.
- **GIVEN** the user opens Kandev Settings → External MCP, **WHEN** they click "Copy Claude Code config", **THEN** they receive a JSON snippet they can paste into `~/.claude.json`.
- **GIVEN** an external agent calls a session-scoped tool that does not exist on this endpoint, **THEN** the call returns a tool-not-found error and the rest of the session is unaffected.

## Out of scope

- Authentication / bearer tokens / OAuth (v1 relies on `127.0.0.1`-only binding).
- Binding to non-loopback addresses for remote access.
- Session-scoped tools (`create_task_plan_kandev`, `ask_user_question_kandev`, plan get/update/delete).
- Per-workspace scoping of the endpoint.
- Exposing `agentctl`'s per-session MCP externally — architectural mismatch (per-session, ephemeral, no auth boundary).
