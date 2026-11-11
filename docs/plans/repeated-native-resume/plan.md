---
created: 2026-11-11
status: complete
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
  - ../../specs/platform/system-design/session-recovery-convergence.md
legacy_specs: []
---

# Implementation plan: Repeated native Resume

## Overview

Repair explicit Resume after a session has already completed a silent restart restore.
Preserve the native conversation, historical uncertainty and existing ownership checks.
Implement historical proof first, then restart presentation repair, then desktop and phone regression evidence.
Each work order runs sequentially in the primary conversation.

This package addresses [issue #5256](https://github.com/kdlbs/kandev/issues/5256).
The issue is assigned to `carlosflorencio`.
The user authorized implementation. Tasks 01-03 are complete; PostgreSQL checks remain gated by a test DSN.

## Evidence and confirmed root cause

Investigation baseline: `bceeaf1c1d30ad05b953fd800b56ae46ebf5f2bf`, also the remote `main` head on 2026-11-11.
The issue and both comments were read. They contain no image attachments.
The reported deployment used `a268e5c409`.
All reproduction databases were disposable fixtures. No running instance or user database was changed.

1. `hasResolvedNativeResumeRecoveryTx` and `hasLegacyResolvedNativeResumeHistoryTx` accept only the historical action `resume`.
   A real `CommitSilentRestore` instead resolves the source fence with `models.SilentRestoreAction`.
2. `restoreNativeResumeMetadataTx` compares a later submission-bound fence against the older restored notice.
   Generation, submission, stream and phase differ, so the transaction returns false.
3. The production authenticated path has the same defect independently.
   `restoreAuthenticatedNativeNoticeTx` sends every older notice through `hasResolvedContextContinuationNoticeTx`, which excludes `restored`.
4. `recreateRestartRecoveryBlock` unconditionally upserts a new reason when invoked with older metadata and an existing current unknown-outcome fence.
   The uniqueness key includes the reason, so the helper adds a generic companion.
   Ordinary serial presentation repair already reuses one existing fence. The failing helper test demonstrates the missing creation guard separately.
5. `ResolveSessionRecoveryBlockWithNativeEvidence` already supports multiple matching incident fences.
   Its stale-notice rejection prevents that mechanism from recovering the issue's existing duplicate shape.

### Reproduction results

Temporary `TestRepro5256RepeatedRecovery` used `newSilentRestoreFixture`, `prepareSilentRestoreCandidate` and a real `CommitSilentRestore`.
The commit created generation 2 while preserving the generation-1 restored notice.
A new generation-2 unknown submission then exercised the real repository resolution methods.

| Case | Baseline result |
| --- | --- |
| Exact historical silent-restore proof | Expected success failed |
| Legacy empty-stream historical silent-restore proof | Expected success failed |
| Current generic fence after silent restoration | Expected success failed |
| Current submission-bound fence with stale restored notice | Expected success failed |
| Complete native evidence with stale restored notice | Expected success failed |
| Complete native evidence with two matching current fences | Expected success failed |
| Historical action changed to `resume`, generic control | Passed |
| Absent historical notice, complete native-evidence control | Passed |

Temporary orchestrator tests produced one further expected failure and one passing control.
`TestRepro5256RecreateAddsCompanionFence` observed two open fences instead of one.
`TestRepro5256RestartRepairReusesExistingFence` passed through ordinary presentation repair.
The companion-helper fixture constructed historical SQL state. It did not prove the reporter's full failed-Resume call sequence.

Commands ran from `apps/backend`:

```bash
go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^TestRepro5256' -count=1 -v
go test -trimpath -tags fts5 ./internal/orchestrator -run '^TestRepro5256' -count=1 -v
```

Both commands exited 1 because their defect assertions failed as expected.
The final runs contained seven failing assertions and three passing controls.
Temporary reproduction files were removed after recording this evidence.

## Requirement conformance and assumptions

Platform owns the failed contract because durable-delivery admission and historical uncertainty are shared runtime guarantees.
Reuse [REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006](../../specs/platform/requirements/durable-agent-delivery.md).
Relevant criteria are .5, .8, .9, .12, .23 and .24.
The broader requirements file remains draft. This package repairs already implemented behavior described by the current designs and accepted ADR.
It creates no new product requirement and does not promote that document's status.

The accepted [outcome/admission decision](../../decisions/2026-10-12-total-session-recovery.md) remains authoritative.
The [harness boundary decision](../../decisions/2026-09-10-durable-harness-session-boundaries.md) retains native-first recovery and explicit replacement authority.
No new ADR is needed because this repair preserves those decisions.

Confirmed scope: diagnose the issue, assign it after establishing a reliable repair, and produce a fix package.
Verified intent: native Resume preserves the conversation and unknown outcome without historical prompt dispatch.
No material behavior choice blocks this package.

## Scope

### In scope

- Recognize exact silent-restore history without widening current explicit-Resume admission.
- Resolve complete current native proof despite a verified older restored notice.
- Preserve safe generic fallback behavior and refuse unsupported missing bindings.
- Reuse existing restart fences through a transaction that prevents repair-created companions.
- Recover already duplicated matching incident fences through the authenticated resolver.
- Preserve independent restrictions, complete inventory checks, unknown payloads and terminal absence.
- Prove repeatability and distinct follow-up prompt admission on desktop and phone.

### Out of scope

- A destructive discard action, new recovery controls, new copy or localization changes.
- Relabeling unknown submissions as cancelled, failed or completed.
- Editing a live database, journal removal, blanket fence clearing or automatic old-prompt resend.
- The follow-up `continue_from_history` generation mismatch and Stop success report.
  Those reports were read, but their full failures were not reproduced here.
  Continuation already selects a successor stream before its later SQL commit.
  The precise failing ownership boundary needs separate evidence before changing that path.
- The reported terminal reconnect/idle-reclaim loop, OOM prevention and unrelated resource management.

The repaired native Resume path supplies recovery for the proven state without a destructive escape hatch.
This package does not claim that all follow-up comments are fixed.

## Technical approach

### Historical proof and current native resolution

Update `native_resume_generic_block.go` and `native_resume_historical_metadata.go` to recognize exact prior silent restoration.
Bind SQL action values with the existing constants.
Keep `HasResolvedNativeResumeRecovery` in `native_resume_recovery.go` strict for current explicit Resume.

In `native_resume_binding.go`, distinguish verified historical native restoration from context continuation before its older-notice branch.
Require the resolved source fence and unchanged native lineage across all crossed generations.
Retain current execution, native ID, inventory, cursor and pending-attempt checks under the existing write locks.
Keep the old restored notice unchanged. Do not mark native restoration as context continuation.

For a missing current stream binding, reuse the complete authenticated proof and existing incident binding transaction.
Do not infer the current stream from the stale notice or merely ignore older metadata.
The generic fallback retains its exactly-one-open-fence condition.
Existing duplicate fences require the complete authenticated resolver.

The inventory rule is universal: every unresolved SQL submission must have matching authenticated retirement evidence.
One valid or retired submission cannot override a live, foreign or unmatched sibling.
Cover mixed current and historical rows, plus matching and independent fences in the same session.

### Restart presentation repair

Add the narrow repository capability `EnsureRestartRecoveryBlock` in `interface.go`.
Implement it in a small SQLite repository file, using existing turn/delivery locks and helpers.
Acquire PostgreSQL advisory locks before reading the current generation and open-fence inventory.
Return one existing eligible delivery fence unchanged, or create the candidate when no open fence exists.
Multiple or independent fences prevent insertion.

Use that operation from `recreateRestartRecoveryBlock`.
Propagate storage errors and preserve the current binding through restart reconstruction.
Keep general `UpsertSessionRecoveryBlock` semantics, migrations and independent consumer behavior unchanged.

### Compatibility

| Shape | Intended behavior | Evidence |
| --- | --- | --- |
| Durable native Resume after exact silent restoration | Preserve native ID and accept complete current proof | SQLite and PostgreSQL regression matrix |
| Legacy historical fence with empty stream | Accept only its existing strict legacy evidence | Positive and negative legacy tests |
| Current submission fence with empty stream | Bind from authenticated proof | SQL and real-journal integration |
| Already duplicated matching incident fences | Resolve all matching fences atomically | Duplicate and independent-sibling tests |
| Live work, unknown owner, future generation or missing native state | Preserve the typed refusal and all restrictions | Guard matrix |
| Legacy peer without authenticated proof support | Preserve established fallback restrictions | Existing native Resume compatibility tests |
| Office, automation or unrelated recovery consumer | Preserve its existing admission authority | Independent-consumer tests |

No provider is newly supported. Test transport and identity shapes instead of assuming support from a provider name.

## Tests

| Acceptance criteria | Permanent evidence |
| --- | --- |
| 006.9, 006.12, 006.24 | `native_resume_after_silent_restore_test.go`: `TestNativeResumeAfterSilentRestore` and PostgreSQL counterpart |
| 006.12, 006.23 | Same file: mixed-inventory, lineage, action, timestamp, cursor, restore-attempt and rollback subtests |
| 006.5, 006.24 | `restart_recovery_block_ensure_test.go`: `TestRestartRecoveryBlockEnsureReusesIncident` and PostgreSQL concurrent counterpart |
| 006.9, 006.24 | `repeated_native_resume_test.go` in orchestrator: `TestRepeatedNativeResumeAfterSilentRestore` |
| 006.12, 006.24 | `repeated_native_resume_test.go` in lifecycle: `TestRepeatedNativeResumeAfterSilentRestoreJournal` |

Permanent tests first reproduce the defects before production edits.
Use a real silent-restore commit and retained journal for the integrated repeat-interruption test.
An authenticated SQL proof fixture alone does not prove lifecycle retirement, so both boundaries require evidence.

## E2E tests

Add a shared `repeated-native-resume.ts` helper and desktop/mobile specs under `e2e/tests/session/`.
Reuse the isolated backend, silent-restart helpers and interrupted-prompt journal fixture.
Fixtures must produce valid SQL and retained journal identities without a production testing endpoint.

For AC-006.8, .9, .12 and .24, restore generation 1 silently, interrupt a distinct generation-2 prompt, then click existing Resume.
Cover submission-bound, generic and already duplicated incident fences.
Assert unchanged native ID, retained unknown submissions, no historical prompt replay and no automatic queue drain during restoration.
Reload, repeat recovery, and send one distinct instruction through normal admission.
Assert one new dispatch and inspect the real journal retirement evidence.
An independent blocker must preserve the existing blocked feedback.

The existing recovery controls retain their desktop and phone composition.
No rendered markup, layout, navigation or copy change is planned, so an ASCII UI preview is unnecessary.
Phone coverage uses the existing touch target and busy-state assertions.

## Work orders

- [x] [01: Accept verified silent-restore history](task-01-historical-native-proof.md)
- [x] [02: Preserve restart recovery incident fences](task-02-restart-fence-creation.md)
- [x] [03: Prove repeated native recovery end to end](task-03-repeated-recovery-evidence.md)

## Companion package inventory

- [Quick Chat interrupted prompt recovery](../quick-chat-interrupted-prompt-recovery/plan.md): implemented manual Resume and workspace access.
- [Journal shutdown recovery](../agentctl-journal-shutdown-recovery/plan.md): existing silent-restore and journal lifetime work, currently `in_progress`.
- [Session recovery convergence](../session-recovery-convergence/plan.md): existing complete-inventory and incident-fence authority, currently `in_progress`.
- [Delivery record recovery](../durable-agent-record-recovery/plan.md): completed reconstruction contract.
- [Journal growth restart reconciliation](../journal-growth-restart-reconciliation/plan.md): completed restart presentation repair.

This follow-up extends those contracts. Their historical task results remain historical evidence.
The new work orders own issue #5256 regressions and their final results.
Do not mark an earlier package complete based on this package's narrower scope.

## Verification results

Implementation checks passed except for PostgreSQL acceptance, which requires `KANDEV_TEST_POSTGRES_DSN`.
Investigation and design results remain in the earlier sections of this plan.

Design-package checks passed:

- `python3 scripts/list-docs.py validate`: 448 decisions and 1,981 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check -- docs/specs docs/plans/repeated-native-resume`: passed.
- `git status --short -- docs/plans/repeated-native-resume`: all four new package files remain unstaged and uncommitted.
- PR-documentation preflight: all three work orders passed, with six accepted requirement/design references and no errors.

Implementation verification passed:

- SQLite repository, orchestrator and lifecycle race suites passed. The lifecycle suite used retained bbolt journals, reopened the journal, and retried after SQL rollback.
- SQL guard and store-conformance race tests passed. Changed-Go-file lint reported `0 issues`.
- Managed E2E rebuilt the backend, Vite assets and fixture plugin. Desktop passed four repeated-resume and projection tests. Phone passed four corresponding tests.
- Web typecheck and Prettier checks passed. The docs catalog validated 448 decisions and 1,981 specifications; all 36 spec-linter tests and full spec lint passed.
- PR-documentation preflight passed with six accepted references and no errors. `git diff --check` passed.
- PostgreSQL cases did not run because `KANDEV_TEST_POSTGRES_DSN` is unset. No public docs changed; the existing session recovery guide already states that Resume preserves the native conversation and does not resend uncertain prompts.

PR review remediation passed:

- Recovery-block identity now applies to open rows. SQLite rebuilds a legacy constrained table while preserving every row and binding; PostgreSQL removes the old unique constraint before creating the open-only index.
- Resolved bound submissions remain closed. A later generic fence requires retained interrupted work created after the resolved fence. SQLite migration replay, same-incident refusal and distinct-successor creation passed under `-race`.
- Automation's legacy quarantine backfill passed with the open-only index. SQL guard, store-conformance race tests, orchestrator recovery race tests and changed-Go-file lint passed.
- Desktop and mobile repeated-resume E2E reruns each passed 2 tests. The mock-agent trace confirms no historical or interrupted-successor replay, one initial prompt, and one distinct follow-up dispatch. The independent case retains the exact seeded block IDs and consumers across Resume attempts.
- Both managed E2E runs rebuilt the backend, Vite assets and fixture plugin. Web typecheck, Prettier, documentation catalog, all 36 spec-linter tests, full spec lint, PR-documentation coverage and `git diff --check` passed. The latest docs catalog validated 449 decisions and 1,984 specifications.
- PostgreSQL migration and repository cases remain pending because `KANDEV_TEST_POSTGRES_DSN` is unset.

The preflight deliberately includes a planned production path to exercise coverage validation for this documentation-only package.
Run it from the repository root:

```bash
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const directory = 'docs/plans/repeated-native-resume';
const orders = fs.readdirSync(directory)
  .filter(name => /^task-\d{2}-.*\.md$/.test(name))
  .map(name => `${directory}/${name}`);
const documents = [
  `${directory}/plan.md`, ...orders,
  'docs/specs/platform/requirements/durable-agent-delivery.md',
  'docs/specs/platform/system-design/interrupted-prompt-recovery.md',
  'docs/specs/platform/system-design/session-recovery-convergence.md',
];
const fileContents = Object.fromEntries(documents.map(pathname =>
  [pathname, fs.readFileSync(pathname, 'utf8')]));
const result = validateCoverage({
  changedFiles: [
    'apps/backend/internal/task/repository/sqlite/native_resume_generic_block.go',
    ...orders,
  ],
  fileContents,
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

No public documentation change was needed for this implementation.
The change repairs existing Resume behavior without a new endpoint, setting, rendered control or user procedure.
The current session recovery guide already describes the native conversation and no-resend behavior.

## Risks

- Accepting an action name without complete provenance can authorize a foreign or unresolved source.
- Missing-stream recovery must use current authenticated evidence rather than guessed identifiers.
- Open-fence uniqueness must preserve resolved history while allowing a distinct later incident with the same natural key.
- PostgreSQL serialization requires the same session-turn and delivery lock order as existing writers.
- Journal retirement can precede a failed SQL commit. Retry must reuse that proof and preserve unknown labels.
- Reconnect, Stop and terminal-reclaim follow-ups remain outside the demonstrated repair.
