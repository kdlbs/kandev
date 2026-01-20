# MCP Executors: Policies + Shared MCP (HTTP/SSE Only)

## Purpose
Design executor-specific MCP policy resolution and shared MCP behavior for Kandev. This document provides low‑level implementation details and communication diagrams to be executed by another model.

## Scope
- Executor‑specific MCP allowlists, denylists, transport permissions, and URL rewrites.
- Shared MCP usage for transports that support multiple clients (HTTP/SSE/streamable HTTP).
- Integration points in existing Kandev backend lifecycle + agentctl + web UI.

---

## Feature A: Executor‑Specific MCP Policies

### Goals
- Apply different MCP transport rules per executor (local_pc, local_docker, remote_docker, remote_vps, k8s).
- Optionally rewrite MCP URLs per executor (e.g., localhost -> cluster service).
- Allow allowlists/denylists for MCP servers by name.

### Data Model
Add a typed MCP policy to executor config.

**Executor MCP Policy schema**
```
ExecutorMcpPolicy {
  allow_stdio: bool
  allow_http: bool
  allow_sse: bool
  allow_streamable_http: bool
  url_rewrite: map<string,string>
  env_injection: map<string,string>
  allowlist_servers?: string[]
  denylist_servers?: string[]
}
```

**Storage**
- Persist in `Executor.Config` (currently `map[string]string`).
- Store `mcp_policy` as a JSON string in the config map.

### Backend Changes
1) **Models**
- `apps/backend/internal/task/models/models.go`
  - Change `Executor.Config` to `map[string]any`.
  - Update JSON marshal/unmarshal in repository and DTOs.

2) **Service + Repository**
- Update executor create/update logic to read/write `mcp_policy`.
- Add validation in `task/service/service.go`:
  - `allowlist_servers` and `denylist_servers` not both set (or define precedence).
  - Ensure `url_rewrite` keys are valid URLs.

3) **Resolution path**
- Extend MCP resolver with executor policy:
  - Input: `agentName`, `executorID` (or executorType).
  - Load executor config from repo.
  - Build `Policy` from `ExecutorMcpPolicy`.
  - Apply allowlist/denylist.
  - Apply transport allow/deny.
  - Apply URL rewrite & env injection.

4) **Lifecycle integration**
- Include `executor_id` in `LaunchRequest.Metadata` if not already.
- `Manager.resolveMcpServers(...)` uses executorID to build policy.

### UI (Settings)
- Add “MCP Policy” section to executor settings page:
  - toggles for each transport
  - text area for JSON (url_rewrite, env_injection, allowlist, denylist)
- API usage:
  - update executor `config.mcp_policy`

### Validation Rules
- If allowlist is present, only servers in allowlist are considered.
- If denylist is present, servers in denylist are filtered out.
- If both present, either error or allowlist takes precedence.
- For each transport not allowed, server is skipped with warning.

### Tests
- Unit tests for policy merging and filtering.
- Integration test: executor policy with denylist excludes server from ACP payload.

---

## Feature B: Shared MCP Requires HTTP/SSE (No STDIO Proxy)

### Goal
Support shared MCP servers **only** when the transport supports multiple concurrent clients (HTTP/SSE/streamable HTTP). STDIO servers remain per‑session only.

### Policy
- If `mode=shared` and `type=stdio` → **reject** with a clear error.
- Users who want shared servers must choose MCP servers that expose HTTP/SSE/streamable HTTP endpoints.

### Validation Rules
- `mode=shared` + `type=stdio` is invalid.
- `mode=shared` + `type=http|sse|streamable_http` is valid.

### Implementation
- In MCP resolver (backend):
  - If server.Mode resolves to `shared` and server.Type is `stdio`, return error.
  - This blocks session start and surfaces a configuration error to the user.

### Error Message (example)
```
mcp server \"<name>\": shared mode requires HTTP/SSE/streamable HTTP transport (stdio is per-session only)
```

