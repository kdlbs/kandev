---
spec: docs/specs/tasks/requirements/task-launch-failure-recovery.md
created: 2026-08-19
status: done
---

# Implementation Plan: Task Launch Failure Recovery

## Overview

The implementation first defines durable error and repository-target contracts.
It then hardens Git resolution, carries exact row identity, and adds launch policy.
The final waves add task-scoped recovery, bounded delivery, responsive UI, translations, and E2E coverage.

## Confirmed root cause

- `gitref.DefaultBranch` falls back to the current local `HEAD` branch when no origin ref resolves.
  Repository import can therefore persist a feature branch as the default.
- `resolveBaseRefWithFallback` stops when the selected base and stored default are absent.
  It does not refresh `origin/HEAD` before it returns `ErrInvalidBaseBranch`.
- `autoStartTaskForLoadedStep` calls `StartTask` before it checks the relevant PR state.
  No session exists at this point, so session metadata cannot own the gate result.
- The lifecycle request carries `RepositoryID`, but it drops `TaskRepository.ID`.
  The recovery and self-heal paths cannot target one same-repository branch row safely.
- `handleSessionLaunchFailed` creates separate warning messages, archive/delete actions, and toast suppression.
  That path conflicts with a typed task error and its recovery actions.

## Backend

### Error contracts and persistence owners

- `apps/backend/internal/task/models/models.go`
  - Add typed launch categories and recovery-action constants.
  - Extend `LastAgentError` with `RecoveryActions`, `TaskRepositoryID`, and an explicit bounded stamp.
  - Add `TaskLaunchError` and `MetaKeyLastLaunchError` for pre-session outcomes.
  - Add load, store, clear, and stable-stamp helpers for the task metadata record.
- Keep session-created launch errors in `task_sessions.metadata["last_agent_error"]`.
- Store the PR auto-start gate in `tasks.metadata["last_launch_error"]`.
- Add atomic, key-scoped task-error set and compare-and-clear operations.
  They do not rewrite the full metadata object or clear a newer stamp.
- Make repeated writes of the same gate result a semantic no-op.

### Local default detection

- `apps/backend/internal/common/gitref/gitref.go`
  - Return the resolution source with the branch name inside the package.
  - Make `DefaultBranchOrEmpty` return empty for a current-`HEAD`-only result.
  - Keep `DefaultBranch` behavior unchanged for callers that need a display fallback.
- Keep `internal/common/gitref` free of subprocess and network work.

### Live remote-default refresh

- `apps/backend/internal/worktree/manager_lifecycle.go`
  - Add exported `Manager.ResolveRemoteDefaultBranch(ctx, repoPath)`.
  - Read `refs/remotes/origin/HEAD` before the helper runs a command.
  - Run `git remote set-head origin --auto` through the existing Git admission and classification path.
  - Use `Manager.inspectTimeout` and the caller context.
  - Set the existing noninteractive Git environment, including `GIT_TERMINAL_PROMPT=0`.
  - Preserve auth, network, cancellation, and timeout errors as typed causes.
- `resolveBaseRefWithFallback` uses the live default after the stored default fails.
  It returns `ErrInvalidBaseBranch` only when every branch source is absent.

### Exact task-repository identity

Carry `TaskRepositoryID` through every per-repository launch boundary:

```text
executor.repoInfo
  -> executor.RepoSpec
  -> lifecycle.RepoLaunchSpec
  -> lifecycle.LaunchRequest.RepoSpecs()
  -> lifecycle.RepoPrepareSpec
  -> lifecycle.EnvPrepareRequest.RepoSpecs()
  -> lifecycle.RepoWorktreeResult
```

Also add the legacy top-level single-repository field where the request synthesizes one spec.
Update fresh launch, resume, environment reuse, and multi-repository tests.

The launch-failure result and `LastAgentError` record carry the exact row ID.
If identity is missing or ambiguous, omit branch recovery actions.

### Worktree fallback self-heal

- `apps/backend/internal/agent/runtime/lifecycle/env_preparer_worktree.go`
  - Preserve the requested base before worktree creation.
  - Return the resolved `Worktree.BaseBranch` and fallback warning with `TaskRepositoryID`.
- At the orchestrator boundary, inspect each prepared worktree result.
  When fallback occurred, call `Service.UpdateRepositoryBaseBranch` for the exact row.
