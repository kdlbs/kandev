---
id: "03-project-comments-across-task-sessions"
title: "Project comments across task sessions"
status: completed
wave: 3
depends_on:
  - "01-persist-shared-plan-comments"
  - "02-consume-comments-during-prompt-admission"
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-COMMENTS-001
  - REQ-TASKS-PLAN-COMMENTS-002
  - REQ-TASKS-PLAN-COMMENTS-003
acceptance_criteria:
  - AC-TASKS-PLAN-COMMENTS-001.1
  - AC-TASKS-PLAN-COMMENTS-001.2
  - AC-TASKS-PLAN-COMMENTS-001.4
  - AC-TASKS-PLAN-COMMENTS-001.5
  - AC-TASKS-PLAN-COMMENTS-001.7
  - AC-TASKS-PLAN-COMMENTS-002.1
  - AC-TASKS-PLAN-COMMENTS-002.2
  - AC-TASKS-PLAN-COMMENTS-002.4
  - AC-TASKS-PLAN-COMMENTS-002.5
  - AC-TASKS-PLAN-COMMENTS-002.6
  - AC-TASKS-PLAN-COMMENTS-003.1
  - AC-TASKS-PLAN-COMMENTS-003.2
  - AC-TASKS-PLAN-COMMENTS-003.5
  - AC-TASKS-PLAN-COMMENTS-003.6
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
---

# Task 03: Project comments across task sessions

## Summary

Replace session-filtered plan drafts with one task-plan snapshot in the web
application. Render it on every Plan and composer, submit references to the
selected session, and route plan-comment Run to the primary.

## In scope

- Task-keyed plan-comment API, state, WebSocket snapshot reconciliation, and
  hooks.
- Shared Plan highlights/badges and a non-removable composer context item in
  every session.
- Awaited Add/edit/delete behavior with localized inline pending and failure
  states in the desktop Popover and mobile Drawer.
- Selected-session Send and primary-session Run request construction.
- Removal of client-side plan-comment prompt formatting and hidden-context
  duplication while preserving every other comment source.

## Out of scope

- Legacy session-storage migration, owned by Task 04.
- Browser E2E, owned by Task 05.
- Visual redesign of Plan or composer surfaces.

## Acceptance

- Changing selected or primary session never changes the task comment snapshot;
  every mounted Plan/composer updates from the same revisioned source.
- Send supplies the displayed refs to its selected session, Run supplies only
  the clicked ref to the primary, and failures preserve editor/composer state.
- Desktop and mobile use the same state/actions, keep current overlay geometry
  and touch behavior, and expose a visible reason when Run has no eligible
  primary.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- lib/ws/handlers/task-plan-comments.test.ts hooks/domains/comments/use-plan-comments.test.tsx hooks/domains/comments/use-pending-comments.test.ts hooks/domains/comments/use-run-comment.test.ts components/task/task-plan-panel.session-switch.test.tsx components/task/task-plan-panel-draft.test.ts components/task/chat/chat-input-area.test.tsx components/task/passthrough-chat-composer.test.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
```

## Files likely touched

- `apps/web/lib/types/backend.ts`
- `apps/web/lib/types/http-agents.ts`
- `apps/web/lib/api/domains/plan-comment-api.ts`
- `apps/web/lib/ws/router.ts`
- `apps/web/lib/ws/handlers/task-plan-comments.ts`
- `apps/web/lib/state/slices/session/types.ts`
- `apps/web/lib/state/slices/session/session-slice.ts`
- `apps/web/lib/state/slices/comments/types.ts`
- `apps/web/lib/state/slices/comments/comments-store.ts`
- `apps/web/hooks/domains/comments/use-plan-comments.ts`
- `apps/web/hooks/domains/comments/use-pending-comments.ts`
- `apps/web/hooks/domains/comments/use-run-comment.ts`
- `apps/web/components/task/task-plan-panel.tsx`
- `apps/web/components/task/plan-selection-popover.tsx`
- `apps/web/components/task/chat/use-chat-panel-state.ts`
- `apps/web/components/task/chat/chat-context-items.ts`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/hooks/use-message-handler.ts`
- `apps/web/components/task/passthrough-chat-composer.tsx`
- `apps/web/src/locales/*/task.json`

## Dependencies

Tasks 01 and 02.

## Risks

- Several session panels can be mounted at once; effects must not issue
  duplicate loads or overwrite a newer task snapshot.
- The shared context chip cannot retain its current remove callback because
  that would become an unlabeled global delete.
- Removing `PlanComment` from the local union must not change persistence or
  send behavior for agent-message and review comments.

## Parallelism

`sequential`

## Inputs

- Requirements: `REQ-TASKS-PLAN-COMMENTS-001` through `003`.
- System design: frontend projection, composer delivery, Run routing,
  responsive behavior, and failure recovery.
- Existing task-plan state, Tiptap projection tags, shared composer context
  items, and `useRunComment` tests.

## Results

- Added a task-keyed, revision-guarded plan-comment snapshot with API and
  WebSocket reconciliation shared by every mounted session surface.
- Plan annotations and every composer now project the same non-removable
  context; ordinary Send submits all displayed references to the selected
  session, while Run submits only its comment to the current primary.
- Added awaited desktop Popover and mobile Drawer mutations, localized error
  states, reconnect refresh, and admission-conflict reconciliation.
- TypeScript, ESLint, i18n checks, and the focused frontend test suite pass.
