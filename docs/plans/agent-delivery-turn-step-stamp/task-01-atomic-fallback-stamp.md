---
id: "01-atomic-fallback-stamp"
title: "Stamp canonical fallback turns in the projection transaction"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001
  - REQ-TASKS-TRANSITION-LEDGER-001
  - REQ-TASKS-TRANSITION-LEDGER-SCENARIOS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001.4
  - AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001.5
  - AC-TASKS-TRANSITION-LEDGER-001.1
  - AC-TASKS-TRANSITION-LEDGER-SCENARIOS-001.1
system_design:
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
  - ../../specs/tasks/system-design/workflow-task-step-transition-ledger.md
---

# Task 01: stamp canonical fallback turns atomically

## Summary

New agent-woken canonical turns carry their task's locked step and entry in the
same transaction as output and projection progress. Contention releases and
retries the projection; reuse and replay retain their original identities.

## In scope

- First install `TestCanonicalDeliveryTurnWorkflowStamp` with the portable
  recipe in the plan; run it red for the absent step stamp, then add entry checks.
- Reuse `stampTurnWorkflowEntryTx` in the new fallback branch, encoding metadata
  before INSERT. Preserve conflict-ignore, timestamps, profile/routing defaults
  and existing requested/open branch behavior. No second transaction.
- Add a narrowly scoped PostgreSQL NOWAIT read mode using the established task
  row lock; retain ordinary `readTaskStepInTx` callers. Retry only SQLSTATE
  `55P03`, after rolling back the complete canonical projection transaction.
  Delay 10 ms exponentially capped at 250 ms outside SQL resources; caller
  cancellation bounds the operation and releases retry promptly.
- Prove singleton/batch text, reasoning, pending/running foreground shell starts,
  and negative terminal/background/update branches through their public projection
  entry points. Prove no-step omission, missing task/read/ledger/insert/commit
  failures, deterministic ID conflicts, prior closed turns, projected replay,
  and a normal prompt opening a turn while projection retries.
- Assert failure leaves turn/message/effect/projected markers/cursor untouched;
  received inbox survives and exact retry commits once. Preserve metadata on
  requested/open turns, including stamped stale turns and old unstamped turns.
- Real PostgreSQL independent connections: task mover commits and rolls back;
  concurrent projectors, task-first workflow receipt writer, cancellation during
  contention, and step revisit. Observe server lock/contender state with the
  existing `openSecondPostgresConnection`/`waitForPostgresLock` patterns. A
  single pooled connection is insufficient proof.
- Add repository-backed lifecycle retry/ACK evidence. If runtime transient retry
  needs adjustment, keep it limited to this contention path and preserve owner,
  queue, terminal, and shutdown boundaries.

## Out of scope

Entry-aware handler/consumer work (Task 02), global lock ordering, historical
backfill, ordinary creation's telemetry degradation, CLI behavior, UI and schema.

## Acceptance

1. `TestCanonicalDeliveryTurnWorkflowStamp` fails for missing metadata before
   the correction, then all new fallback variants commit the exact locked step
   and entry. No-step, requested/open, and duplicate cases preserve their contracts.
2. Rollback/retry preserves inbox and exactly-once projection; real PostgreSQL
   contention cannot create a mixed stamp, wait cycle or leaked retry resource.
3. Canonical/lifecycle integration emits no notification or ACK before commit
   and converges after contention without an additional user prompt.

## Verification

Run from the repository root. PostgreSQL must point at a disposable real server;
`OpenIsolatedPostgres` isolates each test schema. Do not record an environment
skip as a pass. Capture full command output to retain individual test statuses.

```bash
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run 'TestCanonicalDeliveryTurn|TestCanonicalAgentDeliveryProjection|TestInbox|TestIncrementalShell|TestCreateTurnWithStepStamp|TestTurnEntry' -count=1)
: "${KANDEV_TEST_POSTGRES_DSN:?Set a disposable real PostgreSQL DSN before acceptance}"
(cd apps/backend && go test -race -v ./internal/task/repository/sqlite -run 'TestPostgresCanonicalDeliveryTurn|TestPostgresCreateTurnWithStepStamp|TestPostgresTurnEntry|TestIncrementalShellPostgres' -count=1)
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle -run 'TestDurableDeliveryFallbackProjectionRetry|Test.*Canonical.*Projection|Test.*Projection.*Fail|Test.*Delivery.*Ack' -count=1)
(cd apps/backend && go test -race ./internal/task/service -run 'Test.*StepStamp|Test.*Turn.*Stamp' -count=1)
git diff --check
```

