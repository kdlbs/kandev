---
status: draft
system: ui
requirements:
  - REQ-UI-INBOX-FAILED-001
---

# Inbox Failed Bucket System Design

The Inbox itself is designed in
[`needs-you-inbox-01.md`](needs-you-inbox-01.md),
[`needs-you-inbox-02.md`](needs-you-inbox-02.md) and
[`needs-you-inbox-03.md`](needs-you-inbox-03.md). This design adds one bucket to it,
and changes exactly one thing those three own: the Needs you empty state gains a
single copy clause, specified by `AC-UI-INBOX-FAILED-001.23` and carved out as the
sole permitted delta by `AC-UI-INBOX-FAILED-001.4`. Their behaviour, row set,
ordering, count, answer affordances, dismiss and snooze, and error state are
untouched.

## Purpose and boundaries

UI owns the bucket, the tab, the row, and the exclusion from the count. It does
not own what makes a task failed, when a failed session is retried, or how a
task is archived. Those are the tasks and workflow contracts, consumed here
through a read-only projection.

Two boundaries are worth naming because getting either wrong turns this into a
different change:

- **The badge producer is not touched.** The Inbox badge is produced by the
  clarification-inbox read and its boot-hydration twin. This capability adds a second
  read and a second count, rendered on a tab. No failed-task code path may write the
  badge's per-workspace state key.
- **The failed read is a separate endpoint from the bundle read.** They answer
  different questions over different tables, and merging them would make one
  bucket's failure the other bucket's failure, which
  AC-UI-INBOX-FAILED-001.22 forbids.

## Prior art

**Our own prior reasoning (wiki).** Receipt: the vault resolved through
`~/.obsidian-wiki/config` to `OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki`,
collection `wiki`. It could not then be read: `qmd` is not on `PATH`, its MCP server
is not exposed, `obsidian-wiki` is not installed, and the directory is unreadable
from this sandbox (`Operation not permitted`), so no grep fallback was possible. This leg returned nothing because the vault was
unreachable, not because it held nothing.

**What other products shipped (saas-kb).** Receipt: `search_saas_docs` with
`category: "ai_sdlc"`, queries "inbox badge count actionable items failed runs
triage surface" and "sidebar badge unread count excluded from count failed runs do
not require action", then `get_saas_doc` on the top hit. The first returned
substantive Paperclip and Factory.ai hits; the second collapsed onto unrelated
vendors and was discarded.

