---
status: active
system: ui
created: 2026-08-28
owners:
  - kandev
---

# Threads Conversation Deck Requirements

## Overview

Threads gives a user one horizontally scrollable view of current task
conversations, arranged in full-height columns or a two-row grid. A task can
have several agent sessions, and its tile can switch among those existing
sessions. Only an explicit pending action marks a
question or permission. The deck must show accurate attention states and stay
usable when a workspace has many active tasks.

The UI system owns the responsive interaction and presentation contract. The
platform owns the bounded status and session-stream delivery contract in
[Viewport-bounded Session Delivery](../../platform/requirements/viewport-bounded-session-delivery.md).

## Terminology

- **Task column / tile:** The stable Threads shell for one task. A grid tile
  has the same identity and session-selection behavior as a full-height column.
- **Selected session:** The existing agent session whose conversation is shown
  in a task column.
- **Attention action:** An explicit pending clarification or permission for a
  session.
- **Detail-active column:** A task column that can mount its selected session's
  full conversation under the platform delivery budget.
- **Auto-hide composer:** A presentation preference that collapses routine
  reply controls until hover or keyboard interaction on pointer-based layouts.

## Requirements

### REQ-UI-THREADS-DECK-001: Existing session switching

**Intent:** Let a user follow every existing agent session in a task without
turning Threads into a session-management surface.

**User story:** As a user with several agents on one task, I want to switch the
conversation in its task column, so that I can follow each agent without
leaving Threads.

#### Acceptance criteria

- **AC-UI-THREADS-DECK-001.1:** When a desktop task column has more than one
  session, the system shall show a switch-only session tab list on the right of
  the same header row as the task status, workflow, and step metadata.
- **AC-UI-THREADS-DECK-001.2:** The Threads session control shall not create,
  delete, rename, close, pin, reorder, or change the primary session, and it
  shall not show an add button or a context menu.
- **AC-UI-THREADS-DECK-001.3:** When the user selects a session, only that task
  column shall change its conversation. The selection shall not change another
  task column or the full task page's active session.
- **AC-UI-THREADS-DECK-001.4:** When a valid session stays selected, another
  session status change shall not change the selection.
- **AC-UI-THREADS-DECK-001.5:** When no valid user selection exists, the system
  shall select, in order, a session requested by the URL, a session with an
  explicit pending action, an active session, the primary session, or the
  newest remaining session.
- **AC-UI-THREADS-DECK-001.6:** When the selected session is removed, the system
  shall use the same deterministic fallback without changing the task column's
  position.
- **AC-UI-THREADS-DECK-001.7:** When a task-detail link opens Threads for a
  session that is still a member of the task, the URL shall identify both the
  task and session, the deck shall reveal the task column, and that session
  shall become selected.
- **AC-UI-THREADS-DECK-001.8:** Each selector item shall show the effective agent
  profile name. If profile data is not available, it shall show the custom
  session name or the existing fallback label.
- **AC-UI-THREADS-DECK-001.9:** A settled selector item shall show the agent
  icon. A `STARTING` or `RUNNING` item shall replace that icon with the grid
  spinner.

### REQ-UI-THREADS-DECK-002: Accurate attention state

**Intent:** Show when a person must act without presenting an ordinary
completed turn as an agent question.

#### Acceptance criteria

- **AC-UI-THREADS-DECK-002.1:** When the selected session has a pending
  clarification, the task column shall show a question indicator and a
  localized question label.
- **AC-UI-THREADS-DECK-002.2:** When the selected session has a pending
  permission, the task column shall show a permission indicator and a localized
  permission label.
- **AC-UI-THREADS-DECK-002.3:** When a session is `WAITING_FOR_INPUT` without an
  explicit pending action, the system shall not show a question or permission
  indicator.
- **AC-UI-THREADS-DECK-002.4:** When a task is in its review outcome and no
  session needs an explicit action, the task column shall show a completion
  indicator with the localized label `Ready for review`.
- **AC-UI-THREADS-DECK-002.5:** When a session is `STARTING` or `RUNNING`, its
  selector item shall show the grid spinner without loading its transcript.
- **AC-UI-THREADS-DECK-002.6:** When no valid selection exists and a session
  needs attention, the task column shall select that session without loading
  another transcript first.

### REQ-UI-THREADS-DECK-003: Responsive bounded deck

**Intent:** Keep Threads responsive with many task columns and preserve native
desktop and mobile navigation.

#### Acceptance criteria

- **AC-UI-THREADS-DECK-003.1:** When Threads contains 30 task columns, the deck
  shall preserve stable column order and horizontal scroll geometry while
  detail content is activated only for the current viewport under
  `REQ-PLATFORM-VIEWPORT-SESSION-DELIVERY-001`.
- **AC-UI-THREADS-DECK-003.2:** Before a task column becomes detail-active, the
  system shall show a lightweight shell from task summary data and shall not
  show another task's conversation in that shell.
