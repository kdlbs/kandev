---
status: draft
system: ui
created: 2026-09-14
owners:
  - nova28
---

# Inbox Failed Bucket Requirements

## Overview

The Inbox shipped with one bucket. Its badge counts questions blocking an agent
and nothing else, which is correct and is not changed here. What it leaves out is
the other way an agent stops being useful: it fails, and nothing says so. Measured
2026-09-14, archived excluded: 15 tasks sit in the terminal `FAILED` state, and none
appears on any list an operator opens.

This capability adds the Inbox's second bucket: failed tasks, present and
deliberately off the badge. A failed task is triage, not a decision the operator
owes an agent, so it must be reachable without ever inflating the number that
means "someone is blocked on you". UI owns the bucket, the tab, the row, and the
exclusion from the count. Task failure itself, its state machine, and its retry
behaviour stay owned by the tasks and workflow systems, consumed here unchanged.

This file states the contract. The
[system design](../system-design/inbox-failed-bucket-01.md) argues it, and carries
the input inventory, the prior art and its receipts, and the reasoning behind the
Out of scope entries that turn on a judgement rather than a scope boundary.

## Terminology

- **Failed task:** A task whose own state is the terminal `FAILED` state and which
  is not archived. Defined at the task, not the session: AC-UI-INBOX-FAILED-001.6.
- **Failure instant:** The completion instant of the session
  AC-UI-INBOX-FAILED-001.30 selects. Not the task's last-updated time.
- **Actionable count:** As the Inbox already defines it, the number of rows the
  operator can act on now. Failed tasks are never part of it.

## Requirements

### REQ-UI-INBOX-FAILED-001: Inbox failed bucket

**Intent:** Make every failed task in the active workspace reachable from the
Inbox in one place, ordered newest first, without adding a single unit to the
count that means an agent is blocked on the operator.

**User story:** As an operator, I want the Inbox to show me what failed, so that
a dead agent run is discoverable without opening the board, and so that the
number on the sidebar still only ever means someone is waiting on my answer.

#### Acceptance criteria

**Tab strip and destination**

- **AC-UI-INBOX-FAILED-001.1:** The Inbox shall render a tab strip with exactly two
  tabs, in the order Needs you then Failed, and Needs you shall be selected when no
  tab is otherwise specified.
- **AC-UI-INBOX-FAILED-001.2:** The selected tab shall be addressable through the
  `tab` query parameter on the existing Inbox route, whose only recognised values are
  `needs-you` and `failed`. Opening `?tab=failed` shall render the Failed tab
  directly, without the operator selecting it and without first rendering Needs you.
  An absent, empty, repeated or unrecognised value shall render Needs you rather than
  an error. Selecting a tab shall REPLACE the current history entry, not push one.
- **AC-UI-INBOX-FAILED-001.3:** Where the Inbox feature flag is disabled, no tab
  strip and no failed bucket shall render, by the same gating that already governs
  the Inbox destination.
- **AC-UI-INBOX-FAILED-001.4:** The Needs you tab's row set, ordering, count,
  answer affordances, dismiss and snooze behaviour, and error state shall be
  unchanged by this capability. Its empty state shall be unchanged except for the
  single copy clause AC-UI-INBOX-FAILED-001.23 adds: same render conditions, row
  set, structure and workspace-naming rule, that clause the only observable
  difference. A test shall assert the enumerated behaviour is identical before and
  after, asserting behaviour rather than empty-state copy.

**Row set**

- **AC-UI-INBOX-FAILED-001.5:** The Failed tab shall list exactly those tasks
  that are failed tasks and whose workspace is the active workspace, and no
  others, subject to the bound in AC-UI-INBOX-FAILED-001.12. Switching the
  active workspace shall replace the list with that workspace's failed tasks.
- **AC-UI-INBOX-FAILED-001.6:** A row shall be derived from the task's own terminal
  failed state, never from a session state alone. Given a fixture of tasks whose
  primary session is failed but whose task state is not terminal failed, the Failed
  tab shall list zero rows for them.
- **AC-UI-INBOX-FAILED-001.7:** An archived task shall never be listed, whatever
  its state.
- **AC-UI-INBOX-FAILED-001.8:** When a listed task stops being a failed task for any
  reason, including retry, re-state or archive, the Failed tab shall remove its row
  without the operator reloading or re-navigating. The row set shall converge by
  re-reading the same path, so no exit can strand a row by emitting no event. While
  the Inbox is open, a workspace is active and the browser tab is visible, that
  convergence shall complete within 60 seconds of the change when that window's
  re-read succeeds, and a tab returning to visible shall re-read at once rather than
  waiting out that interval. A failing read falls to AC-UI-INBOX-FAILED-001.22.
  Neither bound depends on which tab is selected.
