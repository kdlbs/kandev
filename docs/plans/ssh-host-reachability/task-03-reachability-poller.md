---
id: "03-reachability-poller"
title: "Reachability poller, hysteresis, and configuration"
status: done
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
- **The environment and YAML paths do not behave alike, and the clamp needs a
  negative branch.** `applyBoundedIntEnv` reads the environment only and
  returns early when it is unset, so a `config.yaml` value never reaches the
  fallback-and-bounds logic; a non-integer YAML value instead hits
  `decodeConfig`'s ordinary typed-key path and refuses boot, exactly like any
  other catalog key. A negative whole number decodes cleanly from either
  source, so the package's own clamp (not the catalog's) needs a `below 0`
  branch yielding the default `60`, alongside `0` disables, `1`-`14` → `15`,
  and above `3600` → `3600`, logged once at startup.
- **An internal off-cycle-probe dispatch primitive**, registered on the
  package's own `WaitGroup`/context (not a caller's), that runs one probe for
  a single executor outside the ticker and persists its result through the
  same write path a scheduled pass uses. `Stop` cancels and awaits it like any
  other in-flight probe. This is the only piece task 04's immediate-probe
  route and its save-triggered reset consume from this package; neither task
  04 concern reimplements dispatch or persistence.
- `executor_ssh_reachability_probe_discarded_total`, incremented when `Stop`
  cancels a probe and its result is dropped: a cancelled probe has no outcome,
  so `probe_total` cannot represent it.
- `executor_ssh_reachability_write_refused_total`, incremented when the write
  path's own guard (ineligible executor, stale `checked_at`) drops a write
  rather than the probe itself failing: a rising value means configuration
  churn, not a fault.
- `executor_ssh_reachability_reset_total`, incremented by
  `ResetExecutorReachability` succeeding with at least one row affected — task
  04's save observer is the only caller, but the counter lives with the other
  reachability metrics.
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
- Calling the off-cycle-probe primitive: the immediate-probe HTTP route and
  the `ExecutorSaveObserver`/reset trigger both live in task 04, which only
  consumes the primitive this task exposes.
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
- The off-cycle-probe primitive runs a probe for one executor outside the
  ticker, persists through the same write path, and is included in whatever
  `Stop` cancels and awaits — a call issued just before `Stop` either completes
  or is cancelled cleanly, never leaking.
- A YAML `config.yaml` value for the interval key that is not a whole number
  refuses boot like any other catalog key; a negative whole number from either
  YAML or the environment falls back to the 60s default rather than the
  nearest bound.

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
- `apps/backend/internal/executors/reachability/off_cycle_probe.go`
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

Implemented `internal/executors/reachability` (`interval.go`, `store.go`,
`metrics.go`, `probe.go`, `poller.go`, plus `goleak_test.go`,
`interval_test.go`, `store_test.go`, `poller_test.go`), the
`executors.sshReachabilityIntervalSeconds` catalog key, and the
`internal/backendapp` wiring, all under strict TDD (RED confirmed before each
GREEN).

**`interval.go`**: compile-time constants `probeTimeout` (10s),
`passConcurrency` (4), `failureThreshold` (2), `DefaultIntervalSeconds` (60),
`MinIntervalSeconds`/`MaxIntervalSeconds` (15/3600), and `ClampInterval`, which
owns the negative/0/below-min/above-max normalization independent of the
catalog layer. `TestClampInterval` covers all nine cases.