- **AC-UI-THREADS-DECK-003.3:** When a desktop user scrolls a task column into
  view, the system shall activate its selected conversation without a page
  reload. When the column leaves the detail window, the shell and its local
  selection shall remain available.
- **AC-UI-THREADS-DECK-003.4:** On a phone, the system shall keep one snapped
  task column detail-active and shall use a compact session picker on the right
  of the metadata row instead of a nested horizontal tab strip.
- **AC-UI-THREADS-DECK-003.5:** The phone session picker shall open a bottom
  sheet with one row per existing session. Each row shall have a touch target
  of at least 44 by 44 CSS pixels. Each row shall show the same agent identity
  or grid spinner as the desktop tabs.
- **AC-UI-THREADS-DECK-003.6:** Long task metadata or many session names shall
  stay inside the task-column header. Desktop session tabs may scroll inside
  their right-side region, and the document shall not gain horizontal
  overflow.
- **AC-UI-THREADS-DECK-003.7:** Loading, empty-session, and recoverable
  session-list failure states shall remain inside their task column and shall
  not block horizontal navigation to other columns.
- **AC-UI-THREADS-DECK-003.8:** On a phone, each snapped conversation shall fill
  the available content width. The composer and prose shall remain contained;
  wide code and tables may scroll within their own content region.
- **AC-UI-THREADS-DECK-003.9:** On a phone, the task title shall open a bottom
  sheet listing admitted threads in stable order with their attention state and
  workflow context. Selecting a row shall reveal its conversation and close the
  sheet. When several threads are available, the page topbar shall show the
  visible task's position and thread count.
- **AC-UI-THREADS-DECK-003.10:** The phone page header shall keep the selected
  view and navigation menu directly reachable without horizontal scrolling.
  Secondary tools and status shall remain available from that menu.
- **AC-UI-THREADS-DECK-003.11:** Phone task headers shall prioritize the title,
  attention state, session picker, and Open task action. Workflow and step
  metadata shall remain available in the thread picker. New standalone controls
  shall have a minimum 44-pixel touch target.
- **AC-UI-THREADS-DECK-003.12:** The phone page header shall group page identity
  and the selected view in one control. Task titles shall have up to two lines.
  With multiple threads, position and bounded, noninteractive page indicators
  shall sit inline in the existing page topbar, without instruction text or an
  additional row. Indicators shall follow the visible thread after swiping or
  picker selection. Small decks shall include page dots. Empty decks and a
  single thread shall omit the indicator.
- **AC-UI-THREADS-DECK-003.13:** While a phone swipe makes a different thread
  nearest the viewport center, the page indicator shall update during the
  gesture, without waiting for release, snap completion, session membership,
  or transcript loading. Reversing the swipe shall restore the indicator for
  the nearest thread. Loading completion alone shall not change its position.

### REQ-UI-THREADS-DECK-004: Conversation layouts

**Intent:** Let users inspect twice as many conversations at a readable width
without changing which tasks or sessions they are following.

#### Acceptance criteria

- **AC-UI-THREADS-DECK-004.1:** Columns shall use the existing full-height,
  horizontally scrolling arrangement. Grid shall use two equal-height rows
  at the same minimum readable tile width, with horizontal scrolling for
  additional tasks and independent vertical transcript scrolling in each tile.
- **AC-UI-THREADS-DECK-004.2:** Grid shall place the stable task sequence from
  top to bottom within each visual column, then proceed to the next column.
  An odd final task shall occupy the upper tile without duplication. A single
  admitted task shall fill the available height in either selected layout.
- **AC-UI-THREADS-DECK-004.3:** Changing layout shall retain the admitted task
  set, stable order, selected session per task, and existing reply drafts. It
  shall keep the last interacted surviving task visible, falling back to the
  current visible task and then the first admitted task when needed.
- **AC-UI-THREADS-DECK-004.4:** A reply or status update shall not move a task
  to another tile. Removing a task, changing filters, or following a deep link
  shall use the existing deterministic admission and recovery behavior in
  either layout; a lower-row task shall be directly reachable.
- **AC-UI-THREADS-DECK-004.5:** Both visible grid rows shall show their selected
  live conversations. Offscreen tiles and unselected sibling sessions shall
  remain subject to the existing platform delivery budget. Hovering a composer
  shall not add a session subscription or mark a conversation read.
- **AC-UI-THREADS-DECK-004.6:** Below the phone breakpoint the system shall
  show one full-width, full-height conversation with the existing swipe,
  position indicator, and thread picker. A saved Grid preference shall be
  retained for wider displays without mounting a hidden desktop grid.
- **AC-UI-THREADS-DECK-004.7:** When the available board content height cannot
  fit two 300-CSS-pixel tiles plus their gap, the system shall use Columns and
  explain that Grid needs more height. Increasing the available height shall
  restore Grid without changing the saved preference or task selection.
