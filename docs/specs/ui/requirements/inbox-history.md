---
status: draft
system: ui
created: 2026-09-14
owners:
  - nova28
---

# Inbox History Requirements

## Overview

`REQ-UI-NEEDS-YOU-INBOX-001` shipped the Inbox with one bucket: the answerable
clarification bundles for the active workspace. A bundle leaves it the moment it
stops being answerable, most often when a new turn starts with the old question
unanswered. Those rows are not deleted. They stay in
`task_session_messages` with a non-terminal status, and
[`ADR 2026-08-14`](../../../decisions/2026-08-14-current-turn-clarification-ownership.md)
calls them, at line 76, "useful audit evidence but not operational state by
itself". The gap this capability closes is that the audit evidence is unreadable:
every shipped read path is turn-scoped by design, so nothing returns a question
whose turn has been superseded. This adds a second, read-only bucket that can.

Neither expiry nor the operational predicate changes; the ADR rejects both
alternatives that would close the same gap. The companion
[system design](../system-design/inbox-history.md) carries those alternatives, the
input inventory, the per-read filter table, the reasoning behind each criterion
below, and the code citation for every symbol named here. UI owns this contract
because the observable outcome is a tab, a list and a label; the durable
clarification record, its resolution semantics and the current-turn predicate
remain owned by the integrations system, consumed here unchanged.

## Input inventory

Sampled from the running instance on 2026-09-14, SQLite `mode=ro`. Record shapes,
the status histogram, both population samples and every code citation live in the
companion system design.

**Units, fixed for the whole document.** A *row* is one `task_session_messages`
record. A *bundle* is the rows sharing a `pending_id`; a permission request is
always a bundle of one. A *bundle row* is the rendered list item for one bundle,
and where a criterion below says "row" it means a bundle row. Every count required
below is in **bundles** unless the criterion names another unit. **Terminal
status** is already decided in code by `InteractionStatus.IsTerminal()`
(`interactions.go`): any status other than absent or `pending`.

**Population.** 46 rows, in 41 bundles across 13 tasks, sit in a step that starts
no agent. That is the number justifying the capability, and the only figure that
reproduced across two samples.

## Requirements

### REQ-UI-INBOX-HISTORY-001: Inbox history bucket

**Intent:** Make the interactions the ADR preserves as audit evidence readable,
without changing what counts as operational.

**User story:** As an operator, I want to see the questions an agent asked and
nobody answered, so a blocked or abandoned line of work stops being invisible.

#### Acceptance criteria

**The bucket and its boundary**

- **AC-UI-INBOX-HISTORY-001.1:** The Inbox shall render a tab strip using the
  shipped `variant="line"` tabs, `Needs you` first and `History` second.
  `Needs you` shall remain the default selected tab, its row set, count and
  behavior unchanged by the strip's presence.
- **AC-UI-INBOX-HISTORY-001.2:** Eligibility shall be evaluated per BUNDLE. The
  History tab shall list a bundle, scoped to the active workspace, when all four
  hold: at least one of its rows carries a non-terminal status per the shipped
  `InteractionStatus.IsTerminal()`; its owning task is not archived; no row in it
  carries a truthy `metadata.parent_question`; and it is unanswerable for at least
  one of exactly three reasons. Each listed bundle shall carry its **exclusion
  reason**, these and no others:
  - **`superseded`**: the bundle's turn is no longer its session's current
    conversational turn, or it is a permission request and a newer permission
    request exists on that same turn, which the operational projection suppresses.
    "Current conversational turn" shall be resolved by `currentTurnAuthority`
    ALONE, never composed with `nonTerminalSessionPredicate`, which would make
    `session_ended` unreachable.
  - **`session_ended`**: the owning session's state is `COMPLETED`, `FAILED` or
    `CANCELLED`, and no later turn superseded it.
  - **`unreadable`**: the bundle is a **clarification** bundle and at least one of
    its questions carries no resolvable question identifier, evaluated per bundle
    to match the shipped mechanism. It shall NOT be applied to a permission
    request, which carries no question identifier by construction. A permission
    bundle shall reach History only via `superseded` or `session_ended`.

  When more than one reason holds, they shall be evaluated in the order listed and
  exactly one reported; a test shall assert that a bundle which is both superseded
  and unreadable reports `superseded`. A bundle matching none is still live and
  shall NOT be listed. The Needs-you per-user dismiss/snooze sidecar shall NOT be
  applied to History.