- Keep the worktree manager and lifecycle packages free of a task-service dependency.
- Treat the update as best effort.
  A write error logs a warning and does not fail the launch.

### PR auto-start gate

- Add a narrow PR reader to the orchestrator.
- Load the task-repository rows and active PR links before `StartTask`.
- Match a row by exact `(repository_id, pr_number)` when metadata has a positive PR number.
- Otherwise match exact normalized repository and checkout/head branch values.
- Normalize branches by trimming whitespace and one documented Git ref prefix.
  Keep the final comparison case-sensitive.
- Skip launch only when at least one relevant PR exists and all relevant states are terminal.
- Store `TaskLaunchError{code: pr_already_closed}` with a stable stamp from relevant PR identity and state.
- Add `mark_review_done` only when the workflow has a valid terminal final step.
- Fail open for lookup errors, no match, open state, empty state, or unknown state.
- Clear the task-owned gate result after a later launch succeeds.

### Session launch classification

- `apps/backend/internal/orchestrator/executor/executor_execute.go`
  - Classify `ErrInvalidBaseBranch` as `base_branch_missing`.
  - Attach the exact `TaskRepositoryID` and only valid recovery actions.
  - Persist the extended `LastAgentError` with the existing failed state changes.
- Inject a narrow review-completion eligibility capability into the executor.
  It requires a valid terminal final step and terminal relevant PR state.

### Legacy launch-guidance removal

- Remove the handled missing-branch message path from `handleSessionLaunchFailed`.
- Remove its archive/delete action metadata and `missing_pr_branch_recovery_claimed` use.
- Do not set `suppressToast` for a handled typed launch error.
- Retain unrelated recovery and toast-suppression behavior.

### Task launch recovery action

- Register `task.launch.recover` in the orchestrator handler set and shared WS action constants.
- Authorize the task before any lookup.
- Require `error_stamp` and compare it with the current projected source record.
- Validate the optional session-task pair and repository-row ownership.
- For `retry_default`, use the worktree manager remote-default resolver.
  Update the repository default and exact task base before relaunch.
- For `pick_base_branch`, validate the selected remote branch.
  Update the exact task base before relaunch.
- For `mark_review_done`, list workflow steps and select the final valid terminal step.
  Call `task.Service.MoveTaskWithOptions` so WIP, state, history, and events stay consistent.
- Re-evaluate relevant PR state before the move.
  Reject the action when a relevant PR is open, unknown, or no longer matches.
- Add a recovery-only move option that permits `FAILED` to become `COMPLETED` at that step.
  Do not change failed-state preservation for normal moves.
- Clear the source error only after the recovery action succeeds.
- Use a compare-and-clear write so success cannot erase a newer error.
- Replace it with `default_branch_unresolved` when a default cannot resolve.

### Bounded status projection

- Update `apps/backend/internal/task/statussummary/model.go` with optional `SessionID`, optional
  `TaskRepositoryID`, `Category`, and `RecoveryActions`.
- Bound category and row IDs to 64 and 256 UTF-8 bytes.
- Accept only the three known action values.
  Deduplicate them, preserve order, and keep at most three.
- Update live projection and authoritative rebuild to read both error sources.
- Select the newest active record by `occurred_at`, then stable stamp.
- Ignore malformed task metadata instead of rejecting the complete summary.
- Update `docs/specs/platform/requirements/bounded-task-status-delivery.md` with this contract.

## Frontend

### Types and live summary selection

- Extend `apps/web/lib/types/task-status-summary.ts` with the new bounded fields.
- Extend `OfficeTask` and local task-detail `Task` with `statusSummary`.
- Map `status_summary` in `apps/web/app/office/tasks/[id]/page.tsx`.
- Add a selector that returns the newest summary for a task across detail data and live kanban caches.
- Update `statusSummaryActiveErrorPreview` so task-owned errors do not require a session ID.
- Extend `LastAgentError`, `RunError`, and their parsers with recovery fields and row identity.

### Shared launch-error card

- Add `apps/web/components/task/simple/components/task-launch-error-entry.tsx`.
- Render the card from `TaskStatusSummary.active_error` before the Chat empty state.
- Reuse the card from `RunErrorEntry` for typed session launch errors.
  Keep existing resume, fresh-start, and managed-runtime behavior unchanged.
