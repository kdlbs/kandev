---
id: "03-entry-point-dots"
title: "Entry-point dot rendering"
status: complete
wave: 2
depends_on: ["01-unseen-idle-state"]
plan: "plan.md"
spec: "../../specs/ui/requirements/quick-chat-idle-dot.md"
---

# Task 03: Entry-point dot rendering

Render the decorative red dot on the five Quick Chat entry icons — sidebar
rail item, sidebar shortcut, tablet header button, mobile header button, and
mobile task-switcher sheet button — driven by
`selectQuickChatHasUnseenIdle` for the active workspace.

- **Acceptance:**
  1. The sidebar rail Quick Chat item (`app-sidebar-primary-nav.tsx`, collapsed) shows the dot when the active workspace has an unseen idle session and not otherwise.
  2. The sidebar Quick Chat shortcut (`app-sidebar-new-task-item.tsx`, `sidebar-quick-chat-shortcut`) shows the dot on the `h-3.5 w-3.5` icon when unseen, not otherwise.
  3. The tablet header button (`tablet-quick-chat-button`), the mobile header button (`mobile-quick-chat-button`), and the mobile task-switcher sheet button (`mobile-sheet-quick-chat` in `session-task-switcher-sheet.tsx`) show the dot when unseen, not otherwise.
  4. The dot is `aria-hidden` with `data-testid="quick-chat-unseen-dot"`, positioned in the icon's top-right corner, and does not change button size, labels, or tooltips.

- **Verification:**
  ```sh
  cd apps && pnpm --filter @kandev/web test -- --run \
    components/app-sidebar/app-sidebar-primary-nav.test.tsx \
    components/app-sidebar/app-sidebar-new-task-item.test.tsx \
    components/kanban/kanban-header-mobile.test.tsx \
    components/task/mobile/session-task-switcher-sheet.test.tsx
  ```

- **Files likely touched:**
  - `apps/web/components/app-sidebar/app-sidebar-nav-item.tsx` — `dot?: boolean` prop; relative icon wrapper + absolute dot.
  - `apps/web/components/app-sidebar/app-sidebar-primary-nav.tsx` — pass `dot` to the Quick Chat nav item.
  - `apps/web/components/app-sidebar/app-sidebar-new-task-item.tsx` — `RowActionButton` `dot?: boolean`; pass to the Quick Chat shortcut.
  - `apps/web/components/kanban/kanban-header.tsx` (`TabletQuickActions`) — dot over `IconMessageCircle`.
  - `apps/web/components/kanban/kanban-header-mobile.tsx` — dot over `IconMessageCircle`.
  - `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` (`TaskSwitcherSurfaceHeader`) — `dot` prop threading through the sheet header; dot over `IconMessageCircle` on `mobile-sheet-quick-chat`.
  - Tests: `app-sidebar-primary-nav.test.tsx`, `app-sidebar-new-task-item.test.tsx`, `kanban-header-mobile.test.tsx`, `session-task-switcher-sheet.test.tsx` — extend store fixtures with `quickChat.sessions` + `unseenIdleByWorkspace` (+ `lastSettledAtBySession` where the fixture constructs full `QuickChatState`). Note the existing mocked app-store states are minimal (e.g. `app-sidebar-primary-nav.test.tsx:9-12`, `app-sidebar-new-task-item.test.tsx:28-40`): they must gain `quickChat: { isOpen, sessions, unseenIdleByWorkspace, lastSettledAtBySession }` or the component's `selectQuickChatHasUnseenIdle` call reads undefined state.

- **Dependencies:** task 01 (selector `selectQuickChatHasUnseenIdle(state, workspaceId: string | null | undefined)` — callers pass nullish ids; the selector returns false for them). Task 02 is not required for rendering (the marker actions are consumed by tests), but the feature is only observable end-to-end after task 02 lands.
- **Parallelism:** sequential.

- **Inputs:**
  - Spec: What bullets 1 and 5 (dot placement, decorative).
  - Plan: "Entry-point dots" section.
  - Existing patterns: dot styling like `agent-card-error-dot` (`h-2 w-2 rounded-full bg-red-500`) with a surface ring (`ring-2 ring-background`) so it reads over any background; `RowActionButton` body at `app-sidebar-new-task-item.tsx:48`.

- **Output contract:** summary, files changed, tests run with counts, blockers, risks; update task + plan statuses in the same conversation.

## Results

Rendered decorative markers on sidebar, tablet, mobile header, and task-switcher entries.
Component tests passed.
