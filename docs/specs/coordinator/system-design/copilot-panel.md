---
id: coordinator-copilot-panel-design
title: Coordinator copilot panel design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-28
last_updated: 2026-10-06
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
| `REQ-COORDINATOR-COPILOT-004` | [Panel](#panel), [Path reset boundary](#path-reset-boundary) |
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
  `spa-routes.tsx`). `CoordinatorCopilotResetBridge`, mounted in `app-shell.tsx`,
  keeps it only for the same coordinator's recognized Needs you or Queue path
  and resets it on other paths through the [path reset boundary](#path-reset-boundary).
  Closing the panel changes only `open`, so reopening it on
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

## Path reset boundary

`AC-COORDINATOR-COPILOT-004.12` is enforced by the bridge's pathname effect,
`coordinatorIdFromPath` in `hooks/domains/coordinator/coordinator-path.ts`,
and `useCopilotStore.keepOnlyFor`. The helper returns a coordinator identity
only for `/workspaces/:workspaceId/coordinator/:coordinatorId`, optionally
followed by `/queue`, with an optional final slash on either view. The generic
coordinator page, missing components, other suffixes and unrelated pages
return `null`.

Capture both raw path components and apply the existing
`lib/routing/path.ts::safeDecodePathSegment` once to each through its existing
`matchDouble` helper. Both must decode
successfully before returning the coordinator identity. An invalid escape or
invalid UTF-8 sequence in either component returns `null`, never an exception.
Do not decode the whole pathname or split a decoded component: encoded slashes
are opaque identity content, and a valid decoded percent must not be decoded
again. The helper's return type and coordinator-only slot ownership stay intact;
validating the workspace does not add a workspace key or membership lookup.

`src/spa-routes.tsx::resolveCoordinatorRoute` already safely decodes both
components and declines invalid routes. This local recognition uses the same
decoder without importing the SPA graph, changing its fallback, or introducing
a shared router policy. `lib/routing/client-router.ts::useParams` has a separate
decoder and is outside this boundary; this design makes no universal
malformed-route or full-SPA safety claim.

The bridge continues to read the real `usePathname` subscription and pass the
helper result to `keepOnlyFor`. A different coordinator or `null` resets the
single in-memory slot, including `draftsSwept`; the same coordinator preserves
the complete entry. Close/reopen uses `setOpen` and preserves chip/draft.
No store, controller, transport, conversation or persistence change is needed.

Regression evidence belongs at two levels: direct helper input/output tests
and a mounted bridge with the actual native pathname subscription, history and
popstate events, real Zustand store, and a React error boundary with a mounted
marker. Assert complete slot preservation/reset and marker survival, rather
than a copied parser or mocked pathname. This is viewport-independent state
normalization inside the existing bridge. It changes no layout, touch, scroll,
navigation or copy contract; targeted helper/component evidence satisfies the
mobile-parity pure-state exception without a new browser/E2E scenario.

## Activity display

The panel renders its transcript through opt-in `QuickChatSessionView` props,
the same pattern as `hideSessionSelectors`, so every other chat is unchanged
(`AC-COORDINATOR-COPILOT-006.5`). The props are `hideStartupRows` and
`activityDisplay` (boolean, default `false`; the coordinator panel sets it).
The Ask about this chip is called the context chip below; the tool-call chip
is the activity chip (`ActivityChip`, under `app/coordinator/copilot/`).

- `hideStartupRows`: `hideSuccessfulStartupRows`
  (`components/quick-chat/startup-rows.ts`) drops `prepare_progress` items and
  successful agent-boot `script_execution` messages, but only once a
  successful boot exists in the transcript; a failed or still-starting first
  boot keeps every row. After an earlier successful boot, a later restart
  that fails keeps its own failed boot row while its preparation row is
  hidden (`AC-COORDINATOR-COPILOT-006.4`). A turn group left empty is dropped.
- A status line while the turn runs, above the composer, showing a plain verb
  for the running tool and the elapsed seconds. Activity of the running turn
  is not rendered as rows.
- When the turn ends, its tool calls render as one collapsed activity chip
  with the call count and the turn duration; expanding it shows the existing
  tool rows. `propose_task_kandev` calls stay outside the chip, so the
  proposal card ([proposals](proposals.md)) always renders.
- Every string goes through `t()` in all shipped locales. The exact rules
  below take precedence over these bullets.

### Activity display: exact rules

Each rule cites the acceptance criterion it serves.

- **Turn and running.** A turn is the messages sharing one `turn_id`; a
  message with no `turn_id` is never hidden, never chipped and renders as
  today. The running turn id is derived on every render: the id in
  `turns.activeBySession[sessionId]`
  (`lib/state/slices/session/turn-actions.ts`) when set; otherwise the
  `turn_id` of the latest loaded message that has one (latest `created_at`,
  ties by transcript order). The fallback is needed because the store clears
  the active id when the session settles, which includes
  `WAITING_FOR_INPUT`, and does not restore it when the session returns to
  `RUNNING` after the decision; the derived id therefore carries over from
  the wait to the resumed run without a stored marker. The turn is running
  while the session state is `RUNNING`, or `WAITING_FOR_INPUT` with at least
  one pending permission request (below) whose `turn_id` equals the running
  turn id or is absent; a pending request of an older turn does not keep a
  newer state running. It has ended when neither holds, whatever the reason
  (completed, stopped `AC-COORDINATOR-COPILOT-004.5`, failed
  `AC-COORDINATOR-COPILOT-004.6`); its activity then joins the chip. If the
  state is `RUNNING` and no loaded message has a `turn_id` either, no message
  is hidden or chipped as running, and the status line shows the generic verb
  counting from the client's first sight of that state; the first message
  with a `turn_id` takes over. The status line is not shown while the state
  is `STARTING` (`AC-COORDINATOR-COPILOT-006.9`).
- **Pending permission.** A `permission_request` message is pending when its
  `metadata.status` is absent or `pending`; `approved`, `rejected`, `denied`,
  `expired` and `cancelled` are not. A tool call is awaiting permission when
  its `metadata.tool_call_id` equals a pending request's, for every tool
  message type (`tool_call`, `tool_edit`, `tool_read`, `tool_execute`,
  `tool_search`). It and the request render exactly as today (merged into the
  tool row for `tool_call`, a separate request row with Approve and Deny for
  the other types), never in the chip and never hidden, until the request is
  no longer pending, after which the call joins the chip if its turn has
  ended. A request with no matching tool call renders on its own as today.
- **Per turn, not per group.** With `activityDisplay`, the view builds the
  chip from the visible message list that feeds the default grouping (which
  already excludes subagent children, setup scripts and session status rows).
  The default `groupActivityMessages` is not changed
  (`AC-COORDINATOR-COPILOT-006.5`). The chip holds every activity message of
  the turn (types `tool_call`, `tool_edit`, `tool_read`, `tool_execute`,
  `tool_search`, plus `thinking`) except the exceptions above and below, even
  with agent text between them, and sits at the position of the turn's first
  such message. A user message inside the turn closes the chip, and later
  activity of that turn forms a second chip (a segment); each chip counts and
  measures its own segment. A chip is keyed by turn id plus segment ordinal,
  so a permission-pending call joining it later does not reset its expanded
  state. Agent text, proposal cards and
  permission rows render in transcript order around it. The chip step replaces only the `groupActivityMessages` call inside
  `buildGroupedRenderItems`; the prepare-progress item, the last-agent-error
  item and the footer-action split are built as today, so a failed start and
  a failed session keep their rows (`AC-COORDINATOR-COPILOT-006.4`,
  `006.8`). `hideSuccessfulStartupRows` runs afterwards on the result.
- **Proposals.** A call is a proposal when `kandevToolStemOf(message)` is
  `propose_task`, whatever its status; it is never in the chip
  (`AC-COORDINATOR-COPILOT-006.3`). A proposal shows its card when its call
  has returned without error and its result carries a `proposal_id` (the
  condition `ProposeTaskRenderer` already uses), and it does so as soon as the
  call returns, even while the turn runs (`AC-COORDINATOR-COPILOT-006.6`).
  While the turn runs, a proposal call that has not returned, ended in
  `error` or has no `proposal_id` is hidden (the status line says "Drafting a
  proposal" for a running one); once the turn has ended it renders as today
  (the plain row), still outside the chip.
- **While the turn runs** (`AC-COORDINATOR-COPILOT-006.1`, `006.6`): every
  activity message of the running turn is hidden, completed ones included,
  except the exceptions above.
- **Tool status.** Through `normalizeToolCallStatus` and
  `isTerminalToolCallStatus` (`lib/utils/tool-call-status.ts`): running is any
  status that is not terminal, failed is normalized `error`. A shell call's
  exit code does not change either.
- **Derived, not stored.** The chip, count and duration are computed from the
  currently loaded messages on every render, so a late message or status
  update after the turn ends amends them, and ties in `created_at` fall back
  to transcript order. Only loaded messages count: when older pages load
  later, a turn cut by pagination shows a partial count and duration and
  updates as pages arrive; it is not marked.
- **Chip content.** The count is the number of tool-call messages in the chip
  (`thinking` is inside but not counted; a subagent or rich-output message is
  never grouped, as today). A turn with no such call, or only excepted calls,
  gets no chip; exactly one call gives a chip with count 1. Expanding shows
  the turn's activity messages in transcript order with the existing row
  components. When a call in the chip ended in `error`, the label adds the
  failed count as text ("2 failed") (`AC-COORDINATOR-COPILOT-006.7`).
- **Duration.** For a chip: from `created_at` of the first to that of the
  last message of its segment (any type in the list above with that
  `turn_id`). Only timestamps `parseTurnTimestamp`
  (`lib/state/slices/session/turn-actions.ts`) accepts count; a message with
  an absent or malformed one is skipped for the measure and ordered by
  transcript position, and a segment with no accepted timestamp shows no
  duration (the label omits it). Rounded down to whole seconds, minimum `1s`;
  `Ns` under a minute, `Nm Ss` under an hour, `Nh Mm` beyond. A stopped or
  failed turn is measured to its last message. A call left non-terminal after
  the turn ended is shown with its status as stored, and counted.
- **Chip label.** `Checked {{count}} sources · {{duration}}` with
  `_one`/`_other` plurals (`Checked 1 source`); with failures
  `... · {{failed}} failed` follows the duration; with no duration the
  label is `Checked {{count}} sources` plus the failed part. The chip is a button with
  `aria-expanded`, collapsed by default. Its expanded state is component
  state: it survives new messages and closing and reopening the panel (whose
  content stays mounted); it resets on a page reload and on Ask about this,
  which remounts the view.
- **Status line.** With `activityDisplay`, `QuickChatContent` renders it
  directly above `ChatInputArea` and below `ClarificationPanelSection`, while
  the turn is running. The prop turns off `AgentStatus`'s `RUNNING` spinner
  and its idle last-turn duration only; the `STARTING` label, the background
  work label and the `FAILED` card stay. Verb: the running tool call (tool
  status rule above) of the running turn with the latest `created_at` (ties:
  later in transcript order), keyed by `kandevToolStemOf(message)`. With no
  running tool call, a non-Kandev tool, a missing stem or an unknown stem, the
  generic verb. While the turn runs only because a permission is pending, the
  verb is the waiting row.

  | Stem | Verb |
  | --- | --- |
  | `list_tasks` | Reading tasks |
  | `get_task_conversation` | Reading a conversation |
  | `list_workflows` | Checking workflows |
  | `list_workflow_steps` | Checking workflow steps |
  | `list_repositories` | Checking repositories |
  | `get_coordinator_item` | Looking up an item |
  | `propose_task` | Drafting a proposal |
  | (permission pending) | Waiting for your decision |
  | (anything else) | Working |

  Elapsed seconds are `max(0, now - created_at)` of the running turn's first
  message (client's first sight when its `created_at` is not accepted), floored, formatted like the chip duration including its `1s`
  minimum, refreshed once a second, so a reload mid-turn continues the count
  (`AC-COORDINATOR-COPILOT-002.4`); with no message yet it counts from the
  client's first sight. The element is `role="status"` with
  `aria-live="polite"`; the seconds sit in an `aria-hidden` span, so only a
  change of verb is announced. No animation under `prefers-reduced-motion`.

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
