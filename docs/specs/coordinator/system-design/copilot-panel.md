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

### Activity display: exact rules

These rules resolve what the bullets above leave open; each cites the
acceptance criterion it serves.

- **Turn and running.** A turn is the messages sharing one `turn_id`. A turn
  is running while the conversation session's state is `RUNNING`, and ended
  when the state leaves `RUNNING` for any reason (completed, stopped
  `AC-COORDINATOR-COPILOT-004.5`, failed `AC-COORDINATOR-COPILOT-004.6`). The
  status line is not shown while the state is `STARTING`; the start-up rows
  stay visible then (`AC-COORDINATOR-COPILOT-006.4`).
- **Per turn, not per group.** The view builds the chip itself, from the
  turn's messages, when the opt-in prop is set. The default
  `groupActivityMessages` (runs of two or more consecutive same-turn activity
  messages) is not changed, so every other chat groups as today
  (`AC-COORDINATOR-COPILOT-006.5`). Agent text between tool calls does not
  split the turn: the chip holds every activity message of the turn (tool
  calls of types `tool_call`, `tool_edit`, `tool_read`, `tool_execute`,
  `tool_search`, plus `thinking`) except the exceptions below, and sits at the
  position of the turn's first such message. Agent text, proposal cards and
  permission rows render in transcript order around it.
- **Never inside the chip** (`AC-COORDINATOR-COPILOT-006.3`,
  `AC-COORDINATOR-COPILOT-006.6`): every `propose_task_kandev` call, whatever
  its status, including a refused or errored one, which renders as it does
  today (the card, or the plain row on error); and any tool call that has a
  pending permission request, which renders as its own row with Approve and
  Deny until the request is answered, after which it joins the chip if its
  turn has ended. A permission request with no matching tool call renders on
  its own as today.
- **While the turn runs** (`AC-COORDINATOR-COPILOT-006.1`,
  `AC-COORDINATOR-COPILOT-006.6`): every activity message of the running
  turn is hidden, completed ones included, except the exceptions above. A
  `propose_task_kandev` card appears as soon as its call returns.
- **Chip content.** The count is the number of tool-call messages in the chip
  (`thinking` messages are inside when expanded but not counted; a subagent
  or rich-output message is never grouped, as today). A turn with no such
  call, or only excepted calls, gets no chip. A turn with exactly one call
  gets a chip with count 1. Expanding shows the turn's activity messages in
  transcript order with the existing row components. When at least one call
  in the chip ended in `error`, the chip label adds the failed count as text
  ("2 failed"), so status is not carried by colour alone
  (`AC-COORDINATOR-COPILOT-006.7`).
- **Duration.** From `created_at` of the turn's first message to `created_at`
  of its last message (any type, all with that `turn_id`), from the client's
  loaded messages; it does not depend on `Turn` rows. Rounded down to whole
  seconds, minimum `1s`; format `Ns` under a minute, `Nm Ss` under an hour,
  `Nh Mm` beyond. A stopped or failed turn is measured to its last message.
  A call left in a non-terminal status after the turn ended is shown in the
  expanded rows with its status as stored, and is counted.
- **Chip label.** `Checked {{count}} sources · {{duration}}` for a turn with
  no failures, with `_one`/`_other` plurals (`Checked 1 source`); with
  failures `... · {{failed}} failed` follows the duration. The chip is a
  button with `aria-expanded`, collapsed by default. Its expanded state is
  component state: it survives new messages and closing and reopening the
  panel (whose content stays mounted), and resets on a page reload.
- **Status line.** Rendered by the coordinator panel above the composer while
  the state is `RUNNING`, replacing `MessageList`'s working indicator (the
  opt-in prop turns that indicator off for the coordinator panel only).
  Verb: the running tool call of the running turn with the latest
  `created_at` (ties: the later message in transcript order); running means a
  status other than `complete`, `error` or `cancelled`. The verb key is
  `kandevToolStemOf(message)`; a non-Kandev tool, a missing stem or a stem not
  in the table gives the generic verb. With no running tool call (thinking,
  streaming the answer, between calls) the generic verb shows.

  | Stem | Verb |
  | --- | --- |
  | `list_tasks` | Reading tasks |
  | `get_task_conversation` | Reading a conversation |
  | `list_workflows` | Checking workflows |
  | `list_workflow_steps` | Checking workflow steps |
  | `list_repositories` | Checking repositories |
  | `get_coordinator_item` | Looking up an item |
  | `propose_task` | Drafting a proposal |
  | (anything else) | Working |

  Elapsed seconds are `max(0, now - created_at)` of the running turn's first
  message, floored, refreshed once a second, so a reload mid-turn continues
  the count (`AC-COORDINATOR-COPILOT-002.4`); when the turn has no message yet
  the count starts at the client's first sight of the running state. Format as
  for the chip duration. The element is `role="status"` with
  `aria-live="polite"`; the seconds sit in an `aria-hidden` span, so only a
  change of verb is announced. It has no animation under
  `prefers-reduced-motion`.
- **Empty turn.** A turn with no activity messages renders no chip.

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
  Before it is written into the prefix, `<id>` has every run of CR and LF
  characters replaced by one space and is trimmed, so the prefix is always on
  the message's first line; nothing else in `<id>` is escaped, and a title may
  contain `: `, `[` or `]`. `<ref>` is an opaque id (a UUID today) and
  contains no whitespace and no `]`.
  The standing instructions tell the agent that a bracketed reference names
  the item and that `get_coordinator_item_kandev` (for `proposal` and
  `stall`) or the task tools (for `task`) read its evidence. Context ids on
  the wire as structured data stay phase 2 (decision D12); phase 1 carries
  the reference in the message text.
- `user-message-body.tsx` gains a coordinator branch, used only when the task
  origin is `coordinator`. It tries two patterns against the start of the
  stored text, in this order, and uses the first that matches:
  1. the referenced form, `^About (.+?) \[(task|proposal|stall):([^\]\s]+)\]: `
     (`.` does not match a newline; the id is the shortest run that is
     followed by a bracketed reference and `: `), so a title such as
     `Fix: login` gives the tag `about Fix: login`;
  2. the legacy form of earlier messages, `^About ([^\n]+?): `, the id ending
     at the first `: `.
  On a match it renders the text after the match (which may span lines) plus
  an `about <id>` tag; the bracket never appears in the tag or the text. A
  text matching neither pattern, or a matched remainder that is empty,
  renders unchanged with no tag. The stored text is never rewritten. A
  message whose own body happens to contain ` [task:x]: ` after a prefix is
  unaffected, because the shortest match ends at the prefix's bracket. Unit
  tests cover a plain id, a title containing `: `, `[` and `]`, the legacy
  form, a title that contained a newline, and text beginning `About` that
  matches neither.
