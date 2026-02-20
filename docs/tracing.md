# Backend Tracing

Kandev includes optional OpenTelemetry (OTel) tracing for debugging agent communication. Two tracing layers are available:

1. **Agent protocol** (`kandev-agentctl`) — traces protocol events between agentctl and the agent subprocess (ACP, StreamJSON, Codex, OpenCode, Amp, Copilot)
2. **Transport** (`kandev-transport`) — traces HTTP, WebSocket, and session lifecycle between the backend and agentctl

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

Any OTLP-compatible collector works. Point `OTEL_EXPORTER_OTLP_ENDPOINT` to its HTTP receiver (typically port 4318).

### Jaeger (quickstart)

```bash
docker run --rm --name jaeger \
        -p 16686:16686 \
        -p 4317:4317 \
        -p 4318:4318 \
        -p 5778:5778 \
        -p 9411:9411 \
        cr.jaegertracing.io/jaegertracing/jaeger:2.15.0
```

- UI: http://localhost:16686
- OTLP HTTP receiver: `http://localhost:4318`

### SigNoz / Grafana Tempo

Point `OTEL_EXPORTER_OTLP_ENDPOINT` to the OTLP HTTP receiver (e.g. `http://localhost:4318`).

## What Gets Traced

### Session lifecycle

Each agent session gets a long-lived root span (`session` or `session.recovered` for recovered sessions after backend restart). This span groups all activity for the session and is exported when the session ends.

A short-lived `session.init` child span covers the initialization phase (stream connection, ACP handshake, session creation). This span ends immediately after init, making init-phase operations visible in the trace backend while the session is still active.

> **Note:** The root session span appears as "Missing Span" in trace UIs while the session is active. This is expected — the OTel batch exporter only exports completed spans. Once the session ends (stop, completion, or deletion), the root span is exported and the full trace tree becomes visible.

### Transport layer (`kandev-transport`)

**HTTP calls** — all backend-to-agentctl HTTP requests are traced with method, path, status code, and execution ID.

**Agent WebSocket stream** — outgoing requests (initialize, session/new, session/load, prompt, cancel, permission responses) are traced as round-trip spans. Incoming agent events (`agent.event.<type>`) are traced as instant spans with the full raw JSON event attached as a span event. MCP request relays are also traced.

**Workspace WebSocket stream** — only low-volume events are traced to avoid noise (git operations, file changes, process status, connection/error events). High-volume events (shell I/O, process output, ping/pong) are skipped.

**Turn lifecycle** — a `turn_end` span is created when an agent turn completes, recording the stop reason and error state.

All transport spans include `execution_id` and `session_id` attributes for filtering.

### Agent protocol layer (`kandev-agentctl`)

Traces protocol events between agentctl and the agent subprocess. Each event span includes both the raw protocol JSON and the normalized `AgentEvent` as span events, enabling side-by-side comparison. Prompt spans include `session_id` and prompt length.

## Debug Log Files

When `KANDEV_DEBUG_AGENT_MESSAGES=true` is set, raw and normalized protocol events are also written to JSONL files in the working directory (or `KANDEV_DEBUG_LOG_DIR`):

- `raw-{protocol}-{agentId}.jsonl` — raw protocol JSON
- `normalized-{protocol}-{agentId}.jsonl` — normalized AgentEvent

## Architecture

```
internal/agentctl/tracing/
├── otel.go           # Shared OTel TracerProvider init (lazy, sync.Once)
└── transport.go      # Transport-layer span helpers (session, HTTP, WS, events)

internal/agentctl/server/adapter/transport/shared/
├── tracing.go        # Protocol-layer tracing (delegates OTel init to tracing/)
└── debug.go          # JSONL file logging + protocol constants

internal/agentctl/client/
├── client.go         # HTTP call tracing
├── client_stream.go  # WS request/response tracing
├── agent.go          # Agent event + MCP dispatch tracing
└── workspace_stream.go  # Workspace event tracing

internal/agent/lifecycle/
├── session.go        # Session span creation (session.init)
├── manager_lifecycle.go  # Recovery span creation (session.recovered)
└── types.go          # AgentExecution.SessionTraceContext / EndSessionSpan
```

The `executionID` (instance ID) is passed to each `Client` via `WithExecutionID()` and appears on every span, making it easy to filter traces for a specific agent session.

## Example: Debugging a Failed Resume

1. Start your trace collector and set the env vars
2. Trigger a session resume in the UI
3. Search for service `kandev-agentctl` or filter by `execution_id=<instance-id>`
4. Look for the `session.init` span — init-phase operations (initialize, session/load, prompt) will be nested under it
5. If a span has an error status, expand it to see the recorded error
