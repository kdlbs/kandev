---
id: "07-race-and-regression-coverage"
title: "Race, parity, and regression coverage"
status: pending
wave: 5
depends_on:
  - "02-wave-identity-persistence"
  - "03-wire-cascade-producer"
  - "04-wire-engine-routed-producers"
  - "05-wire-orchestrator-producer"
  - "06-backstop-admission"
plan: "plan.md"
requirements:
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-002
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-003
  - REQ-OFFICE-WAKE-WAVE-IDENTITY-004
system_design:
  - ../../specs/office/system-design/parent-wake-wave-identity.md
---

# Task 07: Race, parity, and regression coverage

## Summary

Close the loop with the tests that need every producer and the rewritten
admission query to exist at once: the cascade-vs-reconciler race through
`office/scheduler`'s own queue path, full cross-producer derivation parity,
the coalescing exclusion end-to-end, and the read-skew guarantee.

## In scope

- **Race test:** cascade (P1) racing one `ParentWakeReconciler` tick for the
  same parent and wave, driven through `office/scheduler.QueueRun` (not only
  `runs/service`) — the two classification sites this initiative added.
  Asserts exactly one `runs` row and that the losing side logs no failure.
  Mirrors `TestQueueRun_DedupesOnIdempotencyIndexRace`
  (`runs/service/service_test.go:578`) and its Postgres twin.
- **Cross-producer parity:** for one fixture parent and child set (including
  an ephemeral and an automation-origin child), all four producers derive
  byte-identical `WakeWaveKey`/`WakeWaveString`.
- **Coalescing:** a second wave-carrying request for a *different* parent,
  same target agent, inside the coalescing window, does not merge into the
  first parent's queued (also wave-carrying) run — two rows exist, each with
  its own identity.
- **Read skew:** a wave-member read that observes a non-terminal member
  queues nothing; a later read of the same parent, once genuinely all
  terminal, is delivered normally (not permanently suppressed).
- **Payload parity end-to-end:** the cascade's queued run and an
  equivalent engine-routed run for a step with the same
  `on_children_completed` `queue_run` payload carry the same resolved
  payload keys.
- Confirm the "MUST NOT be weakened" tests
  (`reactivity_children_completed_test.go`,
  `scheduler_wake_reconciler_identity_test.go`) still pass unmodified.

## Out of scope

- Any further production code change — this task is test-only. A finding
  here that requires a production fix routes back to the relevant earlier
  work order, not a change made inline in this one.

## Acceptance

- The race test fails on the pre-Task-03/04 code (verify by temporarily
  reverting to confirm it actually exercises the new constraint, then
  restore) and passes on the finished tree.
- All new tests pass on both SQLite and PostgreSQL where a dialect twin is
  required.

## Verification

```bash
cd apps/backend
go test ./internal/office/scheduler/... ./internal/office/service/... \
  ./internal/office/repository/sqlite/... ./internal/orchestrator/... \
  ./internal/runs/service/... ./internal/runs/repository/sqlite/... \
  -race -count=1
KANDEV_TEST_POSTGRES_DSN=... go test ./internal/runs/service/... \
  ./internal/office/repository/sqlite/... -run Postgres -count=1
```

## Files likely touched

- `internal/office/service/scheduler_wake_reconciler_test.go` (or a new
  shared-harness test file, since the race test needs both a direct
  `office/scheduler.QueueRun` caller and a `ParentWakeReconciler` against
  one database)
- `internal/runs/service/service_test.go`
- `internal/runs/service/service_postgres_test.go`
- A new cross-producer parity test file (exact location depends on which
  package can import all four producers' derivation call sites without a
  cycle — likely a `_test.go` file in `internal/office/service` with build
  tags or fakes for the orchestrator piece, confirmed at implementation
  time)

## Dependencies

Tasks 02 through 06 — this is the closing verification pass.

## Risks

- **Test-only task that could hide a production gap.** If the race test
  cannot actually be made to race deterministically (e.g. both paths commit
  through the same test transaction manager without true concurrency), the
  test would pass without proving anything. Use `synctest` or explicit
  goroutine synchronization (start both inserts, block one at a lock point,
  release both) per the backend's testing conventions — not
  `time.Sleep`-based interleaving.

## Parallelism

`sequential`

## Inputs

- System design: "Testing" section in full.
- Requirements document: "REQUIRED TESTS" section of the task's Kandev
  running plan (this initiative's own card), which enumerates the same
  suite from the review history.
- `internal/runs/service/service_test.go:578` and
  `service_postgres_test.go:27` as the race-test pattern.

## Results

Pending.
