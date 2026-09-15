---
status: draft
system: ui
created: 2026-09-12
owners:
  - nova28
---

# Needs-you Inbox Requirements

## Overview

A Kandev agent that calls `ask_user_question_kandev` blocks until a human
answers, and that question is answerable in exactly one place: a panel over the
chat input of the owning task's session. Nothing in the kanban workspace says a
question exists, so a blocked agent is discoverable only by opening the task
that is already blocked. Measured with the shipped bundle read path on
2026-09-12 at 01:06Z, the Kandev workspace held zero answerable bundles and the
whole instance held one, created 2026-09-03T04:54:02Z and unanswered for 8 days
and 20 hours. That is the failure this capability removes.

The Needs-you Inbox is a workspace-scoped destination that lists every
clarification bundle genuinely waiting on a human and lets the operator answer
it without leaving the page. UI owns this contract because the observable
outcome is a navigation destination, a count, and an answer affordance. The
durable clarification record, its resolution semantics, and the bundle
visibility rules remain owned by the integrations system. Because the
population is small on a healthy board, the empty state is the primary screen
rather than an edge case, and must always name what it is not counting.

## Terminology

- **Bundle:** Every question an agent asked in one `ask_user_question_kandev`
  call, resolved as a unit and identified by a `pending_id`.
- **Answerable bundle:** A bundle a caller can resolve right now, as the
  clarification contract already defines it: at least one question still open,
  the bundle's turn is its session's current turn, the session is not terminal,
  every question resolves an identifier, and it is not a parent-question record.
- **Actionable count:** The number of rows the operator can act on now, never a
  queue size or a lifetime total.
- **Sidecar:** Per-user dismiss and snooze state stored beside the
  clarification record, never inside it.

## Requirements

### REQ-UI-NEEDS-YOU-INBOX-001: Needs-you Inbox

**Intent:** Give an operator one workspace-scoped place that names every
question blocking an agent, and lets them clear it to zero without navigating
to each task.

**User story:** As an operator, I want one list of the questions waiting on me,
so that a blocked agent is never invisible and I can unblock it in place.

#### Acceptance criteria

**Destination and gating**

- **AC-UI-NEEDS-YOU-INBOX-001.1:** Where the Needs-you Inbox feature flag is
  enabled, the workspace sidebar shall present a Needs-you entry that navigates
  to the Inbox destination for the active workspace.
- **AC-UI-NEEDS-YOU-INBOX-001.2:** Where the feature flag is disabled, the
  sidebar shall present no Needs-you entry and the Inbox destination shall not
  render Inbox content.
- **AC-UI-NEEDS-YOU-INBOX-001.3:** The Needs-you entry's presence shall not
  depend on Office mode. When Office mode is off and the feature flag is on,
  the entry shall be present; enabling or disabling Office mode alone shall not
  change whether the entry is present.
- **AC-UI-NEEDS-YOU-INBOX-001.4:** The feature flag shall default to disabled
  in the production profile and enabled in the development and end-to-end
  profiles.

**Row set**

- **AC-UI-NEEDS-YOU-INBOX-001.5:** The Inbox shall list exactly the answerable
  bundles whose owning task belongs to the active workspace, and no others.
  Switching the active workspace shall replace the list with that workspace's
  bundles.
- **AC-UI-NEEDS-YOU-INBOX-001.6:** A row shall exist only when its bundle is
  answerable. The Inbox shall not derive a row from session state. Given a
  fixture of sessions in a waiting state that hold no answerable bundle, the
  Inbox shall list zero rows.
- **AC-UI-NEEDS-YOU-INBOX-001.7:** Given a bundle whose newest turns are
  lifecycle-only, the Inbox shall still list that bundle, because the current
  turn is resolved by the shared turn-authority rule that excludes
  lifecycle-only turns.
- **AC-UI-NEEDS-YOU-INBOX-001.8:** The Inbox shall not list a bundle that is a
  parent-question record, that has no resolvable question identifier, whose
  turn is superseded, or whose session is terminal.
- **AC-UI-NEEDS-YOU-INBOX-001.31:** If a read names a workspace outside the
  caller's visible workspace set, then the server shall reject the read with a
  non-disclosing not-found outcome rather than an empty success, and the Inbox
  shall render the error state of AC-UI-NEEDS-YOU-INBOX-001.21 rather than the
  empty state of AC-UI-NEEDS-YOU-INBOX-001.20, so a permission boundary is
  never reported to the operator as "nothing is waiting". Where the scope an
  endpoint requires is one that a caller reaching the workspace can lack, the
  server shall reject with a forbidden outcome and the Inbox shall render the
  same error state; where the required scope is the reach scope itself, the
  not-found outcome above is the only reachable rejection and the forbidden
  outcome shall be unreachable by construction rather than unimplemented. The
  design shall name, per endpoint, which of the two applies. Workspace
  scoping shall be enforced by the server from the request identity; a
  client-supplied workspace identifier shall not by itself grant access. Where
  authentication is disabled, the single-user identity shall be treated as
  having access to every workspace, and this AC shall be satisfied by the
  identity resolution rather than bypassed.