- **AC-UI-THREADS-DECK-004.8:** Both layouts shall retain the existing Open
  task, task-action, and session-selection controls. Layout selection shall not
  add an in-place enlargement mode or change Open task navigation.

### REQ-UI-THREADS-DECK-005: Composer disclosure

**Intent:** Reclaim transcript space while monitoring conversations and keep
normal replying, attention handling, and agent controls reachable.

#### Acceptance criteria

- **AC-UI-THREADS-DECK-005.1:** With auto-hide disabled, each detail-active
  conversation shall retain its normal composer. With auto-hide enabled and
  no ongoing interaction, draft, or required action, routine composer content
  and controls shall collapse, leaving only the existing CI popover trigger
  when applicable and increasing the visible transcript height. No Reply,
  Stop, plugin, toolbar, or other routine footer controls shall remain visible
  in the collapsed state. With no CI status, the footer shall occupy no space.
- **AC-UI-THREADS-DECK-005.2:** On a fine pointer, dwelling over a task tile
  shall reveal that tile's composer without taking keyboard focus. A pointer
  crossing a tile for less than 150 milliseconds shall not reveal it. After
  leaving for 300 milliseconds, it shall collapse only when no keep-open
  condition remains.
- **AC-UI-THREADS-DECK-005.3:** Keyboard focus on a task tile shall reveal its
  composer, allowing normal navigation to the editor and controls. Explicit
  editor-focus actions shall reveal before focusing the editor. Collapsed controls shall not
  remain invisible tab stops or appear as active controls to assistive tools.
- **AC-UI-THREADS-DECK-005.4:** While an editor has focus, unsent text or
  attachments, a host-owned upload or send in progress, or a focused owned
  menu, picker, or dialog, auto-hide shall keep that composer visible.
  Hiding shall not unmount the editor or its plugin buttons, cancel a plugin
  operation, or revoke its composer capability. Plugin-specific activity is
  not required to be observable by the host; plugin controls hide with the
  rest of the composer when no host-observable hold remains.
  Session changes shall not transfer these states or drafts to another session.
- **AC-UI-THREADS-DECK-005.5:** Pending clarification or permission actions
  and actionable session recovery shall remain visible and usable without
  hover. Required composer content shall stay expanded until that condition
  clears. Agent Stop shall remain available through the revealed composer and
  the existing full task page, using the same cancellation-pending feedback.
- **AC-UI-THREADS-DECK-005.6:** Revealing or collapsing a composer shall not
  resize neighboring tiles or scroll the board. A transcript following the
  latest output shall continue to do so; a user reading history shall retain
  the visible message and its offset, subject only to scroll-range clamping.
  The editor and final messages shall remain reachable in a short grid tile.
- **AC-UI-THREADS-DECK-005.7:** Phone and coarse-pointer layouts shall retain
  the normal visible composer, without overwriting the saved auto-hide
  preference. On pointer-based layouts, explicit Collapse shall preserve the
  draft and return focus to the task tile without immediately reopening it.
  A later tile entry or explicit keyboard reveal shall restore the composer.
  Required actions and host-owned sends/uploads shall prevent collapse and
  explain why.
- **AC-UI-THREADS-DECK-005.8:** On phones, composing shall remain inside the
  single conversation surface. The software keyboard, safe areas, long text,
  and composer menus shall not cover the active reply action or create
  document-level horizontal overflow. New touch controls shall have hit areas
  of at least 44 by 44 CSS pixels.
- **AC-UI-THREADS-DECK-005.9:** Hover, disclosure, and layout changes shall
  preserve normal model, mode, attachments, queue/steering, and submission
  behavior. They shall neither start an agent nor submit, discard, or clear a
  prompt. Full task pages and other chat hosts shall keep their existing
  composer behavior.
- **AC-UI-THREADS-DECK-005.10:** Leaving the detail window shall release the
  chat even if its composer was expanded. Returning shall restore the existing
  session draft without widening the detail window. Transient hover, menu,
  and focus state shall not be restored or persisted as view settings.
- **AC-UI-THREADS-DECK-005.11:** Composer reveal and collapse shall smoothly
  change the allocated height and fade routine content within 250 milliseconds
  after the existing disclosure delay. A reversed interaction shall continue
  from the current visual position. CI shall remain visible and interactive
  throughout. Reduced-motion users shall receive an immediate change; initial
  page rendering and always-visible touch composers shall not animate.

## Out of scope

- Creating, deleting, renaming, reordering, or changing the primary session
  from Threads.
- Persisting a Threads-only session selection across browser restarts.
- Copying transcripts or an unbounded session list into a task status summary.
- Replacing the full task workbench or its independent agent-tab behavior.
- Full DOM virtualization of lightweight task-column shells.
- Adjustable row counts, independent tile resizing, masonry, drag reordering,
  and a new in-place expansion mode.
- Multiple simultaneous phone transcripts or a hover-only touch workflow.

Task-column scope, filters, sort, limits, and saved presentation persistence are in
[Threads Saved Views](threads-saved-views.md).