### Tests
- Attempt to save or resolve a config with `shared+stdio`.
- Expect validation error and no MCP servers passed to agent.

---

## Communication Diagrams

### A) Executor Policy Resolution (per session start)
```
User → Backend (start task)
  → Orchestrator/Executor
    → lifecycle.Manager.StartAgentProcess
      → resolveMcpServers(agentName, executorId)
        → fetch Agent MCP config
        → fetch Executor MCP policy
        → apply allowlist/denylist + transport rules + rewrites
        → return resolved MCP server list
      → agentctl ACP session/new (with MCP servers)
```

### B) Shared MCP (HTTP/SSE only)
```
Agent Session (HTTP/SSE MCP client)
  └──> MCP Server (HTTP/SSE endpoint)
        └──> shared across multiple sessions
```

### C) MCP Resolve with Shared Enforcement
```
resolveMcpServers:
  if server.mode=shared && server.type=stdio:
     return error (stdio cannot be shared)
```

---

## MCP Server Startup Responsibility (Per Executor)

### Core Rule
- STDIO MCP servers are started by the agent process (not by the executor runtime).
  - Kandev passes MCP stdio configs to the agent via ACP `session/new`.
  - The agent launches those commands in its own runtime environment.
- HTTP/SSE/streamable HTTP servers are external services.
  - The agent only connects to the provided URL.
  - Kandev/executor does not start them (unless the user runs them separately).

### Executor-by-Executor Behavior

#### 1) local_pc (standalone agentctl)
- Agent process location: host machine.
- STDIO servers: started by agent on host (must be installed on host PATH).
- HTTP/SSE servers: must be reachable from host (localhost or network).
- Notes: if MCP server requires extra deps (node/python), they must exist on host.

#### 2) local_docker
- Agent process location: inside container.
- STDIO servers: started by agent inside container.
  - MCP server binaries (e.g., `npx`, `python`) must exist in container image.
- HTTP/SSE servers: must be reachable from container network.
  - `localhost` means container itself; use host networking or service DNS when needed.

#### 3) remote_docker
- Agent process location: container on remote host.
- STDIO servers: started by agent inside remote container.
- HTTP/SSE servers: must be reachable from that remote container.
  - URLs may require rewrite to remote host/service DNS.

#### 4) remote_vps (SSH)
- Agent process location: remote host (via SSH-managed runtime).
- STDIO servers: started by agent on remote host.
- HTTP/SSE servers: must be reachable from remote host.
  - URLs may require rewrite to remote network endpoints.

#### 5) k8s
- Agent process location: pod.
- STDIO servers: started by agent in the same pod (same container image).
- HTTP/SSE servers: must be reachable via cluster DNS/service.
  - Example: rewrite `http://localhost:PORT` to `http://mcp-svc.namespace.svc.cluster.local:PORT`.

### Implications for Executor Policies
- For containerized executors, stdio MCP servers must be included in the image or installed at runtime.
- HTTP/SSE servers should be treated as external dependencies.
- URL rewrites are essential for:
  - local_docker / remote_docker (host vs container network)
  - k8s (service DNS)
  - remote_vps (public/private endpoints)

### UI Guidance
- Add a helper note in the MCP config UI:
  - “STDIO servers run inside the agent’s runtime; ensure required binaries are available there.”
  - “Shared servers must be HTTP/SSE/streamable HTTP and reachable from the executor network.”

---

## Implementation Checklist

### Executor Policies
- [ ] Update Executor.Config type + migrations
- [ ] Add policy schema + validation
- [ ] Add API to update executor config (existing endpoint)
- [ ] Apply policy in MCP resolver
- [ ] Update executor settings UI

### Shared MCP (HTTP/SSE only)
- [ ] Enforce shared transport validation in resolver
- [ ] Add UI validation message for shared+stdio
- [ ] Add tests for shared+stdio rejection

---

## Notes
- STDIO remains per‑session only.
- Shared mode is supported only for HTTP/SSE/streamable HTTP.
- Keep MCP config centralized; no proxy layer required.