**Ordering and boundaries**

- **AC-UI-NEEDS-YOU-INBOX-001.9:** Rows shall be ordered by bundle creation
  time ascending, and ties shall be broken by `pending_id` ascending. This
  order shall be total and reproducible: the same set of bundles shall always
  produce the same row order. The oldest waiting question shall appear first.
- **AC-UI-NEEDS-YOU-INBOX-001.10:** Questions inside an expanded row shall
  appear in the bundle's canonical question order, the same order the task
  session's own clarification panel uses. That order shall be question index
  ascending, ties broken by question identifier ascending. A question whose
  index is absent, negative, or not a number shall sort as index zero and then
  by identifier, so the order is total and reproducible for any bundle.
- **AC-UI-NEEDS-YOU-INBOX-001.11:** A single read shall return at most a
  bounded page, defaulting to 50 rows and capped at 200. When more listable
  bundles exist than the page returns, the Inbox shall indicate that the list
  is truncated rather than silently showing a partial list as complete. A
  bundle hidden by the operator's own dismiss or snooze shall not consume a
  page slot: given a fixture of 60 answerable bundles of which 20 are hidden
  and a limit of 50, the first page shall return 40 rows and shall not report
  truncation.
- **AC-UI-NEEDS-YOU-INBOX-001.38:** A read shall reject a missing or empty
  workspace identifier, a non-numeric or non-positive limit, and a cursor it
  cannot decode, each as a client error that lists no rows and renders the
  error state; it shall not silently substitute a default for any of them. A
  read that matches nothing shall return an empty list rather than an absent
  one, and shall render the empty state rather than the error state. When two
  reads for different workspaces are in flight, only the response matching the
  currently active workspace shall be applied, so a slower earlier response can
  neither replace the active workspace's list nor resurrect a resolved row.

**Count**

- **AC-UI-NEEDS-YOU-INBOX-001.12:** The sidebar count and the list shall be
  derived from the same response of the same read path. For any fixture, the
  count shall equal the number of rows the Inbox lists, including a fixture
  whose newest turns are lifecycle-only. Any change the Inbox applies to the
  listed rows before the next response confirms it shall apply to the count in
  the same update, so no rendered state exists in which the count disagrees
  with the number of listed rows.
- **AC-UI-NEEDS-YOU-INBOX-001.13:** When the actionable count is zero, no count
  shall be rendered on the sidebar entry. Removing every listed row shall behave
  identically whether those rows were answered or hidden. Where the list is not
  truncated, removing every listed row shall drive the rendered count to absent. Where the list is truncated,
  removing every listed row shall replace the page with the next one and the
  count shall continue to reflect that page, so the count reaches absent only
  when no listable bundle remains in the workspace. Given a fixture of 60
  listable bundles with a limit of 50, dismissing all 50 listed rows shall leave
  the Inbox listing the remaining 10 and shall not render the empty state, so a
  truncated page emptied by hiding can never be reported as caught up.
- **AC-UI-NEEDS-YOU-INBOX-001.14:** When the page is truncated, the rendered
  count shall present the bounded value as a capped indicator rather than as an
  exact total.
- **AC-UI-NEEDS-YOU-INBOX-001.34:** The sidebar count shall be present and
  correct before the operator has opened the Inbox in the current browser
  session. Given a fixture with answerable bundles in the active workspace, a
  fresh load of any workspace route other than the Inbox shall render the count
  on the sidebar entry, and resolving a bundle elsewhere shall reduce that count
  without the Inbox route ever being mounted. A count that becomes correct only
  after the destination is visited does not satisfy this criterion, because the
  operator would have to open the Inbox to learn that they need to.
- **AC-UI-NEEDS-YOU-INBOX-001.40:** Every produced value of the actionable count
  shall carry the bound it was computed under, and every producer shall apply
  the same bound, the same workspace scope, and the same per-operator hiding
  rule as the list read. A produced count shall be accompanied by whether
  further listable bundles exist beyond that bound, so the capped presentation
  of AC-UI-NEEDS-YOU-INBOX-001.14 is representable by every producer. Given a
  fixture of 60 listable bundles and the default limit of 50, the count before the
  Inbox is opened and the count rendered after it is opened shall be the same
  value and shall both present as capped. A produced count shall never be
  applied to the badge while the list it labels is stale: where one response
  cannot supply both the count and the rows it counts, the Inbox shall re-read
  rather than apply the count alone.