Paperclip's Inbox is the closest shipped analogue (tabs: Mine, Recent, Unread,
Blocked, All). Three decisions taken from it: tab switching navigates, since its
`/inbox/<tab>` "doesn't just hide content", so a tab is bookmarkable; failed runs
are a category rather than a queue, though we place them in a dedicated tab rather
than a filter inside a firehose, because two buckets are not six categories and a
filter must be discovered; and per-item hiding is scoped to one tab, which we reach
for the different reason in [Out-of-scope reasoning](#out-of-scope-reasoning).

Where we differ deliberately: Paperclip's Blocked tab carries a severity dot and a
blocked-reason chip, and its Mine tab is "the queue that matters most". We rank no
failures by severity and do not treat this bucket as a queue.

## Requirement mapping

`REQ-UI-INBOX-FAILED-001` maps to the sections below, per acceptance criterion.

| Acceptance criteria | Design section |
| --- | --- |
| .1-.4 | [Components and responsibilities](#components-and-responsibilities) |
| .5-.9 | [Input inventory](#input-inventory), [The failed-task predicate](#the-failed-task-predicate) |
| .9a | [Data and contracts](#data-and-contracts) |
| .10, .10a, .11-.13 | [Ordering, paging, and the failure instant](#ordering-paging-and-the-failure-instant) |
| .14-.17 | [Count separation](#count-separation), [Ordering, paging, and the failure instant](#ordering-paging-and-the-failure-instant) |
| .18, .18a, .19, .19a | [Data and contracts](#data-and-contracts), [Copy and coverage](#copy-and-coverage) |
| .20, .20a, .30, .30a | [Session resolution](#session-resolution) |
| .21-.23, .27 | [Failure and recovery](#failure-and-recovery), [Count separation](#count-separation), [Control flow](#control-flow) |
| .24 | [Security](#security) |
| .8, .25-.26 | [Control flow](#control-flow) |
| .28-.29 | [Copy and coverage](#copy-and-coverage) |

## Input inventory

Sampled from the live instance on 2026-09-14 against
`/Users/henry/.kandev/data/kandev.db` opened read-only. Every number below is a
measurement, not an estimate.

| Candidate predicate, archived excluded | Rows |
| --- | --- |
| `tasks.state = 'FAILED'` | 15 |
| task has a primary session in state `FAILED` | 18 |
| task has any session in state `FAILED` | 22 |

The three-row gap decomposes as: 15 tasks where both hold, plus one task in
`CREATED`, one in `REVIEW` and one in `SCHEDULING`, each with a failed primary
session. No task is in `FAILED` state without a failed primary session, so the first
set is a strict subset of the second and task state is reliably written.

Other measured facts the contract depends on:

- `task_sessions.error_message` on a failed session: never empty across all 111
  failed sessions, mean 199 bytes, maximum 4096.
- `task_sessions.completed_at` on a failed session: never null across those 111.
- `tasks.updated_at` drifts from the session completion instant: zero on 14 of the
  15 failed tasks, and 538,095 seconds (6.2 days) on one.
- Origin of the 15 failed tasks: 13 `automation_run`, 2 `manual`. `is_ephemeral`
  is 0 on all 18 primary-failed tasks, so it does not discriminate and `origin`
  does.
- `tasks.labels` is `[]` on every failed task, so the deck's row subtitle cannot be
  sourced from labels.
- Workspace spread of the 15: 14 in the Kandev workspace, 1 elsewhere, so the
  default page bound is never reached today.
- 42 archived tasks carry a failed primary session, which is why
  AC-UI-INBOX-FAILED-001.7's exclusion is load-bearing rather than theoretical.

## The failed-task predicate

**Decision: a row exists when `tasks.state` is the terminal failed state and
`tasks.archived_at` is null.** Rejected alternatives:

- **Primary session is `FAILED`.** What the deck measured, and wrong as a predicate.
  It admits the `SCHEDULING` task, which the system is actively re-placing and which
  will run again without anyone triaging it, and the `REVIEW` task, which has already
  moved on. The deck's 17 was a volume-and-recency argument, not a definition, and the
  deck does not own the data contract.
- **Any session is `FAILED`.** Admits every task that ever failed once and then
  succeeded: 22 rows today, at least 7 of them recoveries.

The chosen predicate is the one the rest of the product already uses:
`IsTerminalTaskState` in `apps/backend/internal/task/models/models.go` treats
completed, failed and cancelled alike. Deriving the bucket from a fact the task model
already asserts means a retry removes the row by changing task state, which is the
convergence AC-UI-INBOX-FAILED-001.8 requires, rather than by a session count the
Inbox would re-derive.

The cost, stated plainly: a task that dies without its state being written to failed
will not appear. Today that set is empty. If it stops being empty, the fix is in the
task state machine, not a second predicate here.

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| Failed-bucket read endpoint | Workspace-scoped, bounded, read-only projection of failed tasks. New. |
| Failed-bucket store query | One query joining the task to the session [Session resolution](#session-resolution) selects, for the failure instant and reason. Usually the primary session but not always, so the join follows the selection rule, not `is_primary`. New. |
| Inbox page tab strip | Two tabs, `variant="line"` from the shared `Tabs` component; each count a secondary-variant `Badge` inside its tab control. New. |
| Failed row | Title, reason, relative failure time, open-task control, and the shared module's TASK-state failed marker. New. |
| Failed-bucket state slice | Per-workspace rows, count, truncation flag, read status, generation guard. New, modelled on the existing Needs-you slice. |
| Needs-you bucket, its slice, its controller, its badge | Unchanged. |
| Shared thread status vocabulary | Its `failed` entry is reused for row status, reached directly rather than through `resolveThreadSessionStatus`: see [Session resolution](#session-resolution), the resolvers take a session and this row has none. |

The deck settles where the count goes: inside the tab, not the page header, because
the header is where a title would go and this page has none.

## Data and contracts

The failed row carries, per task: `task_id`, `title`, `workspace_id`, `origin`,
`failure_instant`, and `reason`. `failure_instant` is OMITTED when unresolvable,
never null and never a zero instant, which is how the client tells it from a real
one; the other five keys are always present, empty string rather than absent. It carries no session identifier and
no session state in the row model, because the row addresses a task, the
open-task control needs only the task, and the row's status is fixed rather than
resolved from a session (see [Session resolution](#session-resolution)).

**Origin, and which values count as a person.** `manual` is the only origin that
is a person; the other five (`agent_created`, `routine`, `onboarding`,
`automation_run`, `automation_task`, at `models.go:1024-1030`) are not, and each
renders its own marker. The partition is stated rather than inferred because the
enum has six values while the live failed set has only two of them, so a builder
reading the input inventory alone would be guessing about the other four the day
one of them fails. They are not collapsed into a single "automated" word because
an agent creating a task and a schedule creating a task are different triage
facts and the task model already spends six constants distinguishing them. The
set is owned by the tasks system and can grow without this spec changing, so an
unrecognised value renders a generic non-person marker rather than nothing, the
raw stored string, or a broken row.

**Failure reason.** The session's `error_message`, truncated server-side to 512
Unicode code points before it enters the response. The bound stops a 4096-byte
message multiplied by a 200-row page from becoming an 800KB payload for a list the
operator scans. The unit is code points, not bytes, and truncation never splits one,
so the field is valid UTF-8 for every input including one that is not. This is stated
because the measured field is measured in bytes and the obvious Go implementation,
slicing at byte offset 512, produces invalid UTF-8 the first time a non-ASCII message
runs long. Code points cap the worst case at 4 bytes each, still far under the
payload the bound prevents. The client
renders it clamped to one line and never blank: an unresolvable reason becomes
stated fallback copy, per AC-UI-INBOX-FAILED-001.19. Nothing flags a truncated
reason to the client, per AC-UI-INBOX-FAILED-001.19a, because the row is already
clamped to one line and the full text is on the task the row opens.

**Why the reason and not the deck's subtitle.** The deck draws a subsystem label
under each title. That label has no source: `tasks.labels` is empty on every failed
task, and subsystem tags are a plugin-owned, human-assigned dimension the read path
does not have. The failure reason is always present and is the actual triage
content, so it takes the slot.

**The endpoint.** `GET /api/v1/failed-inbox`, taking `workspace_id` (required) and
`limit` (optional), parsed by the same rules the bundle read applies. It takes no
`cursor`, the one query parameter the sibling accepts and this read does not.

**Response envelope: rows, `count`, and an explicit `truncated` boolean.** The
truncation signal is a FIELD OF ITS OWN, and this is the one place the sibling
cannot be copied. The bundle read carries no truncation field at all: it signals
"there is more" solely by emitting `next_cursor`, which the client turns into
`hasMore` and then into the capped `"+"` suffix on the count. That whole chain
hangs off the cursor. Since this read deliberately has none
(`AC-UI-INBOX-FAILED-001.13`), nothing in the sibling's envelope is left to mirror,
and a builder told to "mirror the bundle read" would either re-introduce the cursor
to get the signal back or invent a field silently. So it is named here: `truncated`,
set when the store had more rows than the bound returned, computed the way the
sibling computes `HasMore`. The CLIENT-side presentation
of a capped count is still shared with the sibling per
`AC-UI-INBOX-FAILED-001.17`; it is only the wire signal that differs, and it
differs because the cursor it used to ride on is gone.

**The tab lives in the address as the `tab` query parameter on the existing Inbox
route**, not as a second route path: `?tab=failed` selects Failed, `?tab=needs-you`
and every other value, including an absent, empty, repeated or unrecognised one,
selects Needs you. The destination is one page with two buckets; giving Failed its
own path would make the sidebar entry's active-route match ambiguous and would
duplicate the page shell. Paperclip's `/inbox/<tab>` shape is the right instinct
about bookmarkability and the wrong shape for a route that already exists and is
matched exactly.

Selecting a tab REPLACES the current history entry rather than pushing one. Both
forms are equally bookmarkable, so the only question is what the back control does,
and that follows from what the strip is: two views of one destination. Pushing would
make back walk the operator through their own tab clicks before letting them leave
the Inbox. Replacing means back returns where they came from, and the address still
names the tab for a bookmark or reload.

## Session resolution

The row's failure instant and reason come from one session, chosen by a rule that
must be total because the schema does not enforce what the data currently happens to
satisfy. `task_sessions.is_primary` has no unique index, so more than one primary
session per task is representable, and nothing requires a task to have a session.

Measured 2026-09-14: zero tasks have more than one primary session, zero failed tasks
have no primary session, zero have no session at all. The rule below therefore costs
nothing today and exists so a builder is not forced to invent it, and so a future data
shape cannot silently drop a failed task.

Chosen session, in order: the primary session; if several are marked primary,
the most recently started; if none is, the most recently started session of any
kind; and where two candidates tie on start instant, the lower
`task_sessions.id` compared as an ascending byte-ordered string.

**That last tiebreak is load-bearing, not tidiness.** One of the fields read from
the chosen session is `completed_at`, which
[Ordering, paging, and the failure instant](#ordering-paging-and-the-failure-instant)
makes the primary sort key. Two sessions tying on `started_at` while carrying
different `completed_at` values would put the row in two different positions on two
reads of unchanged data, breaking both the total order and the idempotency
`AC-UI-INBOX-FAILED-001.25` requires. `TaskSession.StartedAt` is a `time.Time` and
not a pointer, so this covers an equal value rather than an absent one.

Where the task has no session, both fields are unresolvable. The row is still listed.
It sorts FIRST and states the failure time is unknown, rather than borrowing
`tasks.updated_at` and presenting a drifted value as the failure time, the mistake
[Ordering, paging, and the failure instant](#ordering-paging-and-the-failure-instant)
exists to prevent. First rather than last is what lets
`AC-UI-INBOX-FAILED-001.30`'s promise survive a bounded page: a row sorted last
is the first row a bound discards, so "sorts last" and "no resolution failure
removes a failed task from the list" could not both hold. First is also right on
the merits, because a task that failed and cannot say when is the row most likely
to need a person.

The status the row renders does not come from this chosen session. It is fixed to
the shared vocabulary's `failed` entry for every row on the tab, per
`AC-UI-INBOX-FAILED-001.20`. `resolveThreadSessionStatus` tests `pending_action`
before it tests `state`, and its `permission` and `clarification` entries both
carry `hasAttention: true`, so resolving status per row from whichever session
this rule picked could put an attention flag on the one bucket whose premise is
that it never carries one. Fixing the status is also why the row model needs no
session-state field, and why the no-session case needs no special handling here.

**How the entry is reached, since it cannot be reached the usual way.** That module
exports two resolvers and no status value: both take a session-shaped argument, and
the `STATUS` table holding the entries is module-private. Fixing the status while
carrying no session field therefore leaves no way to call either resolver honestly.
The row must NOT fabricate a `{state: "FAILED"}` stand-in to pass to one: that
reintroduces exactly the `pending_action`-first precedence
`AC-UI-INBOX-FAILED-001.20a` exists to exclude. The entry is instead made directly
reachable by exporting it from the module that already defines it. Exporting an
existing value adds no entry and defines no second vocabulary, so
`AC-UI-INBOX-FAILED-001.20`'s prohibition is satisfied: it forbids a NEW or
DUPLICATED status, not a wider export of the one that ships. The module is not
reused unchanged: it gains an export.

## Ordering, paging, and the failure instant

**Order: unresolvable-instant first, then failure instant descending, then
`tasks.id` ascending as a byte-ordered string.** Descending because triage reads
newest-first, which is also what the deck draws. `tasks.id` is the final tiebreak
because it is the row's primary key: unique by construction, stable across reads,
and therefore enough to make the order total. An order that is only usually total is
a bug waiting for a fixture.

**That last key names its comparison for the same reason the session tiebreak
does.** `AC-UI-INBOX-FAILED-001.30a` already pins the session-id tiebreak to an
ascending byte-ordered string; leaving the TASK-id tiebreak as bare "ascending"
leaves the comparison to the engine's text collation, the same class of
engine-dependent behaviour the null-placement key below exists to stamp out. A
locale- or case-sensitivity-dependent collation would order two rows differently on
SQLite and PostgreSQL while both satisfied the words "task identifier ascending",
and `AC-UI-INBOX-FAILED-001.10a` requires one order under every engine. The tiebreak
that makes the order total cannot itself be the key that varies.

**The first key is explicit, and must not be left to the engine.** A row whose
failure instant is unresolvable sorts ahead of every resolved row, and that
placement is written as its own leading sort key rather than inherited from how
the store happens to place absent values. This matters because the product runs
two engines and they disagree: under `ORDER BY ... DESC`, SQLite places absent
values last and PostgreSQL places them first. An ordering that relies on the
default is therefore reproducible on one engine and not the other, which
`AC-UI-INBOX-FAILED-001.10`'s totality claim forbids outright. There is no
precedent to copy here: the backend contains no `NULLS FIRST` or `NULLS LAST`
anywhere today, because this is the first ordering key in it that can be absent.
The dual-dialect coverage that already exists for this layer
(`apps/backend/internal/db/dialect/` and the `*_postgres_test.go` twins under
`apps/backend/internal/task/repository/sqlite/`) is where the order gets asserted
on both, rather than on whichever one the developer ran.

**The failure instant is the chosen session's `completed_at`.** Not
`tasks.updated_at`, which the input inventory shows drifting 6.2 days past the real
failure on a live row. A row saying "failed 6 days ago" when it failed 12 days ago is
a quiet lie in the one field the operator uses to decide whether to care.

**Paging: default 50, cap 200**, the same bound the bundle read uses. Reusing the
bound rather than inventing a second one keeps AC-UI-INBOX-FAILED-001.17's capped
presentation identical across both tabs. The truncation SIGNAL is not shared; see
[Data and contracts](#data-and-contracts) for why this read carries its own
`truncated` field instead.

**The bound is the only thing that may elide a row, and it says so.** Two ACs made
absolute-sounding promises a bounded page can break: `AC-UI-INBOX-FAILED-001.5`
lists "exactly those tasks ... and no others", and `AC-UI-INBOX-FAILED-001.30`
promises no resolution failure removes a failed task. Both are now subject to this
bound, because at 201 qualifying tasks the first is arithmetically false, and a
workspace with over 200 unresolvable instants pushes some past the bound despite
sorting first. That is not a flaw in sorting them first: first-of-N is still
discarded when N exceeds the bound, and no ordering fixes it. What sorting first
DOES guarantee is that unresolvable rows are the LAST thing the bound reaches, which
is why the key leads. The honest contract: the bound is the only force that elides a
row, and `truncated` discloses every time it did.

**One page, and no cursor at all.** The read takes no cursor and returns none.
This is a decision, recorded in the requirements' `## Out of scope`, not an
omission: the truncation indicator is the whole of what this read says about rows
past the bound. Why it is left out rather than
finished: [Out-of-scope reasoning](#out-of-scope-reasoning).

**Where the count comes from.** The tab's count is the length of the row array in the
same response, never a separately computed total. That makes
`AC-UI-INBOX-FAILED-001.16`'s "no rendered state exists in which the two disagree"
true by construction rather than by discipline: there is one number and the rows are
it. The sidebar badge's count is produced separately by the bundle read, untouched
here.

## Count separation

The badge and the tab count are produced by different reads and stored under
different per-workspace keys. The separation that matters is a WRITE separation,
and stating it as a write rule is what keeps it both strict and satisfiable: no
failed-bucket code path writes the Needs-you slice, and no Needs-you code path
writes the failed slice.

Reading across the two slices is NOT forbidden, and one place needs it: the Needs
you empty state reads the failed slice's count for
`AC-UI-INBOX-FAILED-001.23`. Tightening this to forbid READS looks like extra safety
and is a contradiction: it forbids the only honest source for a fact another AC
demands. The invariant worth
protecting is that no failed task can ADD TO the badge's number, which is a property
of who WRITES the badge's key. A read cannot inflate a counter.

The Needs-you empty state gains one clause naming failed tasks among what it does
not count, and pointing at the tab. That clause is conditional on the workspace
actually holding a failed task: a workspace with none must not be told that some
exist, which is the same rule the hidden-bundle disclosure already follows.

## Control flow

The failed bucket is read on exactly these triggers and no others: Inbox mount,
tab selection changing,
active workspace changing, the browser tab returning to visible, and a periodic
re-read every 60 seconds while the browser tab is visible and a workspace is active.
Every one of them fires REGARDLESS of which tab is currently selected.

**The 60 seconds is a maximum staleness, not a rate limit.** It is the longest a
visible Failed tab may go without re-reading, which is what makes
`AC-UI-INBOX-FAILED-001.8`'s convergence and `AC-UI-INBOX-FAILED-001.23`'s
present-tense clause bounded rather than merely eventual. A ceiling on request
FREQUENCY would not do that: "at most once every 60 seconds" is equally satisfied by
reading once an hour, or once at mount and never again. A hidden tab is exempt while
hidden and re-reads AT ONCE on returning, not at the next tick, so the exemption
cannot turn an hour in the background into an hour of stale rows. One read per
return, not one per browser event; a return commonly fires several.

It is never read outside the Inbox destination, so it does NOT subscribe to the
Needs-you count slice's five refresh triggers: `needs-you-inbox-02.md` makes those
slice-owned and app-wide precisely so the badge stays right while the Inbox is
closed, so reusing them would contradict the clause above and would also put
Needs-you-owned machinery in charge of failed-slice writes, which
[Count separation](#count-separation) forbids. What that severs is the SUBSCRIPTION,
not the capability: the visibility trigger above is this bucket's own, mounted with
the Inbox, and the browser-visibility hook it uses is generic and already shared by
unrelated consumers, so it carries no Needs-you machinery. This bucket runs only
while the Inbox is open because nothing outside the Inbox renders its count.

**Why it does not stop when Needs you is selected.** The read follows the
DESTINATION, not the tab, because two things visible from Needs you depend on it. The
TAB STRIP carries the Failed count from both tabs, so a tab-scoped read leaves that
count absent or arbitrarily stale the whole time the operator is on Needs you. And
`AC-UI-INBOX-FAILED-001.23` requires the Needs you EMPTY STATE to say whether the
workspace holds a failed task; without the read it has no way to know, and the only
other source is the badge producer, which `AC-UI-INBOX-FAILED-001.14` puts off
limits. The cost is one extra bounded read-only request per Inbox open for an
operator who never opens the Failed tab.

Reads are side-effect free and idempotent. Overlapping reads are resolved by the
same generation guard the Needs-you slice uses: each read issues a generation, and
a response whose generation is no longer current is dropped rather than applied.
The guard is keyed on the WORKSPACE and the generation, and deliberately NOT on the
selected tab. Keying it on the tab would discard exactly the responses the two
points above depend on, since those arrive while Needs you is selected; a guard
that drops them would reintroduce the stale count it was supposed to prevent. The
workspace remains part of the key because a response for a workspace the operator
has already left must never be applied, which is what
`AC-UI-INBOX-FAILED-001.26` requires.

## Failure and recovery

- **A failed-bucket read fails.** The Failed tab renders its error state, carries no
  count, and does not claim nothing failed. The Needs you tab's rows, count and state
  are untouched, owned by a different read that did not fail. Its EMPTY STATE is the
  one place the two meet: `AC-UI-INBOX-FAILED-001.23`'s clause needs a failed count
  that no longer exists, so the clause is OMITTED rather than asserting presence or
  absence. Omitting is the honest option: claiming none exist is a lie the operator
  would act on. Recovery needs no operator action and no retry affordance: the
  periodic trigger re-reads within 60 seconds, and a return to visibility re-reads at
  once, so the error state is bounded by the same guarantee the rows are.
- **A bundle read fails while Failed is selected.** The reverse holds. The
  sidebar badge suppresses per the existing rule; the Failed tab keeps rendering.
- **A read that fails after a successful one clears the rows it was replacing**,
  on the same rule and for the same reason the Inbox already applies: rows
  surviving under an absent count is the disagreement the count contract forbids.
- **A task changes state during a read.** The read returns either the row or no
  row. Convergence is by re-reading, never by an event: the row set must be
  correct even for an exit that emits nothing.
- **A row's task is archived or deleted between read and click.** Opening it behaves
  as opening any stale task reference already does; no special handling, no
  optimistic removal.

## Security

The same rule the Inbox read already applies, through the same authorization call:
workspace scope is resolved server-side from the request identity, and a workspace
outside the caller's visible set produces the non-disclosing not-found outcome rather
than an empty success. The required scope is the workspace reach scope itself, so as
with the bundle read the forbidden outcome is unreachable by construction rather than
unimplemented. Where authentication is disabled, the single-user identity reaches
every workspace by identity resolution, not a bypass.

No sidecar, no per-user state, and no writes at all, so there is nothing here
that a second caller can race into.

## Out-of-scope reasoning

The requirements list the exclusions; the argument lives here, so the contract file
stays under its size ceiling.

- **Pagination.** Measured 2026-09-14 the instance held 15 failed tasks, 14 of them
  in one workspace, so the default bound of 50 is not reached and the 200 cap is
  more than an order of magnitude away. A cursor here is also harder than the
  sibling's: `encodeInboxCursor` is keyed on `(created_at, pending_id)` where
  `created_at` is never null, while this sort key is deliberately nullable, so its
  cursor would have to encode and compare the unresolvable-instant case across a page
  boundary, precisely where a row gets silently duplicated or dropped. Retry, move and
  archive all remove rows under AC-UI-INBOX-FAILED-001.8, so the bucket drains by
  being worked rather than scrolled.
- **Dismiss, snooze, restore.** Needs you needs per-user hiding because an
  unanswered question has no exit but an answer. A failed task already has three
  real exits. A fourth, cosmetic one would add a sidecar table, a hidden-count
  disclosure and a restore affordance to hide something the operator can resolve.
- **Severity or grouping.** Grouping repeated identical failures is a real design
  problem and belongs to a follow-up with its own evidence. A severity signal on an
  uncounted bucket would also reintroduce urgency through the side door: the
  Paperclip departure noted under [Prior art](#prior-art).
- **Retention.** Measured 2026-09-14, 42 archived tasks carried a failed primary
  session, all excluded by AC-UI-INBOX-FAILED-001.7. Nothing here decides when the
  unarchived ones age out.

## Copy and coverage

New copy: two tab labels, the failed empty state, the failed error state, the
relative-failure-time phrasing, six origin markers (one per non-person origin
value, plus the generic marker for an origin this capability does not enumerate),
the unknown-failure-time phrasing, the unresolvable reason fallback, and one added
clause in the Needs you empty state.

**No new visual vocabulary.** The row's failure marker is the shared state-icon
module's TASK-state failed pairing; the tab counts are secondary-variant badges in
the tab controls. Neither is a fresh choice. The icon module carries a DIFFERENT
failed pairing under task state and under session state, so "the failed icon" is
ambiguous until the spec picks one: this bucket is defined by task state
(`AC-UI-INBOX-FAILED-001.6`), its row addresses a task, and its status is fixed from
task state, so the task-state pairing is the consistent one. The module's existing
error colour is reused as-is; no hue is added. The badge variant is load-bearing for
a similar reason: the deck chose `variant="line"` specifically BECAUSE a secondary
count badge has contrast on the resulting background and is invisible on the default
variant's track, so shipping `line` without that badge keeps the choice and discards
its justification. All of it goes through `t()`, in five
locales, with no Unicode em dash. The origin markers are six strings rather than
one because `AC-UI-INBOX-FAILED-001.9a` gives each non-person origin its own
marker, so the origin copy is five strings per locale wider than a single
"automated" word would be.

End-to-end coverage asserts the tab strip, a listed failed row with its reason
and relative time, opening the task from the row, and, in the same run, that the
sidebar count did not move when the failed task appeared. That last assertion is
the one that would catch the most likely regression this capability can cause.
