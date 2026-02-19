# Backend Tracing

Kandev includes optional OpenTelemetry (OTel) tracing for debugging agent communication. Two tracing layers are available:

1. **Agent protocol** (`kandev-agentctl`) — traces ACP/StreamJSON events between agentctl and the agent CLI subprocess
2. **Transport** (`kandev-transport`) — traces HTTP and WebSocket communication between the backend and agentctl

Both layers use a shared OTel initialization in `internal/agentctl/tracing/` but have different activation requirements.

## Enabling Tracing

### Transport layer (backend <> agentctl)

Only requires the OTLP endpoint:

```bash
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

### Agent protocol layer (agentctl <> agent subprocess)

Requires both the OTLP endpoint **and** the debug flag:

```bash
export KANDEV_DEBUG_AGENT_MESSAGES=true
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

`KANDEV_DEBUG_AGENT_MESSAGES=true` also enables JSONL debug log files (see below).

Without `OTEL_EXPORTER_OTLP_ENDPOINT`, all tracers are no-op with zero runtime overhead.

## Running a Trace Collector

### Jaeger (quickstart)

```bash
docker run -d --name jaeger \
  -p 16686:16686 \
  -p 4318:4318 \
  jaegertracing/all-in-one:latest
```

- UI: http://localhost:16686
- OTLP HTTP receiver: `http://localhost:4318`

### Grafana Tempo

If you already run Grafana + Tempo, point `OTEL_EXPORTER_OTLP_ENDPOINT` to your Tempo distributor (e.g. `http://localhost:4318`).

## What Gets Traced

### Transport layer (`kandev-transport`)

**HTTP calls** (backend -> agentctl):

| Span name | When |
|---|---|
| `http.POST /api/v1/agent/configure` | ConfigureAgent |
| `http.POST /api/v1/start` | Start agent process |
| `http.POST /api/v1/stop` | Stop agent process |
| `http.GET /api/v1/status` | GetStatus |

Each span records `http.method`, `http.path`, `http.status_code`, and `execution_id`. Errors are recorded on the span.

**Agent WebSocket stream** (`/api/v1/agent/stream`):

| Span name pattern | Direction | Description |
|---|---|---|
| `ws.agent.initialize` | outgoing | Initialize handshake |
| `ws.agent.session.new` | outgoing | Create new ACP session |
| `ws.agent.session.load` | outgoing | Resume existing session |
| `ws.agent.prompt` | outgoing | Send user message |
| `ws.agent.cancel` | outgoing | Cancel current operation |
| `ws.agent.permissions.respond` | outgoing | Respond to permission request |
| `agent.event.<type>` | incoming | Agent events (message_chunk, tool_call, complete, etc.) |
| `mcp.dispatch.<action>` | relay | MCP request forwarded to handler |

Outgoing request spans cover the full round-trip (started on send, ended when response arrives). Agent event spans are instant.

**Workspace WebSocket stream** (`/api/v1/workspace/stream`):

Only low-volume events are traced to avoid noise:

| Traced | Skipped (high volume) |
|---|---|
| `git_status`, `git_commit`, `git_reset` | `shell_output`, `shell_input` |
| `file_change`, `process_status` | `process_output`, `ping`, `pong` |
| `connected`, `error` | `shell_resize`, `shell_exit` |

### Agent protocol layer (`kandev-agentctl`)

Traces ACP/StreamJSON protocol events between agentctl and the agent subprocess. Each event span includes raw protocol JSON and the normalized `AgentEvent` for side-by-side comparison.

## Debug Log Files

When `KANDEV_DEBUG_AGENT_MESSAGES=true` is set, raw and normalized protocol events are also written to JSONL files in the working directory (or `KANDEV_DEBUG_LOG_DIR`):

- `raw-{protocol}-{agentId}.jsonl` — raw protocol JSON
- `normalized-{protocol}-{agentId}.jsonl` — normalized AgentEvent

## Architecture

```
internal/agentctl/tracing/
├── otel.go           # Shared OTel TracerProvider init (lazy, sync.Once)
└── transport.go      # Transport-layer span helpers

internal/agentctl/server/adapter/transport/shared/
├── tracing.go        # Protocol-layer tracing (delegates OTel init to tracing/)
└── debug.go          # JSONL file logging

internal/agentctl/client/
├── client.go         # HTTP call tracing
├── client_stream.go  # WS request/response tracing
├── agent.go          # Agent event + MCP dispatch tracing
└── workspace_stream.go  # Workspace event tracing
```

The `executionID` (instance ID) is passed to each `Client` via `WithExecutionID()` and appears on every span, making it easy to filter traces for a specific agent session.

## Example: Debugging a Failed Resume

1. Start Jaeger and set the env vars
2. Trigger a session resume in the UI
3. Open Jaeger, search for service `kandev-agentctl` or traces with `execution_id=<instance-id>`
4. Look for the sequence: `http.POST /api/v1/agent/configure` -> `http.POST /api/v1/start` -> `ws.agent.initialize` -> `ws.agent.session.load` -> `ws.agent.prompt`
5. If a span has an error status, expand it to see the recorded error