**Answering in place**

- **AC-UI-NEEDS-YOU-INBOX-001.15:** When the operator expands a row, the Inbox
  shall present that bundle's real questions, their options, and the bundle's
  shared context, using the same clarification answer component the task
  session renders.
- **AC-UI-NEEDS-YOU-INBOX-001.16:** When the operator submits an answer or a
  rejection from an expanded row, the system shall resolve that bundle and
  resume the owning task without navigating away from the Inbox, and shall
  remove the resolved row from the list.
- **AC-UI-NEEDS-YOU-INBOX-001.17:** While two callers resolve the same bundle
  concurrently, exactly one shall be recorded as the winner. When the Inbox is
  the losing caller, it shall remove the row and shall not present a failure.
  It shall report the winning outcome in a transient non-error notice that
  names the bundle and survives the row's removal, so the outcome is observable
  after the row is gone.
- **AC-UI-NEEDS-YOU-INBOX-001.18:** When the same resolution is submitted twice
  for one bundle, the second submission shall not create a second resolution
  and shall not change the recorded outcome.
- **AC-UI-NEEDS-YOU-INBOX-001.19:** When a listed bundle stops being answerable
  for any reason, the Inbox shall remove its row and reduce the count without a
  manual reload. This shall hold for every exit in
  AC-UI-NEEDS-YOU-INBOX-001.8, not only for the ones a push notification
  announces: the row set shall converge from re-reading the bundle path, and no
  exit shall be able to strand a row by emitting no event. A design that
  removes the row only when a specific event arrives does not satisfy this
  criterion.
- **AC-UI-NEEDS-YOU-INBOX-001.35:** If a submitted resolution fails for any
  reason other than losing to a winner or finding the bundle no longer active,
  then the Inbox shall restore the row and the count to their pre-submission
  values, shall keep the operator's entered answer, and shall present an error
  that offers to submit again. A failed submission shall never leave the row
  removed, because a removed row for an unresolved bundle hides a blocked agent
  with no way back.
- **AC-UI-NEEDS-YOU-INBOX-001.39:** The shared clarification answer component
  shall report each submission's outcome to whichever surface hosts it,
  distinguishing at least: this caller's resolution was recorded; another caller
  had already won, with the winning outcome available to the host; the bundle is
  no longer active; and the submission failed and may be retried.
  AC-UI-NEEDS-YOU-INBOX-001.17 and AC-UI-NEEDS-YOU-INBOX-001.35 shall be
  satisfied through that report, not through a second submission path: the Inbox
  shall not post the resolution itself. Extending the component shall be
  additive: with the task session and Quick Chat hosts
  unchanged, their rendered behaviour and their existing resolution callback
  shall be identical before and after, asserted by a test.

**Empty and error**

- **AC-UI-NEEDS-YOU-INBOX-001.20:** When the Inbox lists no rows for the active
  workspace, it shall render an empty state that names what it is not counting:
  bundles in other workspaces, pending tool-permission requests,
  parent-question records, and the operator's own snoozed or dismissed rows.
  The trigger shall be the absence of listed rows, not the absence of
  answerable bundles, because a dismissed or snoozed bundle remains answerable
  under AC-UI-NEEDS-YOU-INBOX-001.22. When the operator's own dismiss or snooze
  is hiding at least one bundle, the empty state shall say so and offer the
  restore affordance of AC-UI-NEEDS-YOU-INBOX-001.33; when nothing is hidden it
  shall not imply that anything is.
- **AC-UI-NEEDS-YOU-INBOX-001.21:** If the read fails, then the Inbox shall
  render an error state distinct from the empty state, shall not state that
  the operator is caught up, and no count shall be rendered.

**Dismiss and snooze**

- **AC-UI-NEEDS-YOU-INBOX-001.22:** When the operator dismisses or snoozes a
  row, the underlying clarification record shall be unchanged and the bundle
  shall remain answerable from the task session and from the external
  question-answering tools.
- **AC-UI-NEEDS-YOU-INBOX-001.23:** A dismissed or snoozed bundle shall be
  excluded from both the list and the count by the same read, so the count
  shall continue to equal the number of listed rows.
- **AC-UI-NEEDS-YOU-INBOX-001.24:** When a snooze expires, the bundle shall
  reappear in the list and the count, provided it is still answerable, without
  the operator reloading or re-navigating. A snooze shall be treated as expired
  when its expiry instant is less than or equal to server time, so an expiry
  exactly at the current instant reappears rather than remaining hidden.
- **AC-UI-NEEDS-YOU-INBOX-001.25:** Dismiss and snooze shall be per user and
  idempotent: repeating the same dismiss or snooze for one bundle shall leave
  the same single sidecar state.
