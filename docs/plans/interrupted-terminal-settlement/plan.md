---
created: 2026-10-27
status: completed
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DELIVERY-RETENTION-003
  - REQ-TASKS-TURN-CHANGES-003
  - REQ-TASKS-TURN-CHANGES-007
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
  - ../../specs/tasks/system-design/turn-changed-files.md
legacy_specs: []
---

# Implementation Plan: Interrupted terminal settlement

## Overview

Recover a known projected terminal from its retained SQL effect when shutdown
interrupts full settlement. Stop publication retries for absent turn-change sets.

## Evidence

An incident after an interrupted shutdown reported `native resume terminal
evidence disagrees`. Two temporary focused Go tests established a possible
path: the real repository can commit a completed outcome while terminal stream
and sequence are empty before full settlement, and `nativeResumeSQLRows` returns
the reported error for that state alongside an interrupted journal submission
with a retained terminal. The incident's database and journal are unavailable,
so this path is a hypothesis for that session, not a confirmed incident cause.
Existing projected-terminal recovery tests pass with a fully settled binding.

An implementation experiment called the existing settlement reconciler before
native acknowledgement. A focused negative test with a bound recovery block and
a journal hash mismatch failed: the SQL settlement resolved the block before
the journal acknowledgement rejected it. The experiment was removed. This is
an unsafe ordering, including when the mismatch occurs after a valid SQL outcome.
The repair instead reads terminal evidence without triggering settlement writes.
The incident mismatch itself remains unknown.

Independently, `loadTerminalTurnChangeSet` treats a missing set as a no-op, but
`runtimeTurnChangeCaptureHandler.ProcessTurnChanges` then publishes it; the
publisher returns not found, and `scheduleTurnChangeSummary` retries forever.

## Scope

### In scope

- Prove terminal history from the exact projected settlement effect when its
  final submission binding has not yet been written.
- End summary publication retries for a missing turn-change set.
- Cover interrupted, repeated and conflicting recovery, and ordinary summary
  publication failures.

### Out of scope

- Relaxing recovery identity, ownership, live-work, workflow or cursor checks.
- Fabricating missing evidence or replaying an interrupted instruction.
- Assuming this repair resolves every possible evidence disagreement.
- UI changes, production database edits and new user-facing text.

## Technical approach

Lifecycle may derive a recovery-only terminal binding from the exact SQL terminal
effect identified by the authenticated journal. The SQL submission must already
have the same terminal state and outcome; all final binding fields must be empty,
and full-settlement and payload-pruning markers absent. The effect must match
session, incarnation, generation, submission, stream, sequence and outcome, be
block-resolved or completed, and have no newer terminal for that stream and
submission. A contiguous fully projected cursor must cover it. Partial or
conflicting stored bindings never use this fallback.

A narrow repository verifier reads this evidence without changing any records.
Lifecycle still validates the complete journal inventory, immutable hash, owner
and absence of live work before retirement. The existing block-resolution
transaction repeats the same SQL predicate under its delivery locks. A changed
proof cannot clear a fence. The ordinary persisted-binding path stays intact.

This follows the existing separation of terminal history and admission authority:
workflow/queue cleanup need not be complete to prove an outcome, and native
recovery never completes those effects, stamps full settlement, synthesizes a
workflow receipt or dispatches a prompt. Independent admission guards still apply.
There is no schema change or cross-process transaction. Running the settlement
reconciler before journal validation was rejected because it clears blocks early.

In `runtimeTurnChangeCaptureHandler`, distinguish a missing summary from a
transient publisher failure, so an uncaptured turn ends as a no-op while real
publish errors continue to retry. Preserve actual capture and historical data.

## Tests

- AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5, .12 and .24 and
  AC-PLATFORM-DELIVERY-RETENTION-003.1: real repository plus journal regression
  at interruption, settlement, retirement and repeat recovery. Negative cases
  cover missing/conflicting terminal effects, wrong hash or owner, projection lag,
  stale proof, partial binding and independent live work. Pending workflow effects
  and full-settlement markers remain unchanged.
- AC-TASKS-TURN-CHANGES-003.6 and AC-TASKS-TURN-CHANGES-007.1: backendapp
  regression for missing and transiently failing publication.

## E2E tests

Exercise the existing `apps/web/e2e/tests/session/durable-stream-recovery.spec.ts`
native-resume flow on the `chromium` project, and the corresponding
`mobile-durable-stream-recovery.spec.ts` flow on `mobile-chrome` locally. These
cover the existing recovery UI; the new settlement boundary is verified by
the real backend integration tests.

## Work orders

- [x] [Task 01: Reconcile interrupted terminal settlement](task-01-terminal-settlement.md)
- [x] [Task 02: Stop missing summary retries](task-02-summary-retries.md)

## Verification results

Implementation and task-defined validation complete.

- RED: the real SQLite/bbolt fixture failed with `native resume terminal evidence
  disagrees` for completed, failed and cancelled outcomes before the fix.
- GREEN: targeted native-resume, settlement and turn-change tests passed across
  lifecycle, repository, orchestrator and backendapp packages.
- SQLite and PostgreSQL 16 terminal-proof cases, lifecycle regression cases and
  storage conformance passed under the race detector.
- `go run ./cmd/sqlguard ./internal` passed; scoped Go lint reported zero issues.
- Specification inventory, specification lint and PR documentation-coverage
  preflight passed.
- RED/GREEN summary coverage proves missing publication ends successfully while
  storage and event-bus errors remain retryable.
- Recovery E2E: all five desktop and all five mobile scenarios passed.
- Public documentation: updated the sessions-and-review recovery explanation.
  No UI markup or screenshot changes are required.

Package checks:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/interrupted-terminal-settlement
git status --short -- docs/plans/interrupted-terminal-settlement
```

## Risks

- Pending workflow and queue work retains its own admission and retention gates;
  native recovery must never mark that work complete.
- The original incident may have another mismatch, so test results establish
  only the reproduced case and protected failure cases.
