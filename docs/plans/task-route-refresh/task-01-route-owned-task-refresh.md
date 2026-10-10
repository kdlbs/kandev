---
id: "01-route-owned-task-refresh"
title: "Scope task refresh to its route"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.7
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Task 01: Scope task refresh to its route

## Summary

The task route owns the visible task identity. A delayed status or recovery
request for a previous task must not overwrite data for the task now shown.

## In scope

- Scope task-page hydration and foreground refresh to the routed task ID.
- Reject a fetch response when a later navigation has started another request.
- Add regressions for a stale global task selection and out-of-order responses.
- Ignore a previous route's delayed unarchive callback before starting a refresh.
- Make archive recovery and terminal E2E assertions wait for durable lifecycle
  state.

## Out of scope

- Changing the backend recovery protocol or task-session identity model.
- Changing the public recovery API or adding a new user-facing recovery state.

## Acceptance

- When task navigation changes during an automatic request, the view ignores a
  result owned by the prior task-session identity.
- A late response from an older route request cannot replace the current task.
- Recovery E2E tests wait until archive cleanup or shell teardown reaches its
  durable state before asserting unarchive or reload behavior.

## Verification

- Task-page unit tests pass.
- Focused mobile and desktop E2E cases pass repeated runs with retries disabled.
- Changed-file lint and the PR documentation-coverage evaluator pass.

## Results

The delayed-unarchive regression failed before the request guard: an old callback
invalidated the new route's pending load and left its details unresolved. The
guard now rejects the old task before advancing the request generation. The
task-page unit suite passes, including the delayed callback and older-response
regressions.
