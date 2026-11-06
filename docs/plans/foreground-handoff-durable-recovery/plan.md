---
created: 2026-11-06
status: completed
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
legacy_specs: []
---

# Implementation plan: Foreground handoff durable recovery

## Overview

Fix [issue #5104](https://github.com/kdlbs/kandev/issues/5104) without replaying an interrupted instruction.
First, prevent a foreground handoff from recording successful durable completion.
Then recover existing records through authenticated native retirement and the existing admission transaction.
Both work orders run sequentially after an explicit implementation request.

Platform owns this repair because it owns durable submission outcomes and shared session recovery.
The task workflow, provider-specific liveness policy, and launcher retain their existing boundaries.

## Confirmed evidence

Investigation used repository revision `628da65a0e0b48ada18d2e43f8d623e956becdfb`.
The issue reports macOS service self-update, Claude ACP, and durable delivery.
It contains no image attachments or follow-up comments at investigation time.
The original database and journal were unavailable. The reproduction proves the reported state and refusal, not every incident timestamp.

The failure follows this path:

1. `handlePromptHandoffEvent` signals a generation-matched lifecycle completion with `stop_reason=foreground_idle`.
2. `SessionManager.waitForPromptDone` returns that reason without requiring a terminal delivery event.
3. `finishPromptExecutorDispatch` calls `agentDeliverySubmissionRuntime.markCompleted` for every successful blocking result.
4. `markCompleted` changes SQL from dispatching to completed without terminal stream, sequence, turn, or full-settlement evidence.
5. Retained adoption excludes that SQL row from its active inventory while the journal retains active work.
6. After journal interruption, `nativeResumeSQLRows` requires terminal proof for the SQL completion label and rejects native recovery.

Two temporary tests exercised the real repository and production boundaries:

- `TestRepro5104ForegroundHandoffMustNotCompleteDurableSubmission` called the real dispatch finalizer.
  It failed because the expected dispatching submission became completed with `terminal_sequence=0` and no full-settlement marker.
- `TestRepro5104NativeResumeRejectsSQLCompletionWithoutTerminal` used real SQLite and bbolt stores.
  It reproduced `durable adoption blocked: native resume terminal evidence disagrees` and retained both records unchanged.

The command was:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -p=2 ./internal/orchestrator ./internal/agent/runtime/lifecycle -run '^TestRepro5104' -count=1 -v -timeout 120s)
```

The command exited 1 because the producer regression failed as expected.
The diagnostic recovery assertion passed. Both temporary files were removed after investigation.
No production files, permanent tests, running instances, or user databases changed.

## Requirement conformance

The defect violates the existing honest-outcome and complete-inventory recovery contracts.
AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.7 and .8 make this missing state-pair coverage explicit.
The owning requirement remains [durable restart reconciliation](../../specs/platform/requirements/durable-restart-reconciliation.md).
The [convergence design](../../specs/platform/system-design/session-recovery-convergence.md#foreground-handoff-and-unproven-sql-completion)
defines the correction.

The [existing ADR](../../decisions/2026-10-12-total-session-recovery.md) separates historical labels from admission authority.
This package applies that decision to an unproven SQL completion. It does not downgrade a known terminal result.
No separate repair requirement, UI requirement, or new architecture decision is needed.

## Scope

### In scope

- Preserve an unresolved durable submission after a nonterminal foreground handoff.
- Settle a later terminal against its original submission without releasing a successor.
- Recognize historical SQL completed/journal interrupted-unknown pairs only through complete authenticated proof.
- Preserve historical labels, payloads, hashes, terminal absence, and uncertain outcome semantics.
- Resolve matching delivery fences through existing automatic recovery or explicit native Resume.
- Prove retry after interruption, reload convergence, and later distinct instruction admission.

### Out of scope

- Self-update draining, service signal policy, or keeping every local agent alive during restart.
- Manual SQL repair, blanket fence deletion, a repair CLI, or a new admin action.
- Relaxing dead-owner proof, native identity, cursor, workflow, queue, archive, or automation restrictions.
- The report's separate terminal WebSocket/task mismatch and unexplained later double signal.
- New UI markup, copy, navigation, feature flags, table/column migrations, or journal protocol versions. One replayable partial inbox index for the terminal-absence lookup is in scope.

## Technical approach

### Prevent premature completion

At `finishPromptExecutorDispatch`, retain the nonterminal meaning of `streams.EventTypeForegroundIdle`.
Do not call `markCompleted` for that successful handoff result.
The call still returns successfully, and the live submission stays dispatching.
A real terminal already projected before the return keeps its outcome.
Later canonical terminal projection owns settlement and binds the original submission.
Preserve the existing handoff admission policy and generation fences.

### Recover existing historical records

Add one shared assessment for an unproven SQL completion.
Its exact conjunctive predicate and interruption ordering are in the system design.
It requires verified original-owner termination, the same loaded native conversation,
an idle authenticated successor, complete inventories, and fully projected stream coverage.
Neither store can contain terminal proof for that submission.

Carry an explicit classification through `NativeResumeSubmissionEvidence`.
Keep SQL completed/outcome labels and the journal unknown label unchanged.
Retire only the exact journal history, re-read its marker, and revalidate SQL under existing admission locks.
Count this class separately from proven terminals in `verifyNativeResumeInventoryTx`.
A changed proof or independent unresolved record keeps the fence effective.

Use metadata-only SQL reads and indexed existence queries. Do not load transcript or event payload history to infer terminal absence.
No new schema is required. PostgreSQL must use the same predicate and existing transaction locks.
Crash recovery reuses the journal retirement marker instead of repeating a prompt or fabricating a terminal receipt.

### Compatibility

| Provider and transport | Evidence | Behavior and verification | Unsupported evidence |
| --- | --- | --- | --- |
| Claude ACP, durable V1 | Attested foreground handoff | Keep submission live until real terminal. Producer and successor-generation tests prove ordering. | Unattested events keep existing behavior. |
| Any compatible structured provider, durable V1 | Exact SQL completed/journal unknown pair, no terminal proof | Apply shared native recovery assessment. SQLite/bbolt and PostgreSQL tests prove the contract. | Missing native, owner, hash, cursor, or complete inventory keeps recovery blocked. |
| Surviving original process | Authenticated live owner | Reattach through existing adoption. Never retire active journal work. | Unknown liveness cannot establish termination. |
| Durable V1 with retained terminal | Exact terminal and projection evidence | Use existing terminal settlement, including interrupted final binding. | A partial or conflicting terminal cannot use the new fallback. |
| Legacy, passthrough, or unsupported peer | No compatible proof | Preserve existing delivery and specific blocked results. | No downgrade or replacement conversation. |

Shared code does not establish every provider's native-resume support.
The new recovery class requires the existing provider capability and saved native identity.

## Related implementation records

- [Session recovery convergence](../session-recovery-convergence/plan.md) owns existing producer shutdown and native retirement behavior.
- [Interrupted terminal settlement](../interrupted-terminal-settlement/plan.md) requires a real retained terminal. This package handles terminal absence.
- [Journal growth and restart reconciliation](../journal-growth-restart-reconciliation/plan.md) retains source-owner and restore-only admission authority.

These packages remain the source for their delivered results and independent pending work.
This package does not mark their tasks complete or claim their checks as new evidence.
The new tests extend their recovery matrix with the opposite SQL/journal mismatch.

## Tests

| Acceptance criteria | Planned evidence |
| --- | --- |
| AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.7 | `agent_delivery_handoff_test.go`: `TestForegroundHandoffPreservesDurableSubmission`, `TestForegroundHandoffLateTerminalSettlesOriginalSubmission`. |
| AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.8 and 001.1-.3 | `native_resume_unproven_completion_test.go`: `TestNativeResumeUnprovenSQLCompletionRecovery`, `TestNativeResumeUnprovenSQLCompletionReload`, `TestNativeResumeUnprovenSQLCompletionHistoricalStream`, and a negative evidence matrix, with real SQLite/bbolt stores. |
| AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.5 and 002.4-.5 | Lifecycle interruption/reopen tests, SQL proof revalidation tests, and a PostgreSQL concurrent-receipt schedule. Include retirement-before-resolution, terminal-arrival, and historical-stream cursor races. |
| AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.3 | Orchestrator `TestUnprovenCompletionRecoveryEntrypoints` exercises automatic loaded recovery, explicit Resume, and later direct/queue admission. |
| AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4 | Desktop and phone flow below: recovery clears the matching notice, survives reload, and accepts one distinct prompt. |

The negative matrix includes mixed eligible/live records and an eligible candidate followed by a conflicting candidate.
It also covers foreign identity, hash change, future generation, missing projection,
partial terminal fields, pruning markers, terminal inbox/effect evidence, and storage errors.
Assert that no rejected case retires a journal record or resolves a fence.

## E2E tests

Extend the existing deterministic native-resume fixture used by
`apps/web/e2e/helpers/native-resume-notice.ts`.
Seed this historical cross-store state in disposable fixture stores.
Do not mock the recovery operation as successful.

Add the same `unproven SQL completion` scenario in:

- `apps/web/e2e/tests/session/durable-stream-recovery.spec.ts`, project `chromium`.
- `apps/web/e2e/tests/session/mobile-durable-stream-recovery.spec.ts`, project `mobile-chrome`.

Require real recovery, reload, preserved session/native identity, retained draft,
no old instruction replay, and exactly one explicit distinct new instruction.
Keep the existing blocked/Stop and control geometry assertions.
No rendered UI changes are planned, so mobile composition and ASCII previews do not apply.
CI runs these suites under the existing convergence package's browser boundary.
Do not run local E2E during this planning turn.

## Work orders

- [x] [Task 01: Preserve durable ownership at foreground handoff](task-01-preserve-handoff-submission.md)
- [x] [Task 02: Recover unproven SQL completion history](task-02-recover-unproven-completion.md)

Task 02 depends on Task 01. Both are sequential. No delegation is authorized.

## Verification results

Planning evidence: the producer reproduction failed for the expected state mismatch.
The SQLite/bbolt diagnostic reproduced the reported recovery refusal.
Temporary tests were removed. Implementation and permanent regression checks are complete.

Implementation evidence:

- Producer: `finishPromptExecutorDispatch` now keeps a durable submission
  dispatching when the prompt result is `streams.EventTypeForegroundIdle`.
  Disabling that guard reproduced the RED state in
  `TestForegroundHandoffPreservesDurableSubmission`. The late-terminal and
  interrupted-before-terminal regressions passed.
- Recovery: one read-only SQL predicate classifies the historical mismatch,
  counts it separately from proven terminals, and revalidates it under the
  existing admission locks. Each unproven submission pins its historical
  stream cursor, and resolution re-reads the exact receive/projection/retire
  watermark. A dedicated replayable partial inbox index keeps the
  terminal-absence lookup off unrelated retained history. Real SQLite/bbolt
  tests cover recovery, repeat retirement, reopen, stale proof, historical
  cursor movement, mixed live work, and conflicting inventory.
- The focused lifecycle suite passed (full package `191.581s`). The focused
  SQLite suite passed; the full SQLite package also passed (`620.827s`).
  The orchestrator entry-point suite passed, and the full orchestrator package
  passed (`262.822s`). `go run ./cmd/sqlguard ./internal`,
  `storeconformance`, and changed-file Go lint passed.
- PostgreSQL execution: skipped locally because `KANDEV_TEST_POSTGRES_DSN` is
  not configured. The new environment-gated tests cover the locked proof
  matrix, the historical cursor, the query plan, and a concurrent receipt
  schedule. They remain for an authorized database run in CI.
- Review fixup: historical cursors also pin the peer-reported high-water mark;
  PostgreSQL fixtures use `Rebind`; negative tests capture metadata before
  acknowledgement; and uncertainty-state E2E assertions document the
  Retry/Stop UI. The focused backend checks and docs validators were rerun.
- Documentation: `list-docs.py validate`, `lint-spec-files.py --all`, and the
  public-docs validators passed. `docs/public/sessions-and-review.md` explains
  the guarded recovery and preserved uncertainty.
- Browser verification: desktop and phone scenarios were added to the existing
  native-resume fixture. They require the real backend recovery path. They were
  not run locally during implementation; CI owns both suites.

Before the design handoff, run:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/foreground-handoff-durable-recovery
git status --short -- docs/plans/foreground-handoff-durable-recovery
```

