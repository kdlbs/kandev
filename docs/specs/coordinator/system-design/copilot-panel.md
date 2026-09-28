---
id: coordinator-copilot-panel-design
title: Coordinator copilot panel design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-28
last_updated: 2026-09-28
requirements:
  - REQ-COORDINATOR-COPILOT-004
  - REQ-COORDINATOR-COPILOT-005
  - REQ-COORDINATOR-COPILOT-006
---

# Coordinator copilot panel System Design

## Purpose and boundaries

The copilot as a right-side panel on the Coordinator screens: its layout, its
launcher, its store, its activity display and **Ask about this**. It replaces
the built popover ([copilot popover](copilot-popover.md)) in task 11. The
conversation task, the attended-only rule, the tool surface and fail-closed
starts are designed in [copilot](copilot.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-COPILOT-004` | [Panel](#panel) |
| `REQ-COORDINATOR-COPILOT-005` | [Ask about this](#ask-about-this) |
| `REQ-COORDINATOR-COPILOT-006` | [Activity display](#activity-display) |

## Panel

**Build sequence.** Tasks 05 and 06 shipped the copilot in a popover, whose
design as built is recorded in [copilot popover](copilot-popover.md):
`ChatPopoverShell`, extracted from `ConfigChatPanel`, which keeps its Expand
(`AC-COORDINATOR-COPILOT-004.9`, pinned by task 05's tests). Tasks 08 and 09
finish on that popover. Task 11 then swaps the popover for the right-side panel
below, before task 10 builds the activity display on the panel.
`ConfigChatPanel` keeps using `ChatPopoverShell` after the swap. The bullets
below describe the copilot after task 11.

- `RightSidePanel` is extracted from `components/kanban-with-preview.tsx`: the
  inline or floating layout from `useKanbanLayout` (inline while the main
  column keeps `PREVIEW_PANEL.MIN_KANBAN_WIDTH_PERCENT` of the container,
  otherwise `fixed` from the right edge above a backdrop, bottom at
  `--app-status-bar-height`), the backdrop's click calling the caller's
  `onClose`, the left-edge `ResizeHandle`, and the width clamp from
  `getRenderedPreviewPanelWidth` and the `PREVIEW_PANEL` bounds. The caller
  supplies the width storage, so the board keeps `setKanbanPreviewState` and
  its key, and the board preview renders through the extracted component with
  its behaviour unchanged (`AC-COORDINATOR-COPILOT-004.13`; its existing tests
  and a layout test pin it).
- **Escape is not part of `RightSidePanel`.** The board keeps its own
  window-level `useEscapeKey` in `KanbanWithPreview`, with its actions-menu and
  step-disclosure gating, unchanged. The copilot handles Escape with a
  `keydown` handler on the panel's root element, so Escape closes the copilot
  only while focus is inside the panel (`AC-COORDINATOR-COPILOT-004.7`); an
  Escape with focus elsewhere does nothing to the copilot.
- **Mobile full screen is new behaviour.** The board preview has no mobile
  mode: below the mobile breakpoint `KanbanWithPreview` renders the board alone
  and a card click navigates to the task. `RightSidePanel` gains an opt-in
  `mobileFullScreen` prop, default `false`. With it set and
  `useResponsiveBreakpoint().isMobile` true, the panel renders `fixed` over the
  whole viewport above `--app-status-bar-height`, with no backdrop, no
  `ResizeHandle` and no horizontal overflow. The board does not set it and
  keeps its mobile behaviour. The copilot sets it
  (`AC-COORDINATOR-COPILOT-004.8`). Task 11 owns it, with a `RightSidePanel`
  unit test and `tests/coordinator/mobile-copilot.spec.ts` on `mobile-chrome`.
- `CoordinatorCopilot` renders `RightSidePanel` beside the Coordinator
  screens' list, full content height, header title `Coordinator: <name>`,
  Close and no maximize. Its width is stored in local storage under its own
  key (`kandev.coordinatorCopilot.width`), default
  `PREVIEW_PANEL.DEFAULT_WIDTH_PX`, shared by every coordinator. Close, Escape
  inside the panel and the backdrop click (when floating) set `open` to false
  and return focus to the launcher.
- The launcher renders at the bottom right only when the user holds
  `workspace.manage` and the panel is closed. It is busy while the
  conversation session's state is `STARTING` or `RUNNING`, and the panel
  header shows the same state while open.
- **Launcher state without an open.** The launcher never calls the
  conversation route (a POST that can create a task). The copilot controller
  takes `conversation_task_id` from the coordinator GET it already holds
  ([coordinators](coordinators.md#routes)). When it is null, the launcher shows
  idle. Otherwise the controller sends the existing read-only
  `task.session.list` request for that task (the request `useTaskSession`
  sends), takes the session with `is_primary` (else the first) and its
  `state`, writes it to the session store, and subscribes to that session
  with the WS client's `subscribeSession` (`lib/ws/client.ts`, the
  `session.subscribe` action) so later state changes reach the launcher; it
  unsubscribes when the controller unmounts or the id changes. Neither request resumes, restores or starts anything
  (`AC-COORDINATOR-COPILOT-002.2`, `002.4`). An empty list or a failed request
  leaves the launcher idle and is not retried until the coordinator GET is
  refetched; the next open corrects the state. When the panel opens, the open
  route's `session_id` replaces this one if they differ.
- The copilot store is `{coordinatorId, open, chip: {id, label} | null,
  draft}`, one shape used by this section and [Ask about this](#ask-about-this).
  It is an in-memory client store, not a component state, so it survives the
  page swap between Needs you and Queue (separate routes in
  `spa-routes.tsx`). The copilot controller that both screens render keeps it
  while the path stays under `/workspaces/:id/coordinator/:coordinatorId` with
  the same id, and resets it to closed, no chip and no draft on any other path
  or coordinator id. Closing the panel changes only `open`, so reopening it on
  the same coordinator's screens shows the same chip and draft. It is not
  persisted, so a reload starts closed with no chip and no draft
  (`AC-COORDINATOR-COPILOT-004.12`).
- The body is `QuickChatSessionView` with new optional props, all defaulting
  to today's behaviour: `automaticRecovery` (default `true`; see
  [Attended only](copilot.md#attended-only)), `hideSessionSelectors` (default `false`;
  hides the mode and model selectors), `taskArchiveState` (when given, it is
  used instead of `resolveTaskArchiveState`, whose fallback cannot see an
  ephemeral task outside the Quick Chat store), `initialDraft` and
  `transformOutgoing`. The panel builds the `QuickChatSession` value it
  passes from the route's response with `kind: "chat"`;
  `QuickChatSessionKind` (`"chat" | "config"`) is not widened, so the Quick
  Chat tab list, selection and `serverIdsByKind` types are untouched. The
  panel passes the route's `archive_state` as `taskArchiveState`; without
  the prop the view behaves as today.
- The empty state shows the intro text and one suggestion that fills the
  composer.
- A Playwright check at a 1440px viewport with the sidebar expanded and the
  default width asserts the panel is inline and overlaps no item action; a
  second check at a narrower viewport asserts it floats with a backdrop.

## Activity display

The panel renders its transcript through opt-in `QuickChatSessionView` props,
the same pattern as `hideSessionSelectors`, so every other chat is unchanged
(`AC-COORDINATOR-COPILOT-006.5`).

- `hideStartupRows`: `hideSuccessfulStartupRows`
  (`components/quick-chat/startup-rows.ts`) drops `prepare_progress` items and
  successful agent-boot `script_execution` messages, but only once a
  successful boot exists in the transcript; a failed or still-starting boot
  keeps every row. A turn group left empty is dropped.
- A status line above the composer, shown while the session is running, maps
  the latest running tool call to a plain verb through a fixed table keyed by
  tool name, with a generic fallback verb, and shows the seconds since the
  turn started. Running tool calls are not rendered as rows.
- When the turn ends, the turn group's tool calls render as one collapsed chip
  with the call count and the turn duration; expanding it shows the existing
  tool rows. `propose_task_kandev` calls are taken out of the group before
  collapsing, so the proposal card ([proposals](proposals.md)) always renders.
- Every string goes through `t()` in all shipped locales.

## Ask about this

- The copilot store (`{coordinatorId, open, chip: {id, label, ref: {kind, id}} | null, draft}`,
  see [Panel](#panel)) holds the chip and the draft. `ref.kind` is `task`,
  `proposal` or `stall`; `ref.id` is the proposal id for a proposal and the
  task id otherwise, taken from the Needs you or Queue item, never parsed from
  display text.
  **Ask about this** sets the chip and the draft `Why is <id> here?`, opens the
  panel and focuses the composer; a second call replaces both.
- `transformOutgoing` prefixes `About <id> [<kind>:<ref>]: ` while the chip is
  set; the hint under the composer shows the readable form `About <id>: ...`.
  The standing instructions tell the agent that a bracketed reference names
  the item and that `get_coordinator_item_kandev` (for `proposal` and
  `stall`) or the task tools (for `task`) read its evidence. Context ids on
  the wire as structured data stay phase 2 (decision D12); phase 1 carries
  the reference in the message text.
- `user-message-body.tsx` gains a coordinator branch: when the task origin is
  `coordinator` and the text starts with `About `, up to the first `: `, it
  renders the remainder plus an `about <id>` tag, dropping a trailing
  `[<kind>:<ref>]` from the tag. The stored text is unchanged.
