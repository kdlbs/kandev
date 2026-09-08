---
status: draft
system: office
requirements:
  - REQ-OFFICE-AGENT-COMMENT-READS-001
  - REQ-OFFICE-AGENT-COMMENT-READS-002
  - REQ-OFFICE-AGENT-COMMENT-READS-003
  - REQ-OFFICE-AGENT-COMMENT-READS-004
  - REQ-OFFICE-AGENT-COMMENT-READS-005
  - REQ-OFFICE-AGENT-COMMENT-READS-006
  - REQ-OFFICE-AGENT-COMMENT-READS-007
  - REQ-OFFICE-AGENT-COMMENT-READS-008
---

# Office: Agent Comment Reads System Design

## Purpose and boundaries

Office owns the agent-facing office HTTP surface, which tools an Office agent
may call, and what an Office agent is told it can do. This design repairs one
existing endpoint on that surface, `GET /api/v1/office/tasks/{id}/comments`,
and removes a comment read tool from the MCP surface rather than adding one.

It uses, and does not own:

- the `task_comments` store and its repository, owned by Office persistence;
- the cross-task read relation, owned by the task system's handoff access
  guard and shared verbatim with the task document tools;
- the byte budget, ordering, and window projection, owned by the task system's
  handoff comment service, which this design re-points at rather than rewrites;
- the agent JWT middleware that authenticates the office route group.

Two deliberate exceptions to those boundaries are claimed as in scope.

**The shared guard's not-found handling.** This design covers a change to
`loadAccessPair` that has already landed on the branch (commit `260405a9d`);
it is recorded here because it belongs to this contract, not because it is
outstanding. The guard as originally written did not deny a missing task; it
returned the repository's not-found error, and that error embedded the task
identifier.
Fixing that only at this endpoint would leave the comment surface and the
document surface answering the does-this-task-exist question differently, which
`REQ-OFFICE-AGENT-COMMENT-READS-001` forbids in terms: the relation must be
evaluated by the same shared implementation, so the two surfaces cannot
diverge. The blast radius is bounded. `loadAccessPair` has exactly two callers,
both inside the guard file, and no existing test asserts the leaking behaviour.
The change is security-positive for the document tools as well.

