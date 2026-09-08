---
created: 2026-09-02
status: implemented
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
legacy_specs: []
---

# Implementation Plan: Guard Session Resumption During Navigation

## Overview

Prevent an automatic session-status result from the task being left from
updating the recovery state of the task being opened. Treat the task ID and
session ID together as the request identity and prove the navigation race with
a focused hook regression test.

## Confirmed root cause

- During task navigation, the route can supply the destination session ID
  before the task store replaces the previously active task ID.
- `useSessionResetAndCheck` can therefore start one status request with the old
  task ID and destination session ID, followed by a correct request for the
  destination task and the same session ID.
- The callback guard in `use-session-resumption.ts` compares only the session
  ID. Because both requests use the same session, the stale mismatch response
  remains authorized after the task ID changes.
- A successful current response does not clear an error written later by the
  stale response, so the task view shows `session does not belong to task`.
- The backend ownership check is correct and must remain unchanged.

## Scope

### In scope

- Make the automatic status and recovery callback guard identify both the task
  and session captured for the request.
- Ignore all feedback and session-status writes from a request whose task or
  session is no longer current.
- Add a deterministic hook test that keeps the session ID constant, changes
  the task ID, and resolves the old request after the current request starts.
- Keep the same behavior in desktop and mobile because both use the shared
  session-resumption hook.

### Out of scope

- Relaxing backend task-session ownership validation.
- Changing task routing, store hydration, or session selection behavior.
- Changing recovery copy, layout, controls, or localization.
- Adding a new retry or recovery workflow.

## Technical approach

Replace the session-only active-request reference in
`useSessionResetAndCheck` with a task-session identity and a monotonic
navigation generation. Publish the identity during commit, capture it when a
status check begins, and let guarded setters mutate error, notice, attempt,
and status state only while the complete identity remains current. Keep the
request and recovery protocol unchanged.

Start with a failing regression in the existing stale-callback test group. The
test rerenders the hook with a different task ID and the same session ID, lets
the current request succeed, then delivers the old task's mismatch result. The
observable state must remain successful with no stale recovery error.

## Work orders

- [x] [Task 01: Guard automatic recovery by task-session identity](task-01-guard-resumption-request-identity.md)

## Dependency order

```text
Task 01
```

The production guard and its regression test share one behavioral boundary,
so the package is sequential.

## Verification strategy

- Run the new test before the production edit and record its expected failure.
- Run the focused session-resumption test file after the minimal guard change.
- Run targeted lint and the web typecheck.
- No new Playwright scenario is required. This is pure shared state
  normalization with no rendered layout, touch target, navigation control, or
  desktop/mobile composition change.

## Risks

- A session-only check can reintroduce the race when two tasks reference the
  same route session during store convergence. Keep the complete identity in
  one comparison boundary and include a generation for repeated navigation to
  the same pair.
- Publishing the identity in a passive effect leaves a commit-to-effect race.
  Update it in a layout effect before asynchronous callbacks can run.
- An over-broad cancellation mechanism could discard a valid response for the
  current task. Test that the current request still updates state normally.
- Updating only error setters would leave stale status or notice writes
  possible. Apply the same identity guard to every callback from the request.

## Package handoff

Implement the single work order with TDD. Record the red and green commands in
the work order and update both plan statuses when verification passes.

## Results

- Automatic status, recovery feedback, and delayed remote-status callbacks are
  guarded by the complete task-session request key and a monotonic generation.
- The active identity is published in a commit-phase layout effect, so a
  previous callback cannot run through the passive-effect setup window.
- Session status snapshots are also keyed by the complete identity, so a task
  change cannot reuse status merely because its session ID is unchanged.
- The deterministic navigation regressions cover both task changes and
  navigation that returns to the same task-session pair. The original test
  failed before the production change with `session does not belong to task`.
- Both focused test files pass 21 tests. Targeted lint, web typecheck, Prettier,
  specification lint, and `git diff --check` pass.
- No Playwright test was added because the change normalizes shared state and
  does not alter layout, touch behavior, scrolling, navigation, or responsive
  composition. Desktop and mobile consume the same corrected hook.
