---
id: "11-panel-swap"
title: "Swap the popover for the right-side panel"
status: pending
wave: 5
depends_on:
  - "08-proposals-ui"
  - "09-session-recovery"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COPILOT-002
  - REQ-COORDINATOR-COPILOT-004
acceptance_criteria:
  - AC-COORDINATOR-COPILOT-002.4
  - AC-COORDINATOR-COPILOT-004.1
  - AC-COORDINATOR-COPILOT-004.2
  - AC-COORDINATOR-COPILOT-004.7
  - AC-COORDINATOR-COPILOT-004.8
  - AC-COORDINATOR-COPILOT-004.11
  - AC-COORDINATOR-COPILOT-004.12
  - AC-COORDINATOR-COPILOT-004.13
system_design:
  - ../../specs/coordinator/system-design/copilot.md
  - ../../specs/coordinator/system-design/copilot-panel.md
  - ../../specs/coordinator/system-design/coordinators.md
---

# Task 11: Swap the Popover for the Right-Side Panel (WP-4e)

## Summary

Tasks 05 and 06 shipped the copilot in a popover, and tasks 08 and 09 finish on
it. A maintainer asked for a right-side panel laid out like the board's task
preview panel (PR #3981). This work order swaps the popover for that panel, a
frontend change with no backend change, so that task 10 builds the
activity display on the panel. The copilot's content (transcript, composer,
chip, proposal card, recovery feedback) moves into the panel unchanged.

## In scope

