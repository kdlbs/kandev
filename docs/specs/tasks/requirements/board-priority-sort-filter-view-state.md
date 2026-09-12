---
status: active
system: tasks
created: 2026-09-03
owners:
  - kandev
---

# Board priority sort and filter view state Requirements

## Overview

Part of the three-document contract headed by
[Board priority sort and filter](board-priority-sort-filter.md), which owns the
overview, terminology, dependencies, the ordering decision, and the priority
filter requirement. Read it first; its `## Terminology` defines every term used
here. The board sort requirement lives in
[Board priority sort and filter ordering](board-priority-sort-filter-order.md).

This document carries the three requirements that are about the two stored view
values rather than about what they do to the board: reaching the controls at
every breakpoint, persisting and converging the values, and keeping the stored
tokens separate from the labels shown for them. The `## Out of scope`
exclusions, which govern **all three** files, live in
[Board priority sort and filter ordering](board-priority-sort-filter-order.md);
`## Prior art` lives in the head document. The division is driven by the
specification size limit, which the set exceeds as one file, not by a boundary in
the contract. The three documents are one contract and none is complete alone.

## Requirements

### REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-003: The controls are reachable at every breakpoint

**Intent:** Keep the board's triage capability whole on a phone, where the
desktop dropdown does not exist.

#### Acceptance criteria

- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-003.1:** The priority filter and the
  board sort control shall each be reachable and operable at every breakpoint at
  which the board is rendered, through the display surface appropriate to that
  breakpoint.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-003.2:** A selection made at one
  breakpoint shall be in effect at every other breakpoint without being re-made.
  From the same persisted values, no breakpoint shall render a different set of
  cards, and none shall render the workflow steps themselves in a different
  order while no drag is in progress; the desktop board already narrows the
  displayed steps for the duration of a drag where the mobile surface does not,
  and that transient affordance predates this capability and is untouched by it.
  The invariance is over the two values this capability persists and over
  the set of cards they admit; it is **not** over the rendered card sequence,
  because the effective view is breakpoint-dependent and already is today:
  `getEffectiveView` (`apps/web/lib/kanban/view-registry.ts`) returns the kanban
  view whenever the breakpoint is mobile, discarding a stored pipeline view.
  Since `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-002.2` pins each view to its own
  native comparator, a person holding `pipeline` and `created_desc` shall see
  workflow-step index then `position` on desktop and `createdAt` descending on
  mobile; that difference shall satisfy this criterion rather than violate it.
  Reordering the mobile surface to match the pipeline view in order to equalise
  the two shall not satisfy this criterion, and is excluded.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-003.3:** Each control shall have an
  accessible name that conveys what it selects, and each priority option shall
  convey its selected state, so that both are operable without relying on the
  visual treatment.

### REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-004: The view selection persists and converges

**Intent:** Make the board come back the way it was left, and make two open tabs
agree, using the mechanism the board's other view options already use.

#### Acceptance criteria

- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.1:** The board sort token and the
  priority filter selection shall persist across a full page reload and across
  closing and reopening the application, through the same server-held user
  settings record that already carries the board's repository filter and hidden
  step ids. The two values shall be carried under exactly the keys `kanban_sort`
  and `kanban_priority_filter_tokens`, in that spelling, at every hop: the client
  payload, the transport, the server request object and the stored record. A hop
  that spells either key differently shall not satisfy this criterion, because
  the server applies a field only when the request carries it, per
  `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.5`, so a mismatched spelling discards
  the write silently rather than failing it.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.2:** Both values shall be scoped
  per user and shall apply to every workflow the person views, matching the
  repository filter and `tasks_list_sort`. They shall not be scoped per
  workflow, per workspace or per browser tab.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.3:** When a stored board sort token
  is absent, empty, or not one of the two defined tokens, the system shall
  resolve it to `created_desc` rather than failing, rendering an empty board,
  or rendering an unspecified order. Resolution shall behave identically on the
  server and on the client, which requires both sides to trim surrounding
  whitespace before matching and then to compare the trimmed value against the
  two tokens exactly. The server's `NormalizeTasksListSort` already trims and
  the client's `parseTasksListSort` does not, so the client is the side that
  shall gain trimming. Replicating that existing asymmetry, under which `"
  priority_desc"` resolves to `priority_desc` on the server but to
  `created_desc` on the client, shall not satisfy this criterion.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.4:** When a stored priority filter
  selection contains a value that is not one of the four priority tokens, the
  system shall ignore that value and apply the remaining valid selections. A
  stored selection whose values are all invalid shall behave as the empty
  selection, displaying every task, rather than displaying none. A stored value
  that is not a list at all, `null` and a bare string included, shall likewise
  resolve to the empty selection rather than being retained, coerced, or
  allowed to reach the membership test. This differs from an **omitted** value,
  governed by `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.7`: omitted keeps what
  the client already holds, malformed resolves to empty.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.5:** When two clients change these
  values concurrently, the system shall accept both writes and the last write
  to **commit** shall determine the stored values. Commit order is not arrival
  order: the server writes under an expected-revision compare-and-set which, on
  a revision conflict, re-reads the row and re-applies the losing request
  against it, so a request that arrived first may commit last and win. This
  criterion is written against commit order deliberately, and a test asserting
  that the earlier-arriving request always loses shall not satisfy it. Each
  client shall converge on the stored values through the existing user-settings
  update notification and its revision guard, which shall continue to drop an
  update older than the state a client already holds. No client shall re-apply
  or resurrect its own superseded selection after converging. Convergence is
  **snapshot-level over the display-settings payload, not per field**: the
  client sends a fixed snapshot of that payload's fields on every write rather
  than a delta of the field that changed, so when two clients concurrently
  change different fields *of that payload* the later write carries its own
  copy of the others and silently overwrites the earlier client's change. That
  is the accepted semantics, matching the repository filter and hidden step ids
  that already travel in that payload, and these two values shall be added to
  it rather than sent as a delta of their own. The overwriting is **bounded to
  that payload**: the server applies each field only when the request carries
  it, so values held in the same record but outside the payload,
  `tasks_list_sort` among them, are preserved. Widening the payload toward the
  whole record in order to satisfy this criterion would violate
  `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.7` and is excluded. After
  converging, every client shall display the stored record as it last arrived.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.6:** When persisting either value
  fails, the board shall continue to render the selection the person just made
  and the control shall continue to show that same selection, so the view and
  the control never disagree. The observable for "not presented as saved" is
  the **absence of any success affirmation**: the system shall not display a
  saved, synced, or confirmed indication for either value at any time, on
  success or on failure, so a failed write is never affirmed. This constrains
  persistence feedback only and does not touch the selected-state affordance
  required by `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-003.3`, which conveys what
  is selected rather than whether it reached the server. A failed persistence
  shall not clear the board, drop cards, revert the view to the previous
  selection, or leave the board in an order matching neither the old nor the
  new selection. Persistence is best effort through the shared settings
  channel, which reports no failure to its caller, so the only user-visible
  consequence of a failed write is that the selection does not survive a
  reload. Adopting a second, failure-reporting persistence path for these two
  values is excluded; see `## Out of scope` in
  [Board priority sort and filter ordering](board-priority-sort-filter-order.md).
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.7:** Adding these values shall not
  clear, reset or alter any other value in the user-settings record, including
  the repository filter, hidden step ids, the task list sort and group, and the
  preview-on-click toggle. A settings update that omits these values shall not
  clear values the client already holds.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.8:** At the request boundary an update
  shall distinguish four cases per value, and
  `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.4` shall govern only values read back
  from storage, never the inbound payload:
  - key **absent**: preserve what is stored, per
    `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.7`;
  - key carrying **`null`**: preserve, identically to absent. Both new values shall
    be optional pointers on the request object, each applied only when non-nil, as
    the fields already in that object are, so `null` and omitted are
    indistinguishable once decoded. Reading 004.4's
    naming of `null` as a licence to clear the stored selection here shall not
    satisfy this criterion;
  - `kanban_priority_filter_tokens` carrying an **empty list**: apply it, clearing
    the selection. The empty list, not `null`, is how a person clears the filter,
    and that distinction shall survive every hop;
  - either key carrying a value of the **wrong JSON type**, a number for
    `kanban_sort` or a bare string for `kanban_priority_filter_tokens` among them:
    reject the whole request with a bad-request error and apply none of its fields,
    which is what decoding already does before any field is examined.

  A hop that collapses `null` onto the empty list, or the empty list onto absent,
  shall not satisfy this criterion, because each pair differs in whether a stored
  selection survives.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.9:** A value the server accepts shall be
  normalized **before** it is stored, so the stored record is always canonical rather
  than merely resolvable. The server shall trim `kanban_sort`, resolve an empty
  result to `created_desc`, and reject a non-empty value outside the two tokens with
  a validation error rather than silently substituting the default, matching
  `applyTasksListPreferences`
  (`apps/backend/internal/user/service/service.go`), which trims, defaults an empty
  value and rejects an invalid one. For `kanban_priority_filter_tokens` the server
  shall reject a member outside the four priority tokens, remove duplicates from the
  remainder per `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-001.8`, and store it in priority
  rank order, so two clients selecting the same set store identical values and
  produce no revision that differs only by ordering. Storing a received value
  verbatim and leaning on read-time resolution shall not satisfy this criterion,
  because `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.5` defines convergence over the
  stored values. `AC-TASKS-BOARD-PRIORITY-SORT-FILTER-004.3` and `004.4` remain the
  **read**-side rules and are not made redundant: they govern records this writer did
  not produce, a row written before this capability existed among them.

### REQ-TASKS-BOARD-PRIORITY-SORT-FILTER-005: Priority tokens are persisted and priority labels are localized

**Intent:** Keep the machine-readable token separate from the human-readable
label, so translating the interface can never change what is stored, compared or
sent.

#### Acceptance criteria

- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-005.1:** The system shall persist,
  transmit and compare priority tokens and board sort tokens verbatim in lower
  case, and shall never store, transmit or compare a translated string for
  either.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-005.2:** The system shall resolve every
  label on these controls at render time, so that changing the active locale
  re-renders them without a reload.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-005.3:** The system shall reuse the four
  kanban priority labels and shall not introduce a second spelling of them. See
  `## Dependencies` for where they are defined.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-005.4:** The system shall provide the
  copy this capability adds in every supported locale, with no value left
  identical to English except where declared verbatim.
- **AC-TASKS-BOARD-PRIORITY-SORT-FILTER-005.5:** No label or related copy added
  by this capability shall contain a Unicode em dash (U+2014).