- **AC-UI-INBOX-HISTORY-001.3:** A bundle shall appear in at most one of the two
  tabs. A bundle is non-terminal when any of its rows is, matching the shipped
  `has_pending`. For any fixture, the intersection of the two bundle sets shall be
  empty, and their union shall equal the non-terminal, unarchived bundles LESS the
  parent-question records and LESS the live bundles Needs-you itself excludes. A
  test shall assert both halves on a fixture holding at least one ordinary live
  answerable clarification bundle AND one live current-turn permission request.
  Exactly three reasons shall permit a non-terminal bundle to
  appear in neither tab, and a test shall assert no fourth arises: the task is
  archived (AC .4); it is a parent-question record (AC .2); or it is live (AC .2)
  AND Needs-you excludes it by a filter of its own, those being permission-type
  and the per-user dismiss/snooze sidecar.
- **AC-UI-INBOX-HISTORY-001.4:** When the owning task is archived, the bundle
  shall not be listed in either tab.

**The additive read path**

- **AC-UI-INBOX-HISTORY-001.5:** The history read shall be a separate path from
  the operational read. It shall not be reachable from any code path that gates a
  workflow transition, emits a pending-action event, resolves an answer, or
  contributes to a pending-action projection. The read shall live in the shipped
  `apps/backend/internal/task/repository/sqlite` package and shall REUSE that
  package's unexported `currentTurnAuthority` and `nonTerminalSessionPredicate`
  rather than duplicating either. Because it shares a package with most of those
  sinks, where no import statement exists to forbid, unreachability shall be
  enforced STATICALLY by a SOURCE-TEXT scan rather than an import scan: a new
  architecture-lint rule `ARCH-INBOX-HISTORY-ISOLATION`
  (`docs/architecture-lint.md`, `make lint-architecture`) shall forbid a closed set
  of sink files from referencing the history read's exported entry-point
  identifier. That set shall be exactly the five files named by path in the
  companion system design; a rule scanning fewer does not satisfy this criterion,
  and neither does a runtime test. The rule shall ship with an EMPTY baseline under
  `config/architecture-lint/`. A test shall assert both that the rule is registered
  with no baseline entries AND that it flags a deliberately violating fixture
  placed in the SAME package as the read.
- **AC-UI-INBOX-HISTORY-001.6:** The persisted `status` of a listed bundle shall
  be unchanged by listing it, and reading the tab shall perform no write.
- **AC-UI-INBOX-HISTORY-001.7:**
  `TestListPendingInteractionsAgreesWithPendingActionProjection` shall pass
  unmodified.
- **AC-UI-INBOX-HISTORY-001.8:** A regression test shall drive the real sequence:
  ask a question, then start a new turn without answering it, asserting both that
  the operational listing does not return the bundle and that the history listing
  does.

**What a row says**

- **AC-UI-INBOX-HISTORY-001.30:** A row shall present what was actually asked.
  For a **clarification** bundle it shall render each
  question's `title` and `prompt`, the option labels from `options[]` where the
  question carried any, and the bundle's shared `context`. Every question in a
  listed bundle shall be reachable, ordered per AC .19, INCLUDING a question whose
  own status is already terminal: eligibility is a bundle property (AC .2) and
  shall not filter a bundle's questions. A **permission** bundle carries no
  `question` object and so no `title`, `prompt`, `question_index` or `context`; it
  shall render the message `content` as the request text and the `name` values of
  `metadata.options[]` as the offered choices, omitting the clarification-only
  fields rather than rendering them empty. Long `context` may be truncated with the
  full value reachable in the row, never dropped.
- **AC-UI-INBOX-HISTORY-001.9:** A row shall state the time the question was
  asked, and its AC .2 exclusion reason rather than the raw `pending` status, so
  the row does not claim an answer is still awaited. The three reasons shall be
  distinguishable: `superseded` says the question was superseded by later work,
  `session_ended` says the session ended without an answer, and `unreadable` says
  the question itself could not be read. No row shall assert supersession unless
  its reason is `superseded`.
- **AC-UI-INBOX-HISTORY-001.10:** A row shall identify the turn that asked it.
  A row whose exclusion reason is `superseded` shall additionally identify the
  turn that superseded it, EXCEPT a permission superseded within its own turn
  (AC .2), where the superseding request shares that turn and there is none. That
  case, and a reason of `session_ended` or `unreadable`, shall omit the field
  rather than render a blank, a placeholder or a fabricated identifier; a test
  shall assert the omission for each of those three cases.
- **AC-UI-INBOX-HISTORY-001.11:** When the owning task's current workflow step
  starts no agent, the row shall say so. A step starts no agent when its `on_enter`
  action list contains no entry whose type is `auto_start_agent`; an **absent or
  empty** `on_enter` therefore satisfies it. Presence of that entry is the only
  test: the shipped `OnEnterAction` carries a type and a config and no disablement
  field, so no disabled entry exists to discount, and an unrecognized type string
  is absent by construction. The step evaluated is
  the task's current step at read time; if the task is mid-transition or its
  workflow cannot be read, the row shall omit the label rather than guess, and
  omission shall never read as a claim that an agent will start. This concept shall
  not reuse the `parkedOnBackgroundWork` symbol, and the label shall say the step
  will not start an agent, not that no agent can ever run.
