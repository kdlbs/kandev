# MCP Servers Low-Level Design (Kandev)

## Purpose
Define how Kandev configures and runs MCP servers per agent, across executor types, with support for shared (one instance for all sessions) and per-session instances. This document is intended to be sufficient for implementation without additional context.

## Terminology
- Agent: model/tool persona (e.g., Codex, Claude Code, Cursor).
- Executor: runtime target (local PC, local Docker, remote Docker, remote SSH, K8s operator).
- MCP server: external tool provider (stdio/http/sse/streamable_http).

## Goals
- Centralize MCP configuration in Kandev as the source of truth.
- Support per-agent enable/disable and per-server mode selection.
- Resolve MCP servers at runtime based on executor capabilities.
- Avoid client-side config stitching; backend should handle canonical config.
- Support shared and per-session MCP server instances.

## Data Model (Canonical)
Store canonical MCP config per agent (and optionally per executor override).

AgentMcpConfig
- agent_id: string
- enabled: bool
- servers: map<string, McpServerDef>
- meta: optional map<string, McpMetaDef>

McpServerDef
- type: "stdio" | "http" | "sse" | "streamable_http"
- command?: string
- args?: string[]
- env?: map<string,string>
- url?: string
- headers?: map<string,string>
- mode?: "shared" | "per_session" | "auto"  (default: auto)

McpMetaDef
- name?: string
- description?: string
- url?: string
- icon?: string

ExecutorMcpPolicy
- allow_stdio: bool
- allow_http: bool
- allow_sse: bool
- allow_streamable_http: bool
- http_url_rewrite?: map<string,string>  (e.g., localhost -> k8s service)
- env_injection?: map<string,string>

## API
Use agent-based query, not executor.

GET /api/mcp-config?agent=CODEX
- Response: AgentMcpConfig

POST /api/mcp-config?agent=CODEX
- Body: { enabled: bool, servers: map<string, McpServerDef> }
- Response: success/error

## Resolution Pipeline (Per Run)
Inputs: agent_id, executor_type, session_id

1) Load AgentMcpConfig for agent_id.
2) If enabled == false, return empty MCP list.
3) Apply ExecutorMcpPolicy:
   - Filter out disallowed transports.
   - Rewrite URLs via http_url_rewrite.
   - Inject env vars into server env (merge with server env; server overrides policy only if explicit allow).
4) Resolve per-server mode:
   - mode == auto:
     - stdio -> per_session
     - http/sse/streamable_http -> shared
   - mode == shared with stdio: reject unless stdio proxy is enabled.
5) Build ResolvedMcpServer list for runtime.

ResolvedMcpServer
- name
- transport
- mode
- endpoint (stdio pipes or http url)
- env/headers

## Shared vs Per-Session Semantics
Shared
- One MCP server instance shared across all agent sessions.
- Works natively for http/sse/streamable_http.
- For stdio, requires a proxy that exposes a shared HTTP/SSE endpoint.

Per-Session
- One MCP server instance per agent session.
- Works for stdio and http/sse/streamable_http.

Auto
- Default behavior based on transport:
  - stdio -> per_session
  - http/sse/streamable_http -> shared

## Executor Feasibility Matrix
Local PC
- Shared: run local HTTP/SSE server once; connect from multiple sessions.
- Per-session: spawn stdio or HTTP server per session.

Local Docker
- Shared: run sidecar or host service once.
- Per-session: spawn server inside container per session.

Remote Docker
- Shared: one service per host/cluster.
- Per-session: spawn server inside container per session.

Remote SSH
- Shared: run persistent MCP service on remote host.
- Per-session: spawn stdio over SSH per session.

K8s Operator
- Shared: MCP server as Service (cluster or namespace scoped).
- Per-session: sidecar in pod per session.

## UI Requirements
Settings page per agent:
- Toggle: enable/disable MCP for agent.
- Server list editor (canonical JSON or structured UI).
- Per-server mode selector (shared/per_session/auto).
- Validation warnings:
  - shared + stdio -> error unless proxy enabled
  - transport disallowed by selected executor -> warning

## Centralized vs File-Based
Default: centralized config in Kandev DB/config.

Optional file materialization (only if required by agent runtime):
- Generate per-run config file in temp dir.
- Inject via env var or CLI flag if supported.
- If not supported, write to agent-specific config path inside executor environment.
- Do not treat agent-owned files as source of truth.

## Notes on Agent Support (feasible transports)
- Codex: stdio + streamable_http
- Claude Code: stdio + http + sse
- Cursor: stdio + sse + streamable_http
- Gemini: stdio + sse + streamable_http
- Qwen Code: stdio + sse + streamable_http
- Opencode: local (stdio) + remote (http)

## Implementation Checklist
Backend
- AgentMcpConfig storage (DB or config file).
- ExecutorMcpPolicy per executor type.
- Resolution pipeline and validation.
- API endpoints: GET/POST /api/mcp-config.
- Optional stdio shared proxy support.

Frontend
- Agent MCP settings page.
- Enabled toggle per agent.
- Server list editor with mode selector.
- Validation errors and warnings surfaced.

Testing
- Unit tests for resolution logic (mode, policy filtering).
- API tests for GET/POST.
- UI tests for enable/disable and validation behavior.
