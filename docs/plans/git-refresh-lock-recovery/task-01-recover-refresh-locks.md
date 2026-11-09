---
id: "01-recover-refresh-locks"
title: "Recover brief base-refresh lock contention"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
  - REQ-PLATFORM-GIT-SUBPROCESS-ADMISSION-002
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.10
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.13
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.14
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.29
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.30
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.31
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.32
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.33
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.34
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.3
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.4
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.5
  - AC-PLATFORM-GIT-SUBPROCESS-ADMISSION-002.6
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
  - ../../specs/workspaces/system-design/worktree-base-refresh-recovery.md
  - ../../specs/platform/system-design/git-subprocess-execution.md
---

# Task 01: Recover brief base-refresh lock contention

## Summary

Add bounded lock retries to the existing backend base-fetch and fast-forward-pull paths.
Recover a partial fast-forward through normal Git re-execution and preserve safe failure details after exhaustion.
Update the existing public Git guidance in the same implementation pass.

## In scope

- One private retry helper with the recovery design's attempt, delay, admission, and deadline rules.
- Integration into `pullBaseBranchWithPolicy` and `pullCurrentBranchOrFallback`.
- Known-cause classification, exhausted-lock error propagation, and fixed diagnostic fields.
- Scripted runner tests and real Git tests through `Manager.Create`.
- Public guidance for brief contention, persistent locks, and partial updates.

## Out of scope

- Lock removal, force updates, resets, process inspection, or a second Git repair algorithm.
- Changes to the shared subprocess API, provider callback retries, or other Git operations.
- New settings, persistence, UI controls, event types, metrics, and automatic task restarts.
- Changes to required PR freshness, credential selection, or base substitution.

## Acceptance

1. Transient lock failures recover within the designed deadline and attempt limit. Cancellation releases resources and starts no later attempt.
2. A real failed fast-forward recovers through the retried command. Independent edits survive, and unresolved required refresh creates no usable worktree.
3. Exhausted errors retain fixed lock context and error identities without raw output. Non-lock failures and all existing fallback policies remain compatible.

## TDD and regression evidence

