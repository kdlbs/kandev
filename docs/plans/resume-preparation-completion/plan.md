---
created: 2026-09-01
status: implemented
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
legacy_specs: []
---

# Implementation Plan: Complete Resumed Workspace Preparation

## Overview

Restore the terminal preparation event for worktree-backed ACP resumes. Add a
backend regression at the lifecycle event boundary, then extend the existing
desktop and mobile session-recovery scenarios to prove that the chat settles to
its real idle state.

## Confirmed root cause

PR #3216 correctly made worktree ACP resumes call environment preparation when
Git recovery may be required. The older `publishLaunchPrepareCompleted` guard
still returns for every request with an ACP session ID. The resulting resume
publishes `executor.prepare.progress` without `executor.prepare.completed`.
The web store remains `preparing`, `useSessionState` folds that into
`isWorking`, and a `WAITING_FOR_INPUT` session is mislabeled as background work.

## Scope

### In scope

- Pair every resumed worktree preparation progress stream with one terminal
  preparation event on success or failure.
- Keep ordinary ACP resumes that skip preparation free of synthetic preparation
  events.
- Add a failing Go regression for the exact ACP worktree resume path.
- Prove desktop and mobile chats no longer show **Preparing workspace** or
  **Background work is running** after recovery becomes idle.
- Keep the existing shared chat composition and mobile interaction model.

### Out of scope

- Changing ACP provider session identity, resume-token persistence, or branch
  replacement authorization.
- Redesigning chat status labels or the composer.
- Changing genuine detached background-work presentation.
- Reworking preparation snapshot hydration or live-event precedence.

## Technical approach

Update `publishLaunchPrepareCompleted` in
`apps/backend/internal/agent/runtime/lifecycle/manager_launch.go` so its resume
suppression uses the same `shouldPrepareEnvironment` decision that gates
environment and runtime progress. This preserves the no-event behavior for ACP
resumes that reuse a prepared non-worktree workspace while allowing worktree
resumes to publish their terminal result.

Add a lifecycle test in `manager_launch_test.go` that uses an ACP session ID,
worktree preparation, and a tracked event bus. It must fail before the
correction because no terminal payload is published, then pass with one
successful payload containing the preparation step. Extend the existing
session-resume recovery Playwright scenarios after their idle wait to assert
the shared chat surface has no stale preparation or background-work status.

The mobile surface already uses the same chat state and composition as desktop.
No layout, navigation, touch target, safe-area, or scroll behavior changes. The
existing `mobile-session-resume-recovery.spec.ts` is the nearest shipped mobile
exemplar and provides the rendered mobile regression.

## Tests

- `AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.9`: add
  `TestLaunch_WorktreeResumePublishesPrepareCompleted` in
  `apps/backend/internal/agent/runtime/lifecycle/manager_launch_test.go`.
- Preserve the existing fresh-launch completion test and cover the
  no-preparation ACP resume case in the same focused lifecycle suite.

## E2E tests

- `AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.9`: extend
  `apps/web/e2e/tests/session/session-resume-recovery.spec.ts` to assert the
  recovered desktop chat becomes idle without stale preparation or background
  status.
- Extend
  `apps/web/e2e/tests/session/mobile-session-resume-recovery.spec.ts` with the
  same user outcome in the `mobile-chrome` project. No mobile geometry changes
  are expected.

## Work orders

- [x] [Task 01: Balance resumed preparation events](task-01-balance-resumed-preparation-events.md)

## Verification results

- The lifecycle regression failed before the fix with preparation progress but
  zero completion events, then passed after terminal suppression was aligned
  with `shouldPrepareEnvironment`.
- The focused lifecycle suite passes for resumed worktree preparation, ordered
  runtime progress, terminal error publication, and a launch-level event-free
  ACP resume fast path.
- The existing desktop and mobile branch-recovery flows pass with post-idle
  assertions for both stale preparation and false background work.
- Specification lint and diff validation pass.

## Risks

- Removing the ACP guard entirely would emit synthetic completion for resumes
  that deliberately skip preparation. Reuse `shouldPrepareEnvironment` instead.
- A unit test that invokes only the publication helper could miss divergence in
  launch gating. Exercise the real launch path with tracked events.
- E2E assertions made before the recovered session settles could mistake a
  legitimate transient status for the defect. Assert only after
  `waitForChatIdle`.
