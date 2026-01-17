# Agentctl session lifecycle (Kandev)

This document explains how **agentctl** fits into the Kandev execution flow, what “ready” means, and which parts of the system depend on agentctl being up.

## What agentctl is

`agentctl` is a sidecar process that runs inside each agent container (or standalone runtime). It provides:

- **HTTP API** for lifecycle operations (`/health`, `/api/v1/start`, `/api/v1/stop`, etc.)
- **ACP bridge** (stdin/stdout JSON‑RPC) to the agent process (Auggie/Codex/etc.)
- **WebSockets** for streaming ACP updates and agent output
- **Shell access** to the workspace (terminal output + input)

It is the backend’s *control plane* for an agent execution.

## High‑level flow (runtime‑agnostic)

```
User action (start task)
   |
   v
Kandev orchestrator
   |
   | 1) Create task session in DB
   | 2) Launch agent runtime (Docker or standalone)
   v
agentctl process starts
   |
   | 3) HTTP server comes up (health + control APIs)
   v
Backend waits for agentctl readiness
   |
   | 4) agentctl ready => shell/workspace access available
   v
ACP session initialization
   |
   | 5) initialize -> session/new or session/load
   | 6) prompt is sent
   v
Agent runs, ACP updates stream
   |
   | 7) session/update notifications => WS to client
   v
Task session state transitions
```

## “Ready” concepts (important distinction)

There are **two different “ready” moments**:

1) **agentctl ready**
   - Meaning: agentctl HTTP server is healthy and can accept requests.
   - Practical impact: shell/workspace endpoints become usable.
   - Source: `lifecycle.Manager.waitForAgentctlReady`.
   - Today: only logged; **not emitted as a WS event**.

2) **agent ready** (prompt completed)
   - Meaning: agent finished the current prompt and is idle/ready for follow‑up.
   - Source: `events.AgentReady` (after prompt completion).
   - Today: internal event for orchestrator state transitions.

The frontend should not rely on “agent ready” when it needs shell access; shell access depends on agentctl readiness instead.

## Current backend signals

- **agentctl readiness**
  - `agentctl is ready` (agentctl client log)
  - `agentctl ready - shell/workspace access available` (lifecycle manager log)

- **agent ready** (post‑prompt)
  - `events.AgentReady` internal event (used by orchestrator state transitions)

There is **no WS event** that tells the client “agentctl is ready.”

## Where shell streaming depends on agentctl

- Client calls `shell.subscribe` (WS request)
- Backend checks:
  - agent execution exists for the task
  - agentctl is connected
- Backend starts a streaming loop that relays agentctl shell output as `shell.output` WS notifications

If agentctl isn’t ready yet, `shell.subscribe` fails and the frontend currently retries.

## Suggested improvement (optional)

Introduce a WS notification like:

```
{ "action": "task.session.agentctl_ready", "payload": { "task_id": "...", "task_session_id": "..." } }
```

So the client can subscribe once and only request shell after it receives readiness.

## End‑to‑end diagram (detailed)

```
Client                      Backend (Kandev)                   agentctl                Agent
  |                                |                             |                      |
  | orchestrator.start             |                             |                      |
  |------------------------------->|                             |                      |
  |                                | create task session         |                      |
  |                                | launch runtime              |                      |
  |                                |---------------------------->| process starts       |
  |                                |                             | HTTP server up       |
  |                                | waitForAgentctlReady        |                      |
  |                                |---------------------------->| /health              |
  |                                |<----------------------------| 200 OK               |
  |                                | (agentctl ready)            |                      |
  |                                | ACP initialize              |                      |
  |                                |---------------------------->| initialize           |
  |                                | ACP session/new             |                      |
  |                                |---------------------------->| session/new          |
  |                                | ACP prompt                  |                      |
  |                                |---------------------------->| session/prompt       |
  |                                |                             |--------------------->|
  |                                |                             |  agent runs          |
  |                                | WS stream                   |                      |
  |<-------------------------------| (session/update notifications)                      |
```

## References

- `apps/backend/internal/agent/lifecycle/manager.go` (waitForAgentctlReady)
- `apps/backend/internal/agentctl/client/client.go` (WaitForReady)
- `apps/backend/internal/agent/handlers/shell_handlers.go` (shell.subscribe)
- `apps/backend/internal/events/types.go` (`AgentReady`)
