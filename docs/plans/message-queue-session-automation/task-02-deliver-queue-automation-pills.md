---
id: "02-deliver-queue-automation-pills"
title: "Deliver Queue Automation Pills"
status: completed
wave: 2
depends_on:
  - "01-persist-session-auto-merge-overrides"
plan: "plan.md"
requirements:
  - REQ-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001
  - REQ-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001
acceptance_criteria:
  - AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.1
  - AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.2
  - AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.7
  - AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.8
  - AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.9
  - AC-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001.10
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.1
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.2
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.3
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.4
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.11
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.12
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.13
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.14
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.15
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.16
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.17
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.18
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.19
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.20
  - AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.24
system_design:
  - ../../specs/ui/system-design/message-queue-automation-controls.md
---

# Task 02: Deliver Queue Automation Pills

## Summary

Drive session incarnation, status epoch/generation, policy availability, and
the effective tuple through frontend API, store, live-event, and mutation paths
with stale-update protection. Recompose the expanded queue header into compact
Auto-run and Auto-merge pills, with desktop and mobile Playwright scenarios
written and observed failing before UI implementation.

## In scope

- Bootstrap workspace dependencies once if this worktree has no
  `apps/node_modules`.
- Write failing queue API, store/event ordering, settings, header, desktop/mobile,
  and raw WebSocket helper/browser coverage before production frontend code.
- Preserve the backend JSON names `queue_incarnation_id` and `task_id` on
  `TaskSession`, and `session_incarnation_id` on every session-scoped queue
  payload; do not introduce a partial camel-case mapping layer. Reject the
  whole status when it does not match the current session, and reset prior
  policy ordering only when a new matching incarnation replaces stored queue
  metadata.
- Preserve matching-incarnation status epoch/generation, policy availability,
  and the known Auto-merge value/source/revision through fetch, clear, reorder,
  WebSocket notification, and older-payload compatibility paths.
- Apply availability only from the newest generation in the current process
  epoch, retire prior epochs, reject stale same-source revisions, and reject
  global-source status after the irreversible first session override within
  that incarnation.
- Replace `QueueState.isLoading[sessionId]` booleans with
  `activeOperationBySessionId` tokens containing the current session
  incarnation and a monotonically allocated client operation generation.
  `beginQueueOperation` verifies the current incarnation, rejects a conflict,
  installs a fresh token, and returns it; `finishQueueOperation`
  compare-and-clears only that token while the incarnation remains current.
  Independent refetches own a token, while a mutation's terminal refetch retains
  its caller token. Route every success, error, cancellation, early return,
  refetch, and `finally` path through these APIs.
- Make session and task removal clear queue entries, metadata, and the active
  operation token for every removed session. In `workspace.deleted`, snapshot
  affected session IDs before state removal by joining normalized
  `TaskSession.task_id` records to locally cached tasks for that workspace,
  then call the same session cleanup for each ID. Incarnation validation
  guarantees cached queue metadata has a corresponding current `TaskSession`.
- Add same-process delete/recreate coverage with delayed old-incarnation status
  success and unavailable events plus mutation success, error, refetch, and
  `finally` cleanup while a new-incarnation mutation remains pending. Also
  cover workspace deletion with empty-queue metadata and an active operation
  token.
- Require `task_id`, `session_id`, and `session_incarnation_id` in every
  frontend queue request, including status get and all mutations. `useQueue`
  supplies current identity. Raw E2E helpers require the same triplet, and the
  clean signature cutover migrates every caller rather than inferring identity
  from textual session ID. Stale responses refetch but never retry. One
  matching-token conflict predicate covers both switches, Send Now, and reorder;
  Auto-merge also requires policy availability.
- Place both labeled switch pills beside the queue summary in one wrapping
  header composition and remove the dedicated Auto-run row.
- Preserve the desktop-only pin, all queue actions, one internal scroll owner,
  keyboard semantics, and coarse-pointer hit areas.
- Update the global settings description and test to explain inheritance until
  a session override.
- Add complete English, Portuguese, Simplified Chinese, Hong Kong Chinese, and
  Traditional Chinese locale entries using the repository's generation flow.

## Out of scope

