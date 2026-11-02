---
created: 2026-10-22
status: implemented
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
legacy_specs: []
---

# Implementation Plan: Preserve workspace identity during reuse

## Overview

Restore task resume and workspace access when the persisted managed source
paths and Git registration identify the same physical directories with different
capitalization. The task system owns this repair because task-environment
inventory owns the persisted checkout identity and its admission contract.

This is a regression against existing acceptance criteria
AC-TASKS-MANAGED-CLONE-RELOCATION-001.5 and .001.6. No new product requirement,
architecture decision, migration, or UI contract is needed. The existing
relocation boundary and directory identity helper remain authoritative.

## Evidence and root cause

The DSE-4493 task failed to resume on October 22, 2026 at 09:53:46 Lisbon time.
Backend diagnostics show the same refusal for workspace restoration, cumulative
diff, and commit requests:

```text
recorded source clone identity does not match Git registration
```

The second repository's saved clone path uses an uppercase home-directory
component; Git reports the lowercase spelling. Read-only `os.path.samefile`
checks proved that both clone spellings, and both `.git` spellings, identify
the same directories. Both task checkouts resolve the expected branches and
commits and have clean tracked and ordinary untracked status. Ignored content
was not inventoried, and no relocation or live mutation was attempted.

`inspectRegisteredLegacyClone` already verifies physical Git-directory identity
but calls `verifyRecordedManagedCloneSource`, which compares the resolved paths
as strings. `canonicalExistingPath` uses `filepath.EvalSymlinks`, which preserves
this casing difference on the incident filesystem. The later string comparison
incorrectly rejects the earlier physical identity proof.

