---
id: coordinator-copilot-popover-design
title: Coordinator copilot popover design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-28
last_updated: 2026-09-28
requirements:
  - REQ-COORDINATOR-COPILOT-004
  - REQ-COORDINATOR-COPILOT-005
---

# Coordinator copilot popover System Design

## Purpose and boundaries

The popover is the manager's view of a coordinator's conversation on the
Coordinator screens: its launcher, its coordinator read, the open sequence
over the conversation route, the empty state and **Ask about this**. The
conversation task, the attended-only rule, the tool surface and fail-closed
starts are designed in [copilot](copilot.md); the profile messages are in
[coordinators](coordinators.md#validation).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-COPILOT-004` | [Popover](#popover), [Coordinator read](#coordinator-read), [Launcher busy state](#launcher-busy-state), [Opening the conversation](#opening-the-conversation), [Empty conversation](#empty-conversation), [Ended session](#ended-session) |
| `REQ-COORDINATOR-COPILOT-005` | [Ask about this](#ask-about-this) |

## Popover

- `ChatPopoverShell` is extracted from `ConfigChatPanel` (position, size,
  header, close, Escape handling, focus return). `ConfigChatPanel` keeps its
  Expand and behaviour; a snapshot test pins it.
- `CoordinatorCopilot` renders the shell at 420 by 550 pixels, bottom right,
  title `Coordinator: <name>`, no Expand, full width below 640px. It mounts on
  both Coordinator screens for the viewed coordinator only when
  `features.coordinator` is on and the user holds `workspace.manage`;
  otherwise it renders nothing and issues no request.
- The body is `QuickChatSessionView` with new optional props, all defaulting
  to today's behaviour: `automaticRecovery` (default `true`; see
  [Attended only](copilot.md#attended-only)), `hideSessionSelectors` (default `false`;
  hides the mode and model selectors), `taskArchiveState` (when given, it is
  used instead of `resolveTaskArchiveState`, whose fallback cannot see an
  ephemeral task outside the Quick Chat store), `initialDraft` and
  `transformOutgoing`. The popover builds the `QuickChatSession` value it
  passes from the route's response with `kind: "chat"`;
  `QuickChatSessionKind` (`"chat" | "config"`) is not widened, so the Quick
  Chat tab list, selection and `serverIdsByKind` types are untouched. The
  popover passes the route's `archive_state` as `taskArchiveState`; without
  the prop the view behaves as today.
- On wide screens the popover is placed so it leaves the item column's action
  area uncovered at 1200px; a Playwright check asserts no overlap.

### Coordinator read

The screens' coordinator comes from the list, which carries no profile
statuses. The controller therefore issues `GET
/api/v1/workspaces/:id/coordinators/:cid` (`getCoordinator`) itself:

1. once when the screen's coordinator resolves (mount, and again whenever the
   viewed coordinator id changes), for the launcher's busy state. A response
   to this GET that arrives after the viewed coordinator id changed is
   discarded: it sets no launcher session id, subscribes nothing, and only
   the GET issued for the currently viewed coordinator drives the launcher;
2. again each time the popover opens (launcher, **Ask about this**, or Try
   again), before any conversation-route call, so the profile statuses are
   current at open.

The popover does not poll. Statuses that change while it is open are seen at
the next open; in between, the route's 409 or the session start's fail-closed
error ([Fail closed](copilot.md#fail-closed)) covers the change. When the GET fails, the
launcher still renders, not busy, and the popover body shows the open-error
state below for the GET's outcome; no conversation route is called.

### Launcher busy state

The launcher needs a session id without calling the conversation route,
because an open creates a conversation task and a screen visit must not.
The session id is, in order: the `session_id` of the latest successful route
response for this coordinator in this page; otherwise, when the GET's
`conversation_task_id` is not null, the primary session of that task read
through the existing `useTaskSessions(conversation_task_id)` hook (a read;
it resumes and restores nothing). With neither, the launcher is not busy and
nothing is fetched. The controller keeps that session subscribed with the
existing `useSession(sessionId)` hook while either Coordinator screen is
mounted, so state changes arrive whether the popover is open or closed. The
launcher is busy exactly when `isSessionWorking` (`lib/session-working.ts`:
`STARTING`, `RUNNING` or background foreground activity) holds for that
session, the same predicate the Quick Chat launcher uses; any other state,
a missing session or a failed session read shows not busy. The busy launcher
carries a translated accessible name distinct from the idle one.

### Opening the conversation

Each open runs the GET, then, when both statuses are `ok`, the conversation
route. One sequence runs at a time per coordinator: an open while one is in
flight joins it. A response that arrives after the viewed coordinator changed
is discarded. There is no automatic retry. Every outcome maps to one body:

| Outcome | Popover body | Retry |
| --- | --- | --- |
| GET ok, a status not `ok` | the profile messages of [coordinators](coordinators.md#validation), no composer; the route is not called | next open |
| GET 404, or route 404 | "This coordinator no longer exists." | none |
| GET other failure (5xx, other status, network) | "Could not load the coordinator." with **Try again** | **Try again** re-runs the sequence |
| route 200 | `QuickChatSessionView` on the returned session | none needed |
| route 409 `coordinator_profile_unavailable` | the profile messages built from the body's two statuses | next open |
| route 409 `conversation_conflict` | "The conversation changed while opening." with **Try again** | **Try again** (the next open of `AC-COORDINATOR-COPILOT-001.9`) |
| route 502, 500, any other status, network | "Could not open the conversation." with **Try again** | **Try again** |

Every error body keeps the header and Close, and Escape still works; no
composer renders, `SessionRecoveryFeedback` is not shown (it needs a
session), and the Coordinator lists are unaffected. `SessionRecoveryFeedback`
is shown only on the route-200 body, once a session exists and has ended
([Ended session](#ended-session), `AC-COORDINATOR-COPILOT-004.6`). The four
messages and **Try again** are translated copy.

### Empty conversation

When the session's transcript has no messages, the body above the composer
shows the intro text of `AC-COORDINATOR-COPILOT-004.4` and a "Try asking"
heading with one suggestion button whose text is the fixed, translated
"What needs me first, and why?". It does not depend on the list, so it is the
same on both screens and with an empty list. Choosing it replaces the
composer's text with the suggestion and focuses the composer; it sets no chip
and sends nothing. The intro and suggestion disappear once the transcript
has a message.

### Ended session

`QuickChatSessionView` runs with `automaticRecovery={false}`
([Attended only](copilot.md#attended-only)), so `useSessionResumption` never
sets the `error` or `notice` that its own `SessionRecoveryFeedback` renders
from, and its non-passthrough branch does not mount that feedback at all.
The popover therefore owns its ended-session display. Neither
`useSessionResumption` nor `QuickChatSessionView` changes: both are shared
with the task page, mobile and the Settings chat.

- **Detection.** The route-200 body (`ReadyBody`) reads the session row
  `taskSessions.items[session_id]` from the store for the route's
  `session_id`. The row is loaded by `QuickChatSessionView`'s
  `useEnsureTaskSession` and kept live by the launcher's `useSession`
  subscription ([Launcher busy state](#launcher-busy-state)). The session
  has ended exactly when `isTerminalSessionState(row.state)`
  (`lib/ws/handlers/agent-session.ts`: `FAILED`, `CANCELLED`, `COMPLETED`)
  holds. A missing row, or any other state, has not ended. An agent that
  fails to start after the manager's message ends in `FAILED` with the
  launch error in `error_message`, so a start failure needs no separate
  signal.
- **Display.** While the session has ended, the body renders Kandev's
  `SessionRecoveryFeedback` (`components/task/ensure-session-error.tsx`)
  directly above `QuickChatSessionView`, below the chip row, with: `error` =
  the row's `error_message` trimmed, or, when that is empty or absent, the
  existing translated `task:backendRejectedSessionRequest`; `notice` =
  `null`; `recoveryFailure` = `null`; `workspaceId` = the popover's
  workspace; `onRetry` = the open sequence's `retry`; `retryDisabled` =
  `true` while the open sequence is `loading`; the default test id
  `session-recovery-error`. The banner's title, detail and Retry label are
  its existing translated copy; this design adds no copy. The empty-state
  intro and suggestion ([Empty conversation](#empty-conversation)) are not
  shown while the session has ended, even when the transcript is empty. The
  transcript stays visible. The composer is unchanged Kandev behaviour: a
  send into an ended session is refused client-side
  (`requireSessionInputMode`, "Session has ended...") and nothing is sent.
- **Retry.** Retry runs the same open sequence as **Try again** (GET, then
  the route), joining one already in flight. The route replaces the ended
  task ([copilot](copilot.md#conversation-task) step 2), so a 200 carries a
  new `session_id`; the controller stores it as the held route session, the
  launcher follows it, and `QuickChatSessionView` is keyed on the session id
  as well as `askKey`, so no view state of the ended session carries over —
  including the controller's own one-shot chip-seeded draft
  (`useCoordinatorCopilot`'s `pendingDraft`): the controller clears it
  whenever the incoming ready session's id differs from the one it
  previously held for this coordinator, so a question seeded into the ended
  session cannot reappear in the composer of its replacement.
  The new session is `CREATED` and not ended, so the feedback disappears and
  the empty state shows. Any other outcome replaces the body with that
  outcome's row of [Opening the conversation](#opening-the-conversation), as
  for any open. Retry starts no agent and sends no message. The chip entry
  is kept; unsent composer text belongs to the ended session's draft
  storage and does not carry over.
  A new conversation whose agent also fails to start ends the same way and
  shows the feedback again; each Retry archives one ended task. Phase 1 sets
  no cap: every such task needed a manager's message, and all of them are
  deleted with the coordinator or the workspace.
- **Next open and reload.** Every open already runs the open sequence, so
  closing and reopening the popover, **Ask about this** on any item, or
  reloading the page returns the new conversation directly and shows no
  feedback. The feedback appears only
  while the popover holds a session that ended after it was returned.

## Ask about this

- **Store.** A client-memory store holds one entry per coordinator id:
  `{open: boolean, chip: {id, label} | null, draft: string}`, initially
  `{false, null, ""}`. It is not persisted, so a reload starts every entry
  from the initial value; the transcript comes from the server and typed,
  unsent composer text from the composer's existing per-session draft
  storage. Navigating between Needs you and Queue of one coordinator keeps
  its entry, so an open popover stays open with its chip. Viewing another
  coordinator uses that coordinator's own entry, so a chip never carries to a
  different coordinator; returning finds the first entry as it was. A 404 for
  the coordinator (GET or route) clears the entry's `chip` and `draft` at
  once and does not change `open`. When the popover is open it stays
  mounted and shows the gone body of
  [Opening the conversation](#opening-the-conversation), and the entry is
  removed when that popover closes (Close or Escape); when it is closed (the
  launcher's GET of [Coordinator read](#coordinator-read)), the entry is
  removed at once and the launcher shows not busy. A later open starts from
  the initial value. Closing the popover keeps `chip` and
  `draft`. `label` always equals `id` in phase 1; the chip renders as
  `about <id>` with the same translated key as the transcript tag.
- **Item id.** `<id>` is derived from the card: a question, stall or error
  item uses its task's `identifier`, else the task title; a proposal with a
  source task uses that task's `identifier`, else its title; a proposal
  without one uses the proposal title. This is the text the card's head
  already shows, except that a proposal without a source task uses its title
  instead of "New task". The raw value is then normalised so the transcript
  parser (`AC-COORDINATOR-COPILOT-005.3`) always reads it back whole: every
  run of whitespace, line breaks included, becomes one space; leading and
  trailing spaces are removed; and, until none remains, the first `: ` is
  replaced with ` - `. Task and proposal titles are non-empty after
  trimming, so the result is never empty.
- **Ask about this** sets the entry's chip to that id, sets `draft` to the
  translated question `Why is <id> here?`, and sets `open`. The composer's
  text is replaced by the question (typed text is discarded) and focused, not
  sent. A second call, for any item including the same one, replaces the chip
  and the composer text again (`AC-COORDINATOR-COPILOT-005.5`). After the
  question is applied, `draft` returns to `""`.
- `transformOutgoing` prefixes `About <id>: ` (the untranslated wire form)
  while the chip is set; the hint under the composer shows the stored form.
  The chip stays set after a send. Removing the chip sets it to `null` and
  leaves the composer text as it is; the next send has no prefix.
- **Readers.** A viewer without `workspace.manage` keeps **Ask about this** on
  every item as a native `disabled` button with no handler
  (`AC-COORDINATOR-NEEDS-YOU-002.1` still holds), wrapped in a focusable span
  with a translated tooltip "Only workspace managers can ask the
  coordinator."; nothing opens and no request is made.
- `user-message-body.tsx` gains a coordinator branch: when the task origin is
  `coordinator` and the text starts with `About `, up to the first `: `, it
  renders the remainder plus an `about <id>` tag. The stored text is unchanged.

## Related decisions

- [Workspace coordinator in core](../../../decisions/2026-09-26-workspace-coordinator.md)