First add the planned tests from the [test matrix](plan.md#tests).
Run the transient-lock and partial-fast-forward regressions against the unchanged implementation.
Record the expected first-error failure before adding the helper.

For the real fixture, use a local bare origin and a checkout with an available fast-forward.
Hold only the fixture's `refs/heads/main.lock` during the first pull.
Record the old branch ref and advanced index before releasing that lock.
Use a deterministic wrapper or command barrier instead of a timing race.
The second attempt must complete the same normal pull and create a usable task worktree.

Include staged and unstaged independent changes, a permanent lock, a non-lock error after a lock error, and cancellation during backoff.
Keep Git fixture configuration isolated from host hooks and commit signing.
Reuse the existing fake-Git helper's platform limits rather than claiming unsupported Windows script coverage.

## Verification

Run from the repository root after implementation:

```bash
(cd apps/backend && go test -trimpath -race ./internal/worktree -count=1)
bash scripts/lint-go-changed apps/backend/internal/worktree/manager_git.go apps/backend/internal/worktree/refresh_diagnostics.go apps/backend/internal/worktree/refresh_lock_retry.go apps/backend/internal/worktree/refresh_lock_retry_test.go apps/backend/internal/worktree/manager_refresh_lock_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Run the coverage preflight from the repository root with the actual implementation diff:

```bash
node - <<'JS'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const documents = [
  'docs/specs/workspaces/requirements/worktree-base-refresh.md',
  'docs/specs/platform/requirements/git-subprocess-admission.md',
  'docs/specs/workspaces/system-design/worktree-base-refresh.md',
  'docs/specs/workspaces/system-design/worktree-base-refresh-recovery.md',
  'docs/specs/platform/system-design/git-subprocess-execution.md',
  'docs/plans/git-refresh-lock-recovery/plan.md',
  'docs/plans/git-refresh-lock-recovery/task-01-recover-refresh-locks.md',
];
const paths = [...new Set([
  ...execFileSync('git', ['diff', '--name-only', 'HEAD'], { encoding: 'utf8' }).split('\n'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' }).split('\n'),
])].filter(Boolean);
const result = validateCoverage({
  changedFiles: paths.map(filename => ({ filename, status: 'modified' })),
  fileContents: Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify({ status: result.status, errors: result.errors }));
if (!result.ok || result.status !== 'covered') process.exitCode = 1;
JS
```

This implementation check must report `covered`, not the documentation-only `exempt` result.

The package changes no rendered UI. Real Git materialization tests provide end-to-end evidence without a browser flow.
Record every required command and result before changing this work order to `done`.
The implementation removed the draft-amendment markers and promoted the new recovery design to `current`.
Keep the existing active requirement documents active.

## Files likely touched

- `apps/backend/internal/worktree/manager_git.go`
- `apps/backend/internal/worktree/refresh_diagnostics.go`
- `apps/backend/internal/worktree/refresh_lock_retry.go` (new)
- `apps/backend/internal/worktree/refresh_lock_retry_test.go` (new)
- `apps/backend/internal/worktree/manager_refresh_lock_test.go` (new)
- `docs/public/git-operations.md`
- This plan, work order, and the linked requirement/design amendment statuses.

Lifecycle error wrapping already uses `%w` and needs no planned production change.
If evidence requires another file, keep its responsibility within this work order and update the verification list.

## Dependencies

None. This is the only implementation work order.

## Risks

- Broad substring matching can retry an unrelated failure. Include negative classification cases.
- A new per-attempt deadline can multiply the execution budget. Retain the first admitted deadline.
- An exhausted pull can leave partial Git state. Preserve it without a rollback promise.

## Parallelism

`sequential`

## Inputs

- [Base-refresh requirement](../../specs/workspaces/requirements/worktree-base-refresh.md), especially criteria .29 through .34.
- [Recovery design](../../specs/workspaces/system-design/worktree-base-refresh-recovery.md).
- [Managed Git execution](../../specs/platform/system-design/git-subprocess-execution.md).
- [Retry boundary decision](../../decisions/2026-11-08-worktree-refresh-lock-retries.md).
- `manager_pull_fallback_test.go`, `manager_stacked_pr_base_fallback_test.go`, and `refresh_diagnostics_test.go`.
- `internal/common/subproc/shared.go` and its classified admission tests.

## Results

Implemented on 2026-11-08.

- RED: `TestCreateWorktree_RefreshLockReleased` and
  `TestCreateWorktree_RefreshLockPreservesEdits` failed against the unchanged
  refresh path. `Create()` used the local base after the first lock failure, so
  local `main` stayed at the old head instead of the retried fast-forward.
- GREEN: added the bounded helper, partial-fast-forward recovery, fixed
  exhausted-lock error preservation, diagnostic attempt count, and the public
  Git guidance.
- Review fixes: retry recognition now requires a concrete `File exists`,
  running-process, or ref compare-and-swap cause in a Git error line, so wrapped
  permission, read-only, and ref-namespace failures keep their original cause
  and single-attempt behavior. Exhausted wording is ownership-neutral.
  Cancellation is triggered deterministically at the backoff log barrier, and
  the budget test holds a Git slot, observes queued admission, and proves the
  first command still receives its full post-admission budget.
- `(cd apps/backend && go test -trimpath -race ./internal/worktree -count=1)`:
  passed.
- `bash scripts/lint-go-changed ...` for the five changed Go files: no issues.
- `python3 scripts/list-docs.py validate`: 440 decisions and 1,947
  specifications validated.
- `python3 scripts/lint-spec-files.test.py`: all 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed.
- `node scripts/validate-public-docs.mjs`: 55 published docs pages validated.
- Documentation coverage preflight: `covered` with no errors.
- `git diff --check`: passed.
- No rendered UI changed. Real Git materialization tests through
  `Manager.Create` provide the end-to-end evidence.

Files changed: `manager_git.go`, `refresh_diagnostics.go`,
`refresh_lock_retry.go` (new), `refresh_lock_retry_test.go` (new),
`manager_refresh_lock_test.go` (new), `docs/public/git-operations.md`, and the
linked requirement/design/plan statuses.