- Suppress the summary card when a matching session run-error card already renders.
- Send the three new actions through `task.launch.recover`.
- Keep action logic and error state shared across desktop and mobile.
- Replace the raw launch toast with `task:launchFailedSeeDetails`.

### Branch picker and mobile contract

- Extract reusable branch-list and repository-resolution logic from
  `apps/web/components/task/base-branch-picker.tsx`.
- Desktop opens the existing popover pattern from the Pick a base branch action.
- Phone opens `apps/web/components/task/mobile/mobile-picker-sheet.tsx` from the same action.
- The task Chat surface is the desktop and phone entry point.
- The error card stays inline because it is short and task-specific.
- Actions wrap and use at least 44px touch targets.
- The task Chat surface owns page scrolling.
  The picker owns list scrolling and safe-area padding.
- No document-level horizontal overflow is permitted.

### Internationalization

- Add category headlines, action labels, picker text, and pointer-toast text to all five task catalogs.
- Write English, `pt-pt`, and `zh-cn` values directly.
- Generate `zh-hk` and `zh-tw` with `pnpm run i18n:zh-hant`.

## Tests

- **Local default detection**
  - File: `apps/backend/internal/common/gitref/gitref_test.go`
  - Prove that current-`HEAD`-only persistence returns empty.
  - Prove that origin and local main/master refs keep current behavior.
- **Live remote default**
  - File: `apps/backend/internal/worktree/manager_lifecycle_test.go`
  - Prove success from existing `origin/HEAD` and refreshed `origin/HEAD`.
  - Prove cancellation, bounded timeout, auth/network classification, and unresolved behavior.
- **Repository identity and self-heal**
  - Files: executor multi-repository tests and lifecycle worktree-preparer tests.
  - Prove exact identity for same-repository multi-branch rows, fallback writes, no-op, and write-error continuation.
- **PR gate**
  - File: `apps/backend/internal/orchestrator/event_handlers_workflow_test.go`
  - Cover merged, closed, open, mixed states, unknown state, no row, lookup error, and replay idempotency.
  - Cover exact PR-number matching and same-repository sibling branches.
- **Launch classification**
  - File: `apps/backend/internal/orchestrator/executor/executor_execute_test.go`
  - Start with a regression that lacks a typed error and exact row target.
  - Prove missing-base and generic categories plus ambiguous-target action removal.
- **Legacy consolidation**
  - File: `apps/backend/internal/orchestrator/task_launch_failure_test.go`
  - Prove that handled errors create no warning message, archive/delete actions, claim, or toast suppression.
- **Recovery action**
  - File: the owning orchestrator WS handler test.
  - Cover task/session/repository ownership, all three actions, forged payloads, and retry idempotency.
  - Cover a workflow with no valid terminal final step and a PR that reopens before the action.
- **Status projection**
  - Files: `apps/backend/internal/task/statussummary/*_test.go` and rebuild service tests.
  - Cover newest-source selection, restart rebuild, bounds, known-action filtering, and malformed metadata.
- **Frontend**
  - Files: focused tests beside the parser, selector, Chat component, launch-error card, and `RunErrorEntry`.
  - Prove the no-session card, exact WS payloads, existing recovery preservation, and branch picker composition.

## E2E Tests

- **Desktop merged-PR gate**
  - File: `apps/web/e2e/tests/task/launch-failure-recovery.spec.ts`
  - Prove no session starts, the durable card survives reload, and Mark review done moves the task.
- **Desktop missing-base recovery**
  - Same file.
  - Prove exact-row retry and branch selection, relaunch, self-heal, reload, and pointer toast.
- **Legacy missing-branch regression**
  - File: `apps/web/e2e/tests/pr/pr-watcher-missing-branch.spec.ts`
  - Replace warning-message and archive/delete expectations with the typed task-error outcome.
- **Mobile recovery parity**
  - File: `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts`
  - Prove the same branch-selection outcome through `MobilePickerSheet`.
  - Prove 44px action targets, viewport containment, internal picker scrolling, and no page overflow.

Use the managed runner so each E2E command builds production assets and performs cleanup.

## Verification Results

Implemented and verified on 2026-08-20.