- **AC-UI-NEEDS-YOU-INBOX-001.32:** Snooze shall offer a bounded set of
  durations of 1 hour, 4 hours, and 24 hours, shall apply 4 hours when the
  operator selects no duration, and shall reject a duration outside that set.
  Expiry shall be evaluated against server time.
- **AC-UI-NEEDS-YOU-INBOX-001.33:** Dismissal shall be reversible. The Inbox
  shall disclose how many answerable bundles in the active workspace are hidden
  by the operator's own dismiss or snooze, both when the list is empty and when
  it is not, and shall provide a way to restore a hidden bundle to the list. A
  bundle blocking an agent shall never become permanently unreachable through
  the Inbox as a result of one dismissal.
- **AC-UI-NEEDS-YOU-INBOX-001.36:** Dismiss, snooze, and restore shall each be a
  server-side write with an addressable contract, authorized on the workspace
  owning the addressed bundle by the same rule as the read. Addressing a bundle
  the caller cannot see shall produce the same non-disclosing not-found outcome
  as AC-UI-NEEDS-YOU-INBOX-001.31, and shall write no sidecar state. Restoring
  a bundle that is not hidden shall succeed and change nothing.
- **AC-UI-NEEDS-YOU-INBOX-001.37:** The operator's hidden bundles shall be
  enumerable, not merely countable: the Inbox shall be able to list the bundles
  its own dismiss or snooze is hiding in the active workspace, with enough
  identity to restore a chosen one. Each enumerated bundle shall carry which of
  dismiss or snooze is hiding it and, when snoozed, when that snooze expires.
  The enumeration shall report a workspace-wide total that is independent of its
  own page bound, and the hidden count of AC-UI-NEEDS-YOU-INBOX-001.33 shall
  equal that total for the same operator and workspace. A page-scoped length
  shall not be used to satisfy this criterion: given a fixture of 60 hidden
  bundles and a limit of 50, the total and the hidden count shall both be 60.
- **AC-UI-NEEDS-YOU-INBOX-001.41:** A snooze expiring shall restore its bundle to
  the count even when the Inbox has never been mounted in the current browser
  session. Given a fixture holding one snoozed answerable bundle, with the
  operator on any other workspace route, the sidebar count shall increase when
  that snooze expires, without a reload and without the Inbox being opened. The
  client shall be able to learn when the operator's earliest snooze expires from
  a response it already reads.

**Consistency with Threads**

- **AC-UI-NEEDS-YOU-INBOX-001.26:** The Inbox shall render each row's status
  using the shared thread status vocabulary, and shall not define a second
  status classifier.
- **AC-UI-NEEDS-YOU-INBOX-001.27:** Given a fixture in which the only pending
  action on every task is an answerable clarification bundle, the Inbox row set
  and the Threads needs-action preset shall name the same tasks. Where the two
  differ by contract, a test shall assert the difference rather than hide it:
  a pending tool-permission request, a parent-question record, a bundle with no
  resolvable question identifier, and a terminal session shall each appear in
  the Threads preset and shall not appear in the Inbox.

**Non-collision, copy, and coverage**

- **AC-UI-NEEDS-YOU-INBOX-001.28:** The Needs-you count shall be stored under
  its own per-workspace state key. No Needs-you code path shall write the
  Office inbox count key.
- **AC-UI-NEEDS-YOU-INBOX-001.29:** All Inbox copy shall be localized in the
  five shipped locales, shall contain no Unicode em dash, and the repository
  i18n ratchet shall pass.
- **AC-UI-NEEDS-YOU-INBOX-001.30:** An end-to-end specification shall cover
  listing an answerable bundle, expanding it, answering it, and observing the
  row removed and the task resumed without navigation.

## Out of scope

- **Re-ask.** Cut from v1 by product decision on 2026-09-11. The Inbox answers
  or rejects a bundle; it never re-issues one.
- **The history bucket and the failed-task bucket.** Separate sibling
  capabilities. v1 renders one bucket and therefore no tab strip; the variant a
  future strip uses is recorded in the system design.
- **Pending tool-permission requests.** They already surface in the Office
  inbox, and admitting them here would give the Inbox two sources and break the
  guarantee that the count and the list come from one read. The empty state
  must name them as uncounted.
- **Cross-workspace rows.** Workspace-scoped by product decision on
  2026-09-11; permissions are the barrier.
- **Any change to the meaning of the current-turn predicate**, the bundle
  visibility rules, or the resolution semantics. This capability consumes them
  unchanged; a change to them is a change to the integrations contract.
- **Backfill or notification on first enable.** Turning the flag on shall not
  emit notifications for bundles that already exist.
- **Retention or archival of sidecar rows.**
