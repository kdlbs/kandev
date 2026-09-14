---
id: "03-reachability-poller"
title: "Reachability poller, hysteresis, and configuration"
status: pending
wave: 2
depends_on: ["01-probe-and-classification", "02-reachability-persistence"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-REACHABILITY-001.1
  - AC-EXECUTORS-SSH-REACHABILITY-001.2
  - AC-EXECUTORS-SSH-REACHABILITY-001.3
  - AC-EXECUTORS-SSH-REACHABILITY-001.4
  - AC-EXECUTORS-SSH-REACHABILITY-001.5
  - AC-EXECUTORS-SSH-REACHABILITY-001.7
  - AC-EXECUTORS-SSH-REACHABILITY-001.9
  - AC-EXECUTORS-SSH-REACHABILITY-001.10
  - AC-EXECUTORS-SSH-REACHABILITY-001.11
  - AC-EXECUTORS-SSH-REACHABILITY-001.12
  - AC-EXECUTORS-SSH-REACHABILITY-001.13
  - AC-EXECUTORS-SSH-REACHABILITY-001.14
  - AC-EXECUTORS-SSH-REACHABILITY-001.15
  - AC-EXECUTORS-SSH-REACHABILITY-001.16
  - AC-EXECUTORS-SSH-REACHABILITY-001.17
  - AC-EXECUTORS-SSH-REACHABILITY-001.20
  - AC-EXECUTORS-SSH-REACHABILITY-001.23
  - AC-EXECUTORS-SSH-REACHABILITY-001.24
  - AC-EXECUTORS-SSH-REACHABILITY-001.25
  - AC-EXECUTORS-SSH-REACHABILITY-001.26
system_design:
  - ../../specs/executors/system-design/ssh-reachability.md
---

# Task 03: Reachability Poller, Hysteresis, and Configuration

## Summary

Add `internal/executors/reachability`: the ticker that sweeps every eligible
SSH executor on a configurable interval with bounded concurrency, the write
path that owns asymmetric hysteresis, the operator configuration key with its
clamp and disable behavior, and the metrics. This is the first code that calls
the probe from task 01 and the store from task 02.

## In scope

- A probe cancelled by `Stop` is discarded, never written: a graceful shutdown
  must not leave failure records behind. Test it by cancelling mid-pass and
  asserting the stored records are byte-identical to before.
- The poller: an immediate pass on `Start`, then a pass per tick. `Start` and
  `Stop` idempotent, `Stop` draining the in-flight pass, a `goleak` `TestMain`
  guarding the package — `healthpoll`'s conventions adopted without importing
  it.
- One pass: list eligible executors, dispatch in ascending `executors.id`
  order over a semaphore of `passConcurrency`, await all workers. A
  mutex-guarded `passRunning` flag drops an overlapping tick rather than
  queueing it, and increments a skipped counter.
- The write path owning hysteresis: success writes `reachable` with a zero
  counter from any prior state, an empty reason and message, and
  `last_success_at` set to that probe's completion timestamp; a failure leaves
  `last_success_at` untouched and keeps its reason readable until a later
  probe succeeds, so a reason is empty only when the most recent completed
  probe succeeded or none has; failure increments the counter and writes
  `unreachable` only on the probe where it reaches `failureThreshold`.
  A sticky failure (`config` or `host_key`) writes `unreachable` on that probe
  regardless of the counter: both repeat until a human changes something, so
  the threshold would buy no confidence and would delay a security event.
- A record that does not exist reports `unknown`; the state is never inferred.
- Compile-time constants `probeTimeout` 10s, `passConcurrency` 4,
  `failureThreshold` 2, each with the rationale recorded in the design.
- Catalog entry `executors.sshReachabilityIntervalSeconds` /
  `KANDEV_EXECUTORS_SSHREACHABILITYINTERVALSECONDS`, default `60`, supported
  range 15 to 3600. Only `0` disables. Absent, empty, negative, fractional or
  unparsable falls back to the **default**, never to a bound, so a typo cannot
  silently switch monitoring off; `1`-`14` clamps up to `15` and above `3600`
  clamps down, logged once at startup. The catalog's minimum must stay `0`, or
  `0` falls below it and the kill switch becomes a 60-second cadence. Requires a matching entry in
  `auditedStartupEnvironmentInventory()` in `catalog_test.go`, a new
  `ExecutorsConfig` section on `config.Config`, and its `SetDefault`.
- `expvar` counters `executor_ssh_reachability_probe_total` (by outcome),
  `…_state_transitions_total` (by destination state), `…_pass_skipped_total`,
  `…_probe_duration_ms`, each also a structured `zap` log. Transitions log at
  `Warn` downward and `Info` upward; an unchanged probe does not log.
- Wiring in `internal/backendapp/main.go` alongside the integration pollers,
  with `addRuntimeCleanup` calling `Stop`.

## Out of scope

- Publishing the change event and the HTTP routes. Task 04 owns both; this
  task's write path exposes the change signal as a return value that task 04
  consumes.
- The immediate-probe action and the reset-on-config-change behavior.
- Any launch-path interaction.
- A runtime feature toggle. The interval key's `0` is the kill switch, and a
  toggle would leave a retired identity to carry forever.
- Retry inside a pass. The next tick is the retry.

## Acceptance

- A pass probes every eligible SSH executor exactly once in ascending
  `executors.id` order, probes no other executor type and no inactive
  executor, and never has more than `passConcurrency` probes in flight.
- A sequence of non-sticky failures leaves the state `reachable` until the
  probe on which the counter reaches `failureThreshold`, which writes
  `unreachable`; the very next success writes `reachable` with a zero counter
  without a confirming probe.
- A single `config` or `host_key` failure writes `unreachable` on that first
  probe, with the counter still below `failureThreshold`.
- A below-threshold failure stores the new reason and counter while leaving the
  state exactly as it was, so a reason is reported before a state change.
- An interval of `0` runs no scheduled pass; a negative or unparsable interval
  starts the poller at the 60s default rather than disabling it; an interval
  outside 15 to 3600 starts clamped with one log line rather than refusing to
  boot; and `Stop` leaves no goroutine behind.

## Verification

Start with the hysteresis table as a failing test — drive a
success/fail/fail/success sequence and assert the exact probe index on which
each direction flips — and confirm a naive flip-on-first-result implementation
fails it. Then run:

```bash
# From apps/backend:
go test -tags fts5 -race ./internal/executors/reachability/...
go test -tags fts5 -race ./internal/common/config/ -run 'Catalog|SSHReachability'
go test -tags fts5 -race ./internal/backendapp/ -run 'Wiring|Cleanup|Poller'
make lint
```

Use `testing/synctest` for the ticker, matching `healthpoll_test.go`, so no
test sleeps on wall-clock time.

## Files likely touched

- `apps/backend/internal/executors/reachability/poller.go`
- `apps/backend/internal/executors/reachability/store.go`
- `apps/backend/internal/executors/reachability/metrics.go`
- `apps/backend/internal/executors/reachability/poller_test.go`
- `apps/backend/internal/executors/reachability/hysteresis_test.go`
- `apps/backend/internal/executors/reachability/goleak_test.go`
- `apps/backend/internal/common/config/config.go`
- `apps/backend/internal/common/config/catalog.go`
- `apps/backend/internal/common/config/catalog_test.go`
- `apps/backend/internal/backendapp/main.go`

## Dependencies

Tasks 01 and 02. The probe and the store must both exist and be tested before
anything sweeps hosts on a timer.

## Risks

- `auditedStartupEnvironmentInventory()` is deliberately independent of
  `startupCatalog`; updating only one fails
  `TestConfigurationCatalogMatchesAuditedEnvironmentInventory`. This costs a
  cycle rather than shipping a defect, but it is the most likely first failure.
- A `goleak` `TestMain` on a package whose poller holds a semaphore will report
  a leak for a pass abandoned by a cancelled context. `Stop` must drain, not
  just cancel, or the guard turns into a flake in CI before it turns into a
  local failure.
- Dropping an overlapping tick is correct but silent by design. Without the
  skipped-pass counter, an interval shorter than a pass duration is
  indistinguishable from a healthy poller, so the counter is not optional
  instrumentation.

## Parallelism

`sequential`

## Inputs

- System design, sections *The poller*, *State transitions*, *Configuration*,
  *Failure and recovery*, and *Observability*.
- `internal/integrations/healthpoll/healthpoll.go` and its `goleak_test.go`
  for the lifecycle conventions.
- `internal/backendapp/main.go` around the Linear and Sentry poller wiring.
- `internal/common/config/catalog.go` and `catalog_test.go`.

## Results

Pending.