- **AC-UI-INBOX-FAILED-001.9:** A failed task shall be listed regardless of origin.
  Given a fixture in which most failed tasks are automation runs, the Failed tab
  shall list all of them rather than suppressing them.
- **AC-UI-INBOX-FAILED-001.9a:** `manual` shall be the only origin treated as a
  person; every other value shall be treated as not a person. A `manual` row shall
  render no origin marker. Each of the five non-person origins shall render its own
  distinct marker, not one shared word: `agent_created`, `routine`, `onboarding`,
  `automation_run`, `automation_task`. A non-empty origin this capability does not
  enumerate shall render a stated generic non-person marker, never nothing, never
  the raw value, never a failed row. An absent or empty origin shall render no
  marker, taking the `manual` path.

**Ordering and boundaries**

- **AC-UI-INBOX-FAILED-001.10:** Rows shall be ordered by three keys in this
  order: first, whether the failure instant is unresolvable, unresolvable before
  resolved; second, failure instant descending; third, `tasks.id` ascending,
  compared as a byte-ordered string as in AC-UI-INBOX-FAILED-001.30a, never by a
  collation the storage engine chooses. The order shall be total: the same set of
  failed tasks shall always produce the same row order, so the unresolvable group is
  itself ordered by `tasks.id`.
- **AC-UI-INBOX-FAILED-001.10a:** The placement of unresolvable failure instants
  shall be an explicit sort key and shall never be left to the storage engine's
  default placement of absent values, on which this product's two engines disagree.
  The row order shall be identical under every engine this product supports, and a
  test shall assert it under each rather than one.
- **AC-UI-INBOX-FAILED-001.11:** The failure instant shall be the completion instant
  recorded on the session AC-UI-INBOX-FAILED-001.30 selects, never the task's
  last-updated time. Given a fixture whose task was written after it failed, the row
  shall report the instant it failed rather than when it was last written. Where that
  session records no completion instant, the failure instant is unresolvable and
  AC-UI-INBOX-FAILED-001.30 governs; no other field shall be substituted.
- **AC-UI-INBOX-FAILED-001.12:** A single read shall return at most a bounded
  page, matching the bound the Needs you bucket already uses: an absent limit
  shall default to 50, a limit above 200 shall be clamped to 200 rather than
  rejected, and a limit of 200 or below shall be honoured exactly. When more
  failed tasks exist than the page returns, the Failed tab shall indicate the list
  is truncated rather than presenting a partial list as complete. The response
  shall carry that truncation as its own explicit field, never inferred from the
  presence of a cursor, a next-page link, or any other optional field.
- **AC-UI-INBOX-FAILED-001.13:** A read shall reject a missing or empty workspace
  identifier and a non-numeric or non-positive limit, each as a client error that
  lists no rows and renders the error state, never silently substituting a default.
  A read matching nothing shall return an empty list rather than an absent one. The read shall accept and return no cursor: it serves
  exactly one bounded page per AC-UI-INBOX-FAILED-001.12, whose truncation
  indicator is all it says about rows beyond the bound.

**Count**

- **AC-UI-INBOX-FAILED-001.14:** The sidebar count shall be unchanged by this
  capability. No failed task shall contribute to it through any producer, including
  the boot-hydrated producer that renders it before the Inbox is opened.
- **AC-UI-INBOX-FAILED-001.15:** Given a fixture with one or more failed tasks and
  no answerable bundles in the active workspace, the sidebar entry shall render no
  count at all. A zero shall not be rendered in place of the absent count, and the
  presence of failed tasks shall not make it appear.
- **AC-UI-INBOX-FAILED-001.16:** The Failed tab shall carry its own count, on the
  tab and not in the page header, rendered as a secondary-variant badge inside the
  tab control. That count shall be the number of rows carried by the most recent
  response successfully applied for the active workspace, never a separately computed
  total, a count from the start of the result set, or any number that response did
  not carry alongside its rows. Whenever the Failed tab is selected that response is
  also the one supplying its rows, so no rendered state exists in which the count and
  the visible list disagree. It shall remain rendered while the other tab is selected,
  since the tab strip is visible from both. No count shall be rendered before
  a response has been applied for the active workspace, nor where it is zero; an
  unread bucket shall not be presented as one known to hold zero. Changing the active
  workspace shall clear it until a response for the new one is applied.