Run `bash scripts/lint-go-changed` with every changed Go path explicitly from
repo root after implementation. Record the exact expanded list/result. Run
`gofmt -l` for those files; require no output. Do not use a full backend lint.

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/agent_delivery_projection.go`
- `apps/backend/internal/task/repository/sqlite/step_transitions.go`
- New focused `agent_delivery_turn_step_stamp_test.go` and
  `agent_delivery_turn_step_stamp_postgres_test.go` beside those files.
- Existing `agent_delivery_test.go`, `agent_delivery_shell_projection_test.go`,
  `agent_delivery_shell_postgres_test.go` only where shared fixture coverage needs it.
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_fallback_test.go`
  and narrowly related `streams.go`/stream-processor code only if proven necessary.
- `apps/backend/internal/db/` or repository error classification only if needed
  to share existing dialect error classification; do not match error prose.

## Dependencies

None. Read plan and referenced designs before editing. Preserve unrelated edits.
The fresh diagnostic uses Go overlays, not an existing permanent regression.

## Risks

A blocking FOR UPDATE under delivery locks creates an inverse edge with task-first
workflow writes. Do not silently replace the documented nonblocking design with
that implementation. Do not retry SQL failures using an already aborted PostgreSQL
transaction. Do not obtain a second transaction from SQLite's single writer.
If a new material persistence/locking design is needed, update the design package
before coding it. Do not weaken atomicity to satisfy a test.

## Parallelism

`sequential`

## Inputs

- Completion design: Durable output-turn attribution; ledger design: Canonical
  output turns; ADR-2026-10-22-entry-bound-delivery-turns.
- `turn_step_stamp_test.go`, `turn_step_entry_stamp_test.go`,
  `turn_step_stamp_postgres_test.go`, canonical and incremental-shell tests.
- `agent_delivery_event_locks.go`, `agent_delivery_workflow_outcome.go`,
  `agent_delivery_reconstruction.go`, and normal `session.go` lock order.
- Backend AGENTS and `/tdd` including `references/backend-tests.md`.

## Results

Implementation uses the existing stamp helper in the projection transaction and
retries PostgreSQL task-row NOWAIT contention after full rollback. Existing turn
reuse and deterministic conflicts preserve their metadata. No runtime retry or
ACK policy change was necessary.

Red/green: the permanent portable stamp regression failed on the absent stamp;
PostgreSQL contention initially timed out instead of returning SQLSTATE 55P03.
Both pass after the correction. Rollback cases cover turn/message/ledger and
deferred commit failures; exact retry preserves inbox and creates one projection.
Lifecycle integration proves no callback/ACK on failure and one after retry.

Validation from `apps/backend`, on implementation base
`3dd5a524b2d8be2454285a067486e186df37da6f`:

- Required SQLite race command above: PASS, 111.179 s. Additional final focused
  repository race run includes shell eligibility and commit-failure cases.
- Required PostgreSQL race selector above plus `TestPostgresStepCompletionEntry`
  and `TestCanonicalDeliveryTurnRollbackAndRetry`: PASS, 19.949 s. Real
  PostgreSQL 17 Alpine at localhost:55475, disposable database `kandev_test`,
  isolated schema per test; no PostgreSQL environment skips. Final new PostgreSQL
  cases (`TestPostgresCanonicalDeliveryTurn|TestPostgresStepCompletionEntry`):
  PASS, 10.981 s. Independent connections cover task/session inversion,
  committed step move, no-step replay, cancellation, and stale claim after revisit.
- Required lifecycle race command above: PASS, 7.091 s.
- Required task-service race command above: PASS, 2.260 s.
- Formatting, scoped lint, whitespace and shared docs results are recorded in
  Task 02. CI and review evidence belong to the PR delivery record.

No live CLI self-wake was exercised. The permanent backend receipt/projection,
handler, lifecycle and advancement fixtures provide portable integration proof.
