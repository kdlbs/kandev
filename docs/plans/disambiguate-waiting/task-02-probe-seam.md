---
task: 02
title: "Probe seam — BackgroundProbe port, agentctl stub, client, manager"
wave: 0
status: todo
spec_acs: AC-45, AC-62, AC-68, AC-73, AC-81
---

# Task 02 — Probe seam

Define the full `BackgroundProbe` abstraction chain as stubs that return `unknown`. The
real probe walks the process tree (task-04); the projection (task-07) depends on the port,
not the implementation. This is the wave-0 seam that decouples projection testing from
executor details.

## Files to change

### 1. `internal/agentctl/server/api/agent.go`

Add `"agent.background.probe"` to the `handleAgentStreamRequest` switch:

```go
case "agent.background.probe":
    return s.handleWSBackgroundProbe(ctx, msg)
```

Stub handler (in a new or existing file under `server/api/`):

```go
func (s *Server) handleWSBackgroundProbe(ctx context.Context, msg *ws.Message) *ws.Message {
    resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{"result": "unknown"})
    return resp
}
```

The stub returns `unknown` for every call. Task-04 replaces this with the real
process-tree walk.

**Completeness test**: `internal/agentctl/server/api/agent_test.go` has an enumeration
test for known actions. Add `"agent.background.probe"` to it.

### 2. `internal/agent/runtime/agentctl/client.go` (or new `client_probe.go`)

Add `ProbeBackgroundWorkloads(ctx context.Context, acpSessionID string)` to `Client`.
The argument is the **ACP** session id (already translated by the caller). Pattern from
`sendStreamRequest`:

```go
func (c *Client) ProbeBackgroundWorkloads(ctx context.Context, acpSessionID string) (string, error) {
    resp, err := c.sendStreamRequest(ctx, "agent.background.probe", map[string]any{
        "session_id": acpSessionID,
    })
    if err != nil {
        return "unknown", err
    }
    var payload struct {
        Result string `json:"result"`
    }
    if err := resp.ParsePayload(&payload); err != nil {
        return "unknown", err
    }
    result := payload.Result
    if result != "live" && result != "settled" {
        result = "unknown"
    }
    return result, nil
}
```

Every error path maps to `"unknown"` (spec: exhaustive error→unknown mapping).

### 3. `internal/agent/runtime/lifecycle/` (new file `manager_probe.go`)

Define the `BackgroundProbe` port interface and implement it on `*Manager`:

```go
// BackgroundProbe is the injectable port for querying whether background
// processes spawned by a session are still live.
type BackgroundProbe interface {
    Probe(ctx context.Context, kandevSessionID string) (string, error)
}

// ProbeBackgroundWorkloads implements BackgroundProbe. It translates the
// Kandev task-session id to the ACP session id and delegates to the agentctl
// client. Every error maps to "unknown".
func (m *Manager) ProbeBackgroundWorkloads(ctx context.Context, kandevSessionID string) (string, error) {
    execution, err := m.GetOrEnsureExecution(ctx, kandevSessionID)
    if err != nil {
        return "unknown", nil
    }
    c, err := m.getClient(execution)
    if err != nil {
        return "unknown", nil
    }
    result, err := c.ProbeBackgroundWorkloads(ctx, execution.ACPSessionID)
    if err != nil {
        return "unknown", nil
    }
    return result, nil
}
```

Note the triple namespace:
- Port receives **Kandev session id** (caller-facing)
- Manager reads `execution.ACPSessionID` (internal translation)
- `Client` method receives **ACP id** (agentctl-facing)

Passing the wrong id makes `AgentPID` return `ok==false` for every call — the feature
ships, probes always return `unknown`, and nothing ever parks (AC-45 guard).

### 4. Wire the port into the orchestrator

In the orchestrator's constructor/provider (`internal/orchestrator/service.go` or
`orchestrator.go`), accept a `lifecycle.BackgroundProbe` (or equivalent interface) and
store it. The projection (task-07) will call it from the sampling loop.

For Wave 0, the stored field is nil-safe: the projection doesn't exist yet, so no call
is made. Add a `SetBackgroundProbe(BackgroundProbe)` setter or constructor parameter —
follow the existing `SetCancellationPendingProvider` / `SetTaskAccessChecker` patterns.

### 5. Probe budget config

Add to `internal/orchestrator/` (or wherever orchestrator config lives) two env-backed
duration fields:

```
KANDEV_BACKGROUND_PROBE_BUDGET   — per-probe timeout, default 5s, must be > 0
KANDEV_BACKGROUND_SAMPLE_INTERVAL — sampling interval, default 30s
```

Follow `getEnvDuration("KANDEV_ACP_IDLE_TIMEOUT", time.Hour)` pattern in
`internal/agentctl/server/config/config.go:285`. The probe budget deviates from the
"`0` disables" idiom: non-positive values must be rejected (AC-81). Both are read by
the backend, not by agentctl.

## Tests to write

- **Client method test**: verify `ProbeBackgroundWorkloads` with a fake WS server that
  returns `{"result": "live"}`, `{"result": "settled"}`, and an error — confirm the
  three results and the error→unknown mapping.

- **Manager method test**: verify the Kandev-id→ACP-id translation by stubbing
  `GetOrEnsureExecution` and confirming the client receives `execution.ACPSessionID`.

- **Completeness test update**: add `"agent.background.probe"` to the action enumeration
  in `agent_test.go`.

- **Config test**: verify that non-positive probe budget is rejected (AC-81).

## Acceptance criteria closed

- **AC-45** (guard): Kandev id → ACP id translation documented and tested.
- **AC-62**, **AC-68**, **AC-73** (partial): port interface exists and is injectable.
  Full closure when task-07 drives the port in the sampling loop.
- **AC-81**: non-positive probe budget rejected at config time.
