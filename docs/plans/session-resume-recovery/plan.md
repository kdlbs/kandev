---
created: 2026-08-31
status: implemented
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-003
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
legacy_specs: []
---

# Implementation Plan: Visible Session Resume Recovery

## Overview

Make every session recovery failure visible. Add an explicit path that creates
a new branch after confirmed branch loss while it keeps the same provider
conversation. Persist an honest warning that separates conversation recovery
from code recovery.

## Confirmed root cause

- `session.recover` already returns a descriptive backend error, but
  `SessionStoppedBanner` and `RunErrorEntry` catch and discard it.
- `useSessionResumption` discards the first resume cause, silently substitutes
  read-only workspace restore, and returns a generic error only if both calls
  fail.
- The WebSocket client rejects requests with a plain `Error`, so it drops the
  backend error code and details that can authorize a specific recovery action.
- The worktree manager returns a wrapped `ErrBranchUnrecoverable` after local
  and remote branch recovery fail. The resume launch path preserves that error
  chain through workspace preparation.
- Normal worktree creation already owns unique branch templates and suffixes.
- The model-selection warning path provides the existing pattern for an
  idempotent warning status message and localized chat rendering.

## Scope

### In scope

- Preserve WebSocket error code and structured details in the web client.
- Show the real recovery failure near each manual Resume control.
- Keep errors visible after busy state ends and after repeated attempts.
- Keep automatic read-only restore, but show the resume cause and read-only
  result.
- Show both causes when resume and read-only restore fail.
- Add the explicit `resume_new_branch` recovery action.
- Create a unique branch from the configured task base after confirmed branch
  loss.
- Confirm branch loss with a bounded authoritative remote probe when local and
  remote-tracking refs are absent.
- Compensate replacement checkout and branch creation if environment-record
  persistence fails.
- Keep the same task session, ACP session ID, and resume token.
- Persist one `branch_recreated` warning per replaced repository branch.
- Persist that warning on every terminal path after replacement materializes.
- Disable shared Retry controls while recovery requests are pending.
- Render honest localized warning copy in desktop and mobile chat.
- Add backend, frontend, desktop E2E, and mobile E2E regressions.

### Out of scope

- Recovering commits, files, or uncommitted work from the lost branch.
- Automatically switching branches after resume fails.
- Treating network, authentication, timeout, or unknown Git errors as branch
  loss.
- Changing the provider conversation after branch replacement.
- Changing task-environment worktree ownership.
- Adding a new page, navigation flow, modal, or mobile-only recovery path.

## Technical approach

The first work order adds a TDD boundary at the worktree and resume layers.
Normal resume continues to return `ErrBranchUnrecoverable`. The explicit action
passes a narrow replacement permission through executor workspace preparation.
The worktree manager then uses its normal base selection and branch-name
generation to replace only a confirmed lost branch. Attach-only preflight uses
authoritative remote evidence, and replacement persistence compensates newly
created filesystem and Git state if its database update fails.

The second work order adds the typed recovery protocol and durable warning.
The WebSocket handler maps confirmed branch loss to a conflict with structured
recovery details. The orchestrator compares repository branch state around
workspace preparation, persists one claimed warning for each replacement on
every terminal resume path even when a later repository or provider step
fails, and reclaims timestamped claims left by a crash when no matching warning
exists.

The third work order makes every frontend recovery path observable. It retains
WebSocket error details, uses the existing alert pattern, and exposes explicit
actions. It also renders automatic read-only fallback as a notice and maps the
persisted warning metadata to localized copy.

The final work order verifies the complete user path on desktop and mobile and
runs the requested repository gates.

## Work orders

- [x] [Task 01: Create replacement worktrees without changing sessions](task-01-create-replacement-worktrees.md)
- [x] [Task 02: Expose typed branch recovery and persist warnings](task-02-persist-branch-recovery-warning.md)
- [x] [Task 03: Show recovery errors and actions](task-03-show-recovery-errors.md)
- [x] [Task 04: Verify desktop and mobile recovery](task-04-verify-recovery-flows.md)

