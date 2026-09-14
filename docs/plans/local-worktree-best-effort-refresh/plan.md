---
created: 2026-08-31
status: completed
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
legacy_specs: []
---

# Implementation Plan: Restore Best-Effort Local Worktree Refresh

## Overview

Restore local-first worktree creation without weakening remote-only
materialization. The implementation first corrects base selection and warning
behavior. Then it updates desktop, mobile, and public documentation evidence.

## Confirmed root cause

Commit `527cf1e9fa` changed `pullBaseBranch` from best-effort fetch to a required
refresh gate. The gate runs before Kandev uses an existing local base.

A branch that exists only locally now fails at `git fetch origin <branch>`.
Kandev never reaches the valid `git worktree add` path.

The focused reproduction uses one unpublished local branch. It fails with
`PullBeforeWorktree=true` and succeeds with the same setting disabled.

The permanent regression test is
`TestCreateWorktree_PullEnabledUsesLocalOnlyBase` in
`apps/backend/internal/worktree/manager_pull_fallback_test.go`.

## Scope

### In scope

- Use a verified local base after host refresh errors.
- Stop preparation when the caller cancels the operation.
- Preserve local history after missing remote refs or divergent refs.
- Keep warnings bounded and credential-safe.
- Keep remote materialization strict when no usable local base exists.
- Preserve the empty-remote bootstrap contract.
- Replace desktop and mobile E2E expectations that encode universal fail-closed refresh.
- Update public Git and executor documentation.

### Out of scope

- Pushing local branches during worktree preparation.
- Changing Docker, SSH, Sprites, or provider clone requirements.
- Changing the repository setting schema or default.
- Changing worktree settings UI composition or labels.
- Changing Git commands after agent startup.

## Technical approach

### Local base selection

Update `internal/worktree.Manager` in
`apps/backend/internal/worktree/manager_git.go`. Verify the local base before
remote refresh.

If the local base exists, convert fetch, pull, missing-ref, divergence, and
ancestry errors into bounded warnings. Return the local base without changing
Git refs.

If no local base exists, keep the current error path. Remote materialization
must provide the requested base before worktree creation.

Keep the successful-refresh selection rule. Use the remote ref when it contains
the local ref. Use the local ref when it contains the remote ref.

### Warning propagation

Use the existing `SyncProgressEvent` path for refresh warnings. Do not include
raw Git output in user-visible warnings or generic task errors.

Keep structured logs limited to repository identity, failure class, branch,
and selected fallback ref.

### User-facing evidence

Replace the required-refresh failure cases in the desktop and mobile launch
recovery specs. The new cases keep a local base, make origin unavailable, and
prove that the task starts without an active launch error.

The change does not alter layout, navigation, touch behavior, or scroll
ownership. Desktop and mobile use their existing task-session surfaces.

### Public documentation

Update the Git operations reference and executor explanation. State that host
refresh is best effort when a local base exists. Keep strict remote
materialization limits explicit.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.1` | Existing disabled-policy worktree tests remain green. |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.2` | A focused manager test proves that the fetch attempt occurs. |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3` | `TestCreateWorktree_PullEnabledUsesLocalOnlyBase` proves fallback and bounded warning output. |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.4` | Existing containing-ref tests remain green. |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.5` | A manager test proves that divergence preserves the local ref with a warning. |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6` | A manager test proves that missing local and remote refs stop preparation. |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.7` | Existing multi-repository preparation tests gain a local-fallback case if manager coverage cannot prove propagation. |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.8` | Existing repository-specific launch-error tests remain green. |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.9` | Existing empty-remote worktree tests remain green. |
| `AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.10` | A manager test proves that caller cancellation does not create a fallback worktree. |

## E2E tests

- Desktop: `apps/web/e2e/tests/task/launch-failure-recovery.spec.ts` proves that
  an origin outage does not block a task with a valid local base.
- Mobile: `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts`
  proves the same task outcome in the `mobile-chrome` project.

## Work orders

- [x] [Task 01: Restore Local Base Fallback](task-01-restore-local-base-fallback.md)
- [x] [Task 02: Prove Desktop and Mobile Launch](task-02-prove-local-fallback-launch.md)
- [x] [Task 03: Update Public Refresh Documentation](task-03-update-refresh-documentation.md)

## Dependency order

```text
Task 01 -> Task 02 -> Task 03
```

The work orders are sequential. No work order authorizes subagents.

## Verification results

- `go test ./internal/worktree ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor -run 'Test(CreateWorktree|PullBaseBranch|PreferRefreshedRemoteRef|ResolveBaseRefWithFallback|Prepare.*Worktree|.*Refresh)' -count=1` passed (69 tests).
- `make -C apps/backend test` passed with the task-session configuration overrides removed.
- Changed-file `golangci-lint` passed with no issues.
- Web typecheck and focused E2E checks passed for Chromium and `mobile-chrome`.
- Public documentation validation and specification lint passed.

## Risks

- A broad fallback can hide a missing remote-only branch. The code must verify
  the local base before fallback.
- A broad fallback can ignore user cancellation. Cancellation must remain fatal.
- A warning can expose Git credential output. The warning must use bounded
  failure classes.
- Provider-managed refresh can fail before local selection. The implementation
  must not weaken remote-only materialization.
- Existing E2E cases encode the superseded invariant. Replacement cases must
  keep unrelated launch-error recovery coverage intact.
- Empty remotes use a separate marked baseline. The local fallback must not
  bypass that marker contract.