- **AC-UI-INBOX-HISTORY-001.12:** A listed permission bundle shall be visually
  labelled as a permission using the shipped `IconShieldQuestion` and
  `text-amber-500` pairing. No new hue shall be introduced.
- **AC-UI-INBOX-HISTORY-001.13:** A clarification bundle holding at least one
  question with no resolvable identifier shall still be listed, whole, and shall
  not prevent the remaining bundles from rendering. This is a deliberate divergence
  from the Needs-you listing, which drops such bundles; a test shall assert it.
  This criterion governs LISTING only: the reason reported follows AC .2's
  precedence. The bundle shall render whatever question content survives under
  AC .30; when none survives it shall say so rather than render blank.

**Read-only**

- **AC-UI-INBOX-HISTORY-001.14:** The History tab shall expose no affordance that
  answers, rejects, re-asks, dismisses, snoozes or otherwise mutates an
  interaction. The only actions shall be opening the owning task and copying its
  identifier.
- **AC-UI-INBOX-HISTORY-001.15:** The history read path shall expose no endpoint
  accepting a request method other than a read, and the History tab shall issue no
  request to any mutating endpoint. A test shall assert that rendering, paging and
  acting within the History tab issues no call to the Needs-you sidecar dismiss and
  snooze routes. Those shipped routes shall NOT be modified and shall keep
  accepting any durable bundle identifier the caller can already reach: this
  criterion constrains what the History surface CALLS, not what they ACCEPT.

**Counting**

- **AC-UI-INBOX-HISTORY-001.16:** The History count shall render on its tab in
  the `Badge variant="secondary"` idiom. It shall count **bundles**, not rows and
  not tasks, and shall be the workspace-wide total of AC .20 rather than the
  current page's size, so paging does not change it. At zero the badge shall be
  omitted rather than rendered as `0`.
- **AC-UI-INBOX-HISTORY-001.17:** No history code path shall write the sidebar
  badge state key or the Needs-you count key. Enforced BOTH by the
  `ARCH-INBOX-HISTORY-ISOLATION` rule of AC .5, extended to forbid the history
  modules scoped in the system design from referencing those two keys, AND by a
  runtime test asserting the badge is unchanged by a fixture that populates History
  and leaves `Needs you` empty; the rule covers paths the fixture misses.

**Ordering, bounds and pagination**

- **AC-UI-INBOX-HISTORY-001.18:** Bundles shall be ordered by `created_at`
  ascending, with ties broken by `pending_id` ascending, matching the shipped
  sibling listing, so the oldest neglected question appears first.
- **AC-UI-INBOX-HISTORY-001.19:** Within a bundle, questions shall be ordered by
  `question_index` ascending, ties broken by `question_id` ascending, then by the
  message `id` ascending. The first two keys are the Inbox's shipped
  `orderInboxMessages` and the sibling's `AC-UI-NEEDS-YOU-INBOX-001.10`, NOT the
  MCP-resolution order of `orderBundleMessages`. The third key breaks the
  remaining tie for the empty-`question_id` bundles AC .13 admits, which Needs-you
  never lists, so the two Inbox tabs still never order a shared bundle differently.
  An index that is absent, negative or not a number shall sort as zero and then by
  the remaining keys, so the order is total.
- **AC-UI-INBOX-HISTORY-001.20:** The read shall be bounded, matching the
  sibling's parse behavior exactly. An absent limit shall default to 50; a limit
  above 200 shall be clamped to 200; a limit that is zero, negative or not a
  base-10 integer shall be **rejected with 400, not clamped and not defaulted**
  (`TestHttpListInbox_ZeroLimit_RejectedNotClamped`). The response shall carry a
  workspace-wide total, in bundles, distinct from the page count. The cursor shall
  be the keyset pair (`created_at`, `pending_id`) matching the AC .18 sort;
  forward-only in that same ascending direction; **exclusive** of the bundle it
  names; opaque to the client; and absent on the final page, which is how a caller
  detects the walk's end. Matching the shipped sibling encoder, it encodes
  that pair ONLY and carries no workspace: the request's own `workspace_id` scopes
  every page, so a cursor minted elsewhere shall merely position the walk inside
  the caller's own workspace. A malformed or undecodable cursor shall be rejected
  with 400 and shall never silently restart from page one. A cursor naming a bundle
  that has since left History is not an error: the walk resumes at the next bundle
  greater than that keyset position.
