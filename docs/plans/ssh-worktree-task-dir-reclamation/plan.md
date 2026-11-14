---
created: 2026-11-12
updated: 2026-11-14
status: implemented
requirements:
  - REQ-SSH-TASKDIR-RECLAMATION-001
  - REQ-SSH-TASKDIR-RECLAMATION-002
  - REQ-SSH-TASKDIR-RECLAMATION-003
  - REQ-SSH-TASKDIR-RECLAMATION-004
  - REQ-SSH-TASKDIR-RECLAMATION-005
  - REQ-SSH-TASKDIR-RECLAMATION-006
  - REQ-SSH-TASKDIR-RECLAMATION-007
system_design:
  - ../../specs/executors/system-design/remote-task-directory-reclamation.md
legacy_specs: []
---

# Implementation Plan: Worktree-Aware SSH Task-Directory Reclamation

## Overview

Correct [issue #5295](https://github.com/kdlbs/kandev/issues/5295) so an enabled
SSH profile can reclaim clean linked worktrees with ignored build output.
Unrelated branches and stashes in a retained shared repository must not block
the task. Git must remove each linked registration together with its directory.

The executor system owns this capability because it defines remote storage
boundaries and executor-specific cleanup safety. Task cleanup remains its caller.
There is no rendered UI change.

Default SSH preparation uses `defaultSSHPrepareScript` in
`apps/backend/internal/agent/runtime/lifecycle/default_scripts.go`: it initializes
or reuses the root checkout and fetches the configured branch. It creates no
linked worktree. Linked layouts come from custom scripts or existing repositories.
The depth-1 clone at line 191 belongs to Docker, not SSH. Shallow standalone
checkouts still need compatibility coverage because custom or retained checkouts
can use them.

Implement and prove the complete backend path in task 01. Update public
documentation and delivery evidence in task 02. Both tasks run sequentially.
The user explicitly authorized implementation on 2026-11-14. The work orders
remain sequential. No commit, push, or PR is authorized.

## Evidence and root cause

The investigation used checkout `94e35cd2234608185643c91e9f1aeebf1f1956ca`.
The remote `main` head matched that commit on 2026-11-12. No PR referencing the
issue appeared in the issue-body search during this investigation.

`SSHTaskDirReclaimer.probeCheckout` always calls the same repository-wide
commands, regardless of repository topology. `sshReclaimStatusCmd` explicitly
requests ignored entries. `isKandevRuntimeStatus` exempts only ignored `.kandev/`
data. `remove` always calls `sshReclaimRemoveCmd`, which recursively removes the
directory without updating Git's worktree inventory.

A temporary reproduction used a local bare remote, a primary checkout, and two
linked worktrees on Git 2.47.3. It produced these observations:

| Observation | Current command | Result |
| --- | --- | --- |
| Ignored build file only | `status --porcelain --untracked-files=all --ignored` | `!! node_modules/output.js` |
| Same working tree | `status --porcelain --untracked-files=all --ignored=no` | Empty |
| Unpublished sibling commit | `rev-list --count HEAD --branches --not --remotes` | 1 commit |
| Published task HEAD | `rev-list --count HEAD --not --remotes` | 0 commits |
| Unpublished task HEAD | `rev-list --count HEAD --not --remotes` | 1 commit |
| Sibling stash, viewed from task | `stash list --format=%gs` | `On other/private: sibling stash` |
| Additional task stash | `stash list --format=%gs` | Also contains `On task/clean: task stash` |
| Recursive filesystem removal | Existing removal behavior | Stale worktree registration remains |
| Targeted Git removal without force | `git --git-dir=<common-dir> worktree remove -- <checkout>` | Ignored-only directory and registration disappear |
| Locked linked worktree | Same Git removal | Git refuses removal and preserves the directory |

The Git removal also preserved distinct sibling content, refs, and a shared
stash. Both named and default `WIP on <branch>: ...` stash descriptions appeared
in the fixtures. All temporary reproduction files were removed.

Additional disposable Git 2.47.3 checks on 2026-11-14 established:

- The exact `--git-dir=<common-dir>` removal command works with a non-bare shared
  repository and its main worktree intact. From a sibling working directory,
  through an outer shell and `bash -lc`, it removes only the absolute target.
  The main worktree, sibling, HEAD, and shared refs remain intact.
- `worktree list --porcelain -z` exposes the target's `locked` attribute and
  optional reason. Known locks can therefore produce safety skips before removal.
- Absolute Git paths and `pwd -P` identify the same physical task through a
  symlinked workspace root. A task path with an apostrophe survives shell quoting.
- A real `file://` depth-1 clone reports `is-shallow-repository=true` and an
  all-branch publication count of zero for its pushed HEAD.
- `submodule status --recursive` detects initialized and uninitialized submodules.
  Git refuses removal of a clean linked worktree with an initialized submodule.
- The old and new status commands both report a dirty submodule under ordinary
  configuration. Both hide it with `submodule.<name>.ignore=all`; `--ignored=no`
  is not the cause. The separate submodule-preservation probe avoids that dependency.

These checks exercised Git commands, not a completed Kandev implementation.
The fixtures were removed automatically after the experiments.

These findings confirm the reported mechanisms. They do not diagnose the 40
`probe_failed` events in the report, or validate the reporter's live host.

The old implementation conforms to its original requirements. The revised
[requirements](../../specs/executors/requirements/remote-task-directory-reclamation.md)
and [ADR](../../decisions/2026-11-12-scope-ssh-reclamation-to-deleted-storage.md)
record the intended compatibility change. This is not a claim that ignored
files are recoverable from Git.

## Scope

Three changes belong in the existing SSH reclaimer:

1. Stop requesting ignored files from `git status`.
2. Scope commit and stash checks to the task for linked worktrees.
3. Use Git to remove linked worktrees and their registrations.

Standalone clones retain checks across all local branches and stashes. Preserve
unsupported layouts and standalone clones with additional registered worktrees.
Keep the existing profile opt-in, ownership, path, timeout, and cancellation guards.
Known locks and all registered submodules are safety skips before mutation.

No sweep, deployment, new setting, schema change, recovery protocol, or broader
executor support belongs in this correction. Unexplained probe failures remain
outside this package.
Child-repositories-only roots also remain out of scope. Current SSH launch
verification requires a primary checkout at the task root when a repository URL
exists. Legacy/custom roots without that checkout remain preserved. This is a
possible probe-failure condition, not a diagnosis of the reported 40 failures.

## Technical approach

### Small changes in the current reclaimer

Keep the public `Reclaim` signature, existing `sshRemoteRunner` seam, and cleanup
job. Implement small helpers in `executor_ssh_reclaim.go` without a new manager
or mandatory production file.

Use `rev-parse --path-format=absolute --show-toplevel --git-dir --git-common-dir`
and `cd <path> && pwd -P` to compare physical task and repository paths. Keep
the canonical task within the canonical profile root's `tasks/` directory.
This supports a symlinked profile root without misclassifying internal storage.
Git older than 2.31, unsupported options, or malformed path output gives
`probe_failed`; no fallback parser is required.

Read `worktree list --porcelain -z` for both checkout kinds. A linked checkout
needs its matching registration, external common storage, and no lock.
A standalone clone must have its own `.git` directory and no additional
registered worktrees. Known locks use `shared` with a lock-specific `Detail`.
Unknown shapes preserve the task. No repository-dependency traversal is required.

Any non-empty `git submodule status --recursive` result preserves the entire
task with `probe_failed` and a submodule-specific detail. This deliberately
excludes initialized and uninitialized submodules in either checkout kind.

| Check | Linked worktree | Standalone clone |
| --- | --- | --- |
| Working files | No tracked changes or non-ignored untracked files | Same |
| Commits | `HEAD` must be published | `HEAD` and every local branch must be published |
| Stashes | Recorded branch matches this task, or attribution is unknown: preserve | Any stash: preserve |
| Removal | `git worktree remove` without force | Existing recursive removal |

Use the existing unpushed command with `--branches` removed only for linked
worktrees. Keep its remote-tracking-ref interpretation and read-only behavior.
Parse standard stash descriptions with an exact branch delimiter. Unknown
descriptions, or any stash with detached `HEAD`, preserve the linked worktree.

Probe the root and existing direct-child checkout set before any removal.
Carry only each checkout's path, kind, and common-directory path to removal.
Remove linked children before the root. A failed Git command never permits
recursive removal as a fallback. Keep the existing final path-absence check.
A root with non-ignored untracked children is dirty and vetoes every removal.
The correction does not ignore those children or remove them first to clean the root.

Git owns its registration update. Assert registration removal in real-Git tests
rather than adding a second inventory verification in production. Remove the
earlier plan's captured-identity snapshots and repeated full-probe pass.
Retries keep the existing cleanup-job behavior and run fresh probes.

Reuse `probe_failed`, `stash_present`, and `shared` with descriptive details.
Update the existing comment that reserves `shared` for higher-level callers.

### Public documentation

Task 02 updates the SSH entry in `docs/public/feature-status.md` and the SSH
cleanup section in `docs/public/executors.md`. Both pages are reference material.
The text must state that ignored files, including secrets, no longer block
reclamation. It must distinguish linked worktrees from standalone repositories.
It must retain the opt-in default and terminal-only trigger.
Clarify default standalone preparation versus custom linked worktrees, required
Git cleanup capabilities, and preservation of locked/submodule layouts.

### Related delivery records

The [original package](../ssh-remote-task-dir-reclamation/plan.md) remains a
completed historical record. This package supersedes its task 01 safety and
removal policy and its task 05 public-policy wording. Its opt-in, cleanup-job,
and settings tasks retain their original scope and completion state.

## Tests

New names in this table are implementation targets, not tests already present.
Task 01 must show the regression fails against the old implementation first.

| Acceptance criteria | Test file and test target |
| --- | --- |
| `AC-SSH-TASKDIR-RECLAMATION-002.2`, `.002.8` | `executor_ssh_reclaim_git_test.go`: new `TestSSHTaskDirReclaimGitIgnoredFiles` covers ignored build output and `.env`, plus tracked, staged, and non-ignored untracked changes. |
| `.002.3` | Same file: new `TestSSHTaskDirReclaimGitCommitScope` covers linked sibling commits, own unpublished HEAD, detached HEAD, standalone side branches, and a pushed shallow standalone clone. |
| `.002.4`, `.002.9` | Same file: new `TestSSHTaskDirReclaimGitStashScope` covers another branch, the current branch, mixed stashes, default/custom messages, exact delimiters, detached HEAD, and standalone stashes. |
| `.002.1`, `.002.5`, `.002.10`, `.003.4` | `executor_ssh_reclaim_git_test.go`: new `TestSSHTaskDirReclaimGitCheckoutKinds` covers standard clone/link detection, additional worktrees, symlinked roots, internal common storage, unsupported path-format output, and child-repositories-only roots. Retain existing path-guard tests. |
| `.001.1`, `.001.2`, `.001.5`, `.002.1`, `.002.2` | `executor_ssh_reclaim_git_test.go`: new `TestSSHTaskDirReclaimGitLinkedRemoval` covers root/child registrations, mixed safe/unsafe children, a dirty root with untracked children, sibling sentinels, and shared refs/stashes. Use the exact common-dir command from an unrelated cwd. |
| `.002.7`, `.002.11`, `.002.12` | Same file: new `TestSSHTaskDirReclaimGitPreservesLocksAndSubmodules` covers lock records with/without reasons and initialized/uninitialized submodules. Assert safety-skip outcomes and zero removal commands. Compare old/new status forms with ordinary and ignore-all submodule configuration. |
| `.002.5`, `.002.6`, `.005.1`, `.005.3` | `executor_ssh_reclaim_git_test.go`: new `TestSSHTaskDirReclaimGitFailures` covers probe errors, a lock appearing after the probe, and no destructive fallback. Retain existing count, cancellation, and read-only assertions. |
| `.002.7`, `.004.1`, `.004.2`, `.005.1` through `.005.4`, `.006.2` | Retain existing lifecycle tests and `resource_cleanup_ssh_test.go` tests, including terminal removal, opt-out, safety skips, retries, and cancelled contexts. The unchanged placement after local cleanup preserves `.005.4`. |
| `.004.3` | Existing `resource_cleanup_jobs_test.go`: `TestCancelIfTaskUnarchivedCancelsPendingJobWithoutClaim`. |
| `.007.2` | Existing `executor_ssh_scripts_test.go`: `TestSSHStopInstanceRunsTerminalCleanupBeforeControllerTeardown`, paired with dirty-tree regressions and the unchanged cleanup-job phase order. |

The full criterion IDs and targeted commands are in the work orders.
All new filesystem fixtures use `t.TempDir()` and a local bare remote.
Use a test-only command runner that runs the production commands locally.
Run `WrapLoginShell("bash", cmd)` through an outer shell, matching
`sshClientRunner.Run`; keep `RunDirect` unwrapped. Include spaces and apostrophes
in fixture paths so the production command quoting is exercised.
Keep subprocess environment and Git configuration isolated. No test opens SSH
or mutates a developer workspace.

## End-to-end tests

The filesystem tests call the real `SSHTaskDirReclaimer.Reclaim` method through
the local runner. They assert both final directory state and shared Git state.
Existing task-service tests cover the terminal cleanup caller and durable outcome
mapping. This is the required backend end-to-end boundary. No browser or Docker
flow changes, so this package adds no Playwright test.

## Work orders

- [x] [Task 01: Correct checkout-scoped SSH reclamation](task-01-correct-checkout-scoped-reclamation.md)
- [x] [Task 02: Publish policy and reconcile delivery evidence](task-02-document-policy-and-evidence.md)

## Verification results

Design-package checks on 2026-11-12:

- `python3 scripts/list-docs.py validate`: passed, with 451 decisions and 1995 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- The task 02 local preflight using `.github/scripts/pr-docs.cjs`:
  `covered`, both work orders accepted, zero errors.
- Catalog discovery: the owning requirement/design pair and new ADR are discoverable.
- Local Markdown links: package and owning-document links resolved.
- `git diff --check`: passed. The package remains unstaged and uncommitted.

At the original design-package handoff, product implementation and permanent
regression tests were pending. The temporary Git experiments established the
failure mechanisms and candidate commands, not a completed product fix.

The 2026-11-13 revision follows the user's request to keep the fix simple.
It removes the general topology model, captured-identity revalidation, mandatory
production helper file, and second inventory verification.
Catalog validation, full specification lint, both work orders' coverage, 15 local
links, and whitespace checks passed for this revision. Application code remains unchanged.

The 2026-11-14 docs-review revision adds explicit skip handling for known locks
and submodules, physical path comparison, exact removal-context evidence, and
default/custom layout guidance. It also links the ADR amendment in both
directions. Its validation results are:

- `python3 scripts/list-docs.py validate`: passed, with 451 decisions and 1995 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- The task 02 coverage preflight: `covered`, two work orders, zero errors.
- Local document links: all 17 resolved, including both ADR amendment links.
- `git diff --check` and new-file whitespace checks: passed.
- The additional disposable Git experiments listed in Evidence: passed on Git 2.47.3.

At the docs-review checkpoint, application code and permanent tests remained
unchanged. All eight then-changed files were documentation.

## Implementation closeout, 2026-11-14

Both work orders are complete. The reclaimer filters ignored files from status,
checks linked worktree commits and recorded stashes at task scope, retains
repository-wide checks for standalone clones, and removes linked worktrees
through Git. Read-only Git probes disable optional index writes. Real-Git tests
cover ignored and non-ignored files, branch and stash scope, supported and
unsupported layouts, locks, submodules, path quoting, and registration cleanup.

Verification passed:

- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle -run '^Test(SSHTaskDirReclaim|SSHReclaim|WrapLoginShell|SSHStopInstanceRunsTerminalCleanupBeforeControllerTeardown|IsTaskDirectoryCheckout)' -count=1)`
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/runtime -run '^TestSSHTaskDirReclaimer' -count=1)`
- `(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/service -run '^(TestSSHReclaim.*|TestCancelIfTaskUnarchivedCancelsPendingJobWithoutClaim)$' -count=1)`
- `bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_test.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_git_test.go apps/backend/internal/agent/runtime/lifecycle/executor_ssh_reclaim_optin_test.go` — 0 issues.
- `make -C apps/backend build` — passed for backend, agentctl, mock-agent, debug, and winjob targets. macOS artifacts were left unsigned because neither `codesign` nor `rcodesign` is available.
- `python3 scripts/list-docs.py validate` — 451 decisions and 1995 specifications.
- `python3 scripts/lint-spec-files.test.py` — 36 tests passed; `python3 scripts/lint-spec-files.py --all` — passed.
- `node --test scripts/validate-public-docs.test.mjs` — 63 tests passed; `node scripts/validate-public-docs.mjs` — 55 pages validated.
- The `.github/scripts/pr-docs.cjs` coverage preflight — `covered`, two work orders, zero errors.
- Public policy review found no stale reclamation guidance in the root README or screenshot catalog.
- `git diff --check` — passed. No files were staged or committed.

