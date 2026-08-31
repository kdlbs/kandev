---
created: 2026-08-25
status: implemented
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
system_design:
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
legacy_specs: []
---

# Implementation Plan: Fail-Closed Worktree Refresh

## Overview

Make a required worktree refresh stop task launch when remote state cannot be
verified. The implementation first routes each repository through the correct
credential boundary. It then makes worktree fetch and base-ref selection
fallible. The final work verifies durable launch errors on desktop and mobile
and updates the public Git guidance.

## Confirmed root cause

- `internal/worktree.Manager.pullBaseBranch` logs fetch failure and returns the
  requested local ref through `handleFetchFallback`.
- `pullCurrentBranchOrFallback` also converts pull failure into a successful
  local or remote ref. It does not reject diverged refs.
- `internal/repoclone.Cloner` treats fetch failure as nonfatal when it finds an
  existing cached checkout through an `Ensure*` path.
- `executor.resolveTaskRepoInfoForSession` runs an exact-scope strict refresh
  only for plugin-managed repositories. Other managed providers can reach the
  worktree manager without a successful authenticated refresh.
- The repository setting is presented as “Always pull before creating a new
  worktree.” Continuing from a stale ref violates this user contract.
- Existing fallback tests confirm that the current behavior is intentional,
  not an isolated command error.

## Scope

### In scope

- Treat pull-before-worktree as a required remote-refresh gate for new and
  recreated worktrees.
- Select a strict refresh route that preserves managed-provider credentials and
  executor-inherited Git transport.
- Stop preparation on fetch failure, missing fetched base, divergent refs, or
  uncertain ancestry.
- Preserve offline behavior when pull-before-worktree is disabled.
- Preserve a local base that contains the fetched remote base.
- Stop multi-repository launch before agent startup when one required refresh
  fails.
- Reconcile a GitHub pull request's current base at launch and during polling.
- Recover a proven deleted remote base through a separately refreshed default
  base while retaining fail-closed behavior for all other fetch failures.
- Project a credential-safe, repository-specific launch error through the
  existing task error surface.
- Document SSH setup and the fail-closed behavior in the public Git guidance.

### Out of scope

- Making setup scripts fatal.
- Changing agent Git transport after launch.
- Refreshing an existing worktree that Kandev reuses without recreation.
- Resetting, merging, rebasing, or deleting user refs.
- Adding a new recovery action or user-interface layout.
- Adding a background refresh worker.

## Technical approach

The executor will resolve the task Git credential policy before lifecycle
preparation. A backend-managed provider checkout will pass a deferred strict
`repoclone` refresh callback with the same provider-specific credentials as
clone. The callback runs only when a worktree is materialized or recreated; a
valid reusable worktree bypasses it. An executor-inherited checkout will retain
its reconciled origin and enter the strict worktree-manager fetch path. Only a
completed refresh can set `RemoteSyncHandled`.

The worktree manager will return `(ref, error)` from required sync and
base-selection functions. Fetch failure will propagate instead of returning a
fallback ref. After a successful fetch, ancestry checks will select a ref only
when one ref contains the other. Divergence and an uncertain check will return
an error without changing either branch.

Lifecycle and executor code will retain repository identity while they wrap
the error. Existing task launch-failure persistence will project the failure.
No database or WebSocket schema change is planned.

## Work orders

- [completed] [Task 01: Route Required Repository Refresh](task-01-route-required-refresh.md)
- [completed] [Task 02: Reject Stale Worktree Fallbacks](task-02-reject-stale-fallbacks.md)
- [completed] [Task 03: Project Refresh Launch Failures](task-03-project-refresh-launch-failures.md)
- [completed] [Task 04: Document Required Repository Refresh](task-04-document-required-refresh.md)
- [completed] [Task 05: Support Stacked PR Base Retargeting](task-05-support-stacked-pr-base-retargeting.md)

## Dependency order

```text
Task 01 -> Task 02 -> Task 03 -> Task 04 -> Task 05
```

The package is sequential. Task 02 consumes the refresh-route contract from
Task 01. Task 03 verifies the combined launch boundary. Task 04 documents the
verified behavior.

## Verification strategy

- Unit tests prove credential-route selection and exact-scope managed refresh.
- Worktree integration tests prove fetch failure, comparable refs, divergence,
  timeout, and the disabled-policy path.
- Orchestrator tests prove repository-specific failure and no agent startup for
  multi-repository tasks.
- Existing launch-failure E2E tests prove the durable error on desktop and
  mobile after reload.
- Public documentation validators prove links, commands, and headings.
- Backend race tests and lint cover the combined affected packages.

## Verification results

- Worktree, lifecycle, orchestrator, and executor race suites passed, including
  6,049 tests in the combined affected-package run.
- Backend build, E2E plugin packaging, and production E2E build passed.
- The launch-failure recovery spec passed all 3 Chromium tests and both
  `mobile-chrome` tests.
- Backend lint, public-doc validators, specification lint, and `git diff
  --check` passed.

### Review remediation results

- Rebuilt the PR branch from the focused worktree-refresh, launch-failure, and
  required fixture-origin commits; unrelated automation and broad E2E timing
  changes are no longer part of the branch.
- Added Create and recreate regressions for normal managed branches and
  `origin/pr/<N>` refs with a pre-existing local branch behind the refreshed
  source. The refreshed source is now passed to `git worktree add -B` after
  ancestry selection.
- Reverified 2,972 affected backend race tests, zero backend lint issues, 3/3
  desktop launch-failure tests, and 2/2 mobile launch-failure tests with strict
  no-retry E2E execution.

### PR fixup remediation results

- Reproduced the six failed E2E shard causes at the focused branch head: local
  origin fixture collisions, ambiguous folder and branch locators, missing
  offline refresh opt-outs, shared Git history, and a GitLab setup that needed
  to remain offline.
- Kept the remediation scoped to the refresh contract: exact folder/branch
  locators, explicit offline options for local fixtures, and a disposable
  repository for the PR-deduplication history assertion. No generic timing
  changes were restored.
- With retries disabled, the affected desktop command passed 21/21 tests, the
  affected mobile command passed 2/2 tests, and the complete Git Changes Panel
  file passed 21/21 tests. Prettier and `git diff --check` also passed.

### Stacked PR base retargeting results

- Added launch-time live GitHub base resolution and polling-time persistence
  for PR-backed task repositories, with best-effort failure handling.
- Added a verified default-base fallback only for missing remote base refs;
  authentication, transport, timeout, and other refresh failures remain fatal.
- The affected backend package tests, complete backend test suite, backend
  lint, backend build, specification validators, and public-documentation
  validators passed.

## Risks

- A managed refresh that ignores task Git policy can replace an
  executor-inherited SSH origin with HTTPS. Route selection needs explicit
  policy tests.
- A naive remote-first rule can hide local-only commits. The ancestry table in
  the system design is the required selection rule.
- Existing tests currently require stale fallback behavior. Replace those
  assertions with fail-closed tests rather than removing coverage.
- `executor_resume.go` is active code with broad resume coverage. Keep the
  route helper narrow and run fresh-launch and resume suites together.
- Error wrapping can leak raw Git output. Use existing redaction and bounded
  launch-error helpers.
- Live GitHub base resolution is best-effort. The polling projection and
  refreshed default fallback must independently cover unavailable lookups.
- A multi-repository preparation attempt can refresh earlier repositories
  before a later one fails. This is safe on-disk cache progress, but agent
  startup must remain atomic.

## Package handoff

Implementation followed the sequential TDD work orders. Each work order and
the plan status are updated after its verification commands passed.
