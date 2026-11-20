---
created: 2026-10-22
status: done
requirements:
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-001
  - REQ-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-003
  - REQ-TASKS-TRANSITION-LEDGER-001
  - REQ-TASKS-TRANSITION-LEDGER-SCENARIOS-001
system_design:
  - ../../specs/tasks/system-design/workflow-explicit-completion-signal.md
  - ../../specs/tasks/system-design/workflow-task-step-transition-ledger.md
legacy_specs: []
---

# Fix plan: workflow attribution for agent-woken delivery turns

## Overview

Fix [issue #4705](https://github.com/kdlbs/kandev/issues/4705) by stamping new
canonical output turns atomically and enforcing the workflow-entry identity
completion already needs. Deliver the repository correction first, then the
entry-aware signal/transition slice. These work orders are sequential and do
not authorize delegation.

The tasks system owns the durable turn and completion eligibility. Platform
owns transport replay; its delivery requirements remain dependencies, not
copied task-system requirements. Implementation and validation results are recorded below and in the work orders.

## Evidence and root cause

Investigation base and verified remote main on 2026-10-22:
`10f5852d8362218767b7471dfa2543ae156c2bd3`.
Implementation base (fast-forwarded before code changes):
`3dd5a524b2d8be2454285a067486e186df37da6f`, verified against remote main
on 2026-10-23. The isolated branch is `feature/fix-completion-signa-8rd`.
Task `c22024db-9a18-44b6-93fc-768608a991a4` is already a top-level task based on
`main`, with no parent. Its agent profile is
`1b54de21-c63b-497c-a0a0-6cc78791b5da` and executor profile is
`2e1be5fd-398d-4e3e-92b7-314a3d77595d`; preserve both. Current session:
`0b90294b-0acf-4b38-abf8-273e8dbbcb15`. Do not create a duplicate task.

`canonicalDeliveryTurnTx` in `agent_delivery_projection.go` returns a requested
turn, reuses an open turn, or inserts a deterministic fallback with `{}`.
`persistCanonicalAgentMessageTx` reaches it for text/thinking, and
`canonicalShellStartTurnTx` reaches it for new foreground shell tools. Other
shell terminal/status/update paths do not create a new fallback. A production
search found only two turn INSERT sites: this one and `session.go:insertTurnRow`.
Ordinary service and completed/lifecycle turn paths use existing stamped creation;
this repair does not replace those APIs.

`stepCompletionLaunchStep` reads only the latest turn and rejects missing step
stamps. That rejection is correct; the fallback producer violates the existing
ledger stamp contract (ledger AC-001.1 and scenarios AC-001.1). The active
completion requirements gain precise self-wake/error criteria 001.4-001.5.

A second coverage gap matters to the requested stale-entry acceptance: the
handler/claim currently compare only step IDs, despite turns storing an entry
ID. An A -> B -> A revisit accepts the earlier turn. Criteria 003.6-003.7 make
this boundary explicit, rather than claiming an existing entry guard works.

### Fresh temporary diagnostics

Both diagnostics used fresh real SQLite repositories through Go overlays; no
production or permanent test files were changed.

1. `TestIssue4705DiagnosticFallbackStamp`: FAIL, 0.274 s package time. Exact
   failure: `fallback turn agent-delivery-turn-audit-stream-1 workflow step stamp = "", want audit-step`.
2. `TestIssue4705DiagnosticStaleEntry`: FAIL, 0.248 s package time. The old turn
   returned `accepted:true`, `step_id:"step-a"`, and published one event after
   the task moved A -> B -> A. Expected rejection.

The first command initially failed during setup because the sandbox could not
write the shared Go build cache. A permitted cache-access rerun produced the
actual assertion failure. These are repository/handler proofs; no live CLI
background-wake or real PostgreSQL reproduction has been performed in this turn.

Portable fallback reproduction, in package `sqlite` (the implementation work
order installs permanent tests):

```go
repo := newTurnStepStampTestRepo(t)
seedTurnStepStampSession(t, repo, "audit-task", "audit-step", "audit-session")
tx, err := repo.db.BeginTxx(t.Context(), nil) // assert no error
// Defer rollback immediately.
id, err := repo.canonicalDeliveryTurnTx(t.Context(), tx,
    "audit-session", "audit-task", "",
    &models.AgentDeliveryEvent{StreamID: "audit-stream", Sequence: 1})
// Assert no error, commit, then GetTurn(id).
// Assert workflow_step_id_at_start == "audit-step" and entry stamp equals
// GetTaskWorkflowStepEntry's accepted entry ID.
```

Portable stale-entry reproduction: use `newTestTaskService`,
`seedStepCompleteTarget` and `createStepCompleteRecoveryTurn` in package
`handlers`; move the task with `repo.UpdateTask` from A to B and back to A;
call the real `handleStepComplete`, and assert validation, no bag, no bus event.
The audit's original `/tmp` artifacts are supplementary only.

## Scope

### In scope

- New text/thinking/shell fallback turns, explicit/open reuse, batch projection,
  deterministic-ID conflict, and replay after commit/rollback.
- Atomic step/entry reads and no-step behavior on SQLite and PostgreSQL.
- Safe PostgreSQL task-lock contention retry with cancellation and no lock cycle.
- Entry-aware claims, duplicates, delayed consumption and final move CAS.
- Current-step success, stale step and A -> B -> A rejection, legacy evidence,
  pending questions, and delivery exactly-once integration.
- Public recovery documentation with implementation; exact recorded checks and PR.

### Out of scope

- Backfilling or re-stamping existing turns/signals; searching earlier turns.
- Global lock-order refactoring, agent CLI/ACP bridge changes, new flags or schema.
- New UI, screenshots, a manual completion fallback button, or workflow policy changes.
- Changing ordinary telemetry-only creation's best-effort policy.
- General late-user-reply completion races listed in ADR 0015, beyond the
  step/entry fence required here; no automatic restart or release.

## Technical approach

Follow the existing completion design sections
[Durable output-turn attribution](../../specs/tasks/system-design/workflow-explicit-completion-signal.md#durable-output-turn-attribution)
and [Entry-bound completion](../../specs/tasks/system-design/workflow-explicit-completion-signal.md#entry-bound-completion).
The [decision](../../decisions/2026-10-22-entry-bound-delivery-turns.md) records
why canonical errors retry and historical evidence is never invented.

Keep the fallback INSERT's deterministic identity and conflict-ignore behavior.
Reuse `stampTurnWorkflowEntryTx` inside its caller's SQL transaction. Do not call
a public creation API that starts a second transaction. PostgreSQL uses the
existing task row lock in a nonblocking mode to avoid a task-after-session wait
cycle; retry the complete projection outside all locks with a fresh snapshot.
Ordinary helper callers keep their blocking behavior.

The second work order stores an optional entry in the existing signal JSON,
uses a task-row-locked claim, compares entry on duplicates/consumption, and
threads `WithWorkflowStepTransitionGuard` into the existing move CAS. It must
cover engine, legacy, subscriber, watchdog, carry/audit and conditional cleanup.
No independent current-step lookup can grant a missing turn stamp.

### Compatibility and evidence

| Path | Behavior | Evidence |
| --- | --- | --- |
| Canonical text and reasoning, singleton/batch | Stamp only a new fallback | Repository + real PostgreSQL tests |
| Foreground shell pending/running start | Same stamped creator | Shell projection tests |
| Requested or open turn | Preserve original ID/metadata | Reuse tests, including stale/unstamped rows |
| Terminal shell status/background tool/update | Preserve existing selection/omission/error rules | Existing shell tests + explicit negative cases |
| Committed event replay or deterministic-ID conflict | No extra turn/message/effect; no re-stamp | Replay tests before/after task move |
| No-step task | Omit both metadata keys | SQLite/PostgreSQL |
| Legacy turn/signal without entry | Readable; never authorize a known entry; fresh eligible signal recovers | Handler + signal decoder/claim tests |
| Legacy task without ledger entry | Explicit zero-entry compatibility; no synthesized row | Repository + handler tests |
| Noncanonical/positively legacy transport | Existing turn creation and degradation | Existing service/stream tests |

These are provider-neutral canonical paths. Claude's reported background wake
motivates the case; no claim is made that every CLI can self-wake. A scripted
canonical-stream fixture proves the backend behavior independently of CLI support.

## Tests

All short completion suffixes below refer to
`AC-TASKS-WORKFLOW-EXPLICIT-COMPLETION-SIGNAL-*`.

| Acceptance | Planned permanent evidence |
| --- | --- |
| 001.4; ledger/scenarios 001.1 | `agent_delivery_turn_step_stamp_test.go:TestCanonicalDeliveryTurnWorkflowStamp`, text/thinking/shell matrix |
| 001.4 replay/reuse | Same file: `TestCanonicalDeliveryTurnReuseAndReplay`, including closed deterministic-ID conflict, batches and task moves |
| 001.5 errors | Same file: `TestCanonicalDeliveryTurnRollbackAndRetry`, cursor/effect/message/turn atomicity |
| 001.4-001.5 concurrency | `agent_delivery_turn_step_stamp_postgres_test.go:TestPostgresCanonicalDeliveryTurnWorkflowStamp`, multi-connection contention/cancellation and task-first receipt cycle |
| 003.1-003.3, 003.6 | `step_complete_delivery_turn_test.go:TestHandleStepComplete_DeliveryFallbackTurn`, plus existing recovery tests |
| 003.6-003.7 | `step_completion_entry_test.go`, `step_completion_entry_postgres_test.go`, decoder tests and same-step claim races |
| 003.7 transition | `agent_delivery_step_completion_test.go:TestDeliveryFallbackStepCompletionAdvancesOnce`, engine/legacy/waiting subscriber and delayed signal/cleanup cases |
| 001.5 real delivery | `durable_delivery_fallback_test.go:TestDurableDeliveryFallbackProjectionRetry`, real repository-backed callback/ACK assertions |

Tests must assert state and effects, not only response text or a stamp being
one of two values. Task moves must use transactional production writers.

## End-to-end evidence

The surface is backend MCP/workflow behavior. The package must join real
canonical receipt/projection to the completion handler, persisted pending signal,
and configured signal-gated workflow transition. Use a deterministic in-process
fixture or local MCP transport; drive the real paths, not a manually stamped turn.
Cover a completed predecessor, self-woken output, final completion, exactly one
transition and duplicate replay; cover same-step revisit and pending clarification.
The lifecycle integration asserts no callback/ACK on failed projection and one
after retry without another prompt. No browser layout changes require Playwright.
A manual live Claude background-command wake is supplementary if available, not
a substitute for the permanent portable backend reproduction.

## Work orders

- [x] [Task 01: stamp canonical fallback turns atomically](task-01-atomic-fallback-stamp.md)
- [x] [Task 02: fence completion by entry and prove advancement](task-02-entry-bound-completion.md)

## Verification results

Implementation complete on the isolated branch based on current main
`3dd5a524b2d8be2454285a067486e186df37da6f` (remote rechecked before delivery).
Both work orders contain exact commands and results. Permanent red/green
regressions, SQLite and real PostgreSQL 17 race suites, MCP handler/server,
lifecycle ACK/retry, task-service stamping and full orchestrator race tests pass.
Full orchestrator: 264.085 s. Final PostgreSQL claims, stamps and move-CAS tests:
13.629 s, no environment skips. Scoped Go lint: 0 issues. Formatting,
specification/public-doc validation and deterministic docs coverage pass.

The original design investigation and aborted diagnostic run are preserved as
historical evidence; neither is counted as a product-check pass. Backend
integration fixtures overlap at production projection/claim boundaries; no live
agent CLI wake or browser reproduction is claimed. Commit, exact-head CI/review
and authorized merge are tracked in the PR delivery artifacts.

## Delivery

After a later explicit implementation request, execute TDD in order, synchronize
work-order statuses, run every listed command, and record results. Re-read
current main/dependency heads before implementation and final delivery. Preserve
unrelated existing companion package results; this package adds coverage to the
completed stale-turn-recovery and delivery projects without claiming to rerun
or reopen their historical work orders.

Once task-defined checks pass, use `/commit`, `/push`, `/pr` with the repository
PR template and docs-coverage gate. Reference `Fixes #4705`, exact validation and
legacy evidence limitations. Address actionable CI/reviewer feedback with
`/pr-fixup`. The user subsequently authorized implementation, PR remediation, and merge
without delegation. Merge only after exact-head CI and review gates pass. No
release was authorized.
The original design package was handed off uncommitted; the later implementation
request authorizes committing the complete tested change.

## Risks

- Task/session lock order: adding a blocking task read under delivery locks can
  deadlock. Require a real task-first contender and prompt cancellation test.
- Retry must release all SQL resources before delay; otherwise no connection is
  left to commit the move that would unblock it.
- Numeric JSON decoding and absent-vs-zero entry identity must remain exact.
- Historical unstamped turns and unproven signals need fresh-turn recovery.
- Entry fencing must survive delayed consumption and the final move boundary,
  not stop at an initial handler snapshot.
- PostgreSQL availability is mandatory for implementation acceptance; it was
  not exercised by the design diagnostics.

## Design-package validation (2026-10-22)

- `python3 scripts/list-docs.py validate`: PASS, validated 398 decisions and
  1,716 specifications.
- `python3 scripts/lint-spec-files.test.py`: PASS, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: PASS, all specification files.
- `git diff --check -- docs/specs docs/plans/agent-delivery-turn-step-stamp docs/decisions`:
  PASS, no whitespace errors. `git status --short` confirmed four modified specs
  and the new ADR/plan/two work orders; no source or permanent tests changed.
- Documentation coverage preflight: PASS via exported
  `.github/scripts/pr-docs.cjs:validateCoverage` on the actual eight changed
  documentation files and their referenced requirement/design contents. Actual
  design-only diff is `exempt`. A prospective diff adding the five named primary
  source files is `covered`, with both work orders accepted and no errors. This
  checks traceability, not unimplemented source or a live PR.
- Preflight runtime: `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`
  executing stdin. `node` was initially absent from PATH. A sandbox restriction
  affected Node child-process spawning; using direct `git status --porcelain
  --untracked-files=all -- docs` output as input allowed the evaluator to run
  without a child process. No application dependencies or installation changed.
- Both Go overlay diagnostics exited 1 with the expected assertions, as recorded
  above. Their exact runs were `go test -overlay /tmp/kandev-4705-overlay.json
  ./internal/task/repository/sqlite -run '^TestIssue4705DiagnosticFallbackStamp$'
  -count=1` and `go test -overlay /tmp/kandev-4705-entry-overlay.json
  ./internal/mcp/handlers -run '^TestIssue4705DiagnosticStaleEntry$' -count=1`,
  each from `apps/backend` with existing Go cache access. Portable fixture recipes
  above keep the evidence independent of those temporary overlay paths.
- At the original design handoff, implementation and delivery were pending.
  The later implementation results above and in both work orders supersede
  that checkpoint without changing the historical diagnostic evidence.

## Review remediation validation

The initial published head passed 84 checks (19 skipped, one neutral), with no
failed or pending checks. One inline handoff finding and one aggregate legacy
lifecycle finding required remediation. Permanent failing regressions preceded
the fixes; final focused orchestrator race checks pass (3.137 s), as do the
SQLite/real PostgreSQL handoff rebase and CAS race checks (16.011 s). Task 02
records exact commands and source-ownership semantics. Initial CI is historical;
final head and advanced-base delivery checks remain externally pending until
recorded on the PR/task.
