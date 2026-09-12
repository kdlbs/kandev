---
id: "07-race-and-regression-coverage"
title: "Race, parity, and regression coverage"
status: done
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

Done, in `test(office): close race, parity, and regression coverage for
wave identity`.

Most of this task's Acceptance items were already covered by Tasks 02-05's
own tests, confirmed unmodified in this pass:

- **Coalescing:** `TestQueueRunCtx_WaveCarryingRequest_NotCoalesced`
  (`office/scheduler/run_test.go`, Task 03) and
  `TestQueueRun_WakeWaveKey_DifferentAgentBothQueued`
  (`runs/service/service_test.go`, Task 02).
- **Read skew:** `TestResolveWaveIdentity_NonTerminalMember_ReturnsNotOK`
  (`office/scheduler/reactivity_children_completed_wave_test.go`, Task 03).
- **Payload parity:** `TestCascadeChildrenCompleted_PayloadParity_*`
  (same file, Task 03).
- **Per-producer derivation parity:**
  `TestQueueRunCallback_OnChildrenCompletedCopiesWaveIdentity` (Task 04),
  `TestChildrenCompletedWaveIdentity_IdenticalAcrossEdgeAndReconcilerPaths`
  (Task 04, P2 vs P3), `TestChildCompletionPayload_MatchesCascadeDerivationForSameParentAndSet`
  (Task 05, P4 vs `waveidentity` directly).
- **"MUST NOT be weakened" regression suites:**
  `reactivity_children_completed_test.go` (5 pre-existing tests) and
  `scheduler_wake_reconciler_identity_test.go`
  (`TestParentWakeReconciler_*`, 7 tests) both pass unmodified.

This task added the two genuinely new closing pieces:

- `internal/office/scheduler/wave_identity_race_test.go`:
  `TestWaveIdentityRace_CascadeVsEngineRoutedPath_CollapsesToOneRun` races
  P1's direct-insert path (`SchedulerService.QueueRunCtx` -> `queueRun` ->
  `ss.repo.CreateRun`, classified by
  `runssqlite.IsWakeWaveUniqueViolation`) against an engine-routed path
  (`runsservice.Service.QueueRun` -> `insertRun` -> `CreateRunTx`,
  classified by Task 02's `errWakeWaveKeyConflict` handling) for the same
  wave identity and target agent, through two goroutines started together
  via a shared barrier channel (not a sequential call pair) — both
  `office/scheduler.SchedulerService` and `runsservice.Service` are wired
  against the exact same in-memory SQLite DB via
  `officeRepo.RunsRepository()`, so the two classification sites this
  initiative added contend on the real `idx_run_wake_wave` index.
  Confirmed the test has teeth: temporarily gave the engine-routed request
  a different `WakeWaveKey` and watched the assertion fail (`runs = 2, want
  exactly 1`), then reverted before committing. Passes reliably across
  repeated `-race -count=10` runs.
- `internal/task/repository/sqlite/wave_member_parity_test.go`:
  `TestListWaveMembersMatchesListChildCompletionRows` builds one shared
  `*sqlx.DB`, initializes both `task/repository/sqlite.NewWithDB` (owns the
  `tasks` table in production) and `office/repository/sqlite.NewWithDB`
  against it, seeds one parent with ordinary/ephemeral/automation-origin/
  archived children, and asserts `ListWaveMembers` (P1/P2/P3's read) and
  `ListChildCompletionRows` (P4's read, id-sorted the way
  `childCompletionPayload` sorts it) return the identical wave-member id
  set — the AC-...-001.8 cross-producer predicate-equivalence proof Task
  01's plan text called for but that file's own tests didn't yet include.

`go build ./...` clean.
`go test ./internal/office/scheduler/... ./internal/office/service/...
./internal/office/repository/sqlite/... ./internal/orchestrator/...
./internal/runs/service/... ./internal/runs/repository/sqlite/...
./internal/task/repository/sqlite/... -race -count=1` — all packages pass.
`golangci-lint run ./internal/office/scheduler/...
./internal/task/repository/sqlite/...
--new-from-rev=cd78236315f28982848de4938d56f7722c7f632f` clean. The
Postgres-gated command in Verification was not run — no
`KANDEV_TEST_POSTGRES_DSN` provisioned in this environment, consistent
with the card's own instruction not to stand up Docker/Postgres here;
every dialect-sensitive change (Task 06's `OrderedIDConcat`) already has
its own gated Postgres twin from that task.