- **AC-UI-INBOX-HISTORY-001.21:** The read shall be idempotent. Two identical
  requests with no intervening data change shall return identical bundle sets in
  identical order, and neither shall have a side effect.

**Concurrency, empty and error**

- **AC-UI-INBOX-HISTORY-001.22:** The listing shall be a point-in-time projection.
  When a bundle is answered, or its turn becomes current again, after a response is
  served, the rendered row shall persist until the next read; acting on it is
  impossible because the tab offers no mutating action. No history row shall
  subscribe to an event stream, enforced by the `ARCH-INBOX-HISTORY-ISOLATION` rule
  of AC .5, extended to forbid those same history modules from importing the
  event-stream subscription entry points.
- **AC-UI-INBOX-HISTORY-001.31:** Pagination shall be consistent across a walk
  without holding a transaction open. Because each page is an independent
  point-in-time read (AC .22), a bundle crossing the History boundary mid-walk may
  be skipped or repeated. The keyset cursor of AC .20 shall bound this: a walk
  shall never return the same bundle twice on one page, and shall never skip a
  bundle whose (`created_at`, `pending_id`) exceeds the cursor and which was
  present throughout the walk. The tab shall not present the page sequence as a
  transactional snapshot.
- **AC-UI-INBOX-HISTORY-001.23:** When two callers read the same bundle
  concurrently, both shall receive it, and neither read shall block on the other,
  take a write lock, or mutate any row. Where a change lands between them the
  results are permitted to differ, and no shared snapshot or cross-request
  transaction shall be introduced to hide it. On the client, when two
  history reads for different workspaces are in flight, only the response matching
  the currently active workspace shall be applied, so a slower earlier response
  cannot replace the active workspace's list or reintroduce another workspace's
  bundles. Mirrors `AC-UI-NEEDS-YOU-INBOX-001.38`.
- **AC-UI-INBOX-HISTORY-001.24:** When the workspace holds no history bundles,
  the tab shall render an empty state naming what the bucket contains rather than a
  bare success message.
- **AC-UI-INBOX-HISTORY-001.25:** When a read names a workspace outside the
  caller's visible workspace set, the server shall reject it with a non-disclosing
  not-found outcome rather than an empty success, and the tab shall render an error
  state rather than the empty state, so a permission boundary is never reported as
  "nothing is here". When a read fails for any other reason, the tab shall render
  that same error state, shall not state that nothing is waiting, and the History
  badge shall render NO count rather than a stale one or a zero. Mirrors
  `AC-UI-NEEDS-YOU-INBOX-001.31` and `.21`; neither shall be weakened.
- **AC-UI-INBOX-HISTORY-001.26:** An absent `metadata.status` shall be treated as
  pending, retaining the existing legacy meaning.

**Delivery constraints**

- **AC-UI-INBOX-HISTORY-001.27:** No schema migration, no data backfill and no
  notification shall be introduced.
- **AC-UI-INBOX-HISTORY-001.28:** All copy shall be localized in the five shipped
  locales, shall contain no Unicode em dash, and the repository i18n ratchet shall
  pass.
- **AC-UI-INBOX-HISTORY-001.29:** An end-to-end specification shall cover
  selecting the History tab; observing a `superseded` bundle listed with its asked
  time, **the text of the question it asked** (AC .30), and its
  step-starts-no-agent label (AC .11); and confirming no answer affordance is
  present. The question text is not optional coverage, and the label shall be named
  by the AC .11 concept, not "parked".

## Out of scope

- **Re-ask.** Cut from Needs-you on 2026-09-11 and cut here too: re-asking
  restarts an agent on a question events may have settled.
- **Clearing, bulk-dismissing or expiring history rows.** The tab makes the
  population visible; it does not drain it. Draining mutates durable audit evidence
  and needs its own decision.
- **Gating the shipped Needs-you sidecar routes.** `PUT` and `DELETE` on
  `/api/v1/clarification-inbox/sidecar/:pendingID` keep accepting any durable
  bundle identifier whose task the caller can reach, including one now in History.
  Gating them would be a write-path change to an operational endpoint, which AC .5
  forbids, and History eligibility is not a stable property of a `pending_id`, so a
  gate must first decide the retroactivity question the system design sets out.
- **The failed-task bucket**, the third tab in the revision-4 mockup deck.
- **Parent-question records.** An agent's question to its parent task, excluded
  from both tabs per AC .2 on an ownership boundary rather than a staleness one.
- **Archived tasks**, per AC .4.
- **Cross-workspace rows.** Workspace-scoped, like Needs-you.
- **Any change to the current-turn predicate, expiry, bundle visibility rules or
  resolution semantics.** Consumed unchanged; they belong to integrations.
- **Retention or archival of the rows this tab reads.**
