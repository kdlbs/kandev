---
created: 2026-10-06
status: complete
requirements:
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-001
  - REQ-TASKS-MANAGED-CLONE-RELOCATION-002
system_design:
  - ../../specs/tasks/system-design/managed-clone-relocation.md
legacy_specs: []
---

# Implementation Plan: Completed relocation continuity

## Overview

Restore ordinary resume and workspace access after users continue work in a
completed replacement checkout. Apply the correction to every eligible host
Worktree environment, including existing completed records after an upgrade.
Implement manager admission first, then prove the executor and session paths.

The task system owns this repair because its environment owns replacement
publication and conversation continuity. The existing requirement defines safe
reuse. New criterion `AC-TASKS-MANAGED-CLONE-RELOCATION-001.7` makes continuity
after completed relocation explicit. The system design separates completed
records from unfinished transfer proofs.

Confirmed intent: create a fix package for all users. Implementation and live
repair are separate later actions. No material product question remains open.

## Evidence and root cause

Read-only diagnosis on 2026-10-06 identified task
`56245922-751c-4e9f-98b7-1c9ce656dc13` and session
`41d61161-da4d-4ccc-ad57-0aa243d6e9b4`.
Backend diagnostics showed repeated resume and workspace-restore refusals:
`published replacement commit could not be verified`.
The session retained its provider resume token.

Both selected repositories had relocation journals with `state: complete`.
Their current branches and Git common directories matched their records.
Current commits were valid, tracked files were clean, and reflogs showed later
commits. Recorded/current commit prefixes were `253a8dbd`/`e996a232` for Kandev
and `6b73eed1`/`4c5f5e1` for the landing repository.
Neither historical commit was an ancestor of its current HEAD.
An ancestry-only relaxation therefore does not cover this incident.
The diagnostic archive was partial due to its byte limit, but contained the
exact failed requests. Current Git inspection independently established the
commit mismatch. No live records or checkouts were changed.

`matchesPublishedManagedCloneRelocation` includes completed journals.
`verifyPublishedManagedCloneRelocation` unconditionally compares current HEAD
with `record.Head`. Every later commit can therefore make normal recovery
report corruption before agent startup. Both entry paths share the defect.

Smallest reproduction: complete a managed-clone relocation, commit in its
published replacement, reconstruct the manager, and call `AdmitRecovery`.
The current code rejects the valid checkout against the old journal HEAD.

## Scope

### In scope

- State-aware validation of complete versus unfinished relocation journals.
- Existing journal compatibility without manual edits or database migrations.
- Current identity, ownership, claim, and complete-inventory validation.
- Commit, amend, rebase, and local-edit continuity after restart.
- Existing launch, ordinary resume, explicit Resume, and read-only restore paths.
- Real-Git regressions and SQLite-backed session integration evidence.

### Out of scope

- Changes to rendered UI, public controls, copy, or translation catalogs.
- Automatic dirty relocation, branch replacement, or corruption repair.
- Resetting user commits, replaying snapshots, or deleting recovery evidence.
- New journal formats, SQL schemas, runtime flags, or provider adapters.
- Live task repair, production deployment, or release publication in this package.

## Technical approach

### Manager admission

Correct `apps/backend/internal/worktree/managed_clone_relocation_archive.go`.
Separate complete-journal validation from unfinished publication reconciliation.
Keep full selected-slot inspection in
`apps/backend/internal/worktree/recovery_admission.go` and the existing final
lifecycle guard. Do not replace exact equality with an ancestor requirement.
Keep exact HEAD proof for unfinished `materialized` records.

For complete records, current canonical checkout proof owns admission.
Ordinary reuse must preserve work, provider identity, and historical journals.
Handle a matching leftover claim or incomplete companion journal only through
the existing operation fences. Refuse unrelated claims and identity drift.
No clean-tree prerequisite applies solely because a complete journal exists.

### Executor and service integration

Use the repaired manager through `PreflightSessionWorktreeRecovery` and
`ResumeSessionWithOptions`. Prove `ResumeTaskSessionWithOptions`,
`RecoverSessionWithOptions`, and `LaunchSession` with `restore_workspace`.
Test a FAILED session carrying the prior corruption error and retained token.
Successful resume must retain the conversation and retire only its current
matching failure. Read-only restore must launch no provider process.
Change caller production code only if these tests expose a remaining defect.

### Compatibility matrix