## Dependency order

```text
Task 01 -> Task 02 -> Task 03 -> Task 04
```

The package is sequential. Each work order starts with a failing test. Task 02
depends on the explicit backend action from Task 01. Task 03 consumes the typed
protocol and warning metadata from Task 02. Task 04 verifies the integrated
behavior and full quality gates.

## Verification strategy

- Worktree tests prove normal error propagation and explicit branch creation
  from the configured base, authoritative remote classification, transient
  probe handling, and compensation after persistence failure.
- Executor and orchestrator tests prove service-level session, ACP identity, and
  token preservation for `resume_new_branch`.
- Persistence tests prove complete warning metadata, state-guarded claims,
  retry after write failure or a stale claim, partial multi-repository failure
  handling, and duplicate suppression.
- Frontend unit tests prove errors are not swallowed and typed recovery actions
  appear only for the matching backend details.
- Status-message tests prove honest localized branch warning rendering.
- Playwright tests prove visible errors, explicit continuation, reload
  persistence, keyboard use, touch targets, and no horizontal overflow.
- Backend test, lint, changed-file complexity, Go formatting, frontend
  typecheck, lint, and internationalization gates cover the full change.

## Risks

- A broad fallback flag could turn transient Git failures into fresh branches.
  Keep the permission narrow and require `errors.Is` at the worktree boundary.
- A branch replacement can accidentally clear the provider identity if it
  reuses Start fresh logic. Test the stored token and ACP session ID before and
  after the action.
- Multi-repository preparation can partially update task environment records.
  Preserve the existing launch failure boundary and test valid repository
  reuse beside one replaced repository.
- Warning creation can duplicate on replay, disappear after a transient
  database error, or leave a claim behind if the process crashes before the
  message write. Use state-guarded timestamped claims, release on write
  failure, and reclaim only stale claims without a matching warning message.
- Localized copy can imply code recovery. Keep the conversation and code
  outcomes in separate sentences in every locale.
- Recovery actions can overflow narrow chat cards. Reuse the stacked mobile
  layout and assert target size and document width.
- Remote branch probes can be mistaken for proof of deletion if their failure
  class is collapsed. Keep confirmed absence distinct from auth and transport
  failures.
- A persistence error after filesystem mutation can orphan a replacement. Keep
  cleanup exact to the new path and branch tip so the prior record stays safe.
- A retry button that ignores the shared busy state can overlap recovery
  requests and repeat branch preparation.

## Package handoff

The work orders are implemented in dependency order with TDD. Focused and full
verification results are recorded below.

## Results

- Task 01 preserves normal resume behavior and adds the explicit
  `resume_new_branch` path. Race-enabled worktree, lifecycle, executor, and
  orchestrator tests pass, including 17 worktree, 124 lifecycle, and 87
  executor/orchestrator tests.
- Task 02 preserves typed branch-loss causes through the WebSocket conflict
  response and persists idempotent `branch_recreated` warnings. The focused
  race-enabled recovery tests pass with 3 tests; the handler selector exits
  cleanly with no matching test names.
- Task 03 retains manual and automatic recovery causes, exposes typed branch
  continuation only for confirmed branch loss, and renders localized warning
  metadata. The focused frontend suite passes 73 tests; typecheck, lint, and
  both internationalization gates pass.
- Task 04 passes the desktop and mobile branch-recovery E2E scenarios. Fresh
  PR evidence is available in `apps/web/.pr-assets/` with one manifest entry
  for each viewport.
- PR fixup hardens replacement eligibility against transient fetch failures,
  preserves warnings after partial preparation failure, retains both manual
  recovery causes, clears stale automatic feedback after an external recovery,
  reclaims crash-stale warning claims, compensates failed replacement
  persistence, and prevents overlapping retry requests.
- PR fixup adds a service-level `resume_new_branch` regression that reloads one
  task session and its original ACP resume token after the action.
- Full backend tests pass with the managed-process configuration variables
  cleared, backend lint and changed-file golangci-lint pass, the production
  web build passes, and specification lint passes.
