---
id: task-07
title: Health poller integration
status: pending
wave: 2
depends_on: [task-05, task-06]
plan: docs/plans/plugins/plan.md
---

# Health poller integration

## Title
Poll each active/error plugin's health endpoint on an interval and drive the
state machine (active↔error), flushing buffered events on recovery.

## Inputs
- Spec `docs/specs/plugins/spec.md` → "State machine" health monitoring: every
  30s, `GET {base_url}{endpoints.health}` must return 200 within 5s; 3 consecutive
  failures → `error` + inbox item; next success → `active` + flush buffered events
  in order.
- Precedent: `internal/integrations/healthpoll` (`Prober`/`Poller`, configurable
  interval). Either reuse `healthpoll.New(name, prober, log)` per plugin or write a
  dedicated small poller loop in `internal/plugins`. Prefer a single loop that
  iterates all plugins every 30s (simpler than N pollers).
- Interacts with task-05 Service (`SetStatus`) and task-06 Deliverer (flush on
  recovery via `Deliverer.Flush(pluginID)` — add that method in task-06 if missing;
  note it in your report so task-06 exposes it).

## Acceptance
1. `internal/plugins/health.go`: `HealthMonitor` with `Start(ctx)`/`Stop()`,
   30s ticker (interval injectable for tests), 5s per-request timeout.
2. Tracks consecutive failures per plugin; 3 → `SetStatus(error)`; success from
   error → `SetStatus(active)` + `Deliverer.Flush(id)`.
3. Updates `last_health_check` on the record via the Service/store.
4. Inbox item on transition to error: emit an event on the bus
   (`plugin.health.degraded` with plugin id) OR call an injected notifier — pick
   the lighter option and document it. (Full office-inbox wiring is out of scope;
   emitting a bus event is sufficient.)

## Files
- `apps/backend/internal/plugins/health.go` + `health_test.go` (httptest server,
  injected short interval)

## Verification
- `go test ./internal/plugins/...` from `apps/backend`
- `make -C apps/backend lint`

## Output contract
Report: single-loop vs per-plugin design chosen, transition thresholds, how
recovery flush is triggered, and how the degraded notification is emitted.

## Dependencies
task-05, task-06.
