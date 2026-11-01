---
created: 2026-11-01
status: complete
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
  - ../../specs/platform/system-design/durable-agent-reattachment.md
legacy_specs: []
---

# Implementation plan: Finish recovery after definitive remote absence

## Overview

Correct the indefinite restart guard reported in [issue #4989](https://github.com/kdlbs/kandev/issues/4989).
When an executor proves that the recorded agent is absent, finish pending
attachment. Let existing recovery paths assess the saved evidence.
Retain the conversation and the interrupted prompt's uncertain outcome.

Platform owns this correction because it owns shared recovery admission and
durable execution consistency. Executor adapters retain their liveness proof.
Tasks 01 and 02 are complete. Task 02 proves public recovery admission after
the exact remote loss and preserves the supported recovery limits.

## Evidence and root cause

Investigation baseline: `293ceed7dceb2ee7fc7ce4d5dc0613824f376611`, also the
remote `main` head on 2026-11-01. The issue reports v0.98.0. It contains no
attachments or comments, and provides no process identity or diagnostic bundle.
The authenticated GitHub user is `carlosflorencio`. Assignment is confirmed.

The confirmed source failure has this path:

1. `PrepareStartupRecovery` or `establishRemoteRecoveryCandidates` queues the
   recorded remote owner. `RetainAsRemotePending` retains its retryable guard.
2. `RecoveryGuard.ReleaseAllExceptRetained` releases ordinary held guards.
   A retained remote guard survives the startup deadline.
3. `attachRemoteRecovery` calls `RecoverInstancesDetailed` but discards its
   outcome map. When no instance returns, it leaves the pending entry intact.
4. A later status tick repeats the same operation indefinitely. Synchronous
   `Manager.Start` also records detailed no-match outcomes without settling
   their pending remote entries.
5. `Manager.Launch` returns `ErrSessionRecoveryGuarded` before allocation.
   `wrapSessionRecoveryGuardError` exposes `session_recovery_in_progress`.

A temporary test, `TestRepro4989ConfirmedMissingRemoteOwnerFinishesReconciliation`,
queued one exact owner and drove the real retry path three times. Each attempt
returned `RecoveryOutcomeNoMatchingInstance`. Remote Docker and Sprites both
still returned `ErrSessionRecoveryGuarded`. Both subtests failed at that assertion.
No Stop or lifecycle event occurred. The diagnostic test was removed before
handoff. Task 01 creates the permanent regression.

Three existing controls passed. Unknown remote ownership remains guarded.
Pending ownership requires the exact current execution. Ordinary startup
guards expire at their deadline. No live instance or database was inspected
or changed.
This proves a current source defect consistent with the issue. It does not
identify the reporter's executor or prove that every unreachable agent is dead.

## Requirement conformance and assumptions

Reuse the active restart requirement's AC-001.1, AC-001.2, AC-001.3, and AC-001.5.
The intended behavior already covers dead execution records, preserved
uncertainty, independent ownership, and repeated reconciliation.
No new requirement or ADR is necessary. The existing delivery requirement is
draft. This package references its implemented recovery contract without
promoting its status or rewriting unrelated acceptance criteria.

Current [session documentation](../../public/sessions-and-review.md) describes
automatic restoration without prompt dispatch when all evidence permits it.
The issue's suggestion of automatic instruction continuation is separate.
This package preserves automatic restore-only behavior and existing manual
actions. Completing a startup guard does not guarantee that Resume or saved
context continuation is safe or successful.

## Scope

### In scope

- Consume definitive absence outcomes at synchronous startup and deferred or
  periodic attachment through one lifecycle disposition.
- Repair the observed runtime row in place before guard release.
- Retain current-owner, Stop, archive, cleanup, and shutdown fences.
- Preserve every independent delivery block and historical outcome. Keep queue
  claims owned during absence settlement; after explicit Resume, verify the
  same queue item returns to pending storage without dispatch.
- Verify the public recovery path and existing desktop and phone controls.

### Out of scope

- Releasing guards solely because a timer expires or a host is unreachable.
- Extending provider death proof to SSH, Kubernetes, or plugin runtimes.
- Reclassifying connection refusal as process death, including a dead control
  endpoint inside an otherwise live container.
- Replaying uncertain prompts, automatic history continuation, or new
  unattended-agent policy, settings, APIs, feature flags, or database schemas.
- Replacing native conversations, deleting saved rows, or mutating live sessions.
- A separate standalone-process defect: its ordinary guard deadline already
  passes the focused control test. A new reproduction needs its own diagnosis.

## Technical approach

### Lifecycle disposition

Add a narrow helper in
`apps/backend/internal/agent/runtime/lifecycle/remote_recovery_disposition.go`.
Call it from `attachRemoteRecovery` and the detailed-outcome consumer in
`Manager.Start`. Use the existing `remoteRecoveryNoMatchProvesStopped` predicate.
Do not infer absence from an empty slice or apply every adapter's no-match
result as equivalent proof.

Read the executor row used for the provider attempt and retain its nonzero
`UpdatedAt`. Re-read the owner snapshot and runtime row before settlement.
Reject changed execution, task, session, runtime, environment generation, route,
or recovery bindings. The final pending-entry check must exclude a tracked
execution, accepted Stop, shutdown, and a replacement pending entry.

Use the existing `executorRunningCASWriter.RepairExecutorRunningDeadIfCurrent`
with execution ID and timestamp. Keep the runtime row, resume token, workspace,
metadata, native identity, and saved submission. Do not call
`repairExecutorRunningAfterCleanup`: its boolean deliberately hides errors.
Missing readers or CAS support and failed writes retain the pending guard.

After durable repair, complete only the exact pending entry. Keep I/O outside
`remoteRecoveryMu`. Preserve `beginRemoteRecovery`/`finishRemoteRecoveryAttempt`
ownership and Stop waiters. A crash after repair must converge on the next
pass or restart without reintroducing a live-runtime claim.
No new durable absence marker or repository mutation method is required.

### Recovery admission

Keep startup attachment and durable prompt recovery separate. Guard completion
permits `session.recover` to reach its existing action-specific checks.
Missing journal or native evidence can still yield a typed blocked or
unavailable result. Source-process proof for native restoration remains
unchanged. Stop is an explicit operation with its own cleanup contract.

Do not publish ready, runtime availability, prompt completion, or workflow
events from absence settlement. Guard completion alone must not resolve
recovery blocks or release `unknown_queue_dispatch_outcome` claims. The explicit
Resume regression checks that its queue item remains pending without dispatch.
Each unrelated live, unknown, Office, automation, archived, or cleanup-owned
candidate retains its own admission.

### Compatibility matrix

| Executor / evidence | Disposition | Verification and fallback |
| --- | --- | --- |
| Remote Docker: exact provider container absent or stopped, or authenticated exact instance missing or harness stopped | Repair row and finish attachment guard | Adapter tests plus lifecycle no-match and real-container restart case. Native restore still needs its separate proof |
| Sprites: authenticated saved Sprite is absent | Same disposition | HTTP-backed adapter test and lifecycle no-match test. Unavailable retained native state remains blocked |
| Remote Docker or Sprites: timeout, authentication error, missing metadata, omitted outcome, or unavailable control endpoint | Keep pending owner | Negative lifecycle matrix. No destructive cleanup or timer-based release |
| SSH, Kubernetes, plugin remote: no matching instance | Keep current conservative behavior | Negative matrix. The current shared no-match predicate does not establish sufficient process proof |
| Surviving authenticated remote instance | Existing adoption and replay | Existing remote replay regressions. No reinitialization or second harness |
| Local/worktree standalone owner | Existing deadline and exact local process proof | Existing guard and source-liveness tests. No remote classification |
| Older adapter with only `RecoverInstances` | Keep unknown when no instance returns | Legacy-adapter regression. No inferred absence |

### Documentation and companion packages

The existing [restart design](../../specs/platform/system-design/durable-restart-reconciliation.md)
now specifies definitive remote absence. The
[reattachment design](../../specs/platform/system-design/durable-agent-reattachment.md)
continues to own unknown remote protection and native restore.

Related packages are `durable-agent-sessions`,
`durable-agent-session-reconciliation`, `durable-agent-reattachment`,
`agentctl-journal-shutdown-recovery`, `journal-growth-restart-reconciliation`,
and `session-recovery-convergence`. Their completed results remain historical.
This package owns the new missing-owner disposition and its tests. Companion
packages retain ownership of their outstanding delivery work.
No linked old work order changes scope or acceptance because of this repair.

Implementation added concise recovery guidance to
`docs/public/sessions-and-review.md`: confirmed absence ends attachment.
Unreachable ownership remains protected. Lost state can still block recovery.
The page is primarily an explanation with recovery procedures.

## Tests

| Acceptance | Permanent test evidence planned |
| --- | --- |
| AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1 | `remote_recovery_disposition_test.go`: `TestRemoteRecoveryConfirmedAbsenceSettlesGuard`, `TestDeferredRemoteRecoveryRetriesUntilConfirmedAbsence`, and `TestDeferredRemoteRecoveryHandlesMissingExecutorRowWithoutReattachment` |
| AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.2 | `TestRemoteRecoveryConfirmedAbsenceSettlesGuard` preserves the executor resume token and workspace metadata. `internal/backendapp/remote_recovery_absence_integration_test.go`: `TestRemoteRecoveryAbsencePublicRecoveryActions` preserves uncertain submissions, open blocks, messages, and queue claims |
| AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3 | `TestRemoteRecoveryAbsenceRejectsChangedRuntimeRow`, `TestRemoteRecoveryAbsenceRejectsChangedOwnerDuringFinalValidation`, `TestRemoteRecoveryAbsencePreservesUnknownSibling`, and `TestRemoteRecoveryAbsenceRespectsStopAndShutdownFences` |
| AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.5 | `TestRemoteRecoveryAbsenceRetriesAfterRepairFailure` and `TestRemoteRecoveryAbsenceConvergesAfterLostRepairResponse`. Include lost success response, another restart, and unchanged successor |
| AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16/.21/.23/.24 | New `remote_recovery_absence_integration_test.go` in `internal/backendapp`: `TestRemoteRecoveryAbsencePublicRecoveryActions`. Compose actual lifecycle, adapter, repository, and orchestrator |

Task 01 records expected RED before production changes. Task 02 proves API
and browser behavior. Mocks of `session.recover` responses do not prove the fix.

## E2E tests

Extend `apps/web/e2e/tests/remote-docker/remote-docker-recovery.spec.ts` with
`desktop Resume reaches recovery after the exact task container stops` and
`phone Resume reaches recovery after the exact task container stops`.
Use the real SSH-backed Docker daemon fixture and its disposable backend.
Stop the owned backend process. Stop only the recorded task container. Restart
the backend against the same database. A stopped exact container
produces a definitive provider result. Blocking its port produces unknown
evidence and cannot substitute for this scenario.

Open the original task through the UI. Verify the executor row is stopped with
the original execution identity. Exercise Retry connection and assert its
visible delivery-recovery result. The independent delivery block and queue
dispatch claim must remain intact at this point. Then send the public
`session.recover` Resume request and assert a response rather than the stale
`session_recovery_in_progress` launch guard. The conversation must retain its
original messages; the resumed agent-boot audit message is allowed. The
interrupted submission remains uncertain. Explicit Resume may return the
parked queue claim to `queued_messages`, where the same item must remain
pending after reload. No original prompt or queued instruction can dispatch.

Run desktop and phone contexts in this same `containers` spec. The phone
describe uses a real phone device context with touch and its viewport.
Container tests cannot move to `mobile-chrome`, which lacks container setup.
The existing recovery card remains the mobile exemplar: one chat scroll owner,
safe-area clearance, visible stacked touch actions, and at least 44px targets.
No rendered layout, copy, controls, or navigation changes are planned, so no
new UI preview or separate UI specification is needed.

Also run the existing desktop and phone durable-reattachment suites. They prove
that eligible native restoration still permits one distinct new instruction
without replay. They do not replace the real remote-absence regression.
All browser runs are sequential and headless, with retries disabled.

## Work orders

- [x] [Task 01: Settle definitive remote absence safely](task-01-settle-remote-absence.md)
- [x] [Task 02: Prove recovery actions after remote loss](task-02-recovery-action-evidence.md)

## Verification results

Design evidence on 2026-11-01:

- Temporary reproduction: expected RED for Remote Docker and Sprites, both
  at the retained-guard assertion after three provider attempts.
- Focused existing controls: three tests passed.
- Issue assignment to `carlosflorencio`: confirmed by a fresh issue read.
- Specification catalog and full specification lint: passed (425 decisions,
  1878 specifications). Specification-linter tests: 36 passed.
- PR-documentation coverage preflight: passed for both work orders with a
  synthetic lifecycle source trigger and every referenced contract.
- Diff whitespace and untracked package inventory: passed.
- Task 01 implementation and regressions: complete. Focused Remote Docker and
  Sprites settlement passed; full lifecycle race suite passed (`182.920s`);
  executor-row CAS race passed (`1.542s`); changed Go lint passed with `0
  issues`; and `make -C apps/backend build` completed for all configured
  targets. Darwin arm64 reported only the expected unsigned-artifact warning.
- Task 02 public recovery integration: the race-detector run passed for Resume,
  saved-context continuation, Retry connection, and Stop through the registered
  websocket handlers with real task and SQLite repositories. Resume and
  continuation reached the fake Remote Docker create boundary and were refused
  there; retry reported unavailable evidence. The uncertain submission, open
  block, transcript, and queue claim remained unchanged.
- Task 02 browser checks: the `containers` project passed all four Remote
  Docker recovery cases, including desktop and phone exact-container loss.
  Existing native reattachment passed on desktop (2 tests) and phone (1 test).
  The worktree `.git` pointer is not available inside the runner container, so
  the container command used host mode with the managed runner rebuilding its
  assets while retaining the real `containers` project and Docker executor.
  The loss fixture captured the
  execution identity, joined the test-owned backend shutdown, stopped the exact
  task container, and only then copied and seeded the closed remote journal.
  Both viewport cases observed the stopped executor row with that same
  execution ID, the visible blocked Retry result, and a successful public
  Resume response past launch admission. The resumed agent-boot audit event was
  the only allowed message addition. The uncertain submission remained
  `interrupted_unknown`; Resume moved the queue claim into the same pending
  queue item, which remained present after reload without dispatch.
  `KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --project containers tests/remote-docker/remote-docker-recovery.spec.ts -- --retries=0`,
  `pnpm e2e:run --host --no-build --project chromium tests/session/durable-reattachment.spec.ts -- --retries=0`,
  and `pnpm e2e:run --host --no-build --project mobile-chrome tests/session/mobile-durable-reattachment.spec.ts -- --retries=0`.
- Task 02 web checks: typecheck passed with `NODE_OPTIONS=--max-old-space-size=8192`;
  ESLint passed for the changed browser fixtures, page object, and spec.
- Deferred-recovery scheduling checks: the exact retry and Stop/shutdown fence
  tests passed 20 repetitions under the Go race detector. An unactivated
  startup reporter keeps the deferred worker gated so each controlled poll
  owns the attempt; the tests use no sleeps.
- Task 02 documentation checks: 63 public-doc tests passed; all 54 published
  pages validated; 425 decisions and 1878 specifications validated; all
  specification files passed lint; and `git diff --check` passed.
- The final actual-diff PR-documentation coverage preflight reports `covered`
  with no errors for both work orders and all referenced contracts.

Exact diagnostic commands:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^TestRepro4989ConfirmedMissingRemoteOwnerFinishesReconciliation$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -run '^(TestUnknownRemoteRecoveryRemainsRetryablyGuardedAndRetriesSameRecord|TestHasPendingRemoteRecoveryRequiresExactCurrentGuardedOwner|TestManagerStartTreatsRecordsAsNotRetrackedOnceRecoveryDeadlineElapses)$' -count=1 -v)
```

The first command requires the temporary test. Task 01 supplies its permanent
replacement and exact implementation commands.

## Risks

- `no_matching_instance` has different meanings across adapters. Widening the
  existing proof predicate can expose a live remote owner to replacement.
- Clearing only the in-memory guard leaves a false live row and repeats the
  problem after restart. Repair must commit before release.
- Repair success can race Stop or owner rotation. Entry identity, row version,
  and final arbitration must all remain effective.
- A missing control endpoint can remain unknown even after host maintenance.
  Unreachable ownership remains protected until its executor supplies proof.
- Native resume can remain blocked after guard completion. Preserve its real
  evidence failure, without claiming that unknown work completed.
- A backend test with a fake no-match result proves lifecycle behavior only.
  Real adapter and container evidence remain required before delivery.