- Backend storage, protocol handlers, and global settings behavior or permissions.
- A Follow global control or inherited/explicit visual badge.
- Changes to row rendering, ordering, manual merge, Send Now, or queue capacity.
- A mobile-only drawer or alternate queue route.

## Acceptance

- RED evidence covers incarnation isolation, complete get/mutation wire
  payloads from typed and raw clients, status ordering, policy availability,
  generation-scoped cleanup, delayed unavailable protection, shared disabling,
  settings explanation, and compact composition.
- Both pills display authoritative state and disable during conflicting work.
  Every queue request sends current task/session/incarnation, never retargets
  after textual ID reuse, and never retries stale work. Focused tests reject
  lower-generation terminal updates, recover from unavailable state, survive
  reload, and preserve global precedence.
- Session/task/workspace removal leaves no queue metadata or operation token.
  Recreating the same textual session ID resets source ordering only for its new
  matching incarnation; delayed status from the removed incarnation is ignored
  before and after the new status. Delayed terminal paths from an old mutation
  cannot clear the active new-incarnation token or re-enable controls.
- Desktop preserves dense pill sizing; `mobile-chrome` proves touch-effective
  controls, wrapping containment, no document horizontal overflow, and the same
  session override outcome.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- --run lib/api/domains/queue-api.test.ts hooks/domains/session/use-queue.test.ts hooks/use-task-removal.test.ts lib/state/slices/session/session-slice.upsert.test.ts lib/state/slices/session/remove-task-session.test.ts lib/ws/handlers/agent-session.test.ts lib/ws/handlers/tasks.deleted.test.ts lib/ws/handlers/workspaces.test.ts components/settings/system/message-queue-settings.test.tsx components/task/chat/queued-ghost-panel-header.test.tsx components/task/chat/queued-ghost-list.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm exec eslint components/settings/system/message-queue-settings.tsx components/task/chat/queued-ghost-panel-header.tsx components/task/chat/queued-ghost-list.tsx hooks/domains/session/use-queue.ts lib/api/domains/queue-api.ts lib/state/app-state-types.ts lib/state/slices/session/session-slice.ts lib/state/slices/session/types.ts lib/ws/handlers/agent-session.ts lib/types/session-events.ts e2e/helpers/api-client.ts e2e/tests/chat/message-queue-scroll-helpers.ts e2e/tests/chat/message-queue.spec.ts e2e/tests/chat/mid-turn-steering.spec.ts e2e/tests/chat/mobile-message-queue-management.spec.ts e2e/tests/ssh/launch-task.spec.ts e2e/tests/task/mobile-sidebar-queued-count.spec.ts e2e/tests/task/sidebar-queued-count.spec.ts e2e/tests/workflow/workflow-queue-helpers.ts e2e/tests/workflow/workflow-manual-move-queue.spec.ts e2e/tests/workflow/mobile-workflow-manual-move-queue.spec.ts)
