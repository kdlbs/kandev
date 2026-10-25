# Task 04: Probe transport (`agent.background.probe` WS action) and config

Spec: §"Probe port (backend)", §"Probe transport (backend ↔ agentctl)",
§"Timing, configuration...", §"Permissions". ACs: AC-45, AC-46, AC-81.
Round-5 findings closed here: F6 (security guard), F8/F10 negative-interval half.

## `BackgroundProbe` port (backend)

```go
package orchestrator // or a sibling package; production impl wraps agentctl.Client

type BackgroundProbe interface {
    Probe(ctx context.Context, sessionID string) (ProbeResult, error)
}
```

Test double: scripted sequence of the three literals, used by task-05's
projection tests so those tests never depend on the transport or the real
process walk.

## Transport

- New WS action `agent.background.probe`, request `{"session_id": "..."}`,
  response `{"result": "live"|"settled"|"unknown"}`.
- New `case` in `internal/agentctl/server/api/agent.go`'s action switch
  (alongside `agent.cancel`, `agent.permissions.respond`, `agent.stderr`).
  **Add it explicitly to `TestHandleAgentStreamRequest_DispatchesCorrectActions`**
  (`agent_test.go:204-237`) — that test is not exhaustive over all 12 existing
  actions, so it will not catch an omission on its own.
- `Client.ProbeBackgroundWorkloads(ctx, sessionID) (ProbeResult, error)` in
  `internal/agent/runtime/agentctl/`, using `sendStreamRequest`
  (`client_stream.go:26`) exactly like `RespondToPermission`.
- Backend applies the probe budget as `context.WithTimeout` around the call;
  no timeout is baked into the transport itself.
- **Every failure resolves to `unknown`** — build the mapping exhaustively over
  the outcome union, not a list of anticipated errors: `ErrAgentStreamNotConnected`,
  context deadline exceeded, any WS error frame (including
  `ErrorCodeUnknownAction`), unparseable/absent body, a `result` value outside
  the three literals, any other transport/marshalling error.

## F6 — security guard (round-5, load-bearing)

`Manager.RespondToPermissionBySessionID` does **not** call `CheckSessionAccess`
(verified: `manager_interaction.go:1546`, no guard call; `CheckSessionAccess`
itself lives in `manager.go:409`). Do not assume the probe path inherits an
authorization boundary from that precedent — it has none. Add an explicit
`CheckSessionAccess`-equivalent call on the backend side of
`agent.background.probe` before it reaches `lifecycle.Manager`, scoped exactly
like the workspace/session access checks used by the passthrough handlers
(`passthrough_handlers.go:56`, `:107`) and the port/vscode proxies. Add a test
asserting a denied session ID yields a rejection, not a probe dispatch.

## Config

- `KANDEV_PARKED_PROBE_BUDGET`, default `250ms`, read by **backend**. `0` or
  negative is **rejected** at config load (warn-logged, default used) — this
  is a deliberate exception to the "0 disables" idiom, because 0 here would
  enable an unbounded blocking call on the turn-settle path. Follow
  `getEnvDuration` (`config/config.go:746`) but add the positive-value guard
  on top of it — do not use `getEnvDuration` unguarded.
- `KANDEV_PARKED_PROBE_INTERVAL`, default `30s`, read by **backend**. `0`
  disables periodic sampling (documented behaviour, task-05). **Negative is
  rejected the same way as the budget** (round-5 F10: `getEnvDuration` returns
  any parseable value and `time.NewTicker` panics on non-positive — closed
  here by rejecting negative at config load, same warn+default pattern; `0`
  itself remains valid and meaningful, per spec).
- Both are plain env config, not `runtimeflags/registry.go` entries (spec is
  explicit these are operational tuning, not a release gate).

## Tests

- AC-45: request shape carries `session_id`, no timestamp; response is one of
  exactly three literals.
- AC-46: each of the five listed failure conditions maps to `unknown` and the
  session reports `parked_on_background_work: false`.
- AC-81: budget `0` and a negative budget are both rejected at config load,
  default used, warning logged.
- Negative interval rejected at config load; `0` interval accepted (task-05
  covers its behavioural meaning).
- F6 guard: an unauthorized caller's probe request never reaches
  `lifecycle.Manager`.