- `cd apps/backend && go test ./internal/orchestrator/... ./internal/agent/runtime/lifecycle/... ./internal/worktree/...`: 5,139 tests passed in 13 packages.
- `cd apps/backend && make build`: passed.
- `cd apps/backend && make e2e-plugin-package`: passed.
- `cd apps/web && pnpm run lint`: passed.
- `cd apps/web && pnpm run typecheck`: passed.
- `cd apps/web && pnpm run i18n:check`: passed. The catalogs contain 7,159 referenced keys and complete translations.
- Focused frontend recovery and status tests: 4 files and 50 tests passed.
- Responsive recovery tests: 2 files and 24 tests passed.
- Desktop task recovery E2E: 2 tests passed.
- Mobile task recovery E2E: 1 test passed.
- Desktop and mobile PR watcher E2E: 1 test passed in each project.
- Desktop and mobile recovery screenshots were captured and inspected.

No public documentation update was required. The change affects internal task recovery behavior.

## PR Fixup Remediation

PR #2832 review and CI findings were addressed before delivery.

- Recovery now reloads all task and session error sources and rejects an action
  when its source is not the newest active error.
- Workflow terminal-step traversal uses one shared helper.
- Session metadata compare-and-clear propagates `RowsAffected` errors.
- Windows treats the wildcard-bind `WSAEACCES` conflict as a port collision.
- Typed launch errors remain visible when they have no recovery action.
- Summary polling keeps the highest revision and duplicate recovery surfaces
  show the same pending action.
- `cd apps/backend && make lint`: passed with zero issues.
- `cd apps/backend && go test ./internal/orchestrator/... ./internal/common/netutil/... ./internal/task/repository/sqlite/...`: 3,468 tests passed in 11 packages.
- `cd apps/backend && go test -c -o /tmp/kandev-netutil-windows.test.exe ./internal/common/netutil`: passed.
- `cd apps/web && pnpm run lint`: passed.
- `cd apps/web && pnpm run typecheck`: passed.
- Focused frontend fixup tests: 3 files and 64 tests passed.

### Second PR fixup

- Removed the no-op launch-failure callback and retargeted its test to the real
  state-transition fallback.
- Added bounded message persistence, exact explicit-stamp matching, sanitized
  unresolved-default details, and an environment-gated Postgres CAS test.
- Replaced stale full-row default-branch writes with an expected-value guarded
  column update.
- Rejected empty relaunch responses, preserved successful relaunches when
  compare-and-clear logging is needed, and added session-owned recovery tests.
- Required matching session error stamps in the chat timeline and rendered
  only typed task errors in the recovery layout branch.
- Added focused positive typed-error assertions and mutex-safe background
  launch checks.

## Implementation Waves And Parallel Candidates

```text
Wave 1 (parallel candidates, user authorization required):
- [x] [task-01-failure-taxonomy-contracts](task-01-failure-taxonomy-contracts.md)
- [x] [task-02-gitref-default-hardening](task-02-gitref-default-hardening.md)

Wave 2:
- [x] [task-03-worktree-live-default-fallback](task-03-worktree-live-default-fallback.md) (depends 02)
- [x] [task-05-pr-review-autostart-gating](task-05-pr-review-autostart-gating.md) (depends 01)
- [x] [task-10-task-base-self-heal](task-10-task-base-self-heal.md) (depends 01)

Wave 3:
- [x] [task-04-launch-failure-classification](task-04-launch-failure-classification.md) (depends 01,10)

Wave 4:
- [x] [task-07-status-summary-projection](task-07-status-summary-projection.md) (depends 01,04,05)
- [x] [task-12-remove-legacy-launch-guidance](task-12-remove-legacy-launch-guidance.md) (depends 04)

Wave 5:
- [x] [task-06-recovery-actions-ws](task-06-recovery-actions-ws.md) (depends 01,02,03,04,05,10,12)
- [x] [task-08-frontend-failure-surface-and-recovery](task-08-frontend-failure-surface-and-recovery.md) (depends 07)

Wave 6:
- [x] [task-11-responsive-launch-error-surface](task-11-responsive-launch-error-surface.md) (depends 06,08)

Wave 7:
- [x] [task-09-e2e-and-i18n](task-09-e2e-and-i18n.md) (depends 11)
```

The default execution order is sequential in the primary conversation.
The waves do not authorize subagents.
