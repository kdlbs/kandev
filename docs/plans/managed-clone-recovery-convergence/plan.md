---
created: 2026-10-02
status: done
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-003
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
legacy_specs: []
---

# Fix plan: Resume and workspace recovery convergence

## Overview

Restore the existing recovery contract for tasks whose source clone actually changed.
The task system owns this repair because its environment owns worktree continuity.
Keep the current file-preserving relocation machinery and correct its entry paths.
Implement the work orders sequentially.

The [requirements](../../specs/tasks/requirements/managed-clone-relocation.md)
already define the required behavior. This package adds no requirement IDs.
The [design clarification](../../specs/tasks/system-design/managed-clone-relocation.md#inspection-contention-and-explicit-preflight)
defines admission ordering and durable error publication.

## Confirmed failure and reproduction

The reported task is `56245922-751c-4e9f-98b7-1c9ce656dc13`.
Its session is `41d61161-da4d-4ccc-ad57-0aa243d6e9b4`.
Its selected environment is `9781b591-86ac-4580-847d-db74459d59d7`.
Read-only investigation found these conditions:

- Both linked checkouts exist and Git can read their branch and common directory.
- Their common directories belong to legacy shared clones. The repository rows
  select distinct workspace-scoped clones of those same repositories.
- The landing checkout has a tracked modification in `next-env.d.ts`.
  Both checkouts contain ignored build or dependency files.
- The session remains CANCELLED. Its stored generic validation error dates from
  September 29. It has no current typed relocation error.

Backend logs on October 2, 2026, in Europe/Lisbon show the failing routes:

| Time | Route | Result |
| --- | --- | --- |
| 00:18:19 | Background workspace reconstruction | Starts inspecting the selected environment |
| 00:18:21 and 00:18:23 | `session.recover`, action `resume` | `selected worktree is already being recovered` |
| 00:18:26 and 00:18:29 | `session.launch`, intent `restore_workspace` | Explicit file-preserving recovery required |

The diagnostic bundle was partial and reported historic file-sink losses.
The retained backend file supplied these events. There was no durable recovery
claim remaining at inspection time. This is not evidence of lost task files.

The root cause has two parts. `lockRecoverySlots` allows waiting only for
authorized dirty relocation, so ordinary manual preflight loses to background inspection.
`launchRestoreWorkspace` and background reconstruction return the typed refusal without
publishing it through the durable session error path. The restore handler also
omits structured relocation details, and the restore hook ignores that category.
The old generic card therefore keeps offering actions that cannot repair this mismatch.

Reproduce with disposable clones, not the live task. Create a cancelled session
with two legacy linked checkouts and a generic historical error. Change the
registered sources to workspace clones. Add tracked and ignored content.
Hold the inspection barrier while submitting one Resume request. Release it
and assert the current relocation action becomes available without another click.
Separately reproduce restore refusal without an earlier Resume request.

## Scope

### In scope

- Bounded, cancellable waiting for authenticated manual recovery preflight.
- Distinct inspection contention without weakening replacement authority.
- One durable relocation error across Resume, Restore, and background reconstruction.
- Existing stamped relocation confirmation, desktop and phone convergence, and reload.
- Real-Git, SQLite, conditional PostgreSQL, unit, and focused browser regressions.
- Public recovery guidance during implementation.

### Out of scope

- Live database edits, manual `.git` rewrites, or repairing the reported task during planning.
- Automatic dirty relocation, bulk migrations, new runtime flags, or schema changes.
- Stopping consumers, expiring claims, or weakening workspace/provider identity checks.
- New branch-loss behavior, expanded filesystem support, or provider conversation replacement.
- Removal of retained originals or snapshots, and broad local verification.

## Technical approach

### 1. Manual admission ordering

Separate the wait policy from `RelocateDirty` in `RecoveryAdmissionRequest`.
Set it only from `PreflightSessionWorktreeRecovery` on the manual recovery path.
The wait is at most 15 seconds and remains outside lifecycle singleflight.
Lifecycle callers still refuse occupied locks promptly. Capture the selected
session binding, owner generation, and complete active repository-slot inventory
before waiting. After acquiring the original worktree locks, compare a fresh
snapshot before resolving or inspecting those slots. Reject drift and do not add
newly discovered slots to the current admission. Do not use a contention response
as mutation authority.

Add a typed internal contention error in `worktree/errors.go`.
Map an expired manual wait to a sanitized conflict response. Do not reuse the
startup-reconciliation guard's restart-specific wording. No client loop retries recovery.

### 2. One durable projection

Move relocation error persistence from the orchestrator-only helper into a
task-service operation shared by manual preflight and workspace reconstruction.
Inject a narrow reporter into lifecycle through backend composition.
Capture the expected session and complete selected inventory identity before
inspection. Compare worktree membership, checkout identity, and registered
repository identity/path in the same transaction that writes or reuses the
durable error stamp.
Reuse the metadata CAS machinery for an absent-or-stamped conditional write.
Extend its predicates without a schema migration, preserving a successor session,
newer error, environment transfer, and unrelated metadata.

Repeated refusal for unchanged inventory retains one active stamp.
Publish `TaskSessionErrorChanged` only after a successful new write.
Keep cancelled sessions cancelled and preserve provider tokens and retained history.
`wsLaunchSession` uses the existing relocation conflict response for restore.
Do not add a separate persisted error category for background readers.

### 3. Shared presentation and acceptance evidence

`use-session-recovery-actions.ts` consumes restore relocation details with the
same operation fence used for Resume. Reuse `selectActiveSessionRecovery`, the
bootstrap card, the relocation confirmation, and the existing action test IDs.
The published record must remove ineffective actions before and after reload.
Maintain one control surface for the current failure.

Extend `session-resume-recovery` fixtures with a second selected repository and
legacy generic-error setup. Do not seed the desired relocation category as proof.
Run the current single-repository cases alongside the new multi-repository cases.
Backend barriers prove contention deterministically. Browser tests prove the user outcome.

| Shape | Intended behavior | Evidence and fallback |
| --- | --- | --- |
| GitHub/GitLab host Worktree, changed source | Existing relocation proof and explicit dirty action | Real-Git admission matrix; unverifiable identity refuses |
| Registered source unchanged | Reuse without transfer | Existing legacy-clone tests |
| Manual preflight contends with inspection | Wait outside singleflight, then classify current inventory | Barrier tests; deadline returns conflict without mutation |
| Live consumer or durable competing claim | Existing refusal remains authoritative | Busy/claim regressions; no forced stop or claim expiry |
| Local, remote, unsupported provider, repo-free | Existing executor handling | Scope tests; no host recovery shortcut |
| Multiple slots with a dirty or invalid sibling | Every selected slot must pass | Mixed-slot tests; no partial agent startup |

## ASCII UI preview

`UI-01` enters through the current task session recovery card.
The screenshot supplies the before state. The after state reuses shipped controls.

```text
UI-01 desktop | before: stale generic workspace error
+----------------------------------------------------------------+
| Required worktree is unavailable for reuse                      |
| [Resume] [Start fresh session] [Restore read-only workspace]     |
+----------------------------------------------------------------+

UI-01 desktop | after: current verified dirty relocation error
+----------------------------------------------------------------+
| Workspace needs repair                                         |
| Files remain in the original checkout.                         |
| [Move files and resume] [Technical details]                     |
+----------------------------------------------------------------+
| Confirmation dialog                                            |
| Original and snapshot remain. Staging choices do not transfer.  |
|                                     [Cancel] [Move and resume]  |
+----------------------------------------------------------------+
```

```text
UI-01 phone | current recovery card in the task view
+----------------------------------+
| Workspace needs repair           |
| Files remain in the old checkout.|
| [Move files and resume]           |
| [Technical details]              |
+----------------------------------+

Inset confirmation drawer
+----------------------------------+
| Original and snapshot remain.    |
| Staging choices do not transfer. |
| [Cancel]                         |
| [Move and resume]                |
+----------------------------------+
```

`UI-02` is an ordinary Resume waiting for inspection. The action group remains
disabled during that request. Use one localized live status, such as
"Checking workspace...". A deadline failure retains the source card and offers
a manual retry; it must not claim corruption or open relocation confirmation.
The wording is illustrative. Reuse existing locale keys where their meaning fits.

The action order, staging disclosure, and one recovery surface are required.
ASCII spacing is illustrative. The existing phone recovery card and
`managed-clone-relocation-confirmation.tsx` provide the mobile exemplar.
Desktop ordinary controls retain 28-pixel sizing. Phone controls measure at least
44 pixels. The phone drawer keeps one internal vertical scroll owner and clears
the safe area. Cancel returns focus to the card. No page horizontal overflow.
Preview coverage: AC 002.1, 002.3, 003.1, and 003.2 of the linked requirement.

## Tests

Implemented regressions cover manual recovery admission, conditional error
projection from all three entry paths, durable compare-and-set behavior, and
Restore response ordering. The regression files are listed below.

| Criteria | Regression and test file |
| --- | --- |
| 001.1, 001.3, 002.1 | `TestManualRecoveryPreflightRequestsInspectionWaitWithoutDirtyAuthorization`, `executor_manual_recovery_contention_test.go`; real held-lock behavior in `recovery_admission_wait_policy_test.go` |
| 001.3, 001.4, 002.4 | Cancellation, deadline, added-sibling/environment/generation drift, and lock-release cases in `recovery_admission_wait_policy_test.go` |
| 002.1, 003.1 | `TestWorkspaceRecoveryProjectsRelocationErrorFromEveryEntryPoint`, new `workspace_recovery_projection_test.go` under task service |
| 002.4, 003.1 | `TestCommitWorkspaceRecoveryErrorPreservesSuccessorState` and selected inventory drift/stamp-reuse cases, repository CAS regression files |
| 003.1 | Restore structured response and hook tests, plus CANCELLED generic-to-typed model tests |
| 002.2, 002.3, 003.2 | Existing real-Git relocation tests and desktop/phone recovery E2E |

## E2E coverage

The new legacy multi-repository scenarios live in
`apps/web/e2e/tests/session/multi-repo-session-resume-recovery.spec.ts` and
`apps/web/e2e/tests/session/mobile-multi-repo-session-resume-recovery.spec.ts`.
The desktop case enters through Resume; the phone case discovers the error
through Restore first. They verify both repository inventories, original and
replacement bytes, ignored sentinels, recorded commits, reload-stable recovery
state, explicit confirmation, and a later response in the existing task/session.
The hook regression covers
a late restore response without replacing a newer error.

The pre-existing single-repository dirty-relocation scenarios remain in their
original desktop and phone specs and were rerun with the guarded runner.
Deterministic lock contention remains covered by backend barriers, not browser sleeps.

## Work orders

- [x] [Task 01: Admit manual recovery after inspection](task-01-manual-admission.md)
- [x] [Task 02: Publish current workspace recovery errors](task-02-error-projection.md)
- [x] [Task 03: Verify recovery on desktop and phone](task-03-recovery-surfaces.md)

Dependencies: Task 01, then Task 02, then Task 03. Each order includes exact checks.

## Verification results

Implementation and package checks passed on October 2, 2026:

- Backend projection, admission, and compare-and-set regressions passed under
  `go test -race`; SQL guard and SQLite store conformance passed. PostgreSQL cases
  were skipped because `KANDEV_TEST_POSTGRES_DSN` was unavailable.
- Six focused frontend Vitest files passed (100 tests) in the implementation run;
  web typecheck, full lint, `i18n:check`, and `i18n:ratchet` passed. The review
  follow-up focused set passed 95 tests across six files after rebasing, and repeated typecheck,
  full lint, and both i18n checks passed.
- Guarded desktop and phone multi-repository E2E scenarios passed, as did the
  existing single-repository dirty-relocation scenarios on both platforms. Each
  guarded run rebuilt the backend and Vite assets.
- Public documentation tests passed (62 tests), all 47 published pages validated,
  and the documentation coverage preflight passed for changed production paths.

- `python3 scripts/list-docs.py validate`: 339 decisions and 1,283 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Existing file paths and documentation links passed inspection. `git diff --check` passed.

Review follow-up: All reported inventory-CAS, stale-execution expectation, and
post-wait snapshot findings were fixed in the existing work orders. The final
race-enabled backend regressions, SQL guard, specification validation, and
  backend build passed. Desktop and mobile multi-repository browser regressions
  passed after fresh host builds. Rebased specification validation passed at 343
  decisions and 1,309 specifications; 36 linter tests passed. PostgreSQL cases
  compiled but were skipped because `KANDEV_TEST_POSTGRES_DSN` was unavailable.
The follow-up also binds the selected environment onto a prepared session before
capturing its recovery snapshot; the new regression failed before the fix and
passed with the recovery-focused race-enabled backend tests.

### Documentation coverage preflight command

This command includes untracked work orders without staging the package. It was
also used with the planned production paths to verify source traceability:

```bash
node <<'JS'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const root = 'docs/plans/managed-clone-recovery-convergence';
const artifacts = fs.readdirSync(root).filter(n => n.endsWith('.md')).map(n => `${root}/${n}`);
const references = ['docs/specs/tasks/requirements/managed-clone-relocation.md', 'docs/specs/tasks/system-design/managed-clone-relocation.md'];
const fileContents = Object.fromEntries([...artifacts, ...references].map(p => [p, fs.readFileSync(p, 'utf8')]));
const tracked = execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const changedFiles = [...new Set([...tracked, ...untracked])].map(filename => ({ filename, status: 'modified', additions: 1, changes: 1 }));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

For package authoring, supply the planned production paths as modified files
to the same API. A documentation-only exemption does not prove source traceability.

## Risks

- Waiting inside lifecycle singleflight can deadlock admission. Only manual outer preflight waits.
- Ignored dependency trees make inspection slow. The wait is bounded. Large content remains subject to existing snapshot limits.
- An unconditional metadata write can replace a successor error. The conditional projection must cover absent and stamped legacy states.
- Multiple repository publications are not one filesystem transaction. Preserve completed progress and every original after a later failure.
- PostgreSQL evidence requires `KANDEV_TEST_POSTGRES_DSN`. Record an unavailable environment explicitly; do not claim that coverage passed.

## Companion packages and delivery

The [original relocation package](../managed-clone-relocation/plan.md) and
[unchanged-source repair](../legacy-clone-resume/plan.md) remain completed evidence.
Retain their scenarios and results. This package records its own regressions and results.
The existing relocation ADR remains authoritative; no new ownership boundary is proposed.

After implementation and the focused checks, the reported live task can validate
the shipped recovery flow through a separately authorized operational recovery.
This package does not grant permission to edit its database, move files, or start an agent.