(cd apps/web && pnpm e2e:run tests/chat/message-queue.spec.ts tests/task/sidebar-queued-count.spec.ts tests/workflow/workflow-manual-move-queue.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-message-queue-management.spec.ts)
```

The Playwright tests run through the managed production build. Record the
expected RED assertion before production UI changes and the final discovered
and passing test counts after implementation.

## Files likely touched

- `apps/web/lib/types/http.ts`
- `apps/web/lib/state/app-state-types.ts`
- `apps/web/lib/state/slices/session/types.ts`
- `apps/web/lib/state/slices/session/session-slice.upsert.test.ts`
- `apps/web/lib/state/slices/session/session-slice.ts`
- `apps/web/lib/state/slices/session/remove-task-session.test.ts`
- `apps/web/lib/types/session-events.ts`
- `apps/web/lib/ws/handlers/agent-session.ts`
- `apps/web/lib/ws/handlers/agent-session.test.ts`
- `apps/web/lib/ws/handlers/tasks.ts`
- `apps/web/lib/ws/handlers/tasks.deleted.test.ts`
- `apps/web/hooks/use-task-removal.ts`
- `apps/web/hooks/use-task-removal.test.ts`
- `apps/web/lib/ws/handlers/workspaces.ts`
- `apps/web/lib/ws/handlers/workspaces.test.ts`
- `apps/web/lib/api/domains/queue-api.ts`
- `apps/web/lib/api/domains/queue-api.test.ts`
- `apps/web/hooks/domains/session/use-queue.ts`
- `apps/web/hooks/domains/session/use-queue.test.ts`
- `apps/web/components/task/chat/queued-ghost-list.tsx`
- `apps/web/components/task/chat/queued-ghost-list.test.tsx`
- `apps/web/components/task/chat/queued-ghost-panel-header.tsx`
- `apps/web/components/task/chat/queued-ghost-panel-header.test.tsx`
- `apps/web/components/settings/system/message-queue-settings.tsx`
- `apps/web/components/settings/system/message-queue-settings.test.tsx`
- `apps/web/src/locales/en/chat.json`
- `apps/web/src/locales/pt-pt/chat.json`
- `apps/web/src/locales/zh-cn/chat.json`
- `apps/web/src/locales/zh-hk/chat.json`
- `apps/web/src/locales/zh-tw/chat.json`
- `apps/web/src/locales/en/system.json`
- `apps/web/src/locales/pt-pt/system.json`
- `apps/web/src/locales/zh-cn/system.json`
- `apps/web/src/locales/zh-hk/system.json`
- `apps/web/src/locales/zh-tw/system.json`
- `apps/web/e2e/tests/chat/message-queue.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/task/sidebar-queued-count.spec.ts`
- `apps/web/e2e/tests/chat/message-queue-scroll-helpers.ts`
- `apps/web/e2e/tests/chat/mid-turn-steering.spec.ts`
- `apps/web/e2e/tests/ssh/launch-task.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-queued-count.spec.ts`
- `apps/web/e2e/tests/workflow/workflow-queue-helpers.ts`
- `apps/web/e2e/tests/workflow/workflow-manual-move-queue.spec.ts`
- `apps/web/e2e/tests/workflow/mobile-workflow-manual-move-queue.spec.ts`

## Dependencies

- Task 01 must provide the queue status field and mutation action before the
  final frontend and browser checks can pass.

## Risks

- Older queue events may omit the new fields; replacing instead of preserving
  metadata would reset the visible switch. Complete tuples and unavailable
  results can also arrive late, so every apply path must enforce status
  epoch/generation, availability, source, and revision semantics.
- A delayed payload from a deleted session can restore stale source precedence
  after textual ID reuse unless queue status is checked against the current
  immutable session incarnation and every removal path clears queue metadata.
- Clearing workspace records before deriving affected sessions loses the only
  local ownership join for empty-queue metadata; snapshot session IDs first and
  reuse the session cleanup action.
- Omitting identity from get, a mutation, or a raw WebSocket helper lets delayed
  work read or change a replacement. Typed/raw clients and every raw-helper
  caller migrate to the server-issued triplet and never retry stale work.
- An unavailable status must reconcile entries without presenting an invented
  OFF value or leaving the control actionable; a lower-generation unavailable
  status must not disable newer success.
- A separate Auto-merge disabled expression can drift from Auto-run and leave
  policy mutation clickable during queue or cancellation work; both use one
  shared conflict predicate.
- Two compact pills plus queue actions may fit desktop but overlap at the mobile
  breakpoint unless the logical groups wrap independently.
- Enlarging the shared switch itself could make desktop controls visually too
  large; coarse-pointer sizing belongs on the effective hit area.
- Global-setting setup in E2E can leak between worker-scoped tests unless the
  original value is restored in teardown.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-MESSAGE-QUEUE-AUTOMATION-CONTROLS-001`
- `REQ-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001`
- `docs/specs/ui/system-design/message-queue-automation-controls.md`
- `apps/web/components/task/chat/queued-ghost-panel-header.tsx`
- `apps/web/hooks/domains/session/use-queue.ts`
- `apps/web/e2e/tests/chat/message-queue.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`

## Results

Completed. Queue clients and store state now retain task/session/incarnation,
status ordering, policy availability, and operation generations. The expanded
header uses wrapping Auto-run and Auto-merge pills with localized accessible
state descriptions and coarse-pointer targets. Focused frontend tests,
typecheck, lint, desktop Playwright, and mobile-chrome Playwright pass.