A disposable real-Git test, `TestDSE4493DiagnosticRecordedSourceCase`, populated
both recorded source fields with a case alias in `newLegacyReuseFixture`. It
verified that `sameDirectoryIdentity` accepted the alias, then called the real
`Manager.AdmitRecovery` path. The test failed with the incident's exact reason:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree -run '^TestDSE4493DiagnosticRecordedSourceCase$' -count=1)
```

The temporary test was removed after the expected failure. Existing case-alias
coverage leaves the persisted source fields empty, so it misses this failure.

## Scope

### In scope

- Compare each persisted source directory with its verified Git counterpart
  using physical identity after the existing path resolution.
- Cover populated recorded evidence, partial or missing evidence, true clone
  mismatches, and mixed repository inventories.
- Prove the persisted executor preflight path preserves inventory and provider
  conversation metadata when the only difference is path spelling.

### Out of scope

- Live database edits, `.git` rewrites, task restarts, or automatic retries.
- New relocation behavior, changed ownership or origin rules, or lowercasing
  stored paths.
- UI, localization, feature flags, or public API changes.

## Technical approach

In `apps/backend/internal/worktree/managed_clone_relocation.go`, change only
the directory comparisons in `verifyRecordedManagedCloneSource`. Resolve both
recorded fields as today and use the existing `sameDirectoryIdentity` helper
for the source clone and common directory independently. Keep errors for failed
resolution, partial evidence, and different physical directories. Preserve the
two-empty-field legacy path.

Both unchanged-source reuse and relocation already call this helper. Keep
provider-origin, branch registration, managed-root containment, slot ownership,
and all-selected-slot validation in their current callers. Do not broaden
other path comparisons without a separate reproduction.

| Shape | Behavior | Evidence |
| --- | --- | --- |
| GitHub/GitLab host worktree with populated case aliases | Reuse proven physical clone | Real-Git admission and SQLite executor tests |
| Linked or main checkout using either recognized legacy layout | Same directory proof | Real-Git admission matrix |
| Existing alias spelling with two empty recorded fields | Existing Git proof | Existing legacy alias regressions |
| Missing or partial evidence, wrong clone or wrong common directory | Refuse without mutation | Portable helper and admission tests |
| Physically distinct paths differing only in case | Refuse on a case-sensitive filesystem | Conditional filesystem test |
| Healthy alias slot plus invalid selected sibling | Refuse the whole admission | Mixed-slot real-Git test |
| Other providers, local and remote executors | Existing handling | No changed routing or proof construction |

## Tests

Add `managed_clone_recorded_source_identity_test.go` alongside the worktree
implementation. Use `TestRecordedManagedCloneSourceIdentity` for the portable
proof matrix, `TestManagerAdmitRecoveryReusesRecordedSourceCaseAlias` for
case-insensitive real-Git admission, and
`TestManagerAdmitRecoveryRejectsRecordedSourceAliasWithInvalidSibling` for the
universal inventory guard. Reference .001.5 on valid reuse and .001.6 on refusals.

Populate recorded evidence explicitly; varying only `RepositoryPath` repeats
the existing incomplete regression. Test aliasing of either recorded field and
both fields, and preserve current source, HEAD, branch, index, file bytes,
inventory, and absent destination. Skip only filesystem-dependent subcases on
unsupported filesystems; portable rejection tests must always execute.

## End-to-end evidence

Extend `executor_legacy_clone_recovery_test.go` with
`TestPreflightSessionWorktreeRecoveryReusesRecordedSourceCaseAlias`. Seed real
Git, SQLite-backed repository/environment records, and both persisted source
fields with the alternate casing. Invoke
`Executor.PreflightSessionWorktreeRecovery` through the real cloner proof and
manager admission. Compare persisted session metadata, state, and complete
inventory before and after. No provider subprocess is needed for this admission
boundary. Browser coverage is unnecessary because no rendered flow changes.

## Work orders

- [x] [Task 01: Compare recorded sources by directory identity](task-01-compare-directory-identity.md)

- [x] [Task 02: Preserve checkout identity after base retargeting](task-02-retarget-worktree-reuse.md)

## Verification results

Diagnosis: read-only live evidence and a failing disposable admission test
confirm the root cause. Both work orders are implemented after the explicit user request.
Permanent regression tests now cover both diagnosed failures.

Design-package validation on October 22, 2026:

- `python3 scripts/list-docs.py validate`: passed, 396 decisions and 1701
  specifications validated.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- Local `validateCoverage` preflight from `.github/scripts/pr-docs.cjs`, supplied
  with the planned production path and new work order: covered, no errors.
  This validates prospective artifact traceability, not implemented code.
- Package Markdown link validation: all targets exist inside the repository.
- `git diff --check`: passed. Final status contains only the requirement link,
  design clarification, and new plan/work-order package; nothing is staged.
- Temporary diagnostic test and extracted logs removed. The original private
  diagnostic ZIP remains under the workspace's `.kandev/diagnostics/` directory.
  No live task, database, Git registration, runtime, or provider token was changed.

## Risks

- Case-dependent positive coverage requires a case-insensitive filesystem;
  Linux must retain portable negative and empty-evidence coverage.
- Replacing path checks with string case folding would accept different
  directories on case-sensitive systems and is forbidden.
- A valid slot must not hide another invalid repository in the task.
- This package does not repair or restart the running installation. After the
  corrected binary is installed, verify recovery through the normal task action;
  no manual stored-path repair should be necessary for the proven casing case.

## Documentation impact

Updated the public Git operations how-to guide to describe checkout continuity
after comparison-base retargeting. Internal design and delivery records capture
both repairs. No command, recovery action, setting, or public term was added.
The existing unchanged-source and relocation packages remain compatibility
evidence; their completed scopes and results are unchanged.

## Authorized scope extension: base retargeting

The user explicitly requested implementation of both diagnosed issues and PR
delivery through merge on October 22. Execute task 01, then task 02, in this
session. Use the `ghp` account-switch alias, verify the authenticated actor, and
follow the normal PR review and merge gates. No delegation or live data edits.

The second reproduction reaches `prepareResumeRepositorySettings` with a
retargeted task base of `main` and a healthy canonical row still keyed by its
original parent. Attach-only reuse fails before lifecycle admission. The old
slug is physical checkout identity; the changed comparison base must not
replace that identity. Preserve the canonical key for an unambiguous existing
slot while retaining the updated base for diff calculation. Explicit checkout
branches and multiple possible slots keep exact branch selection. No bulk slug
rewrite, persistence migration, automatic state reset, or Git mutation is needed.

Task 02 names its own exact verification commands and regression boundaries.
Its backend executor integration provides end-to-end admission evidence; no UI
markup, API, or runtime flag changes are planned.

## Implementation verification

Both work orders completed sequentially in the primary conversation.
The path fix replaces only the two recorded-directory comparisons with the
existing filesystem identity helper. The retargeting fix pins the recorded key
for an unambiguous existing slot owned by the same task before inventory
validation and worktree reuse. Explicit checkout selection, foreign owners,
ambiguous logical/physical slots, placements, invalid rows, and conflicting
worktree IDs retain their refusal semantics. There are no schema or live-state
repairs.

Final combined command:

```bash
(cd apps/backend && TMPDIR=/private/tmp go test -trimpath -tags fts5 ./internal/worktree ./internal/orchestrator/executor -run 'RecordedManagedCloneSourceIdentity|RecordedSourceCaseAlias|RecordedSourceAliasWithInvalidSibling|LegacyClone|LegacyReuse|RegisteredLegacy|Retarget|CanonicalWorktreeIdentity|ReuseEnvironmentInventory|PrepareResumeRepositorySettings|MultiBranch' -count=1)
```

Passed: Worktree 19.052s, executor 11.861s. `TMPDIR=/private/tmp` is the
canonical macOS test directory for this run; work-order commands resolve the
host temporary directory portably.

The managed clone/relocation compatibility command passed in 50.540s, and the
manager required-reuse command passed in 5.017s. Default macOS temporary-path
failures were reproduced with original production code using a Go overlay:
completed-relocation commit reuse, permission-only blocked retry, and clean
managed relocation. These were fixture path-alias failures and passed with the
canonical temporary directory.

Catalog validation, specification lint, 36 specification-linter tests,
63 public-doc tests, validation of 54 public pages, documentation coverage for
both work orders, and diff checks passed. Final changed-package lint and normal
commit-hook results follow in the delivery evidence. CI, automated review, and
merge remain pending until the published head passes their gates.

Final changed-package Go lint passed with zero issues. Normal commit hooks and
published-head CI/review evidence are recorded by the delivery session.

## PR remediation

Initial-head backend CI identified inventory-admission/repair receipt failures.
Canonical key preservation now requires the existing row's complete physical
identity; incomplete rows retain guarded recovery. Explicit repair still checks
the requested key before creating or replaying its receipt. Added missing
identity negatives and the review-requested branch-path propagation assertions.
The combined inventory/attestation/retargeting scope passed in 21.127s. Mutation
proof confirmed the branch-path assertion catches omitted propagation.

The broader local executor run identified four pre-existing macOS fixture
failures. A Go overlay restoring pre-PR production code reproduced all four.
The remaining full executor scope is rerun separately and Linux CI retains the
unfiltered suite. Exact-head CI/review/merge evidence remains external until the
fixup is pushed and its new checks finish.

The final full executor run passed in 94.410s with only the four baseline macOS
fixture cases excluded. The original-source baseline reproduced those cases;
none was hidden from the GitHub Linux gate.

The second failing backend shard exercised the same explicit-repair boundary at
the orchestrator layer. Its two focused recovery tests passed after the fix
(1.821s). Changed executor-package lint passed with zero issues.
