---
created: 2026-09-28
status: complete
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002
system_design:
  - ../../specs/agents/system-design/agent-resume-runtime-recovery.md
---

# Implementation plan: Task route refresh ownership

## Purpose

Keep task-page data aligned with the task in the current route while navigation
or an asynchronous refresh is in progress. This package implements the existing
navigation ownership contract in
[Agent Resume and Runtime Recovery](../../specs/agents/system-design/agent-resume-runtime-recovery.md).

## Scope

- Use the routed task ID for task-page hydration and foreground refresh.
- Ignore responses from an older task request after route navigation.
- Add regressions for route ownership and out-of-order responses.
- Stabilize the related E2E flows that exercise archive recovery, task navigation,
  and terminal state.

## Work order

- [Task 01: Scope task refresh to its route](task-01-route-owned-task-refresh.md)

## Results

The task page now refreshes the task selected by its route and drops stale fetch
responses. A delayed unarchive callback from an earlier route cannot invalidate
the current route's pending request. E2E coverage waits for durable archive
cleanup and server shell teardown before it asserts the resulting state.
Focused repeated E2E runs and the
task-page unit suite passed locally.