- **AC-UI-INBOX-FAILED-001.17:** When the failed page is truncated, the tab's count
  shall present the bounded value as a capped indicator rather than an exact total,
  by the same presentation the Needs you count already uses. Given a bound of 200
  and more than 200 failed tasks, the tab shall render the row count marked as
  capped, never a bare 200 and never a total the response did not carry.

**Row content**

- **AC-UI-INBOX-FAILED-001.18:** A row shall present the task's title, the reason it
  failed, how long ago it failed, and a control that opens the task, reachable at
  every viewport width including where a fixed-width button would be withheld.
- **AC-UI-INBOX-FAILED-001.18a:** A row's failure marker shall be the shipped
  TASK-state failed pairing from the shared state-icon module, not that module's
  session-state one, which is a different pairing: this bucket is defined by task
  state per AC-UI-INBOX-FAILED-001.6. This capability shall add no new colour; the
  marker shall use the module's existing error style. A test shall assert the row
  renders that module's task-state pairing rather than a local copy of it.
- **AC-UI-INBOX-FAILED-001.19:** The failure reason shall be the error the task's
  session recorded, truncated by the server so one long message cannot inflate a
  full page; a message longer than the bound shall be truncated at the boundary
  rather than omitted or rejected. Where no reason can be resolved, or the value the
  server returns is empty or only whitespace, the row shall render a stated fallback
  rather than a blank line, and shall still render the rest of the row. That
  emptiness test applies to the value the server returns, after
  AC-UI-INBOX-FAILED-001.19a's truncation, so a reason whose first 512 code points
  are all whitespace takes the fallback.
- **AC-UI-INBOX-FAILED-001.19a:** The server shall produce the returned reason in
  this order, and the order is part of the contract because the steps are not
  commutative. FIRST, the stored value shall be made valid UTF-8 by replacing each
  byte that is not part of a well-formed sequence with the Unicode replacement
  character, so what is measured and returned is valid UTF-8 for every input,
  including one that is not. SECOND, the result shall be truncated to at most 512
  Unicode code points, not bytes, never splitting a code point. A reason already
  valid UTF-8 and 512 code points or fewer shall be returned unchanged; that covers
  this case only and is no licence to return invalid UTF-8 that happens to be short.
  Given a 600-code-point reason in a script encoding to more than one byte per
  character, the response shall carry exactly the first 512 and no partial trailing
  character. The response shall carry no truncated flag for the reason and the row
  shall render no marker.
- **AC-UI-INBOX-FAILED-001.30:** The failure instant and the failure reason shall
  be resolved from exactly one of the task's sessions, by a rule total and
  deterministic for any task: the session marked primary; where more than one is,
  the one that started most recently; where none is, the session that started most
  recently; and where the task has no session, neither field is resolvable. A failed task shall be listed in every one of those cases, including
  the last: an unresolvable failure instant shall place the row first per
  AC-UI-INBOX-FAILED-001.10a, with the row stating the failure time is unknown
  rather than a substituted time, and an unresolvable reason shall render
  AC-UI-INBOX-FAILED-001.19's fallback. No resolution failure
  shall remove a failed task that AC-UI-INBOX-FAILED-001.12's bound would otherwise
  have included; the bound is the only thing that may elide a row.
- **AC-UI-INBOX-FAILED-001.30a:** Where two or more candidate sessions tie at the
  most recent start instant, the session with the lowest session identifier,
  compared as an ascending byte-ordered string, shall be chosen, so repeating a read
  against unchanged data always selects the same session as
  AC-UI-INBOX-FAILED-001.25 requires. The start instant is non-nullable, so this
  covers an equal value, not an absent one.
- **AC-UI-INBOX-FAILED-001.20:** Every row on the Failed tab shall present the
  shared thread status vocabulary's failed status, the entry the rest of the
  product already uses, which carries no attention flag. It shall be selected by
  the task's own terminal failed state, which every listed row has by
  AC-UI-INBOX-FAILED-001.6, and shall not be resolved per row from any session's
  state. Because that vocabulary's resolvers take a session and this row model
  carries none, the failed entry shall be reached directly, never by synthesising a
  stand-in session to pass to a resolver. Exporting that existing entry from the
  module that already defines it is permitted and is not a new entry. This
  capability shall define no second status vocabulary, add no entry to the existing
  one, and duplicate no entry's value locally.
- **AC-UI-INBOX-FAILED-001.20a:** No row on the Failed tab shall render a status
  carrying an attention flag, under any resolution outcome of
  AC-UI-INBOX-FAILED-001.30, including a task with no session at all. Given a
  fixture whose resolved session carries a pending permission request or a pending
  clarification, the row shall still present the failed status.