**`store.go`**: the narrow `Repository` interface (the four reachability
methods this package needs, not the full `ExecutorRepository`), and `store`,
whose `Observe` brackets Task 02's `UpsertExecutorReachability` with
before/after `GetExecutorReachability` reads to detect a refused write
(`checked_at` didn't move) and a state transition, without changing Task 02's
already-committed write signature. `buildObservation` derives
`InitialState`/`InitialFailures` for the insert-only (never-before-seen row)
path, mirroring the same "sticky reason always unreachable, transient failure
never promotes below threshold" rule the SQL enforces for an existing row.
`TestStoreObserveHysteresisSequence` drives a real SQLite-backed
success/fail/fail/success sequence and asserts the exact probe index each
direction flips on (a naive flip-on-first-result implementation fails it);
`TestStoreObserveStickyFailureIsUnreachableOnFirstProbe` covers `config` and
`host_key` landing `unreachable` on the very first probe, counter still below
threshold.

**`probe.go`**: `defaultProbe` resolves an executor's SSH target via
`lifecycle.SSHTargetFromExecutorConfig` and probes it with
`lifecycle.ProbeSSHHost(ctx, target, probeTimeout)`; a resolution failure is
reported as `SSHReachabilityReasonConfig` without dialing.

**`poller.go`**: `Poller` with idempotent `Start`/`Stop`, a package-owned
`WaitGroup`/context so `Stop` cancels and awaits every in-flight unit of work
— the scheduled loop, the pass it dispatched, and any off-cycle `ProbeNow`
call — gated through a race-safe `acquire()` that registers new work on the
`WaitGroup` under the same mutex `Stop` locks, so a new registration can never
race a concurrent `wg.Wait()`. An interval of `0` starts the poller (so
`ProbeNow` still works) but never spawns the scheduled loop at all. Each tick
is dispatched via `dispatchPass`, a single-flight gate (`passRunning` +
mutex) that drops and counts (`pass_skipped_total`) an overlapping tick
rather than queuing it — this required dispatching each pass on its own
goroutine/WaitGroup unit rather than blocking the ticker-consuming loop
directly, so a tick firing mid-pass is actually observable rather than being
serialized away by the loop's own single-threadedness. `runPass` lists
eligible executors (already ascending by `executors.id` per Task 02's query),
fans out over a `passConcurrency`-slot semaphore, and abandons dispatching
(without abandoning in-flight probes) on cancellation. `probeAndPersist`
discards a `Cancelled` outcome (`probe_discarded_total`) instead of writing
it, satisfying "`Stop` leaves no failure record behind."

**`metrics.go`**: `expvar` counters/maps for all six named metrics
(`probe_total`, `state_transitions_total`, `pass_skipped_total`,
`write_refused_total`, `reset_total`, `probe_discarded_total`,
`probe_duration_ms`), each paired with a structured zap log where the design
calls for one. `RecordReset` is exported (unused within this package, since
Task 04's save observer is the only caller of `ResetExecutorReachability`) so
the counter lives with the rest of this package's metrics rather than being
declared dead code.

**`poller_test.go`**: `testing/synctest`-based coverage (a fake `probeFunc`
and an in-memory fake `Repository`, no real SSH/network/DB) for: an immediate
pass on `Start`; `Start` idempotency; `Stop` before `Start` as a no-op; an
interval of `0` running no scheduled pass while `ProbeNow` still works;
`ProbeNow` refused after `Stop`; the `passConcurrency` bound (verified exactly
met, not just not-exceeded, with more executors than slots); an overlapping
tick being skipped and counted rather than queued; `Stop` discarding a
cancelled in-flight probe's result entirely; and a listing failure abandoning
the pass without writing. Stable across `-race -count=20`.

**Config catalog** (`catalog.go`, `source.go`, `config.go`,
`catalog_test.go`): added `executors.sshReachabilityIntervalSeconds` /
`KANDEV_EXECUTORS_SSHREACHABILITYINTERVALSECONDS` (default `60`) and the
`ExecutorsConfig` section. Confirmed the environment/YAML split the risk
called out: `applyNonNegativeIntEnv` (the existing generic helper, no new
bespoke function needed) already gives the right ENV-path behavior — absent,
empty, unparsable, or negative falls back to 60; `0` and any other
non-negative integer pass through **unclamped** — because the 15-3600
bound-clamping is deliberately `ClampInterval`'s job alone, applied once in
`Poller.New`. A YAML value decodes straight into the typed int field
(verified a YAML `-5` reaches the config struct unclamped, confirming
`ClampInterval` is the only place that normalizes it) and a non-integer YAML
value refuses boot via the ordinary typed-key decode path, with no extra code
required. Added the matching `auditedStartupEnvironmentInventory()` entry in
the same change as the catalog entry, per the plan's flagged risk.

**`internal/backendapp` wiring**: extracted `startSSHReachabilityPoller`
(`ssh_reachability_wiring.go`) rather than inlining construction in
`startAgentInfrastructure`, mirroring the existing `startTaskUsageWriter`
pattern — an AST-based test (`ssh_reachability_wiring_test.go`) asserts the
composition root calls it exactly once, and a direct test proves it starts
the poller, registers exactly one `Stop` cleanup, and that the poller
actually runs its immediate pass (a fake repository signals on a channel from
inside `ListSSHExecutorsForReachability`). Wired alongside the Jira/Linear/
Sentry/WorkflowSync/Office-config-sync poller block using `repos.Task` (which
structurally satisfies the package's narrow `Repository` interface with no
adapter) and `cfg.Executors.SSHReachabilityIntervalSeconds`.

Verification, all green:

```bash
go test -tags fts5 -race ./internal/executors/reachability/...
go test -tags fts5 -race ./internal/common/config/ -run 'Catalog|SSHReachability'
go test -tags fts5 -race ./internal/backendapp/ -run 'Wiring|Cleanup|Poller'
make lint
```

Also ran `go build ./...` and `go vet ./...` across the whole repo (clean)
after the config/backendapp changes.
