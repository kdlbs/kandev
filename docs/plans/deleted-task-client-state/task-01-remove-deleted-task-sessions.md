---
id: "01-remove-deleted-task-sessions"
title: "Remove deleted task sessions from the store"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-DELETED-TASK-CLIENT-STATE-001
acceptance_criteria:
  - AC-UI-DELETED-TASK-CLIENT-STATE-001.1
  - AC-UI-DELETED-TASK-CLIENT-STATE-001.2
  - AC-UI-DELETED-TASK-CLIENT-STATE-001.3
system_design:
  - ../../specs/ui/system-design/deleted-task-client-state.md
---

# Task 01: Remove Deleted Task Sessions

## Summary

Call `removeTaskSession` for each deleted task session in the `task.deleted`
handler.

## In scope

- `apps/web/lib/ws/handlers/tasks.ts`
- `apps/web/lib/ws/handlers/tasks.deleted.test.ts` and its shared helper.

## Out of scope

- Archive handling and eviction of live, unviewed sessions.

## Acceptance conditions

- A handler test with sessions found through all three collection paths shows
  each one removed, and a session of another task untouched.
- The existing `task.deleted` and session removal suites pass unchanged.

## Verification

From `apps/`:

```bash
pnpm --filter @kandev/web exec vitest run lib/ws/handlers/ lib/state/slices/session/remove-task-session.test.ts
pnpm --filter @kandev/web typecheck
```

## Results

The new test failed before the handler change and passes after it. 66 test
files and 601 tests pass in the affected directories; typecheck passes.