| Path or identity | Intended behavior | Evidence or refusal |
| --- | --- | --- |
| GitHub/GitLab managed host Worktree, complete record | Reuse current valid checkout | Provider-parameterized real-Git tests |
| Existing complete records after upgrade/restart | Resume without rewriting old HEAD | Fresh-manager and persisted integration fixtures |
| Commit, amend, or rebase | No equality or ancestry requirement | Non-descendant HEAD cases |
| Local edits, staged/untracked/ignored files | Preserve current content and index | Before/after content and index assertions |
| Materialized, prepared, or blocked operation | Retain existing transfer/refusal rules | Unfinished and blocked regression cases |
| Matching leftover claim or companion journal | Finish only fenced bookkeeping | Crash-window and claim tests |
| Foreign identity, unrelated claim, invalid sibling | Refuse before provider startup | Mixed-inventory and negative cases |
| Local, Docker, SSH, Sprites, Kubernetes executors | Retain existing executor-owned contract | Excluded-executor regressions |

Coverage applies to the existing managed-clone provider shapes.
Unsupported shapes retain current admission behavior.
Linux results do not imply native Windows or macOS execution.

## Tests

| Acceptance criteria | Planned evidence |
| --- | --- |
| `AC-TASKS-MANAGED-CLONE-RELOCATION-001.7` | `TestCompletedRelocationAdmitsCurrentWork` in new `managed_clone_completed_reuse_test.go`: commit, amend, non-descendant rebase, edits, fresh manager, legacy complete JSON |
| `AC-TASKS-MANAGED-CLONE-RELOCATION-001.3`, `.001.6`, `.001.7` | `TestCompletedRelocationRejectsInvalidInventory`: healthy plus invalid/unfinished siblings, clone/branch/owner drift, cancelled inspection |
| `AC-TASKS-MANAGED-CLONE-RELOCATION-001.3`, `.002.4` | `TestCompletedRelocationClaimBookkeeping` and existing published/prepared restart tests: unrelated claims, matching claims, companion crash windows |
| `AC-TASKS-MANAGED-CLONE-RELOCATION-001.1`, `.001.7` | `TestCompletedRelocationExecutorContinuity` in new executor integration file: automatic/manual preflight, current path and retained provider token |
| `AC-TASKS-MANAGED-CLONE-RELOCATION-001.7`, `.002.4` | `TestCompletedRelocationSessionContinuity` and `TestCompletedRelocationExplicitRetryPreservesNewerError`: FAILED retry, same session/conversation, matching error retirement, newer-error preservation, and no startup on refusal |

Task 01 starts with the real-Git failing reproduction.
Task 02 tests must reproduce the same error against the original manager behavior.
Fixtures must not special-case a task ID, journal filename, or provider token.

## End-to-end evidence

Task 02 uses real temporary clones, SQLite canonical inventory, and the real
worktree admission manager through executor and service entry points.
Only external provider startup and readiness events use a recording runtime.
Do not stub `AdmitRecovery` or replace its error with a canned success.
Cover both a single repository and two repositories with mixed states.

The package changes no rendered UI. Backend integration proves the affected
resume and read-only restore outcomes. Existing desktop and phone recovery
controls use those paths without new browser tests or UI previews.

## Companion packages and documentation

The original relocation, legacy-clone resume, recovery convergence, and
workspace-recovery permissions packages retain their recorded implementation
statuses and historical test results. Their scopes do not change.
This package adds completed-record continuity coverage and is complete.

Internal documentation changes belong in the existing requirement/design pair
and this package. Task 02 updates the recovery how-to in
`docs/public/git-operations.md` after its integration tests pass.
Explain normal work after completed relocation and upgrade-and-retry recovery
for versions with the false commit error. Existing Resume and Restore controls
remain the recovery path. Add no manual journal-edit instructions.
The existing relocation ADR retains its identity and preservation boundary.
This correction needs no new ADR.

## Work orders

- [x] [Task 01: Admit current work after completed relocation](task-01-completed-admission.md)
- [x] [Task 02: Prove resume and restore continuity](task-02-resume-restore-continuity.md)

Task 02 depends on Task 01. Execute sequentially.

## Verification results

Task 01 implementation checks passed:

- The new regression failed before the production fix on later commits, amended
  commits, rewritten history, and a missing historical commit object.
- `(cd apps/backend && go test -trimpath ./internal/worktree -run '^TestCompletedRelocation' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath -race ./internal/worktree -count=1)`: passed.
- `git diff --check`: passed.

Task 02 implementation checks passed:

- The new executor and service regressions failed against a disposable checkout
  of the pre-fix `7d55a59950` source with `published replacement commit could not
  be verified`. The temporary checkout was removed after both runs.
- The two-repository invalid-sibling integration test exposed resume request
  setup reconciling a shared Git origin before admission. Resume now validates
  the selected inventory first and reuses that check only while its complete
  selection snapshot stays unchanged; otherwise it repeats final admission.
- Review follow-up kept a persisted legacy empty environment binding in the
  early selected snapshot and moved terminal stale-execution cleanup ahead of
  mutation admission. SQLite/worktree-manager regressions verify normal resume
  preserves the provider token and checkout contents, cleanup permits a
  missing-checkout recovery, and a live sibling remains protected.
- `(cd apps/backend && go test -trimpath -race ./internal/orchestrator/executor ./internal/orchestrator -run 'Test(CompletedRelocation|TerminalResumeCleansStaleExecution|PermissionRecovery|RecoverSessionPermission|ResumeTaskSession_FailedKeepsResumeToken|LaunchRestoreWorkspace_)' -count=1)`: passed after both review fixes.
- `(cd apps/backend && go build -trimpath ./...)`: passed after both review fixes.
- `(cd apps/backend && go test -trimpath -race ./internal/orchestrator/executor ./internal/orchestrator -run 'Test(CompletedRelocation|PermissionRecovery|RecoverSessionPermission|ResumeTaskSession_FailedKeepsResumeToken|LaunchRestoreWorkspace_)' -count=1)`: passed.
- `(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run '^TestValidateLaunchWorkspaceAdmission' -count=1)`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 47 published docs pages validated.
- `python3 scripts/list-docs.py validate`: 356 decisions and 1,406 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- PR documentation coverage preflight with final changed paths: passed with no errors.
- `(cd apps/backend && go build -trimpath ./...)`: passed.
- `git diff --check`: passed.

Design validation on 2026-10-06 passed:

- `python3 scripts/list-docs.py validate`: 356 decisions and 1,406 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Work-order REQ/AC references, design declarations, and package links: valid.
- Documented PR coverage preflight: planned production paths covered by both work orders.
- `git diff --check -- docs/specs docs/plans/completed-relocation-continuity`: passed.
- `git status --short -- docs/plans/completed-relocation-continuity`: three untracked package files.

The package remains unstaged and uncommitted for review.

### Documentation coverage preflight

Run from the repository root to check that the final changed paths link to this
plan and its requirements and system design.

```bash
node <<'JS'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const root = 'docs/plans/completed-relocation-continuity';
const artifacts = fs.readdirSync(root).filter(n => n.endsWith('.md')).map(n => `${root}/${n}`);
const refs = [
  'docs/specs/tasks/requirements/managed-clone-relocation.md',
  'docs/specs/tasks/system-design/managed-clone-relocation.md',
];
const planned = [
  'apps/backend/internal/worktree/managed_clone_relocation_archive.go',
  'apps/backend/internal/worktree/recovery_admission.go',
  'apps/backend/internal/worktree/managed_clone_completed_reuse_test.go',
  'apps/backend/internal/worktree/managed_clone_completed_bookkeeping_test.go',
  'apps/backend/internal/orchestrator/executor/executor_resume.go',
  'apps/backend/internal/orchestrator/executor/executor_worktree_recovery.go',
  'apps/backend/internal/orchestrator/executor/executor_completed_relocation_integration_test.go',
  'apps/backend/internal/orchestrator/session_completed_relocation_integration_test.go',
  'apps/backend/internal/orchestrator/session_launch.go',
  'apps/backend/internal/orchestrator/task_operations.go',
  'docs/public/git-operations.md',
  'docs/public/coverage.json',
];
const fileContents = Object.fromEntries([...artifacts, ...refs].map(f => [f, fs.readFileSync(f, 'utf8')]));
const tracked = execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const untracked = execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0').filter(Boolean);
const changedFiles = [...new Set([...tracked, ...untracked, ...planned])].map(filename => ({ filename, status: 'modified', additions: 1, changes: 1 }));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

## Risks

- Treating every journal as complete can admit an unfinished transfer.
- An ancestry check still rejects legitimate amended or rebased history.
- Skipping current clone or branch identity can weaken workspace isolation.
- Completed journals can coexist with unreleased claims after a crash.
- A passing selected slot must not hide an invalid sibling.
- Updating to the corrected binary is necessary. Old binaries retain the bug.
- Tests with a stubbed admission manager can miss the original defect.
