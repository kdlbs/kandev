---
id: "03-add-threads-session-switcher"
title: "Add the Threads session switcher"
status: completed
wave: 3
depends_on:
  - "02-activate-task-columns-from-viewport"
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-DECK-001
  - REQ-UI-THREADS-DECK-002
  - REQ-UI-THREADS-DECK-003
  - REQ-PLATFORM-VIEWPORT-SESSION-DELIVERY-001
acceptance_criteria:
  - AC-UI-THREADS-DECK-001.1
  - AC-UI-THREADS-DECK-001.2
  - AC-UI-THREADS-DECK-001.3
  - AC-UI-THREADS-DECK-001.4
  - AC-UI-THREADS-DECK-001.5
  - AC-UI-THREADS-DECK-001.6
  - AC-UI-THREADS-DECK-001.7
  - AC-UI-THREADS-DECK-002.1
  - AC-UI-THREADS-DECK-002.2
  - AC-UI-THREADS-DECK-002.3
  - AC-UI-THREADS-DECK-002.4
  - AC-UI-THREADS-DECK-002.5
  - AC-UI-THREADS-DECK-002.6
  - AC-UI-THREADS-DECK-003.4
  - AC-UI-THREADS-DECK-003.5
  - AC-UI-THREADS-DECK-003.6
  - AC-UI-THREADS-DECK-003.7
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.3
  - AC-PLATFORM-VIEWPORT-SESSION-DELIVERY-001.4
system_design:
  - ../../specs/ui/system-design/threads-conversation-deck.md
  - ../../specs/platform/system-design/viewport-bounded-session-delivery.md
---

# Task 03: Add the Threads session switcher

## Summary

Add column-local existing-session selection, agent identity, desktop same-row
tabs, a phone picker, and exact task/session deep links. Only the selected
session can mount a conversation.

## In scope

- Add pure status and deterministic session-selection helpers with failing
  matrices before UI changes.
- Extend the task-column view model with task review outcome and primary
  session identity without attaching the aggregate pending action to that
  session.
- Load and sort compact task sessions only through the activation adapter from
  Task 02.
- Render switch-only desktop tabs on the right of the metadata row.
- Render a phone pill and `MobilePickerSheet` with 44-pixel rows.
- Keep selection local per task and preserve it through status changes.
- Add `sessionId` to Threads links, validation, focus, and local fallback.
- Show agent profile names and icons in session selectors. Replace the agent
  icon with `GridSpinner` while a session is `STARTING` or `RUNNING`.
- Keep question, permission, working, turn-finished, and review-ready states in
  the task-column status.
- Add required locale keys in `en`, `pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw`.
- Keep `TaskChatPanel` on its standard no-custom-`onSend` path.

## Out of scope

- Session management, global active-session changes, plugin contracts, or
  layout persistence.

## Acceptance

- Desktop tabs share the metadata row and have no add or management actions.
- Switching one task column changes only its selected transcript and full
  session registration.
- Unselected compact statuses update without transcript mounts and do not steal
  selection.
- Session selectors show the effective agent profile name. They use a custom
  session name only when profile data is not available. Settled sessions show
  the agent icon, and active sessions show the grid spinner.
- Plain `WAITING_FOR_INPUT` has no question icon in the task-column status.
  Explicit clarification, permission, and review-ready states remain distinct.
- Phone uses one metadata-row pill and an accessible bottom sheet, not nested
  horizontal tabs.

## Verification

Start with pure selection/status expectations and same-row component assertions
that fail against the primary-session-only column. Then run:

```bash
(cd apps && pnpm --filter @kandev/web test -- --run lib/threads/thread-session-selection.test.ts lib/threads/thread-session-status.test.ts components/threads/thread-session-switcher.test.tsx components/threads/threads-board.test.tsx lib/links.test.ts)
(cd apps && pnpm --filter @kandev/web exec eslint lib/threads/active-threads.ts lib/threads/thread-session-selection.ts lib/threads/thread-session-status.ts components/threads/thread-column.tsx components/threads/thread-session-switcher.tsx components/threads/thread-conversation.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/web/lib/threads/active-threads.ts`
- `apps/web/lib/threads/active-threads.test.ts`
- `apps/web/lib/threads/thread-session-selection.ts`
- `apps/web/lib/threads/thread-session-selection.test.ts`
- `apps/web/lib/threads/thread-session-status.ts`
- `apps/web/lib/threads/thread-session-status.test.ts`
- `apps/web/components/threads/thread-column.tsx`
- `apps/web/components/threads/thread-session-switcher.tsx`
- `apps/web/components/threads/thread-session-switcher.test.tsx`
- `apps/web/components/threads/thread-conversation.tsx`
- `apps/web/app/threads/threads-page-client.tsx`
- `apps/web/lib/links.ts`
- `apps/web/lib/links.test.ts`
- `apps/web/src/locales/*/threads.json`
- `apps/web/components/threads/AGENTS.md`

## Dependencies

- Task 01 supplies exact pending status for unselected sessions.
- Task 02 supplies preload/detail ownership and controls when the session list
  and selected conversation can mount.

## Risks

- Do not use global `tasks.activeSessionId`; it would couple task columns.
- Do not reuse `PreviewSessionBody` or pass a custom `onSend`.
- Long translated labels must keep the metadata row single-line and contained.

## Parallelism

`sequential`

## Results

Implemented and verified. Threads now selects existing sessions per column,
supports desktop tabs and a mobile bottom-sheet picker, preserves selection in
local column state, and accepts task plus session deep links. Status precedence
distinguishes explicit permission and clarification actions from ordinary
waiting, working, terminal, turn-finished, and review-ready states. Focused
frontend tests, typecheck, i18n validation, and changed-file ESLint pass.

The review fix-up shares task-level deck eligibility with task detail, including
terminal primary sessions with a review outcome. Initial session-list failures
now show a recoverable Retry state, and the desktop tab flex item has the width
constraint. The automatic initial load stays quiet after an error until an
explicit retry. The focused frontend suite passes 127 tests.

The selector follow-up resolves each agent profile from the workspace store.
Desktop tabs and phone rows show the agent profile name and icon. A `STARTING`
or `RUNNING` session shows `GridSpinner` in place of the icon.