Run the documentation-coverage rehearsal from the repository root:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const base = 'docs/plans/foreground-handoff-durable-recovery/';
const files = [
  base + 'plan.md',
  base + 'task-01-preserve-handoff-submission.md',
  base + 'task-02-recover-unproven-completion.md',
  'docs/specs/platform/requirements/durable-restart-reconciliation.md',
  'docs/specs/platform/system-design/session-recovery-convergence.md',
];
const result = validateCoverage({
  changedFiles: [
    { filename: 'apps/backend/internal/orchestrator/task_operations.go', status: 'modified' },
    ...files.map(filename => ({ filename, status: 'added' })),
  ],
  fileContents: Object.fromEntries(files.map(filename => [filename, fs.readFileSync(filename, 'utf8')])),
});
console.log(JSON.stringify({ rehearsalForPlannedRuntimeChange: true, ok: result.ok, status: result.status, workOrders: result.workOrders, errors: result.errors }, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

The proposed backend path makes the validator assess the references instead of returning documentation-only exemption.
It does not represent a production change in this design turn.

Design validation passed:

- `list-docs.py validate`: 436 decisions and 1929 specifications validated.
- `lint-spec-files.test.py`: all 36 tests passed.
- `lint-spec-files.py --all`: all specification files passed.
- Documentation-coverage rehearsal: both work orders covered, with no reference errors.
- Diff whitespace and package inventory checks passed. All five artifact files remain unstaged and uncommitted.

## Documentation impact

This turn changes internal requirements, design, and delivery records only.
Task 02 updates the recovery explanation in `docs/public/sessions-and-review.md` after implementation.
The explanation must describe uncertainty and existing Resume, without prescribing database edits or promising unconditional recovery.

## Risks

- An enum label alone cannot distinguish a real completion from this historical bug.
  Complete absence-of-terminal proof and authenticated owner checks are mandatory.
- Retirement and SQL resolution use separate stores. Interruption tests must prove convergence without broad fence clearing.
- A terminal can arrive during assessment. Revalidation must reject stale unproven evidence and preserve ordinary terminal authority.
- The issue's original files are unavailable. Missing or conflicting evidence can still require a precise blocked result.
- Existing handoff admission can permit a successor. Late original terminal handling must preserve that successor's ownership.
