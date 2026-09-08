---
created: 2026-08-29
status: done
requirements:
  - REQ-TASKS-QUICK-CHAT-EXPIRATION-001
system_design:
  - ../../specs/tasks/system-design/quick-chat-session-resumption.md
legacy_specs: []
---

# Implementation Plan: Quick Chat Session Resume

## Overview

Mount the existing task-session resumption lifecycle when a persisted Quick Chat tab becomes
visible, then prove the repair at component and browser boundaries. One vertical work order is
sufficient because the component change and its regression evidence share one lifecycle boundary.

## Scope

### In scope

- Resolve the backing task ID for persisted ordinary, configuration, and passthrough Quick Chat
  sessions.
- Invoke the shared open-time session resumption policy for the active Quick Chat tab.
- Preserve the user preference that prevents automatic agent start on open.
- Prove backend-restart recovery and restored session model controls.

### Out of scope

- Backend resume, status, or persistence changes.
- Quick Chat layout, mobile composition, tab navigation, or new user-facing copy.
- Changes to setup tabs, Quick Terminal PTYs, or regular task-page resumption.

## Confirmed root cause

`QuickChatSessionView` calls `useEnsureTaskSession(sessionId)` but does not mount
`useSessionResumption(taskId, sessionId)`. Regular task pages and kanban preview tabs mount that
hook. After a backend restart, opening Quick Chat therefore hydrates the stored session row without
requesting `task.session.status` or launching the resumable agent runtime.

The focused diagnostic reproduction expected `useSessionResumption("task-1", "session-1")` after
rendering a persisted Quick Chat session. Session hydration ran, but the resumption mock had zero
calls.

## Technical approach

### Shared Quick Chat session view

Update `apps/web/components/quick-chat/quick-chat-session-view.tsx` to resolve the backing task ID
from `QuickChatSession.taskId` or `taskSessions.items[sessionId].task_id`, but pass no task ID to
the resumption hook until the task-session row is hydrated. This prevents the status request from
racing the authoritative row fetch. Mount `useSessionResumption` before the persisted session view
selects its presentation. The existing hook owns connection gating, status checks, preference
handling, resume fallback, and stale-result guards.

Keep interrupted task-session hydration retryable when the active Quick Chat tab changes before its
request settles. If another status path inserts a placeholder row first, merge the eventual
authoritative HTTP response instead of cancelling it when the placeholder becomes visible.

### Regression evidence

Create `quick-chat-session-view.test.tsx` with focused mocks that prove descriptor precedence,
hydrated-row fallback, and no resumption before hydration. Add deferred-response coverage to
`use-ensure-task-session.test.ts` for placeholder merging and active-tab cancellation. Add a
backend-restart scenario to `e2e/tests/chat/quick-chat.spec.ts`. The scenario reopens the restored
conversation and observes the resumed-agent boot message. It also verifies that dynamic session
model settings are available.

## Tests

| Acceptance criterion                   | Evidence                                                                                                                                             |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| `AC-TASKS-QUICK-CHAT-EXPIRATION-001.2` | `quick-chat-session-view.test.tsx` proves the active persisted tab mounts shared resumption with current and fallback task identity after hydration. |
| `AC-TASKS-QUICK-CHAT-EXPIRATION-001.2` | Existing `use-session-resumption.test.ts` proves automatic-start preference and resume behavior remain shared.                                       |

## E2E tests

`apps/web/e2e/tests/chat/quick-chat.spec.ts` will cover the user-visible desktop flow: start a Quick
Chat, restart the backend, reload, reopen the restored tab, and observe both successful resume and
restored model controls. The same `QuickChatSessionView` is rendered in the existing full-screen
mobile surface. This repair changes no layout, touch behavior, scrolling, navigation, or viewport
branch. Therefore, shared component coverage and the lifecycle E2E satisfy mobile parity without a
duplicate mobile restart test.

## Work orders

- [x] [Task 01: Resume Persisted Quick Chat Sessions](task-01-resume-persisted-sessions.md) (`done`)

## Verification results

- `pnpm exec vitest run components/quick-chat/quick-chat-session-view.test.tsx`: 4 tests passed.
- `pnpm exec vitest run components/quick-chat/quick-chat-session-view.test.tsx hooks/use-ensure-task-session.test.ts hooks/domains/session/use-session-resumption.test.ts`: 28 tests passed.
- `pnpm run typecheck`: passed.
- `pnpm e2e:run tests/chat/quick-chat.spec.ts -- --grep "resumes a restored session after backend restart" --retries=0`: 1 test passed against the production Vite build.

## Risks

- Resumption must not run for client-only setup tabs or Quick Terminal descriptors.
- Older in-memory conversation descriptors can lack `taskId`. The hydrated task-session fallback
  must trigger the hook when it becomes available.
- Backend restart E2E timing must use session state and visible runtime evidence, not fixed sleeps.