- `RightSidePanel` extracted from `components/kanban-with-preview.tsx`: the
  inline or floating layout from `useKanbanLayout`, the backdrop and its
  click calling the caller's `onClose`, the left-edge `ResizeHandle`, and the
  width clamp (`getRenderedPreviewPanelWidth`, `PREVIEW_PANEL` bounds), with
  the width storage supplied by the caller. `KanbanWithPreview` renders its
  preview through it and keeps `setKanbanPreviewState`, its key, its maximize
  action, its window-level `useEscapeKey` with the actions-menu and
  step-disclosure gating, and its mobile behaviour (board alone, card click
  navigates) ([copilot design](../../specs/coordinator/system-design/copilot-panel.md#panel)).
- New `RightSidePanel` behaviour, opt-in: the `mobileFullScreen` prop
  (default `false`). With it set, below the mobile breakpoint the panel covers
  the viewport above `--app-status-bar-height`, with no backdrop, no resize
  handle and no horizontal overflow. Only the copilot sets it.
- The copilot controller renders `RightSidePanel` in place of the popover
  shell: beside the list, full content height, titled `Coordinator: <name>`,
  Close, no maximize; width under `kandev.coordinatorCopilot.width` (default
  500px); `mobileFullScreen` set. `ChatPopoverShell` stays for
  `ConfigChatPanel` only.
- Escape: a `keydown` handler on the panel's root element closes the copilot
  only while focus is inside it. Close, that Escape and the backdrop click set
  `open` false and return focus to the launcher.
- Launcher: shown only while the panel is closed, busy while the session is
  `STARTING` or `RUNNING`; the panel header shows the same state while open.
  After a reload the controller reads `conversation_task_id` from the
  coordinator GET, sends `task.session.list` for it, takes the primary
  session's `state`, and subscribes with `subscribeSession`. It never calls
  the conversation route for this; a null id, an empty list or a failed read
  shows idle.
- The copilot store becomes `{coordinatorId, open, chip, draft}`: it is kept
  across the same coordinator's Needs you and Queue, reset on any other path
  or coordinator id, kept across Close and reopen, and not persisted.
- The Coordinator screens' list column narrows beside the inline panel, so at
  1440px with the sidebar expanded no item action is covered.

## Out of scope

- Any change to the copilot's content, props or session handling from tasks
  05, 06, 08 and 09.
- The activity display (task 10).
- A maximize or Expand action, the launcher on other pages, a Quick Chat tab
  (phase 3).
- Any backend change.

## ASCII UI preview

From [plan UI-02](plan.md#ui-02-the-copilot-coordinator-screens):

```text
+-- Needs you (list narrows) ---------+ +-----------------------------------+
| ! KAN-418 stalled  [Ask about this] | | * Coordinator: Planner        [x] |
| ? KAN-402 asks a question   [...]   | |-----------------------------------|
|                                     | | You: split KAN-418 into two       |
|                                     | | ( ) list_tasks_kandev             |
|                                     | | Planner: I proposed it.           |
|                                     | | ! create_task  Pending Approval   |
|                                     | |-----------------------------------|
|                                     | | [about KAN-418 x]                 |
|                                     | | Ask the coordinator...       [>]  |
|                                     | | sent as "About KAN-418: ..."      |
+-------------------------------------+ +-----------------------------------+
                          left-edge resize; launcher ( * ) hidden while open
```

## Mockup screenshots and scenarios

Screenshots (visual reference; the acceptance criteria govern). They draw the
copilot as a popover: take the content from them and the frame from
`AC-COORDINATOR-COPILOT-004.1`, `004.8` and `004.11`.

- [`docs/plans/workspace-coordinator/assets/p1-02-ask-about-this.png`](assets/p1-02-ask-about-this.png)
- [`docs/plans/workspace-coordinator/assets/p1-05-chat-create-task-proposal.png`](assets/p1-05-chat-create-task-proposal.png)

Mockup scenario specs to port: none new. Task 06's ports of
`18-v21-copilot-anywhere.spec.ts` keep passing on the panel (see the plan's
[Mockup scenario to repo test](plan.md#mockup-scenario-to-repo-test)).

## Acceptance

- The board preview behaves as before: layout rule, resize bounds, persisted
  width, window-level Escape with its gating, backdrop close, maximize, and
  board-only mobile rendering. Its existing unit tests and `tests/kanban` pass
  unchanged.
- A `RightSidePanel` unit test covers the layout rule, the width clamp with a
  caller-supplied storage key, and `mobileFullScreen` on and off.
- `tests/coordinator/copilot.spec.ts` covers:
  - The launcher hidden while the panel is open, and busy on the launcher and
    in the header.
  - Escape with focus inside closes the panel and returns focus to the
    launcher; Escape with focus in the list does not close it.
  - Backdrop close when floating.
  - Resize bounds, and the width persisted separately from the board's.
  - The panel, chip and draft kept from Needs you to Queue and across Close
    and reopen, and cleared on leaving.
  - The panel inline at 1440px with the sidebar expanded and floating at a
    narrower width.
  - After a reload with a turn running, the launcher busy before any open,
    with no conversation-route request in the network log.
  - Task 06's, 08's and 09's copilot cases still passing on the panel.
- `tests/coordinator/mobile-copilot.spec.ts` (`mobile-chrome`): at 390px the
  panel fills the screen with no resize handle and no horizontal scroll.
- Configuration chat on `/settings` unchanged, Expand included.

## Verification

```bash
cd apps/web && pnpm test -- components/right-side-panel components/kanban-with-preview lib/settings/preview-panel-width hooks/use-kanban-preview app/coordinator/copilot hooks/domains/coordinator
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/kanban tests/coordinator/copilot.spec.ts tests/settings/config-chat-popover.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/coordinator/mobile-copilot.spec.ts
```

## Likely files

- `apps/web/components/right-side-panel.tsx` and test, `kanban-with-preview.tsx`
- The coordinator copilot controller, launcher and store from task 06
- `apps/web/e2e/tests/coordinator/copilot.spec.ts`, `mobile-copilot.spec.ts`

## Dependencies

- Tasks 08 and 09 have passed Review: all three change the copilot's
  container, so the swap goes last on the popover. The branch stacks on task
  08's branch with task 09's merged in. Task 10 follows this work order.

## Risks

- The extraction touches the board preview; its unit tests and the
  `tests/kanban` e2e specs must pass unchanged.
- Leaving the board's Escape and mobile behaviour in `KanbanWithPreview` is
  deliberate; a shared listener would change one caller's behaviour.