**An exported agent-claims accessor.** The office agents package exposes
`CallerFromContext`, which returns the authenticated agent instance and carries
no task identity. The caller task lives in the validated JWT's `AgentClaims`,
set on the request context under an unexported key with no exported reader.
This design adds one exported accessor beside `CallerFromContext`, on the same
justification the existing one records: so other office packages can enforce
agent-scoped authorization without depending on the package's internal
context-key constants.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AGENT-COMMENT-READS-001` | [Purpose and boundaries](#purpose-and-boundaries), [Components and responsibilities](#components-and-responsibilities), [Failure and recovery](#failure-and-recovery), [Security](#security) |
| `REQ-OFFICE-AGENT-COMMENT-READS-002` | [Data and contracts](#data-and-contracts) |
| `REQ-OFFICE-AGENT-COMMENT-READS-003` | [Data and contracts](#data-and-contracts), [Ordering and windowing](#ordering-and-windowing) |
| `REQ-OFFICE-AGENT-COMMENT-READS-004` | [Data and contracts](#data-and-contracts), [Ordering and windowing](#ordering-and-windowing) |
| `REQ-OFFICE-AGENT-COMMENT-READS-005` | [Data and contracts](#data-and-contracts), [Failure and recovery](#failure-and-recovery) |
| `REQ-OFFICE-AGENT-COMMENT-READS-006` | [Persistence](#persistence) |
| `REQ-OFFICE-AGENT-COMMENT-READS-007` | [The advertised-surface contract](#the-advertised-surface-contract) |
| `REQ-OFFICE-AGENT-COMMENT-READS-008` | [Control flow](#control-flow) |

## Components and responsibilities

- **Comment read endpoint.** The existing dashboard handler for
  `GET /api/v1/office/tasks/:id/comments`. It gains a caller branch: an agent
  caller takes the guarded, bounded path; every other caller keeps today's
  behaviour byte for byte.
- **Agent-claims accessor.** The new exported reader described above. It is the
  only source of the caller task identity.
- **Access-checked read.** `ListCommentsForCaller` on the cross-task handoff
  service: applies the shared read guard, then the window, budget, and
  projection. It already exists, and the guard, window, budget, and projection
  are unchanged by the transport move. One thing about it does change, and it
  is not optional. Its first act is a `self`/empty-target sentinel that
  substitutes the caller's own task for the requested one. Under the deleted
  MCP tool that sentinel was an argument convenience; on this transport the
  target is a path segment the CLI interpolates without validation, so
  `--task self` arrives as a literal target and substitutes. That is what
  `AC-OFFICE-AGENT-COMMENT-READS-005.4` forbids in terms, and under a token
  carrying no task claim it yields a missing-target validation error where
  `AC-OFFICE-AGENT-COMMENT-READS-001.13` requires the denial sentinel for
  every target. The sentinel is therefore removed: an unrecognised target is
  resolved by the guard like any other, and only a target the caller is
  entitled to name is read. Blast radius is nil, because once the tool is
  deleted this endpoint is the function's only caller. Two tests assert the
  removed behaviour and are retired with it,
  `TestListCommentsForCallerSelfFallback` and
  `TestListCommentsForCallerRequiresTaskIDWhenNoCaller` — the second cites an
  acceptance criterion this revision deleted. Build replaces them with
  coverage of the contract above; it does not delete them silently, and it
  does not weaken them to pass. This is the only place the guard is called for
  comments.
- **Comment store.** Supplies the windowed read and the total count.
- **agentctl CLI.** `kandev comment list --task <id> [--limit N]` and
  `kandev tasks conversation --id <id>` both already call this endpoint and
  print the response body verbatim. Neither needs a wire change. The
  `--limit` flag help, which reads `0 = all`, is corrected: for an agent caller
  the server defaults and clamps, so `all` is no longer reachable and the flag
  is documented as "0 = server default".
- **Office first-turn context, the skills reference, and role instructions.**
  Advertise the CLI command and direct the coordinator to use it.
- **Deleted.** The Office MCP comment tool, its registration, its backend
  action handler, and the sysprompt line advertising it. The guarded read must
  have exactly one entry point; leaving the tool beside the repaired endpoint
  would be two.

## Data and contracts

The endpoint takes the target task from its path parameter and, for an agent
caller, the caller task from JWT claims. Its only query parameter is `limit`.

### The dual-caller split

This endpoint is consumed by the human dashboard SPA as well as by agents.
Applying the guard and the default limit unconditionally would break the human
Office comment view: it would lose the per-comment run lifecycle fields it
renders the Queued / Working / Failed badge from, and would silently show a
browser user the newest twenty comments of a longer thread. The split is by
caller kind, and the precedent is in the same file: `createComment` already
rejects an agent caller by testing `CallerFromContext(c) != nil`, so agent-kind
branching on this route is established, not invented here.

An agent caller therefore receives the guard, the default and clamp, the byte
budget, the window fields, and the reduced projection. Any other caller
receives exactly what it receives today. The existing Playwright coverage under
`apps/web/e2e/tests/office/` and the SPA's `listComments` client are the
regression surface for that second half.

Which branch each acceptance criterion binds follows from that split, and three
of them must be read with it in hand because they carry no caller qualifier.
`AC-OFFICE-AGENT-COMMENT-READS-003.7` and `-003.8` — the single read
transaction and the repeat-call ordering determinism — and
`AC-OFFICE-AGENT-COMMENT-READS-005.1`, the empty-result total, bind the agent
branch only. None can bind the browser branch without contradicting
`AC-OFFICE-AGENT-COMMENT-READS-003.11`, which preserves it: the legacy list
query sorts on `created_at` with no tiebreak, so meeting the ordering criterion
there would mean changing that query, and the browser response carries no
`total` field at all, so meeting the transaction and empty-result criteria
there would mean adding one. Each of those is precisely the change `-003.11`
exists to prevent. `REQ-OFFICE-AGENT-COMMENT-READS-003` is framed around a
coordinator re-reading a child's thread, which is the agent branch; the browser
branch is preserved by this document, not specified by it.

The browser branch reads no query parameter at all.
`AC-OFFICE-AGENT-COMMENT-READS-003.11` says a valid positive `limit` *may* be
honoured there; this design resolves that permission to no. The handler as it
stands passes only the path parameter to the list call and never touches the
query string, so honouring `limit` would be new behaviour rather than preserved
behaviour, and it would leave the human Office comment view one query parameter
away from a silently truncated thread. A permission that no builder is required
to exercise is one no test can observe either way, so it is settled here in the
negative: the browser branch ignores `limit` entirely and returns the full
list, which is what the same criterion's own first clause already requires.

### Where the limit decision is made

A query string carries no types, so `limit` arrives as a string or not at all
and there is no schema layer that can reject it before the handler. Parsing is
therefore the handler's, and the contract that `limit` never produces an error
from any layer falls out of the transport rather than having to be defended
against one, which is the main simplification the CLI transport buys.

Absent, empty, unparseable, zero, or negative takes the default of 20; above
100 clamps to 100. A fractional `7.5` does not parse as an integer, so it takes
the default rather than truncating to 7. Nothing here is an error, and nothing
here is invisible: `total` and `has_more` report every omission. The clamping
itself is not re-implemented at the handler — the handoff service already
clamps at its own boundary, so the handler passes the parsed value through and
the bound holds even if a future caller reaches the service directly.

### The agent projection

Each comment returned to an agent projects exactly: identifier, task
identifier, `author_type`, `author_id`, `source`, body, creation timestamp,
and — only when the body was cut — a truncation marker plus the original body
length in bytes. `reply_channel_id` and the per-comment run lifecycle fields
are not projected; the first is internal reply routing and the second is a
rendering concern for the human comment list.

Bodies are cut with the existing shared rune-safe byte truncation helper rather
than a new cut, so the boundary rule lives in one place. The per-body cap is
8192 bytes.

Every agent response carries the window: `total`, `returned`, and `has_more`,
which is true exactly when `returned` is below `total`. The window describes
only omitted comments. Whether an individual body was shortened is carried on
that comment as `body_truncated`, never in the window. Two names because they
are two facts, and one name for both is how a coordinator concludes it has the
whole note when it has the first half of it.

Author fields are projected as data. No filter parameter exists for them. Two
reasons: a coordinator needs the human steer on a child task as much as the
agent note, and comment authorship is currently unreliable because reviewer
runs execute inside the author's session, so a filter keyed on authorship would
silently drop rows it should return.

## Ordering and windowing

The comment store's existing ascending list and descending recent-list both
sort on `created_at` with no tiebreak, and the comment DTO already records that
Office writes two timestamps inside the same second. The window here is
therefore explicitly two-phase and uses a named tiebreak in both directions:

1. select the newest `limit` rows ordered by `created_at` descending, `id`
   descending;
2. present that window ordered by `created_at` ascending, `id` ascending.

Using `id` in both directions is what makes the window boundary deterministic:
with a tiebreak in only one phase, two comments sharing a timestamp could
straddle the boundary differently on successive calls. `id` is a random
identifier, so this ordering is repeatable but is not insertion order; the
requirement and the exclusions both say so.

A second bound sits above `limit`: the response carries at most 65536 bytes of
body in total. At the clamped maximum the per-body cap alone would permit a
response near 800KB, which no consuming agent can use, and the point of this
read is that the result is usable. When the selected window exceeds the budget,
whole comments are dropped from the oldest end, preserving the same
newest-wins rule the window selection already uses; the dropped comments leave
`total` untouched, so `has_more` reports them.

The budget never empties a non-empty window. At the stated constants this holds
by construction rather than by a special case: the per-body cap is 8192 bytes
and the budget is 65536, so the newest comment's shortened body always fits
alone and the drop loop can never reach it. The invariant is written down
anyway, because it is the load-bearing one — an empty list is read by a
coordinator as "the child produced nothing" and ends the delegation — and it is
stated independently of the constants, so that if either ever changes such that
one shortened body could exceed the budget, that comment is still returned
alone.

## Control flow

Caller agent → `agentctl kandev comment list` → office HTTP route group → agent
auth middleware → comment read endpoint → access-checked read → comment store,
and the projection back out as the response body the CLI prints. The caller
task identity crosses from validated JWT claims; the target identifier crosses
from the request path. Neither is ever read from a flag the model chose beyond
the target it is entitled to name.

The wiring is a pass-through, not a new assembly: `registerSecondaryRoutes`
already holds the handoff service and already registers the office route group
a few lines later, so the dashboard handler receives it the way the document
handler receives its own service. Office already imports the task service in
this package, so the architecture lint that bans the reverse direction is not
engaged.

The acceptance path: a child agent posts its deliverable through the existing
signed runtime comment path; later, the parent's run runs
`agentctl kandev comment list --task <child>`, the descendant branch of the
read guard admits it, and the body is printed. Nothing between the two runs is
human.

## Failure and recovery

Three outcomes must stay distinguishable, because conflating them is the
reported defect.

- **Accessible and empty** is a success with an empty list, an explicit empty
  array rather than a null, and a zero total.
- **Not permitted** is a forbidden status carrying the existing shared
  access-denied sentinel, whose message is the literal string `document access
  denied`. A missing task and an unrelated task must produce that same forbidden
  outcome and that same message, because distinguishing them would let a caller
  enumerate task identifiers. An agent whose JWT carries no task — routine
  dispatch mints exactly such a token — is denied on this same path for every
  target, rather than being allowed to fall through to the unguarded read.

  That sameness was not what the guard did, and closing the gap is the one
  shared change this design owns; as noted above it has already landed.
  `loadAccessPair` called the task repository for the caller and the target and
  returned the repository's error unchanged. On an absent identifier that
  repository returns a not-found error which embeds the identifier it was
  given. The guard's `current == nil || target == nil` denial
  branch below it is reachable only under test, because the test double returns
  a bare `(nil, nil)` where the repository returns `(nil, not-found)`, and the
  guard's missing-task test discards the error, so the divergence is invisible.
  Normalisation therefore happens inside `loadAccessPair`: a not-found from
  either lookup becomes a plain deny, with no error, before any caller sees it.
  The test double is corrected in the same change to return the repository's
  shape, so the guard's tests stop passing on a branch production never reaches.

  Renaming the `document access denied` sentinel text to something
  surface-neutral is deliberately out of scope. That both surfaces read
  identically is what matters; the wording does not, and changing it would churn
  existing assertions for nothing.
- **Dependency unconfigured or storage read failed** is an internal error. This
  path must not degrade to an empty list. The adjacent per-comment run-status
  lookup in the same handler does degrade to an empty map, which is right there
  and wrong here: an empty comment list is read by a coordinator as "the child
  produced nothing" and ends the delegation. That existing degradation stays on
  the browser branch, where the cost of a missing badge is a missing badge.

## Persistence

The read path is read-only and needs no schema change or new index. The
existing `(task_id, created_at)` index serves the window.

Legacy rows are a known edge, and this design excludes them deliberately rather
than repairing them in passing. `CreateTaskComment` normalises a non-zero
`created_at` to UTC on write, so every row written after that fix compares
correctly under the lexical ordering the window relies on; rows written before
it may carry another offset and do not. Backfilling those is owned by the
follow-up card *Backfill legacy non-UTC `task_comments.created_at`*
(`f5984c8c-ecdb-4e58-b326-1c742c29bfbc`) and is not a precondition of this
work. Until it lands, a row whose stored timestamp is not in the canonical UTC
representation falls outside the ordering guarantee of
`REQ-OFFICE-AGENT-COMMENT-READS-003`. It is still returned, still counted in
`total`, and still subject to the guard and the budget — nothing is hidden and
nothing errors; only its position relative to a canonical row is unspecified.
No startup backfill is described here on purpose: a partial one, whose
behaviour on a value it cannot parse would itself need stating, would reopen
the very ordering question it appears to close.

Both the count and the page are taken inside one read transaction on the
read-only connection, so the returned count never exceeds the reported total. A
comment inserted concurrently either precedes that snapshot or follows it. No
lock is taken that a writer could block on.

## Security

The read relation is not re-derived here. The endpoint calls the same guard the
document reads call, so the two cannot drift into two answers to one question.
The relation admits self, ancestors, descendants, siblings sharing a non-empty
parent, and blockers, and denies any cross-workspace pair.

Placing the guard at the endpoint rather than at a tool is what makes it
complete. The office route group authenticates agents with a workspace-scoped
JWT. The workspace-scope middleware defers this exact agent comment route to
the shared guard because the route has no `:wsId` parameter and must return one
forbidden response for missing, foreign, and unrelated targets. Other Office
routes still use the workspace-scope middleware before dispatch. Before this
change any Office agent could read any task's comments in its workspace, and
both CLI commands reached that read. One handler fix closes both entry points.
A guarded tool beside an unguarded endpoint would have closed neither, which is
why the tool is deleted rather than kept.

Task-identifier enumeration is closed at that guard rather than at this
endpoint, by the `loadAccessPair` normalisation described in
[Failure and recovery](#failure-and-recovery). Before that change an absent
identifier produced an error naming it; after it, a missing task is
indistinguishable from an existing unrelated one on every surface using the
guard.

The caller identity comes from validated JWT claims, so a caller cannot claim
to be a task it is not, and a token with no task claim reads nothing.

Recorded so it is not mistaken for a new hole: the dashboard task **document**
routes apply no relation check either. That gap predates this work, is the same
shape as the one closed here, and is neither widened nor fixed here.

## The advertised-surface contract

The Office first-turn context is asserted to advertise exactly the tool set the
Office surface registers, so removing the tool and leaving its sysprompt line
is a build failure rather than silent drift; the assertion is what keeps the
deletion honest in both directions.

The capability is advertised where Office agents actually read: the injected
comment reference under the task-ops skill currently documents only the write
command and gains the read, and the coordinator role instructions — the `ceo`
role, at
`apps/backend/internal/office/configloader/instructions/ceo/AGENTS.md`, the role
that creates subtasks and reviews their results — gain the direction to read a
delegated child's comments with that command before concluding the child
produced nothing. Without that, the endpoint works and the coordinator still
does not call it.

## Observability

Requests are visible in the office route group's existing HTTP logging, so a
coordinator's read attempts are traceable alongside its other runtime calls. No
new metric: the failure this addresses is a missing capability and an unguarded
read, not a rate to watch.

## Related decisions

- [ADR 0015 — explicit completion signal for auto-advance](../../../decisions/0015-explicit-completion-signal-for-auto-advance.md)