**Empty and error**

- **AC-UI-INBOX-FAILED-001.21:** When the Failed tab lists no rows for the active
  workspace, it shall render an empty state that names the active workspace, does
  not congratulate the operator, and does not claim they are caught up. Where the
  workspace's name cannot be resolved from client state, the copy shall name no
  workspace rather than the wrong one.
- **AC-UI-INBOX-FAILED-001.22:** If a failed-bucket read fails, then the Failed tab
  shall render an error state distinct from its empty state, shall not state that
  nothing failed, and shall render no count. A failure of one tab's read shall not
  empty, error, or otherwise disturb the other tab, except for the single clause that
  AC-UI-INBOX-FAILED-001.23 governs.
- **AC-UI-INBOX-FAILED-001.23:** The Needs you tab's empty state shall name
  failed tasks among the things it is not counting, and shall say where they are.
  Where the active workspace holds at least one failed task, it shall say so
  rather than implying the workspace is quiet; where it holds none, it shall not
  imply that any exist; and where the failed count is not known, because no failed
  read has been applied for the active workspace or the last one failed, it shall
  omit the clause entirely rather than assert either. This shall hold on the first
  render of the Needs you empty state, without the operator selecting the Failed tab
  first.

**Security**

- **AC-UI-INBOX-FAILED-001.24:** If a failed-bucket read names a workspace
  outside the caller's visible workspace set, then the server shall reject it with
  the same non-disclosing not-found outcome the Inbox already uses rather than an
  empty success, and the Failed tab shall render its error state rather than its
  empty state. Scoping shall be enforced server-side from the request identity; a
  client-supplied workspace identifier shall not by itself grant access. Where
  authentication is disabled, the single-user identity reaches every workspace.

**Idempotency, concurrency, and staleness**

- **AC-UI-INBOX-FAILED-001.25:** The failed-bucket read shall be free of side
  effects and idempotent: repeating it with the same parameters against unchanged
  data shall return the same rows in the same order and write nothing. Nothing in
  this capability shall write task state.
- **AC-UI-INBOX-FAILED-001.26:** When two failed-bucket reads are in flight, only
  the response matching the currently active workspace and the most recently
  issued read shall be applied, so a slower earlier response can neither replace
  the active list nor resurrect a row that a newer response dropped. That guard
  shall NOT key on the selected tab: the failed read runs regardless of which tab
  is selected per AC-UI-INBOX-FAILED-001.23, so a response that arrives while Needs
  you is selected shall still be applied.
- **AC-UI-INBOX-FAILED-001.27:** While a task changes state concurrently with a
  read, the read shall return either the row or no row and shall not fail. Two
  operators reading one workspace at the same instant shall each receive a
  self-consistent page.

**Copy and coverage**

- **AC-UI-INBOX-FAILED-001.28:** All copy added by this capability shall be
  localized in the five shipped locales, shall contain no Unicode em dash, and the
  repository i18n ratchet shall pass.
- **AC-UI-INBOX-FAILED-001.29:** An end-to-end specification shall cover observing
  the Failed tab's count before that tab is ever selected, then selecting it,
  observing a failed task listed with its reason and relative failure time, opening
  the task from the row, and observing that the sidebar count did not change when
  the failed task appeared.

## Out of scope

Each entry is a stated exclusion, not an omission.

- **Pagination past the first bounded page.** The read serves one page of at most
  200 rows and says, through AC-UI-INBOX-FAILED-001.12's truncation indicator,
  that more exist; it takes and returns no cursor.
- **Dismiss, snooze, and restore for failed rows.** A failed task already has three
  real exits, all removing the row under AC-UI-INBOX-FAILED-001.8: retry, move,
  archive.
- **Retrying or restarting a task from the Inbox.** The row's only control opens the
  task.
- **The history bucket ("Asked, unanswered" in the deck).** A separate sibling
  capability. The tab strip has room; the third tab is not built here.
- **Severity, ranking, or grouping of failures.** Every failed task is one row. No
  dot, no chip, no rollup of repeated identical failures.
- **Notification on failure.** Enabling this emits no notification for a task that
  already failed, and adds none for a task that fails later.
- **Retention, archival, or pruning of failed rows.** Archived tasks are excluded by
  AC-UI-INBOX-FAILED-001.7.
- **Cross-workspace rows.** Workspace-scoped, by the same product decision and
  permission barrier that scopes the Needs you bucket.
- **Any change to what makes a task failed**, to its state machine, or to the
  workflow engine's behaviour on agent error. Consumed unchanged.