A code-review follow-up added regressions for per-task symlink redirection,
trailing-space task names, colliding branch/tag names, detached-origin stashes,
and branch-probe transport failure. The regressions passed with the targeted
lifecycle race suite and changed-Go lint; details are in task 01 Results.

PR fixup added explicit local Git identities to the primary, standalone-clone,
and separate-storage repositories used by the real-Git fixtures. This prevents
commit and stash tests from depending on the runner's inferred author identity.
The focused lifecycle race tests and changed-Go lint passed; details are in task
01 Results.


## Risks

- Enabled profiles will remove ignored files that previously blocked reclamation.
  This includes `.env` and other ignored secrets.
- Stashes record names rather than durable branch identity. Unknown attribution
  preserves directories, so some conservative skips will remain.
- Arbitrary SSH transport loss can interrupt a removal. This package does not
  add a recovery protocol for missing checkout metadata.
- Standalone clones with additional registered worktrees remain preserved.
  Supporting their coordinated removal is outside this fix.
- Known locks and registered submodules preserve the task as safety skips.
  A lock or submodule introduced after the probe can cause a Git refusal.
  That attempt uses the error path; a retry discovers the preservation condition.
- Git older than 2.31 and hosts without the required probe capabilities preserve
  directories. This is a reclamation limit, not a change to SSH launch support.
- Existing preserved directories are not swept. The report's full backlog and
  unclassified probe failures are not acceptance targets for this correction.
